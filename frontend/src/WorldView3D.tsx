import { useEffect, useRef, useState } from 'react'
import * as THREE from 'three'
import { OrbitControls } from 'three/addons/controls/OrbitControls.js'
import { patchCube, patchOpacity, voxelStride } from './patchStyle'
import { penRadius, visiblePenTrails } from './penTrail'
import { applyPov, type PovMode } from './pov'
import { ShapeLibrary, groupTurtles, shapeScale } from './turtleShape'
import type { Observer3D, World } from './types'
import { WorldCanvas } from './WorldCanvas'
import { wrapSegments, type WrapBounds } from './wrapSegment'

// Interactive view of a NetLogo 3D world. Scene coordinates are NetLogo
// coordinates (x east, y north, z up), with the camera's up vector on +z.
//
// Drag to orbit, scroll to zoom, right-drag or shift-drag to pan. The
// camera follows the model's observer only when an observer camera
// command runs (observer.revision changes), so a running model never
// fights the user's own orbiting.
//
// follow, ride, and watch are the exception: the camera tracks the
// subject turtle every frame. follow keeps the user's orbit around the
// turtle, ride sits on it looking along its heading, and watch leaves
// the camera put and turns it to face the turtle. reset-perspective
// clears that and the observer revision sends the camera home.
//
// Patches and turtles are InstancedMeshes, so a dense world is a couple
// of draw calls per shape. Patch instances are rebuilt only when the voxel
// buffer changes. Turtle geometry is cached per shape and is not rebuilt
// when a turtle moves or changes color; a shape change only moves that
// turtle onto another cached mesh.

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
const maxCapacity = 1 << 20

type Bounds = { minX: number; maxX: number; minY: number; maxY: number; minZ: number; maxZ: number }

class Scene3D {
  private renderer: THREE.WebGLRenderer
  private scene = new THREE.Scene()
  private camera: THREE.PerspectiveCamera
  private controls: OrbitControls
  private box: THREE.LineSegments | null = null
  private patches: THREE.InstancedMesh | null = null
  private links: THREE.LineSegments
  private trails: THREE.InstancedMesh | null = null
  private trailGeometry = new THREE.CylinderGeometry(0.5, 0.5, 1, 5)
  private trailMaterial = new THREE.MeshLambertMaterial({ color: 0xffffff })
  private patchGeometry = new THREE.BoxGeometry(patchCube, patchCube, patchCube)
  private patchMaterial = patchMaterial()
  private shapes = new ShapeLibrary()
  private shapeBatches = new Map<string, { marked: THREE.InstancedMesh | null; fixed: THREE.InstancedMesh | null }>()
  private markedMaterial = new THREE.MeshLambertMaterial({ color: 0xffffff, side: THREE.DoubleSide })
  private fixedMaterial = new THREE.MeshLambertMaterial({ vertexColors: true, side: THREE.DoubleSide })
  private billboards = 0
  private painted: World | null = null
  private voxels: string | undefined
  private boundsKey = ''
  private bounds: Bounds | null = null
  private observerRevision = -1
  private povMode: PovMode = ''
  private followOffset: { x: number; y: number; z: number } | null = null
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
    const cameraMoved = this.controls.update()
    if (cameraMoved) this.dirty = true
    // Side-view shapes face the camera, so an orbit redraws them even
    // when the world itself has not changed.
    if (cameraMoved && this.billboards > 0 && this.painted) this.updateTurtles(this.painted)
    // A drag while following changes the orbit. Keep that offset so the
    // next world frame does not snap back to the old one.
    if (this.povMode === 'follow') {
      this.followOffset = {
        x: this.camera.position.x - this.controls.target.x,
        y: this.camera.position.y - this.controls.target.y,
        z: this.camera.position.z - this.controls.target.z,
      }
    }
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
    this.trailGeometry.dispose()
    this.trailMaterial.dispose()
    this.clearShapeBatches()
    this.shapes.dispose()
    this.markedMaterial.dispose()
    this.fixedMaterial.dispose()
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
      if (!newWorld || obs.revision > 0) {
        this.applyObserver(obs)
        // An observer command (including reset-perspective) chooses a new
        // orbit. Follow recaptures it around the turtle on this frame.
        this.followOffset = null
      } else if (this.observerRevision >= 0) this.resetView()
      this.observerRevision = obs.revision
    }
    if (world.voxels !== this.voxels) {
      this.voxels = world.voxels
      this.updatePatches(world.voxels ?? '')
    }
    this.updateTurtles(world)
    this.updateLinks(world)
    this.updateTrails(world)
    this.applyPerspective(world)
  }

  // applyPerspective moves the camera for follow, ride, and watch.
  // An empty mode restores a free camera without jumping it; reset-perspective
  // has already placed it via the observer.
  private applyPerspective(world: World) {
    const raw = world.perspective ?? ''
    const mode: PovMode = raw === 'follow' || raw === 'ride' || raw === 'watch' ? raw : ''
    const who = world.subject
    const found = who == null ? undefined : world.turtles.find((t) => t.who === who)
    const turtle = found
      ? {
          x: found.x,
          y: found.y,
          z: found.z ?? 0,
          heading: found.heading,
          pitch: found.pitch ?? 0,
          roll: found.roll ?? 0,
        }
      : null
    // Keep the saved orbit only while follow continues. Entering follow,
    // leaving it, or an observer command (which already cleared the offset)
    // starts from the camera as it is now.
    if (!(mode === 'follow' && this.povMode === 'follow')) {
      if (mode !== 'follow') this.followOffset = null
    }
    const pose = applyPov(
      mode,
      turtle,
      { x: this.camera.position.x, y: this.camera.position.y, z: this.camera.position.z },
      { x: this.controls.target.x, y: this.controls.target.y, z: this.controls.target.z },
      mode === 'follow' ? this.followOffset : null,
    )
    const was = this.povMode
    this.povMode = mode
    this.followOffset = pose.followOffset
    if (!mode && !was) return
    if (mode) {
      this.camera.position.set(pose.position.x, pose.position.y, pose.position.z)
      this.camera.up.set(pose.up.x, pose.up.y, pose.up.z)
      this.controls.target.set(pose.target.x, pose.target.y, pose.target.z)
    } else {
      this.camera.up.set(0, 0, 1)
    }
    this.controls.enableRotate = !pose.lockOrbit
    this.controls.enablePan = !pose.lockOrbit
    this.controls.update()
    this.dirty = true
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
    colored = true,
  ): THREE.InstancedMesh {
    if (current && current.geometry === geometry && current.material === material && current.instanceMatrix.count >= n) {
      return current
    }
    if (current) this.dropMesh(current)
    let cap = 64
    while (cap < n && cap < maxCapacity) cap *= 2
    const mesh = new THREE.InstancedMesh(geometry, material, cap)
    mesh.instanceMatrix.setUsage(THREE.DynamicDrawUsage)
    // Create the color attribute now so the shader is built with it.
    // Shape parts that carry their own colors must not be tinted.
    if (colored) mesh.setColorAt(0, this.color.set(0xffffff))
    mesh.count = 0
    mesh.frustumCulled = false
    this.scene.add(mesh)
    return mesh
  }

  private updatePatches(encoded: string) {
    const bytes = decodeBase64(encoded)
    const n = Math.min(Math.floor(bytes.length / voxelStride), maxCapacity)
    const mesh = this.instanced(this.patches, n, this.patchGeometry, this.patchMaterial)
    this.patches = mesh
    mesh.visible = this.patchesVisible
    const alphas = patchAlphas(this.patchGeometry, mesh.instanceMatrix.count)
    const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength)
    for (let i = 0; i < n; i++) {
      const o = i * voxelStride
      this.scratch.makeTranslation(view.getInt16(o, true), view.getInt16(o + 2, true), view.getInt16(o + 4, true))
      mesh.setMatrixAt(i, this.scratch)
      this.color.setRGB(bytes[o + 6] / 255, bytes[o + 7] / 255, bytes[o + 8] / 255, THREE.SRGBColorSpace)
      mesh.setColorAt(i, this.color)
      alphas.setX(i, patchOpacity(bytes[o + 9]))
    }
    mesh.count = n
    mesh.instanceMatrix.needsUpdate = true
    if (mesh.instanceColor) mesh.instanceColor.needsUpdate = true
    alphas.needsUpdate = true
  }

  private updateTurtles(world: World) {
    if (this.shapes.sync(world.shapes)) this.clearShapeBatches()
    const groups = groupTurtles(world.turtles, this.shapes)
    const used = new Set<string>()
    this.billboards = 0
    for (const [key, turtles] of groups) {
      used.add(key)
      const built = this.shapes.resolve(turtles[0].shape)
      if (built.billboard) this.billboards += turtles.length
      const batch = this.shapeBatches.get(key) ?? { marked: null, fixed: null }
      this.shapeBatches.set(key, batch)
      batch.marked = this.paintTurtles(batch.marked, built.marked, turtles, true, built.billboard)
      batch.fixed = this.paintTurtles(batch.fixed, built.fixed, turtles, false, built.billboard)
    }
    for (const [key, batch] of this.shapeBatches) {
      if (used.has(key)) continue
      hideMesh(batch.marked)
      hideMesh(batch.fixed)
    }
  }

  // paintTurtles writes one instanced mesh for a shared geometry. colored
  // meshes take the turtle color; the others use colors baked into the shape.
  private paintTurtles(
    current: THREE.InstancedMesh | null,
    geometry: THREE.BufferGeometry | null,
    turtles: World['turtles'],
    colored: boolean,
    billboard: boolean,
  ): THREE.InstancedMesh | null {
    if (!geometry || turtles.length === 0) {
      hideMesh(current)
      return current
    }
    const material = colored ? this.markedMaterial : this.fixedMaterial
    const mesh = this.instanced(current, turtles.length, geometry, material, colored)
    mesh.visible = true
    const right = new THREE.Vector3()
    const forward = new THREE.Vector3()
    const up = new THREE.Vector3()
    const cam = this.camera.position
    const n = Math.min(turtles.length, mesh.instanceMatrix.count)
    for (let i = 0; i < n; i++) {
      const t = turtles[i]
      const s = shapeScale(t.size)
      if (billboard) billboardBasis(cam, t.x, t.y, t.z ?? 0, this.camera.up, right, forward, up)
      else orientationBasis(t.heading, t.pitch ?? 0, t.roll ?? 0, right, forward, up)
      this.scratch.makeBasis(right.multiplyScalar(s), forward.multiplyScalar(s), up.multiplyScalar(s))
      this.scratch.setPosition(t.x, t.y, t.z ?? 0)
      mesh.setMatrixAt(i, this.scratch)
      if (colored) {
        this.color.setRGB(t.r / 255, t.g / 255, t.b / 255, THREE.SRGBColorSpace)
        mesh.setColorAt(i, this.color)
      }
    }
    mesh.count = n
    mesh.instanceMatrix.needsUpdate = true
    if (colored && mesh.instanceColor) mesh.instanceColor.needsUpdate = true
    return mesh
  }

  private clearShapeBatches() {
    for (const batch of this.shapeBatches.values()) {
      this.dropMesh(batch.marked)
      this.dropMesh(batch.fixed)
    }
    this.shapeBatches.clear()
    this.billboards = 0
  }

  private dropMesh(mesh: THREE.InstancedMesh | null) {
    if (!mesh) return
    this.scene.remove(mesh)
    mesh.dispose()
  }

  // updateTrails draws pen-down strokes as tubes and leaves out whatever
  // pen-erase has removed. clear-drawing arrives as an empty list. Turtles
  // start pen-up, so a model that never calls pen-down keeps this empty.
  private updateTrails(world: World) {
    const pieces = visiblePenTrails(world.drawing ?? [], linkWrapBounds(world))
    if (pieces.length === 0) {
      if (this.trails) {
        this.trails.count = 0
        this.trails.visible = false
      }
      return
    }
    const mesh = this.instanced(this.trails, pieces.length, this.trailGeometry, this.trailMaterial)
    this.trails = mesh
    mesh.visible = true
    const yAxis = new THREE.Vector3(0, 1, 0)
    const dir = new THREE.Vector3()
    const mid = new THREE.Vector3()
    const quat = new THREE.Quaternion()
    const scale = new THREE.Vector3()
    for (let i = 0; i < pieces.length; i++) {
      const p = pieces[i]
      dir.set(p.bx - p.ax, p.by - p.ay, p.bz - p.az)
      const len = dir.length()
      if (len < 1e-8) {
        this.scratch.makeScale(0, 0, 0)
        mesh.setMatrixAt(i, this.scratch)
        continue
      }
      dir.multiplyScalar(1 / len)
      mid.set((p.ax + p.bx) / 2, (p.ay + p.by) / 2, (p.az + p.bz) / 2)
      quat.setFromUnitVectors(yAxis, dir)
      const radius = penRadius(p.size)
      scale.set(radius * 2, len, radius * 2)
      this.scratch.compose(mid, quat, scale)
      mesh.setMatrixAt(i, this.scratch)
      this.color.setRGB(p.r / 255, p.g / 255, p.b / 255, THREE.SRGBColorSpace)
      mesh.setColorAt(i, this.color)
    }
    mesh.count = pieces.length
    mesh.instanceMatrix.needsUpdate = true
    if (mesh.instanceColor) mesh.instanceColor.needsUpdate = true
  }

  private updateLinks(world: World) {
    const shown = world.links.filter((l) => !l.hidden)
    const bounds = linkWrapBounds(world)
    const pos: number[] = []
    const col: number[] = []
    for (const l of shown) {
      const segs = wrapSegments(
        { x: l.x1, y: l.y1, z: l.z1 ?? 0 },
        { x: l.x2, y: l.y2, z: l.z2 ?? 0 },
        bounds,
      )
      this.color.setRGB(l.r / 255, l.g / 255, l.b / 255, THREE.SRGBColorSpace)
      const r = this.color.r
      const g = this.color.g
      const b = this.color.b
      for (const s of segs) {
        pos.push(s.a.x, s.a.y, s.a.z, s.b.x, s.b.y, s.b.z)
        col.push(r, g, b, r, g, b)
      }
    }
    const g = this.links.geometry
    g.setAttribute('position', new THREE.BufferAttribute(Float32Array.from(pos), 3))
    g.setAttribute('color', new THREE.BufferAttribute(Float32Array.from(col), 3))
    g.computeBoundingSphere()
  }
}

// linkWrapBounds is the half-patch wall on each axis. A link is split
// only on axes whose wrap flag is set.
function linkWrapBounds(world: World): WrapBounds {
  return {
    x: { min: world.minPxcor - 0.5, max: world.maxPxcor + 0.5, wrap: world.wrapX },
    y: { min: world.minPycor - 0.5, max: world.maxPycor + 0.5, wrap: world.wrapY },
    z: {
      min: (world.minPzcor ?? 0) - 0.5,
      max: (world.maxPzcor ?? 0) + 0.5,
      wrap: world.wrapZ ?? false,
    },
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

// patchMaterial draws cubes with a per-instance alpha so an RGBA pcolor
// stays as transparent as the model asked, and a normal color stays
// see-through. Depth is not written, so turtles inside the volume show.
function patchMaterial(): THREE.MeshLambertMaterial {
  const material = new THREE.MeshLambertMaterial({ color: 0xffffff, transparent: true, depthWrite: false })
  material.onBeforeCompile = (shader) => {
    shader.vertexShader = shader.vertexShader
      .replace('#include <common>', '#include <common>\nattribute float patchAlpha;\nvarying float vPatchAlpha;')
      .replace('#include <begin_vertex>', '#include <begin_vertex>\nvPatchAlpha = patchAlpha;')
    shader.fragmentShader = shader.fragmentShader
      .replace('#include <common>', '#include <common>\nvarying float vPatchAlpha;')
      .replace('#include <opaque_fragment>', 'diffuseColor.a *= vPatchAlpha;\n#include <opaque_fragment>')
  }
  return material
}

function patchAlphas(geometry: THREE.BufferGeometry, cap: number): THREE.InstancedBufferAttribute {
  const prev = geometry.getAttribute('patchAlpha')
  if (prev instanceof THREE.InstancedBufferAttribute && prev.count >= cap) return prev
  const attr = new THREE.InstancedBufferAttribute(new Float32Array(Math.max(cap, 1)), 1)
  attr.setUsage(THREE.DynamicDrawUsage)
  geometry.setAttribute('patchAlpha', attr)
  return attr
}

function hideMesh(mesh: THREE.InstancedMesh | null) {
  if (!mesh) return
  mesh.count = 0
  mesh.visible = false
}

// billboardBasis faces a flat shape at the camera and keeps it upright.
// local y is the normal toward the camera, local z is up.
function billboardBasis(
  cam: THREE.Vector3,
  x: number,
  y: number,
  z: number,
  camUp: THREE.Vector3,
  right: THREE.Vector3,
  normal: THREE.Vector3,
  up: THREE.Vector3,
) {
  normal.set(cam.x - x, cam.y - y, cam.z - z)
  if (normal.lengthSq() < 1e-8) normal.set(0, -1, 0)
  else normal.normalize()
  up.set(0, 0, 1)
  if (Math.abs(normal.dot(up)) > 0.98) {
    up.copy(camUp)
    if (up.lengthSq() < 1e-8) up.set(0, 1, 0)
    else up.normalize()
  }
  right.crossVectors(up, normal)
  if (right.lengthSq() < 1e-8) right.set(1, 0, 0)
  else right.normalize()
  up.crossVectors(normal, right).normalize()
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
