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
  import type { AppScreen, ConnectionMethod, PairingData } from "$lib/types";
  import QRCode from "qrcode";
  import { listen } from "@tauri-apps/api/event";

  let videoEl: HTMLVideoElement;
  let phantomCanvas: HTMLCanvasElement;
  let orbCanvas: HTMLCanvasElement;
  let qrCanvas: HTMLCanvasElement;
  let containerEl: HTMLDivElement;

  let tracker = new HandTracker();
  let classifier = new GestureClassifier();
  let phantomHand: PhantomHand;
  let fileOrb: FileOrb;

  let screen: AppScreen = "connect";
  let connectionMethod: ConnectionMethod = "qr";
  let pairingData: PairingData | null = null;
  let selectedFile: string | null = null;
  let selectedFileName = "";
  let selectedPeerId = "";
  let statusMessage = "Connect a device to begin";
  let deviceInfo: any = null;
  let incomingPairRequest: any = null;
  let errorDetails: {
    code: string;
    message: string;
    isSecurity: boolean;
  } | null = null;

  // ── State Machine ───────────────────────────────────────────────────────────

  async function transition(to: AppScreen, context: any = {}) {
    const prevScreen = screen;

    // INV-01: Camera management
    const cameraScreens: AppScreen[] = [
      "ready",
      "selecting",
      "sending",
      "receiving",
    ];
    const shouldHaveCamera = cameraScreens.includes(to);
    const hadCamera = cameraScreens.includes(prevScreen);

    if (shouldHaveCamera && !hadCamera) {
      await tracker.startCamera(videoEl, handleFrame);
    } else if (!shouldHaveCamera && hadCamera) {
      tracker.stop();
    }

    // INV-09: Clear file when returning to ready
    if (to === "ready") {
      selectedFile = null;
      selectedFileName = "";
      fileOrb?.detach();
    }

    // Handle context
    if (context.peerId) selectedPeerId = context.peerId;
    if (context.status) statusMessage = context.status;
    if (context.error) errorDetails = context.error;

    screen = to;
    console.log(`[state] transition: ${prevScreen} -> ${to}`);
  }

  // ── Lifecycle ────────────────────────────────────────────────────────────────

  // -- Phase 6: Sync Pairing Data --
  $: if (
    screen === "connect" &&
    activeMethod === "qr" &&
    pairingData &&
    qrCanvas
  ) {
    QRCode.toCanvas(qrCanvas, pairingData.qr_url, {
      width: 256,
      margin: 2,
      color: {
        dark: "#ffffff",
        light: "#00000000",
      },
    });
  }

  onMount(async () => {
    phantomHand = new PhantomHand(phantomCanvas);
    fileOrb = new FileOrb(orbCanvas);
    resize();

    await tracker.initialize();
    await startDiscovery();

    // Listen for mobile pairing data from sidecar
    const unlistenPairingData = await listen<PairingData>(
      "EVT_PAIRING_DATA",
      (event) => {
        console.log("[ui] Received pairing data:", event.payload);
        pairingData = event.payload;
      },
    );

    requestDeviceInfo((info) => {
      deviceInfo = info;
    });

    onIncomingPair((req) => {
      incomingPairRequest = req;
      transition("pairing", {
        status: `Incoming connection from ${req.peer_name}`,
      });
    });

    onPairSuccess((res) => {
      transition("ready", {
        peerId: res.peer_id,
        status: "Paired! Close fist to grab a file",
      });
    });

    onPairRejected((res) => {
      transition("connect", { status: "Connection rejected" });
    });

    // -- Phase 7: Clipboard Sync Notification --
    listen<{ text: string }>("EVT_CLIPBOARD_RX", (event) => {
      const text = event.payload.text;
      const snippet = text.length > 20 ? text.substring(0, 20) + "..." : text;
      statusMessage = `Clipboard synced: "${snippet}"`;
      // Clear message after 3 seconds if in ready state
      if (screen === "ready") {
        setTimeout(() => {
          if (statusMessage.startsWith("Clipboard synced")) {
            statusMessage = "Paired! Close fist to grab a file";
          }
        }, 3000);
      }
    });
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

    // Update visuals based on state
    if (screen === "selecting") {
      phantomHand?.update(landmarks, gesture, "orange");
    } else {
      phantomHand?.update(landmarks, gesture, "cyan");
    }

    fileOrb?.tick(landmarks[0]);

    if (gesture !== prevGesture) {
      onGestureChange(gesture, landmarks);
      prevGesture = gesture;
    }
  }

  async function onGestureChange(gesture: Gesture, landmarks: any[]) {
    if (screen === "ready") {
      if (gesture === Gesture.GRAB) {
        await handleGrab(landmarks[0]);
      }
    } else if (screen === "selecting" && selectedFile) {
      if (gesture === Gesture.OPEN_PALM) {
        handleSend();
      }
    }
  }

  async function handleGrab(palmLandmark: any) {
    statusMessage = "Opening file picker...";
    const path = await pickFile();
    if (!path) {
      transition("ready");
      return;
    }

    selectedFile = path;
    selectedFileName =
      path.split("/").pop() ?? path.split("\\").pop() ?? "file";

    transition("selecting", {
      status: `"${selectedFileName}" selected — open palm to send`,
    });
    fileOrb?.attach(palmLandmark, selectedFileName);
  }

  async function handleSend() {
    if (!selectedFile || !selectedPeerId) return;

    transition("sending", { status: "Sending..." });
    fileOrb?.startSend();

    try {
      await sendFile(selectedFile, selectedPeerId, selectedFileName, 0);
      transition("done", { status: "Sent!" });
      fileOrb?.complete();
    } catch (e: any) {
      transition("error", {
        error: {
          code: "TX_FAIL",
          message: e.message || "Transfer failed",
          isSecurity: false,
        },
      });
    }
  }

  function selectPeer(peer: any) {
    transition("pairing", { status: `Connecting to ${peer.name}...` });
    pairWithPeer(peer.id, peer.name);
  }

  function handleAcceptPair() {
    if (!incomingPairRequest) return;
    acceptPair(incomingPairRequest.peer_id);
    incomingPairRequest = null;
    // Note: onPairSuccess will trigger the transition to 'ready'
  }

  function handleRejectPair() {
    if (!incomingPairRequest) return;
    rejectPair(incomingPairRequest.peer_id);
    incomingPairRequest = null;
    transition("connect", { status: "Connection rejected" });
  }

  let activeMethod: "qr" | "code" | "lan" = "lan";
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

    <!-- 1. CONNECT SCREEN -->
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
          <button
            class="method-btn"
            class:active={activeMethod === "qr"}
            on:click={() => (activeMethod = "qr")}
          >
            <span class="icon">⊞</span>
            <span>QR Code</span>
          </button>
          <button
            class="method-btn"
            class:active={activeMethod === "code"}
            on:click={() => (activeMethod = "code")}
          >
            <span class="icon">#</span>
            <span>Text Code</span>
          </button>
          <button
            class="method-btn"
            class:active={activeMethod === "lan"}
            on:click={() => (activeMethod = "lan")}
          >
            <span class="icon">◈</span>
            <span>Nearby ({$peers.length})</span>
          </button>
        </div>

        {#if activeMethod === "lan"}
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
        {:else if activeMethod === "qr"}
          <div class="placeholder-qr">
            <div class="qr-box">
              {#if pairingData}
                <canvas bind:this={qrCanvas}></canvas>
              {:else}
                <div class="spinner"></div>
              {/if}
            </div>
            <p>Scan with GestureShare on mobile</p>
          </div>
        {:else if activeMethod === "code"}
          <div class="placeholder-code">
            <div class="code-display">
              {pairingData?.text_code || "------"}
            </div>
            <p>Enter this code on the other device</p>
          </div>
        {/if}
      </div>
    {/if}

    <!-- 2. PAIRING SCREEN -->
    {#if screen === "pairing"}
      <div class="panel pairing-panel">
        <div class="spinner-wrap">
          <div class="spinner"></div>
        </div>
        <h2>Pairing...</h2>
        <p class="pairing-status">{statusMessage}</p>

        {#if incomingPairRequest}
          <div class="actions">
            <button class="btn accept" on:click={handleAcceptPair}
              >Accept</button
            >
            <button class="btn reject" on:click={handleRejectPair}
              >Reject</button
            >
          </div>
        {/if}
      </div>
    {/if}

    <!-- 3. READY / SELECTING / SENDING PROGRESS -->
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

    {#if screen === "sending" || screen === "receiving"}
      <div class="transfer-panel">
        <div class="progress-ring-container">
          <!-- Simplified progress ring for now -->
          <svg class="progress-ring" width="120" height="120">
            <circle class="ring-bg" cx="60" cy="60" r="54" />
            <circle
              class="ring-fill"
              cx="60"
              cy="60"
              r="54"
              style="stroke-dasharray: 339; stroke-dashoffset: {339 *
                (1 -
                  ($transferStore.get(selectedPeerId)?.progress || 0) / 100)}"
            />
          </svg>
          <div class="pct-text">
            {($transferStore.get(selectedPeerId)?.progress || 0).toFixed(0)}%
          </div>
        </div>
        <p class="tx-info">
          {screen === "sending" ? "Sending" : "Receiving"} file...
        </p>
      </div>
    {/if}

    <!-- 4. DONE SCREEN -->
    {#if screen === "done"}
      <div class="panel done-panel">
        <div class="done-icon">✦</div>
        <h2>Transfer Complete</h2>
        <p>Keys wiped from memory. Session remains active.</p>
        <button class="btn primary" on:click={() => transition("ready")}
          >Send another file</button
        >
      </div>
    {/if}

    <!-- 5. ERROR SCREEN -->
    {#if screen === "error" && errorDetails}
      <div class="panel error-panel" class:security={errorDetails.isSecurity}>
        <div class="error-icon">{errorDetails.isSecurity ? "⊘" : "!"}</div>
        <h2>{errorDetails.isSecurity ? "Security Alert" : "Oops!"}</h2>
        <p>{errorDetails.message}</p>

        <div class="actions">
          {#if errorDetails.isSecurity}
            <button class="btn reject" on:click={() => transition("connect")}
              >Scan QR Again</button
            >
          {:else}
            <button class="btn primary" on:click={() => transition("ready")}
              >Try Again</button
            >
            <button class="btn reject" on:click={() => transition("connect")}
              >Disconnect</button
            >
          {/if}
        </div>
      </div>
    {/if}
  </div>
</div>

<style>
  /* Connection Placeholders */
  .placeholder-qr,
  .placeholder-code {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 20px;
    padding: 40px 0;
    text-align: center;
    color: rgba(255, 255, 255, 0.6);
    font-size: 14px;
  }
  .qr-box {
    width: 200px;
    height: 200px;
    background: #fff;
    border-radius: 12px;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .qr-mock {
    color: #000;
    font-weight: 800;
    font-size: 40px;
  }
  .code-display {
    font-size: 48px;
    font-weight: 700;
    letter-spacing: 0.1em;
    color: #00d4ff;
    text-shadow: 0 0 20px rgba(0, 212, 255, 0.4);
  }

  /* Pairing Spinner */
  .spinner-wrap {
    display: flex;
    justify-content: center;
    margin-bottom: 24px;
  }
  .spinner {
    width: 50px;
    height: 50px;
    border: 3px solid rgba(0, 212, 255, 0.1);
    border-top-color: #00d4ff;
    border-radius: 50%;
    animation: spin 1s linear infinite;
  }
  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
  .pairing-status {
    text-align: center;
    font-size: 14px;
    color: rgba(255, 255, 255, 0.5);
    margin-bottom: 24px;
  }

  /* Transfer Panel & Ring */
  .transfer-panel {
    position: absolute;
    inset: 0;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    background: rgba(0, 0, 0, 0.4);
    backdrop-filter: blur(5px);
    pointer-events: none;
  }
  .progress-ring-container {
    position: relative;
    width: 120px;
    height: 120px;
    margin-bottom: 20px;
  }
  .progress-ring circle {
    fill: transparent;
    stroke-width: 6;
    transform: rotate(-90deg);
    transform-origin: 50% 50%;
  }
  .ring-bg {
    stroke: rgba(255, 255, 255, 0.1);
  }
  .ring-fill {
    stroke: #00d4ff;
    stroke-linecap: round;
    transition: stroke-dashoffset 0.3s ease;
  }
  .pct-text {
    position: absolute;
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 24px;
    font-weight: 600;
    color: #fff;
  }
  .tx-info {
    font-size: 16px;
    color: rgba(255, 255, 255, 0.8);
  }

  /* Done & Error Panels */
  .done-panel,
  .error-panel {
    text-align: center;
  }
  .done-icon,
  .error-icon {
    font-size: 48px;
    margin-bottom: 16px;
  }
  .done-icon {
    color: #39ff14;
  }
  .error-icon {
    color: #ff3b30;
  }
  .error-panel.security {
    border-color: rgba(255, 59, 48, 0.4);
    background: rgba(40, 0, 0, 0.85);
  }
  .error-panel h2 {
    color: #fff;
  }
  .error-panel p {
    color: rgba(255, 255, 255, 0.6);
    margin-bottom: 24px;
    font-size: 14px;
  }

  .primary {
    background: #00d4ff;
    color: #000;
    margin-bottom: 8px;
  }

  /* Original styles below */
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
