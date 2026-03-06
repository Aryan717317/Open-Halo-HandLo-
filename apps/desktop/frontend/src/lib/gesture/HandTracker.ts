// HandTracker.ts
// Sets up MediaPipe Hands via WebAssembly. Runs fully offline — no API calls.
// Outputs 21 normalized landmarks per hand at ~30fps.

import {
  HandLandmarker,
  FilesetResolver,
  type HandLandmarkerResult,
  type NormalizedLandmark,
} from '@mediapipe/tasks-vision';

export type { NormalizedLandmark };

export class HandTracker {
  private landmarker: HandLandmarker | null = null;
  private animFrame: number = 0;
  private video: HTMLVideoElement | null = null;

  async initialize(): Promise<void> {
    const vision = await FilesetResolver.forVisionTasks(
      'https://cdn.jsdelivr.net/npm/@mediapipe/tasks-vision/wasm'
    );

    this.landmarker = await HandLandmarker.createFromOptions(vision, {
      baseOptions: {
        modelAssetPath:
          'https://storage.googleapis.com/mediapipe-models/hand_landmarker/hand_landmarker/float16/1/hand_landmarker.task',
        delegate: 'GPU', // falls back to CPU automatically
      },
      runningMode: 'VIDEO',
      numHands: 2,
      minHandDetectionConfidence: 0.6,
      minHandPresenceConfidence: 0.6,
      minTrackingConfidence: 0.5,
    });

    console.log('[HandTracker] MediaPipe initialized');
  }

  async startCamera(
    videoEl: HTMLVideoElement,
    onResult: (result: HandLandmarkerResult) => void
  ): Promise<void> {
    this.video = videoEl;

    const stream = await navigator.mediaDevices.getUserMedia({
      video: { facingMode: 'user', width: 1280, height: 720 },
    });

    videoEl.srcObject = stream;
    videoEl.play();

    videoEl.addEventListener('loadeddata', () => {
      this.loop(onResult);
    });
  }

  private loop(onResult: (result: HandLandmarkerResult) => void) {
    if (!this.landmarker || !this.video) return;

    const nowMs = performance.now();
    const result = this.landmarker.detectForVideo(this.video, nowMs);
    onResult(result);

    this.animFrame = requestAnimationFrame(() => this.loop(onResult));
  }

  stop() {
    cancelAnimationFrame(this.animFrame);
    if (this.video?.srcObject) {
      const tracks = (this.video.srcObject as MediaStream).getTracks();
      tracks.forEach((t) => t.stop());
    }
  }
}
