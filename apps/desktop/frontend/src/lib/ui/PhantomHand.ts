// PhantomHand.ts
// Three.js renderer for the phantom hand skeleton overlay.
// Draws translucent bones + glowing joints over the camera feed.

import * as THREE from 'three';
import type { NormalizedLandmark } from '$lib/gesture/HandTracker';
import { Gesture } from '$lib/gesture/GestureClassifier';

// Anatomical bone connections between the 21 MediaPipe landmarks
const CONNECTIONS = [
  [0,1],[1,2],[2,3],[3,4],           // Thumb
  [0,5],[5,6],[6,7],[7,8],           // Index
  [0,9],[9,10],[10,11],[11,12],      // Middle
  [0,13],[13,14],[14,15],[15,16],    // Ring
  [0,17],[17,18],[18,19],[19,20],    // Pinky
  [5,9],[9,13],[13,17],              // Palm cross-connections
];

const COLORS = {
  idle:      0x00d4ff,   // cyan
  grab:      0xff6b35,   // orange — grab/select
  openPalm:  0x39ff14,   // neon green — send/accept
  point:     0xffd700,   // gold — navigate
};

export class PhantomHand {
  private scene: THREE.Scene;
  private camera: THREE.OrthographicCamera;
  private renderer: THREE.WebGLRenderer;
  private bones: THREE.Line[] = [];
  private joints: THREE.Mesh[] = [];
  private palmGlow: THREE.Mesh | null = null;

  constructor(canvas: HTMLCanvasElement) {
    this.scene = new THREE.Scene();

    // Orthographic camera maps to normalized [0,1] landmark coords
    this.camera = new THREE.OrthographicCamera(0, 1, 1, 0, -10, 10);

    this.renderer = new THREE.WebGLRenderer({ canvas, alpha: true });
    this.renderer.setPixelRatio(window.devicePixelRatio);
    this.renderer.setClearColor(0x000000, 0); // transparent background
  }

  resize(width: number, height: number) {
    this.renderer.setSize(width, height);
  }

  update(landmarks: NormalizedLandmark[], gesture: Gesture) {
    this.scene.clear();
    this.bones = [];
    this.joints = [];

    if (landmarks.length === 0) return;

    const color = this.gestureColor(gesture);
    const glowOpacity = gesture === Gesture.GRAB ? 0.6 : 0.35;

    // Draw bones
    const boneMat = new THREE.LineBasicMaterial({
      color,
      transparent: true,
      opacity: 0.75,
      linewidth: 2,
    });

    for (const [a, b] of CONNECTIONS) {
      const lmA = landmarks[a];
      const lmB = landmarks[b];
      if (!lmA || !lmB) continue;

      const points = [
        new THREE.Vector3(lmA.x, 1 - lmA.y, 0),
        new THREE.Vector3(lmB.x, 1 - lmB.y, 0),
      ];
      const geo = new THREE.BufferGeometry().setFromPoints(points);
      const bone = new THREE.Line(geo, boneMat);
      this.scene.add(bone);
    }

    // Draw joints as spheres
    const jointMat = new THREE.MeshBasicMaterial({
      color,
      transparent: true,
      opacity: 0.9,
    });

    for (const lm of landmarks) {
      const geo = new THREE.SphereGeometry(0.006, 8, 8);
      const mesh = new THREE.Mesh(geo, jointMat);
      mesh.position.set(lm.x, 1 - lm.y, 0);
      this.scene.add(mesh);
    }

    // Palm glow — large translucent circle around wrist
    const palm = landmarks[0];
    const glowGeo = new THREE.CircleGeometry(0.08, 32);
    const glowMat = new THREE.MeshBasicMaterial({
      color,
      transparent: true,
      opacity: glowOpacity * (gesture === Gesture.GRAB ? 1.2 : 1),
    });
    const glow = new THREE.Mesh(glowGeo, glowMat);
    glow.position.set(palm.x, 1 - palm.y, -1);
    this.scene.add(glow);

    this.renderer.render(this.scene, this.camera);
  }

  private gestureColor(g: Gesture): number {
    switch (g) {
      case Gesture.GRAB:      return COLORS.grab;
      case Gesture.OPEN_PALM: return COLORS.openPalm;
      case Gesture.POINT:     return COLORS.point;
      default:                return COLORS.idle;
    }
  }

  dispose() {
    this.renderer.dispose();
  }
}
