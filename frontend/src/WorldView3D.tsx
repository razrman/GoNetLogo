import { useEffect, useRef, useState } from 'react'
import * as THREE from 'three'
import { OrbitControls } from 'three/addons/controls/OrbitControls.js'
import type { Observer3D, World } from './types'
import { WorldCanvas } from './WorldCanvas'

// Interactive view of a NetLogo 3D world. Scene coordinates are NetLogo
// coordinates (x east, y north, z up), with the camera's up vector on +z.
//
// Drag to orbit, scroll to zoom, right-drag or shift-drag to pan. The
// camera follows the model's observer only when an observer camera
// command runs (observer.revision changes), so a running model never
// fights the user's own orbiting.
//
// Patches and turtles are InstancedMeshes, so a dense world is a couple
// of draw calls. Patch instances are rebuilt only when the voxel buffer
// changes.

export default function WorldView3D({ world }: { world: World }) {
  const wrapRef = useRef<HTMLDivElement>(null)
  const worldRef = useRef(world)
  const sceneRef = useRef<Scene3D | null>(null)
  const [showPatches, setShowPatches] = useState(true)
  // Without WebGL, fall back to the server's top-down projection.
  const [failed] = useState(() => !webglAvailable())

  useEffect(() => {
    worldRef.current = world
  }, [world])

  useEffect(() => {
    const wrap = wrapRef.current
    if (!wrap) return
    let scene: Scene3D
    try {
      scene = new Scene3D(wrap)
    } catch (err) {
      // webglAvailable said yes but the renderer still failed (for example
      // a lost context). Leave the overlay; there is nothing to draw on.
      console.error('3D view unavailable', err)
      return
    }
    sceneRef.current = scene
    let raf = 0
    const loop = () => {
      scene.frame(worldRef.current)
      raf = requestAnimationFrame(loop)
    }
    raf = requestAnimationFrame(loop)
    const observer = new ResizeObserver(() => scene.resize())
    observer.observe(wrap)
    return () => {
      cancelAnimationFrame(raf)
      observer.disconnect()
      scene.dispose()
      sceneRef.current = null
    }
  }, [])

  useEffect(() => {
    sceneRef.current?.setPatchesVisible(showPatches)
  }, [showPatches])

  if (failed) return <WorldCanvas world={world} />

  const depth = world.depth ?? 1
  return (
    <div className="world3d" ref={wrapRef}>
      <div className="world3d-overlay">
        <button type="button" onClick={() => sceneRef.current?.resetView()}>
          Reset view
        </button>
        <label>
          <input type="checkbox" checked={showPatches} onChange={(e) => setShowPatches(e.target.checked)} />
          Patches
        </label>
        <span className="world3d-badge">
          3D · {world.width}×{world.height}×{depth}
        </span>
      </div>
    </div>
  )
}

const background = 0x0c0e0a
const patchSize = 0.86
const maxCapacity = 1 << 20

type Bounds = { minX: number; maxX: number; minY: number; maxY: number; minZ: number; maxZ: number }

class Scene3D {
  private renderer: THREE.WebGLRenderer
  private scene = new THREE.Scene()
  private camera: THREE.PerspectiveCamera
  private controls: OrbitControls
  private box: THREE.LineSegments | null = null
  private patches: THREE.InstancedMesh | null = null
  private turtles: THREE.InstancedMesh | null = null
  private links: THREE.LineSegments
  private patchGeometry = new THREE.BoxGeometry(patchSize, patchSize, patchSize)
  private patchMaterial = new THREE.MeshLambertMaterial({ color: 0xffffff })
  private turtleGeometry: THREE.BufferGeometry
  private turtleMaterial = new THREE.MeshLambertMaterial({ color: 0xffffff })
  private painted: World | null = null
  private voxels: string | undefined
  private boundsKey = ''
  private bounds: Bounds | null = null
  private observerRevision = -1
  private patchesVisible = true
  private dirty = true
  private scratch = new THREE.Matrix4()
  private color = new THREE.Color()

  private wrap: HTMLDivElement

  constructor(wrap: HTMLDivElement) {
    this.wrap = wrap
    this.renderer = new THREE.WebGLRenderer({ antialias: true })
    this.renderer.setPixelRatio(window.devicePixelRatio || 1)
    this.renderer.setClearColor(background)
    this.renderer.domElement.className = 'world3d-canvas'
    this.renderer.domElement.setAttribute('aria-label', 'World view (3D)')
    wrap.prepend(this.renderer.domElement)

    this.camera = new THREE.PerspectiveCamera(45, 1, 0.1, 1000)
    this.camera.up.set(0, 0, 1)
    this.controls = new OrbitControls(this.camera, this.renderer.domElement)
    this.controls.enableDamping = true
    this.controls.dampingFactor = 0.12
    // Left drag orbits, wheel zooms, right drag pans. OrbitControls also
    // pans on shift/ctrl/meta + left drag.
    this.controls.mouseButtons = { LEFT: THREE.MOUSE.ROTATE, MIDDLE: THREE.MOUSE.DOLLY, RIGHT: THREE.MOUSE.PAN }
    this.controls.screenSpacePanning = true

    this.scene.add(new THREE.HemisphereLight(0xffffff, 0x404040, 1.6))
    const sun = new THREE.DirectionalLight(0xffffff, 1.4)
    sun.position.set(0.4, -0.6, 1)
    this.scene.add(sun)

    // A flat dart: the tip points along heading and pitch, and the flat
    // side shows roll. ConeGeometry points along +y; flatten it on z.
    this.turtleGeometry = new THREE.ConeGeometry(0.42, 1, 4, 1)
    this.turtleGeometry.rotateY(Math.PI / 4)
    this.turtleGeometry.scale(1, 1, 0.4)

    this.links = new THREE.LineSegments(
      new THREE.BufferGeometry(),
      new THREE.LineBasicMaterial({ vertexColors: true }),
    )
    this.links.frustumCulled = false
    this.scene.add(this.links)
    this.resize()
  }

  resize() {
    const w = Math.max(1, this.wrap.clientWidth)
    const h = Math.max(1, this.wrap.clientHeight)
    this.renderer.setSize(w, h, true)
    this.camera.aspect = w / h
    this.camera.updateProjectionMatrix()
    this.dirty = true
  }

  setPatchesVisible(on: boolean) {
    this.patchesVisible = on
    if (this.patches) this.patches.visible = on
    this.dirty = true
  }

  // frame runs once per animation frame. It draws only when a new world
  // frame arrived or the camera moved (including damping after a drag),
  // so an idle view costs nothing.
  frame(world: World) {
    if (world !== this.painted) {
      this.update(world)
      this.painted = world
      this.dirty = true
    }
    if (this.controls.update()) this.dirty = true
    if (!this.dirty) return
    this.dirty = false
    this.renderer.render(this.scene, this.camera)
  }

  dispose() {
    this.controls.dispose()
    this.renderer.dispose()
    this.renderer.domElement.remove()
    this.patchGeometry.dispose()
    this.patchMaterial.dispose()
    this.turtleGeometry.dispose()
    this.turtleMaterial.dispose()
    this.links.geometry.dispose()
    ;(this.links.material as THREE.Material).dispose()
    this.box?.geometry.dispose()
  }

  private update(world: World) {
    const bounds: Bounds = {
      minX: world.minPxcor,
      maxX: world.maxPxcor,
      minY: world.minPycor,
      maxY: world.maxPycor,
      minZ: world.minPzcor ?? 0,
      maxZ: world.maxPzcor ?? 0,
    }
    const key = Object.values(bounds).join(',')
    if (key !== this.boundsKey) {
      this.boundsKey = key
      this.bounds = bounds
      this.buildBox(bounds)
      this.resetView()
    }
    const obs = world.observer
    if (obs && obs.revision !== this.observerRevision) {
      // A higher revision means the model ran an observer camera command:
      // follow it. A first frame, or a lower revision (a new model was
      // loaded), only follows the observer if the model has already moved
      // it; NetLogo's untouched home pose looks straight down, which is the
      // flat view this replaces, so keep the default view instead.
      const newWorld = this.observerRevision < 0 || obs.revision < this.observerRevision
      if (!newWorld || obs.revision > 0) this.applyObserver(obs)
      else if (this.observerRevision >= 0) this.resetView()
      this.observerRevision = obs.revision
    }
    if (world.voxels !== this.voxels) {
      this.voxels = world.voxels
      this.updatePatches(world.voxels ?? '')
    }
    this.updateTurtles(world)
    this.updateLinks(world)
  }

  // resetView frames the whole world from the south-west, a little above.
  resetView() {
    const b = this.bounds
    if (!b) return
    const center = new THREE.Vector3((b.minX + b.maxX) / 2, (b.minY + b.maxY) / 2, (b.minZ + b.maxZ) / 2)
    const size = new THREE.Vector3(b.maxX - b.minX + 1, b.maxY - b.minY + 1, b.maxZ - b.minZ + 1)
    const radius = size.length() / 2
    const dist = (radius / Math.sin((this.camera.fov * Math.PI) / 360)) * 1.05
    const az = (-115 * Math.PI) / 180
    const el = (28 * Math.PI) / 180
    this.camera.position.set(
      center.x + dist * Math.cos(el) * Math.cos(az),
      center.y + dist * Math.cos(el) * Math.sin(az),
      center.z + dist * Math.sin(el),
    )
    this.fitClipping(dist)
    this.controls.target.copy(center)
    this.controls.update()
    this.dirty = true
  }

  private applyObserver(o: Observer3D) {
    const pos = new THREE.Vector3(o.x, o.y, o.z)
    const target = new THREE.Vector3(o.targetX, o.targetY, o.targetZ)
    const off = pos.clone().sub(target)
    const dist = Math.max(off.length(), 0.01)
    // Straight above or below the target leaves lookAt without a heading.
    // Nudge the camera south so north stays up on screen, as in NetLogo.
    if (Math.hypot(off.x, off.y) < dist * 1e-4) pos.y -= dist * 1e-3
    this.camera.position.copy(pos)
    this.fitClipping(dist)
    this.controls.target.copy(target)
    this.controls.update()
    this.dirty = true
  }

  private fitClipping(dist: number) {
    const b = this.bounds
    const span = b ? Math.max(b.maxX - b.minX, b.maxY - b.minY, b.maxZ - b.minZ) + 1 : 10
    this.camera.near = Math.max(0.01, dist / 1000)
    this.camera.far = (dist + span) * 20
    this.camera.updateProjectionMatrix()
  }

  private buildBox(b: Bounds) {
    if (this.box) {
      this.scene.remove(this.box)
      this.box.geometry.dispose()
    }
    const sx = b.maxX - b.minX + 1
    const sy = b.maxY - b.minY + 1
    const sz = b.maxZ - b.minZ + 1
    const edges = new THREE.EdgesGeometry(new THREE.BoxGeometry(sx, sy, sz))
    this.box = new THREE.LineSegments(edges, new THREE.LineBasicMaterial({ color: 0x8a8f7a }))
    this.box.position.set((b.minX + b.maxX) / 2, (b.minY + b.maxY) / 2, (b.minZ + b.maxZ) / 2)
    this.scene.add(this.box)
  }

  // instanced returns a mesh with room for n instances, replacing a smaller
  // one. Capacity grows in powers of two so a growing model does not
  // reallocate every frame.
  private instanced(
    current: THREE.InstancedMesh | null,
    n: number,
    geometry: THREE.BufferGeometry,
    material: THREE.Material,
  ): THREE.InstancedMesh {
    if (current && current.instanceMatrix.count >= n) return current
    if (current) {
      this.scene.remove(current)
      current.dispose()
    }
    let cap = 64
    while (cap < n && cap < maxCapacity) cap *= 2
    const mesh = new THREE.InstancedMesh(geometry, material, cap)
    mesh.instanceMatrix.setUsage(THREE.DynamicDrawUsage)
    // Create the color attribute now so the shader is built with it.
    mesh.setColorAt(0, this.color.set(0xffffff))
    mesh.count = 0
    mesh.frustumCulled = false
    this.scene.add(mesh)
    return mesh
  }

  private updatePatches(encoded: string) {
    const bytes = decodeBase64(encoded)
    const n = Math.min(Math.floor(bytes.length / 9), maxCapacity)
    const mesh = this.instanced(this.patches, n, this.patchGeometry, this.patchMaterial)
    this.patches = mesh
    mesh.visible = this.patchesVisible
    const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength)
    for (let i = 0; i < n; i++) {
      const o = i * 9
      this.scratch.makeTranslation(view.getInt16(o, true), view.getInt16(o + 2, true), view.getInt16(o + 4, true))
      mesh.setMatrixAt(i, this.scratch)
      this.color.setRGB(bytes[o + 6] / 255, bytes[o + 7] / 255, bytes[o + 8] / 255, THREE.SRGBColorSpace)
      mesh.setColorAt(i, this.color)
    }
    mesh.count = n
    mesh.instanceMatrix.needsUpdate = true
    if (mesh.instanceColor) mesh.instanceColor.needsUpdate = true
  }

  private updateTurtles(world: World) {
    const visible = world.turtles.filter((t) => !t.hidden)
    const mesh = this.instanced(this.turtles, visible.length, this.turtleGeometry, this.turtleMaterial)
    this.turtles = mesh
    const right = new THREE.Vector3()
    const forward = new THREE.Vector3()
    const up = new THREE.Vector3()
    const n = Math.min(visible.length, mesh.instanceMatrix.count)
    for (let i = 0; i < n; i++) {
      const t = visible[i]
      orientationBasis(t.heading, t.pitch ?? 0, t.roll ?? 0, right, forward, up)
      const s = t.size || 1
      // Columns: local x -> right, local y (the cone's tip) -> forward,
      // local z (the flat side's normal) -> up.
      this.scratch.makeBasis(right.multiplyScalar(s), forward.multiplyScalar(s), up.multiplyScalar(s))
      this.scratch.setPosition(t.x, t.y, t.z ?? 0)
      mesh.setMatrixAt(i, this.scratch)
      this.color.setRGB(t.r / 255, t.g / 255, t.b / 255, THREE.SRGBColorSpace)
      mesh.setColorAt(i, this.color)
    }
    mesh.count = n
    mesh.instanceMatrix.needsUpdate = true
    if (mesh.instanceColor) mesh.instanceColor.needsUpdate = true
  }

  private updateLinks(world: World) {
    const shown = world.links.filter((l) => !l.hidden)
    const pos = new Float32Array(shown.length * 6)
    const col = new Float32Array(shown.length * 6)
    shown.forEach((l, i) => {
      pos.set([l.x1, l.y1, l.z1 ?? 0, l.x2, l.y2, l.z2 ?? 0], i * 6)
      this.color.setRGB(l.r / 255, l.g / 255, l.b / 255, THREE.SRGBColorSpace)
      col.set([this.color.r, this.color.g, this.color.b, this.color.r, this.color.g, this.color.b], i * 6)
    })
    const g = this.links.geometry
    g.setAttribute('position', new THREE.BufferAttribute(pos, 3))
    g.setAttribute('color', new THREE.BufferAttribute(col, 3))
    g.computeBoundingSphere()
  }
}

// orientationBasis is NetLogo 3D's turtle frame. Forward follows heading
// (clockwise from north) and pitch (up from level); up starts as forward
// tilted 90 degrees upward and rolls toward the right.
function orientationBasis(
  heading: number,
  pitch: number,
  roll: number,
  right: THREE.Vector3,
  forward: THREE.Vector3,
  up: THREE.Vector3,
) {
  const rad = Math.PI / 180
  const sh = Math.sin(heading * rad)
  const ch = Math.cos(heading * rad)
  const sp = Math.sin(pitch * rad)
  const cp = Math.cos(pitch * rad)
  const sr = Math.sin(roll * rad)
  const cr = Math.cos(roll * rad)
  forward.set(cp * sh, cp * ch, sp)
  // Unrolled up and right.
  const ux = -sp * sh
  const uy = -sp * ch
  const uz = cp
  const rx = ch
  const ry = -sh
  up.set(ux * cr + rx * sr, uy * cr + ry * sr, uz * cr)
  right.set(rx * cr - ux * sr, ry * cr - uy * sr, -uz * sr)
}

function webglAvailable(): boolean {
  try {
    const c = document.createElement('canvas')
    return !!(c.getContext('webgl2') || c.getContext('webgl'))
  } catch {
    return false
  }
}

function decodeBase64(s: string): Uint8Array {
  if (!s) return new Uint8Array(0)
  const bin = atob(s)
  const out = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i)
  return out
}
