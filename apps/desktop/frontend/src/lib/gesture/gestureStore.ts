// gestureStore.ts — Svelte reactive store for live gesture state
import { writable, derived } from 'svelte/store';
import { Gesture } from './GestureClassifier';
import type { NormalizedLandmark } from './HandTracker';

export interface GestureState {
  gesture: Gesture;
  landmarks: NormalizedLandmark[];
  confidence: number;
  handVisible: boolean;
}

const initial: GestureState = {
  gesture: Gesture.IDLE,
  landmarks: [],
  confidence: 0,
  handVisible: false,
};

export const gestureStore = writable<GestureState>(initial);

// Derived: only emits when gesture changes — avoids flooding subscribers
export const currentGesture = derived(
  gestureStore,
  ($s) => $s.gesture
);

export const handVisible = derived(
  gestureStore,
  ($s) => $s.handVisible
);
