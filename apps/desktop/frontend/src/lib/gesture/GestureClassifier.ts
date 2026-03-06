// GestureClassifier.ts
// Classifies 21 MediaPipe hand landmarks into discrete gestures.
// Includes debouncer — gesture must hold for N frames to confirm.

import type { NormalizedLandmark } from './HandTracker';

export enum Gesture {
  IDLE      = 'IDLE',
  GRAB      = 'GRAB',       // closed fist — SELECT file
  OPEN_PALM = 'OPEN_PALM',  // all fingers extended — SEND/ACCEPT
  POINT     = 'POINT',      // index only extended — navigate UI
}

// MediaPipe landmark indices
const WRIST        = 0;
const THUMB_TIP    = 4;
const INDEX_TIP    = 8;
const INDEX_PIP    = 6;  // middle joint of index
const MIDDLE_TIP   = 12;
const RING_TIP     = 16;
const PINKY_TIP    = 20;

const GRAB_THRESHOLD      = 0.065; // normalized image coords
const OPEN_THRESHOLD      = 0.13;
const REQUIRED_HOLD_FRAMES = 8;   // ~267ms at 30fps

function dist(a: NormalizedLandmark, b: NormalizedLandmark): number {
  return Math.hypot(a.x - b.x, a.y - b.y, (a.z ?? 0) - (b.z ?? 0));
}

export class GestureClassifier {
  private holdCount = 0;
  private lastRaw = Gesture.IDLE;
  private confirmed = Gesture.IDLE;

  /** Call once per frame with landmarks from one hand */
  classify(landmarks: NormalizedLandmark[]): Gesture {
    const raw = this.rawClassify(landmarks);

    if (raw === this.lastRaw) {
      this.holdCount = Math.min(this.holdCount + 1, REQUIRED_HOLD_FRAMES);
    } else {
      this.holdCount = 0;
      this.lastRaw = raw;
    }

    if (this.holdCount >= REQUIRED_HOLD_FRAMES) {
      this.confirmed = this.lastRaw;
    }

    return this.confirmed;
  }

  private rawClassify(lm: NormalizedLandmark[]): Gesture {
    if (lm.length < 21) return Gesture.IDLE;

    const palm = lm[WRIST];
    const tips = [lm[THUMB_TIP], lm[INDEX_TIP], lm[MIDDLE_TIP], lm[RING_TIP], lm[PINKY_TIP]];

    // GRAB: all fingertips close to palm
    if (tips.every((t) => dist(t, palm) < GRAB_THRESHOLD)) {
      return Gesture.GRAB;
    }

    // OPEN PALM: all fingertips far from palm
    if (tips.every((t) => dist(t, palm) > OPEN_THRESHOLD)) {
      return Gesture.OPEN_PALM;
    }

    // POINT: index extended, middle/ring/pinky closed
    const indexExtended  = dist(lm[INDEX_TIP], palm) > OPEN_THRESHOLD * 0.8;
    const middleClosed   = dist(lm[MIDDLE_TIP], palm) < GRAB_THRESHOLD * 1.5;
    if (indexExtended && middleClosed) {
      return Gesture.POINT;
    }

    return Gesture.IDLE;
  }

  /** Confidence 0–1 based on hold frame count */
  get confidence(): number {
    return this.holdCount / REQUIRED_HOLD_FRAMES;
  }

  reset() {
    this.holdCount = 0;
    this.lastRaw = Gesture.IDLE;
    this.confirmed = Gesture.IDLE;
  }
}
