// FileOrb.ts
// The glowing 3D orb that represents a file being held or transferred.
// Attaches to the palm landmark during GRAB, flies off on OPEN_PALM.

import * as THREE from 'three';
import type { NormalizedLandmark } from '$lib/gesture/HandTracker';

export type OrbState = 'hidden' | 'attached' | 'sending' | 'complete';

export class FileOrb {
  private scene: THREE.Scene;
  private camera: THREE.OrthographicCamera;
  private renderer: THREE.WebGLRenderer;
  private orb: THREE.Mesh | null = null;
  private glow: THREE.Mesh | null = null;
  private state: OrbState = 'hidden';
  private targetPos = new THREE.Vector3();
  private currentPos = new THREE.Vector3(0.5, 0.5, 0);
  private clock = new THREE.Clock();
  private fileName = '';

  constructor(canvas: HTMLCanvasElement) {
    this.scene = new THREE.Scene();
    this.camera = new THREE.OrthographicCamera(0, 1, 1, 0, -10, 10);
    this.renderer = new THREE.WebGLRenderer({ canvas, alpha: true });
    this.renderer.setPixelRatio(window.devicePixelRatio);
    this.renderer.setClearColor(0x000000, 0);
    this.buildOrb();
  }

  private buildOrb() {
    // Core sphere
    const geo = new THREE.SphereGeometry(0.035, 32, 32);
    const mat = new THREE.MeshBasicMaterial({
      color: 0x00d4ff,
      transparent: true,
      opacity: 0.9,
    });
    this.orb = new THREE.Mesh(geo, mat);

    // Outer glow shell
    const glowGeo = new THREE.SphereGeometry(0.055, 32, 32);
    const glowMat = new THREE.MeshBasicMaterial({
      color: 0x00d4ff,
      transparent: true,
      opacity: 0.2,
      side: THREE.BackSide,
    });
    this.glow = new THREE.Mesh(glowGeo, glowMat);

    this.orb.visible = false;
    this.glow.visible = false;
    this.scene.add(this.orb, this.glow);
  }

  attach(palmLandmark: NormalizedLandmark, fileName: string) {
    this.state = 'attached';
    this.fileName = fileName;
    if (this.orb) this.orb.visible = true;
    if (this.glow) this.glow.visible = true;
  }

  startSend() {
    this.state = 'sending';
    // Animate orb flying to top-right (representing "sending to other device")
    this.targetPos.set(1.1, -0.1, 0);
  }

  complete() {
    this.state = 'complete';
    setTimeout(() => this.hide(), 1500);
  }

  hide() {
    this.state = 'hidden';
    if (this.orb) this.orb.visible = false;
    if (this.glow) this.glow.visible = false;
  }

  tick(palmLandmark?: NormalizedLandmark) {
    if (!this.orb || !this.glow) return;

    const t = this.clock.getDelta();
    const pulse = Math.sin(Date.now() * 0.003) * 0.01;

    if (this.state === 'attached' && palmLandmark) {
      // Follow palm centroid (wrist landmark)
      this.targetPos.set(palmLandmark.x, 1 - palmLandmark.y, 0);
      this.currentPos.lerp(this.targetPos, 0.2);
    } else if (this.state === 'sending') {
      this.currentPos.lerp(this.targetPos, 0.05);
    }

    if (this.state !== 'hidden') {
      this.orb.position.copy(this.currentPos);
      this.glow.position.copy(this.currentPos);

      // Pulse scale
      const s = 1 + pulse;
      this.orb.scale.setScalar(s);
      this.glow.scale.setScalar(s * 1.1);

      this.renderer.render(this.scene, this.camera);
    }
  }

  resize(w: number, h: number) {
    this.renderer.setSize(w, h);
  }

  get currentState(): OrbState { return this.state; }
  get currentFileName(): string { return this.fileName; }

  detach() {
    this.hide();
  }

  dispose() {
    this.renderer.dispose();
  }
}
