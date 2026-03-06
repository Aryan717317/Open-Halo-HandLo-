<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { HandTracker } from "$lib/gesture/HandTracker";
  import { GestureClassifier, Gesture } from "$lib/gesture/GestureClassifier";
  import { gestureStore } from "$lib/gesture/gestureStore";
  import { PhantomHand } from "$lib/ui/PhantomHand";
  import { FileOrb } from "$lib/ui/FileOrb";
  import { peers, hasPeers } from "$lib/stores/peerStore";
  import { transferStore } from "$lib/stores/transferStore";
  import {
    startDiscovery,
    pickFile,
    sendFile,
    requestDeviceInfo,
    pairWithPeer,
    onIncomingPair,
    onPairSuccess,
    onPairRejected,
    acceptPair,
    rejectPair,
  } from "$lib/transfer/tauribridge";
  import type { AppScreen } from "$lib/types";

  let videoEl: HTMLVideoElement;
  let phantomCanvas: HTMLCanvasElement;
  let orbCanvas: HTMLCanvasElement;
  let containerEl: HTMLDivElement;

  let tracker = new HandTracker();
  let classifier = new GestureClassifier();
  let phantomHand: PhantomHand;
  let fileOrb: FileOrb;

  let screen: AppScreen = "connect";
  let selectedFile: string | null = null;
  let selectedFileName = "";
  let selectedPeerId = "";
  let statusMessage = "Show your hand to the camera";
  let deviceInfo: any = null;
  let incomingPairRequest: any = null;

  // ── Lifecycle ────────────────────────────────────────────────────────────────

  onMount(async () => {
    phantomHand = new PhantomHand(phantomCanvas);
    fileOrb = new FileOrb(orbCanvas);
    resize();

    await tracker.initialize();
    await tracker.startCamera(videoEl, handleFrame);
    await startDiscovery();

    requestDeviceInfo((info) => {
      deviceInfo = info;
    });

    onIncomingPair((req) => {
      incomingPairRequest = req;
      screen = "pairing";
      statusMessage = `Incoming connection from ${req.peer_name}`;
    });

    onPairSuccess((res) => {
      selectedPeerId = res.peer_id;
      screen = "ready";
      statusMessage = "Paired! Close fist to grab a file";
    });

    onPairRejected((res) => {
      statusMessage = "Connection rejected";
      screen = "connect";
      selectedPeerId = "";
    });

    screen = "connect";
  });

  onDestroy(() => {
    tracker.stop();
    phantomHand?.dispose();
    fileOrb?.dispose();
  });

  function resize() {
    const w = containerEl?.clientWidth ?? 1280;
    const h = containerEl?.clientHeight ?? 720;
    phantomHand?.resize(w, h);
    fileOrb?.resize(w, h);
  }

  // ── Gesture pipeline ─────────────────────────────────────────────────────────

  let prevGesture = Gesture.IDLE;

  function handleFrame(result: any) {
    const landmarks = result.landmarks?.[0] ?? [];
    const gesture = landmarks.length
      ? classifier.classify(landmarks)
      : Gesture.IDLE;

    gestureStore.set({
      gesture,
      landmarks,
      confidence: classifier.confidence,
      handVisible: landmarks.length > 0,
    });

    phantomHand?.update(landmarks, gesture);
    fileOrb?.tick(landmarks[0]);

    // ── Gesture → action mapping ───────────────────────────────────────────────
    if (gesture !== prevGesture) {
      onGestureChange(gesture, landmarks);
      prevGesture = gesture;
    }
  }

  async function onGestureChange(gesture: Gesture, landmarks: any[]) {
    if (screen === "ready") {
      if (gesture === Gesture.GRAB) {
        // Closed fist → open file picker
        await handleGrab(landmarks[0]);
      }
    } else if (screen === "selecting" && selectedFile) {
      if (gesture === Gesture.OPEN_PALM) {
        // Open palm → send file to selected peer
        handleSend();
      }
    }
  }

  async function handleGrab(palmLandmark: any) {
    statusMessage = "Opening file picker...";
    const path = await pickFile();
    if (!path) {
      screen = "ready";
      return;
    }

    selectedFile = path;
    selectedFileName =
      path.split("/").pop() ?? path.split("\\").pop() ?? "file";
    screen = "selecting";
    statusMessage = `"${selectedFileName}" selected — open palm to send`;
    fileOrb?.attach(palmLandmark, selectedFileName);
  }

  async function handleSend() {
    if (!selectedFile || !selectedPeerId) return;
    screen = "sending";
    statusMessage = "Sending...";
    fileOrb?.startSend();

    try {
      await sendFile(selectedFile, selectedPeerId, selectedFileName, 0);
      statusMessage = "Sent!";
      fileOrb?.complete();
      setTimeout(() => {
        screen = "ready";
        selectedFile = null;
      }, 2000);
    } catch (e) {
      statusMessage = "Transfer failed";
      screen = "ready";
    }
  }

  function selectPeer(peer: any) {
    statusMessage = `Connecting to ${peer.name}...`;
    pairWithPeer(peer.id, peer.name);
  }

  function handleAcceptPair() {
    if (!incomingPairRequest) return;
    acceptPair(incomingPairRequest.peer_id);
    selectedPeerId = incomingPairRequest.peer_id;
    incomingPairRequest = null;
    screen = "ready";
    statusMessage = "Paired! Close fist to grab a file";
  }

  function handleRejectPair() {
    if (!incomingPairRequest) return;
    rejectPair(incomingPairRequest.peer_id);
    incomingPairRequest = null;
    screen = "connect";
    statusMessage = "Connection rejected";
  }
</script>

<svelte:window on:resize={resize} />

<!-- ── Main layout ─────────────────────────────────────────────────────────── -->
<div class="app" bind:this={containerEl}>
  <!-- Camera feed (mirrored) -->
  <!-- svelte-ignore a11y-media-has-caption -->
  <video bind:this={videoEl} class="camera" autoplay muted playsinline />

  <!-- Phantom hand overlay (Three.js, transparent) -->
  <canvas bind:this={phantomCanvas} class="overlay" />

  <!-- File orb overlay (Three.js, transparent) -->
  <canvas bind:this={orbCanvas} class="overlay" />

  <!-- UI layer -->
  <div class="ui">
    <!-- Header -->
    <header>
      <span class="logo">GestureShare</span>
      <span class="status-dot" class:active={screen !== "connect"} />
      <span class="status-text">{statusMessage}</span>
    </header>

    <!-- Connection panel (shown until paired) -->
    {#if screen === "connect"}
      <div class="panel connect-panel">
        <h2>Connect a device</h2>

        {#if deviceInfo}
          <div class="device-info">
            <span class="device-name">{deviceInfo.name}</span>
            <span class="device-ip">IP: {deviceInfo.local_ip}</span>
          </div>
        {/if}

        <div class="connect-methods">
          <button class="method-btn">
            <span class="icon">⊞</span>
            <span>QR Code</span>
          </button>
          <button class="method-btn">
            <span class="icon">#</span>
            <span>Text Code</span>
          </button>
          <button class="method-btn" class:active={$hasPeers}>
            <span class="icon">◈</span>
            <span>Nearby ({$peers.length})</span>
          </button>
        </div>

        {#if $hasPeers}
          <ul class="peer-list">
            {#each $peers as peer}
              <li>
                <button class="peer-btn" on:click={() => selectPeer(peer)}>
                  <span class="peer-name">{peer.name}</span>
                  <span class="peer-meta">{peer.os} · {peer.address}</span>
                </button>
              </li>
            {/each}
          </ul>
        {:else}
          <p class="scanning">Scanning local network...</p>
        {/if}
      </div>
    {/if}

    <!-- Pairing Dialog -->
    {#if screen === "pairing" && incomingPairRequest}
      <div class="panel connect-panel">
        <h2>Incoming Connection</h2>
        <div class="incoming-req">
          <div class="peer-name">{incomingPairRequest.peer_name}</div>
          <div class="peer-ip">{incomingPairRequest.address}</div>
        </div>
        <div class="actions">
          <button class="btn accept" on:click={handleAcceptPair}>Accept</button>
          <button class="btn reject" on:click={handleRejectPair}>Reject</button>
        </div>
      </div>
    {/if}

    <!-- Gesture hint -->
    {#if screen === "ready" || screen === "selecting"}
      <div class="gesture-hint">
        {#if screen === "ready"}
          <div class="hint-icon">✊</div>
          <div class="hint-text">Close fist to select file</div>
        {:else}
          <div class="hint-icon">🖐</div>
          <div class="hint-text">Open palm to send</div>
        {/if}
      </div>
    {/if}

    <!-- Transfer progress -->
    {#each [...$transferStore] as [id, tx]}
      {#if tx.status === "active"}
        <div class="progress-bar-wrap">
          <div class="progress-label">{tx.fileName}</div>
          <div class="progress-track">
            <div class="progress-fill" style="width: {tx.progress}%" />
          </div>
          <div class="progress-pct">{tx.progress.toFixed(0)}%</div>
        </div>
      {/if}
    {/each}
  </div>
</div>

<style>
  :global(*, *::before, *::after) {
    box-sizing: border-box;
    margin: 0;
    padding: 0;
  }
  :global(body) {
    background: #000;
    overflow: hidden;
    font-family: "SF Pro Display", system-ui, sans-serif;
  }

  .app {
    position: relative;
    width: 100vw;
    height: 100vh;
    background: #0a0a0f;
    overflow: hidden;
  }

  .camera {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
    transform: scaleX(-1); /* mirror */
    opacity: 0.6;
  }

  .overlay {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    pointer-events: none;
  }

  .ui {
    position: absolute;
    inset: 0;
    display: flex;
    flex-direction: column;
    pointer-events: none;
  }

  header {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 18px 24px;
    background: linear-gradient(to bottom, rgba(0, 0, 0, 0.7), transparent);
    pointer-events: none;
  }

  .logo {
    font-size: 18px;
    font-weight: 700;
    letter-spacing: 0.05em;
    color: #00d4ff;
  }

  .status-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: #444;
    transition: background 0.3s;
  }
  .status-dot.active {
    background: #39ff14;
    box-shadow: 0 0 8px #39ff14;
  }

  .status-text {
    font-size: 13px;
    color: rgba(255, 255, 255, 0.6);
  }

  /* Connect panel */
  .panel {
    pointer-events: all;
    margin: auto;
    background: rgba(10, 10, 20, 0.85);
    backdrop-filter: blur(20px);
    border: 1px solid rgba(0, 212, 255, 0.2);
    border-radius: 20px;
    padding: 32px;
    width: 420px;
    color: #fff;
  }

  .panel h2 {
    font-size: 22px;
    font-weight: 600;
    margin-bottom: 12px;
    color: #fff;
  }

  .device-info {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 10px 14px;
    background: rgba(255, 255, 255, 0.05);
    border-radius: 8px;
    margin-bottom: 24px;
    font-size: 13px;
    color: rgba(255, 255, 255, 0.7);
  }
  .device-name {
    font-weight: 600;
    color: #fff;
  }
  .device-ip {
    font-family: monospace;
  }

  .connect-methods {
    display: flex;
    gap: 10px;
    margin-bottom: 20px;
  }

  .method-btn {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 6px;
    padding: 14px 8px;
    background: rgba(255, 255, 255, 0.05);
    border: 1px solid rgba(255, 255, 255, 0.1);
    border-radius: 12px;
    color: rgba(255, 255, 255, 0.7);
    font-size: 12px;
    cursor: pointer;
    transition: all 0.2s;
  }
  .method-btn.active,
  .method-btn:hover {
    background: rgba(0, 212, 255, 0.1);
    border-color: rgba(0, 212, 255, 0.4);
    color: #00d4ff;
  }
  .method-btn .icon {
    font-size: 20px;
  }

  .peer-list {
    list-style: none;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .peer-btn {
    width: 100%;
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 3px;
    padding: 12px 16px;
    background: rgba(255, 255, 255, 0.04);
    border: 1px solid rgba(255, 255, 255, 0.08);
    border-radius: 10px;
    cursor: pointer;
    transition: all 0.2s;
    color: #fff;
  }
  .peer-btn:hover {
    background: rgba(57, 255, 20, 0.1);
    border-color: rgba(57, 255, 20, 0.3);
  }
  .peer-name {
    font-size: 14px;
    font-weight: 500;
  }
  .peer-meta {
    font-size: 11px;
    color: rgba(255, 255, 255, 0.4);
  }

  .scanning {
    text-align: center;
    color: rgba(255, 255, 255, 0.4);
    font-size: 13px;
    padding: 20px 0;
    animation: pulse 2s infinite;
  }

  @keyframes pulse {
    0%,
    100% {
      opacity: 0.4;
    }
    50% {
      opacity: 1;
    }
  }

  /* Gesture hint */
  .gesture-hint {
    position: absolute;
    bottom: 40px;
    left: 50%;
    transform: translateX(-50%);
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 12px 24px;
    background: rgba(0, 0, 0, 0.6);
    backdrop-filter: blur(10px);
    border: 1px solid rgba(255, 255, 255, 0.1);
    border-radius: 40px;
    color: #fff;
    pointer-events: none;
  }
  .hint-icon {
    font-size: 24px;
  }
  .hint-text {
    font-size: 14px;
    color: rgba(255, 255, 255, 0.8);
  }

  /* Progress bar */
  .progress-bar-wrap {
    position: absolute;
    bottom: 100px;
    left: 50%;
    transform: translateX(-50%);
    width: 360px;
    background: rgba(0, 0, 0, 0.7);
    border-radius: 12px;
    padding: 14px 18px;
    pointer-events: none;
  }
  .progress-label {
    font-size: 13px;
    color: #fff;
    margin-bottom: 8px;
  }
  .progress-track {
    height: 4px;
    background: rgba(255, 255, 255, 0.1);
    border-radius: 4px;
    overflow: hidden;
  }
  .progress-fill {
    height: 100%;
    background: linear-gradient(90deg, #00d4ff, #39ff14);
    border-radius: 4px;
    transition: width 0.2s;
  }
  .progress-pct {
    font-size: 11px;
    color: rgba(255, 255, 255, 0.5);
    margin-top: 5px;
    text-align: right;
  }

  /* Incoming Pair Dialog */
  .incoming-req {
    margin-bottom: 24px;
    text-align: center;
  }
  .incoming-req .peer-name {
    font-size: 20px;
    color: #fff;
    font-weight: 600;
    margin-bottom: 4px;
  }
  .incoming-req .peer-ip {
    font-size: 14px;
    color: rgba(255, 255, 255, 0.6);
  }

  .actions {
    display: flex;
    gap: 12px;
    justify-content: center;
  }
  .btn {
    padding: 10px 24px;
    border: none;
    border-radius: 8px;
    font-size: 14px;
    font-weight: 600;
    cursor: pointer;
    transition: all 0.2s;
  }
  .btn.accept {
    background: #39ff14;
    color: #000;
  }
  .btn.reject {
    background: rgba(255, 255, 255, 0.1);
    color: #fff;
  }
  .btn:hover {
    filter: brightness(1.1);
  }
</style>
