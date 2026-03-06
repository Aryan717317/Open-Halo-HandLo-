// lib/stores/gestureStore.ts
import { writable, derived } from 'svelte/store'
import { Gesture, type NormalizedLandmark } from '$lib/gesture/GestureClassifier'

export interface GestureState {
  gesture: Gesture
  landmarks: NormalizedLandmark[]
  confidence: number
  handVisible: boolean
}

function createGestureStore() {
  const { subscribe, set, update } = writable<GestureState>({
    gesture: Gesture.IDLE,
    landmarks: [],
    confidence: 0,
    handVisible: false,
  })

  return {
    subscribe,
    update: (gesture: Gesture, landmarks: NormalizedLandmark[]) => {
      set({ gesture, landmarks, confidence: 1, handVisible: landmarks.length > 0 })
    },
    clear: () => set({ gesture: Gesture.IDLE, landmarks: [], confidence: 0, handVisible: false }),
  }
}

export const gestureStore = createGestureStore()
export const currentGesture = derived(gestureStore, $g => $g.gesture)
export const handLandmarks  = derived(gestureStore, $g => $g.landmarks)
