import * as THREE from 'three'
import { mergeGeometries } from 'three/addons/utils/BufferGeometryUtils.js'
import type { ShapeElement, TurtleShape } from './types.ts'

// NetLogo 3D turtle shapes.
//
// The desktop 3D view maps a few shape names onto real solids (NetLogo
// manual, "3D Shapes") and draws every other shape from its 2D vector
// definition:
//
//   default, arrow     flattened four-sided cone (the 3D turtle / pyramid)
//   circle             sphere
//   dot                smaller sphere (the 2D dot is 120/300 of the editor)
//   square             cube
//   triangle           round cone
//   cylinder           cylinder, axis along the turtle's up
//   line               thin rod along the turtle's forward axis
//   line half          the forward half of that rod
//   car                a simple 3D car (body, cabin, four wheels)
//
// Arrow is a cone here because this view was asked to draw default and
// arrow as a cone/pyramid. NetLogo's manual extrudes the 2D arrow instead.
// The car is an approximation of the desktop 3D car; the model file only
// stores the 2D drawing.
//
// Other rotatable shapes are the top view, extruded a little along the
// turtle's up ("cookie cutter") and turned with heading, pitch, and roll.
// Other non-rotatable shapes are flat and face the camera, heading ignored,
// which is how NetLogo draws a side view such as a sheep or a person.
//
// Geometry is in the turtle's frame at size 1: +x right, +y forward, +z up.
// One patch is one unit. Callers scale by the turtle's size. Geometries are
// cached; nothing here rebuilds a mesh per frame.

const shapeBox = 300
const extrusionDepth = 0.16
const lineWidth = 0.045

export type BuiltinKind = 'dart' | 'sphere' | 'dot' | 'cube' | 'cone' | 'cylinder' | 'line' | 'line-half' | 'car'

export type BuiltShape = {
  // Stable id for instancing. default and arrow share one.
  key: string
  builtin: BuiltinKind | null
  // True when the mesh is flat in the plane that should face the camera.
  billboard: boolean
  // Painted with the turtle's color.
  marked: THREE.BufferGeometry | null
  // Painted with the colors stored in the shape. Null when every element
  // takes the turtle's color.
  fixed: THREE.BufferGeometry | null
}

export type ShapeTurtle = {
  shape: string
  hidden: boolean
}

const builtinCache = new Map<BuiltinKind, BuiltShape>()

export function canonicalShapeName(name: string): string {
  return (name || '').toLowerCase().trim().replace(/\s+/g, ' ')
}

// builtinKind is the solid a shape name maps to, or null when the name is
// drawn from its vector definition.
export function builtinKind(name: string): BuiltinKind | null {
  switch (canonicalShapeName(name).replace(/ /g, '-')) {
    case 'default':
    case 'arrow':
      return 'dart'
    case 'circle':
      return 'sphere'
    case 'dot':
      return 'dot'
    case 'square':
      return 'cube'
    case 'triangle':
      return 'cone'
    case 'cylinder':
      return 'cylinder'
    case 'line':
      return 'line'
    case 'line-half':
      return 'line-half'
    case 'car':
      return 'car'
    default:
      return null
  }
}

// shapeScale is the instance scale for a turtle size. Zero matches the
// old view (drawn at size 1). A negative size mirrors the shape.
export function shapeScale(size: number): number {
  if (!Number.isFinite(size) || size === 0) return 1
  return size
}

export class ShapeLibrary {
  private key = ''
  private byName = new Map<string, TurtleShape>()
  private built = new Map<string, BuiltShape>()

  // sync installs the model's shape list. False means the list is the
  // same as last time and cached geometries are still valid.
  sync(list: readonly TurtleShape[] | null | undefined): boolean {
    const next = libraryKey(list)
    if (next === this.key) return false
    this.disposeBuilt()
    this.key = next
    this.byName = new Map()
    for (const sh of list ?? []) {
      const name = canonicalShapeName(sh.name)
      if (name) this.byName.set(name, sh)
    }
    return true
  }

  // resolve is the geometry for a turtle shape name. An unknown name uses
  // the library's default shape when that is not itself a solid, and the
  // dart otherwise.
  resolve(name: string): BuiltShape {
    const canon = canonicalShapeName(name) || 'default'
    const kind = builtinKind(canon)
    if (kind) return this.builtin(kind)
    const spec = this.byName.get(canon) ?? this.byName.get('default')
    if (!spec || builtinKind(spec.name) || (spec.elements ?? []).length === 0) return this.builtin('dart')
    const key = `vector:${canonicalShapeName(spec.name)}:${signature(spec)}`
    const hit = this.built.get(key)
    if (hit) return hit
    const built = buildVector(key, spec)
    this.built.set(key, built)
    return built
  }

  dispose() {
    this.disposeBuilt()
    this.key = ''
    this.byName.clear()
  }

  private builtin(kind: BuiltinKind): BuiltShape {
    const hit = builtinCache.get(kind)
    if (hit) return hit
    const built: BuiltShape = {
      key: `builtin:${kind}`,
      builtin: kind,
      billboard: false,
      marked: buildBuiltin(kind),
      fixed: null,
    }
    builtinCache.set(kind, built)
    return built
  }

  private disposeBuilt() {
    for (const built of this.built.values()) {
      built.marked?.dispose()
      built.fixed?.dispose()
    }
    this.built.clear()
  }
}

// groupTurtles buckets visible turtles by the geometry they share.
// A shape change moves a turtle from one bucket to another. Hidden
// turtles are left out.
export function groupTurtles<T extends ShapeTurtle>(turtles: readonly T[], lib: ShapeLibrary): Map<string, T[]> {
  const out = new Map<string, T[]>()
  for (const t of turtles) {
    if (t.hidden) continue
    const key = lib.resolve(t.shape).key
    const list = out.get(key)
    if (list) list.push(t)
    else out.set(key, [t])
  }
  return out
}

export function geometryExtent(geo: THREE.BufferGeometry | null): { min: number[]; max: number[]; count: number } {
  if (!geo) return { min: [0, 0, 0], max: [0, 0, 0], count: 0 }
  geo.computeBoundingBox()
  const box = geo.boundingBox
  if (!box) return { min: [0, 0, 0], max: [0, 0, 0], count: 0 }
  const pos = geo.getAttribute('position')
  return {
    min: [box.min.x, box.min.y, box.min.z],
    max: [box.max.x, box.max.y, box.max.z],
    count: pos ? pos.count : 0,
  }
}

// maxRadiusNear reports the largest horizontal distance from the origin
// among vertices whose |z| is at least zMin. A cylinder's rim stays wide
// at the ends; a sphere's poles do not.
export function maxRadiusNear(geo: THREE.BufferGeometry | null, zMin: number): number {
  if (!geo) return 0
  const pos = geo.getAttribute('position')
  if (!pos) return 0
  let best = 0
  for (let i = 0; i < pos.count; i++) {
    if (Math.abs(pos.getZ(i)) + 1e-6 < zMin) continue
    const r = Math.hypot(pos.getX(i), pos.getY(i))
    if (r > best) best = r
  }
  return best
}

function libraryKey(list: readonly TurtleShape[] | null | undefined): string {
  if (!list || list.length === 0) return ''
  let h = 2166136261
  for (const sh of list) {
    h = mix(h, canonicalShapeName(sh.name))
    h = mix(h, sh.rotatable ? '1' : '0')
    h = mix(h, signature(sh))
  }
  return (h >>> 0).toString(16) + ':' + list.length
}

function signature(sh: TurtleShape): string {
  let h = 2166136261
  for (const el of sh.elements ?? []) {
    h = mix(h, el.kind)
    h = mix(h, el.filled ? 'f' : 'o')
    h = mix(h, el.marked ? 'm' : 'c')
    h = mix(h, String(el.r) + ',' + String(el.g) + ',' + String(el.b))
    h = mix(h, [el.x, el.y, el.w, el.h].map((n) => Math.round(n * 10)).join(','))
    const pts = el.points ?? []
    h = mix(h, String(pts.length))
    for (let i = 0; i < pts.length; i++) h = mix(h, String(Math.round(pts[i] * 10)))
  }
  return (h >>> 0).toString(16)
}

function mix(h: number, s: string): number {
  for (let i = 0; i < s.length; i++) h = Math.imul(h ^ s.charCodeAt(i), 16777619)
  return h
}

function buildBuiltin(kind: BuiltinKind): THREE.BufferGeometry {
  switch (kind) {
    case 'dart': {
      // Four-sided cone, tip forward, flattened so it reads as the
      // classic turtle rather than the round triangle cone.
      const geo = new THREE.ConeGeometry(0.38, 1, 4)
      geo.scale(1, 1, 0.55)
      return geo
    }
    case 'sphere':
      return new THREE.SphereGeometry(0.5, 24, 16)
    case 'dot':
      return new THREE.SphereGeometry(0.2, 16, 12)
    case 'cube':
      return new THREE.BoxGeometry(1, 1, 1)
    case 'cone':
      return new THREE.ConeGeometry(0.5, 1, 24)
    case 'cylinder': {
      const geo = new THREE.CylinderGeometry(0.5, 0.5, 1, 24)
      geo.rotateX(Math.PI / 2)
      return geo
    }
    case 'line':
      return new THREE.CylinderGeometry(lineWidth, lineWidth, 1, 8)
    case 'line-half': {
      const geo = new THREE.CylinderGeometry(lineWidth, lineWidth, 0.5, 8)
      geo.translate(0, 0.25, 0)
      return geo
    }
    case 'car':
      return buildCar()
  }
}

function buildCar(): THREE.BufferGeometry {
  const parts: THREE.BufferGeometry[] = []
  const body = new THREE.BoxGeometry(0.46, 0.92, 0.22)
  body.translate(0, 0, 0.18)
  parts.push(body)
  const cabin = new THREE.BoxGeometry(0.4, 0.42, 0.2)
  cabin.translate(0, -0.06, 0.38)
  parts.push(cabin)
  const corners: [number, number][] = [
    [0.24, 0.26],
    [-0.24, 0.26],
    [0.24, -0.28],
    [-0.24, -0.28],
  ]
  for (const [x, y] of corners) {
    const wheel = new THREE.CylinderGeometry(0.12, 0.12, 0.1, 10)
    wheel.rotateZ(Math.PI / 2)
    wheel.translate(x, y, 0.12)
    parts.push(wheel)
  }
  return mergeAll(parts) ?? new THREE.BoxGeometry(0.5, 1, 0.3)
}

function buildVector(key: string, spec: TurtleShape): BuiltShape {
  const billboard = !spec.rotatable
  const marked: THREE.BufferGeometry[] = []
  const fixed: THREE.BufferGeometry[] = []
  const elements = spec.elements ?? []
  for (let i = 0; i < elements.length; i++) {
    const el = elements[i]
    const geo = elementGeometry(el, i, billboard)
    if (!geo) continue
    if (billboard) layFacingCamera(geo)
    if (el.marked) marked.push(geo)
    else {
      paint(geo, el.r, el.g, el.b)
      fixed.push(geo)
    }
  }
  const built: BuiltShape = {
    key,
    builtin: null,
    billboard,
    marked: mergeAll(marked),
    fixed: mergeAll(fixed),
  }
  if (!built.marked && !built.fixed) {
    built.marked = buildBuiltin('dart')
    built.billboard = false
    built.key = 'builtin:dart'
    built.builtin = 'dart'
  }
  return built
}

function elementGeometry(el: ShapeElement, order: number, billboard: boolean): THREE.BufferGeometry | null {
  const kind = (el.kind || '').toLowerCase()
  let geo: THREE.BufferGeometry | null = null
  if (kind === 'polygon') geo = polygonGeometry(el.points ?? [], el.filled, billboard)
  else if (kind === 'rectangle') geo = polygonGeometry([el.x, el.y, el.x + el.w, el.y, el.x + el.w, el.y + el.h, el.x, el.y + el.h], el.filled, billboard)
  else if (kind === 'circle') geo = circleGeometry(el, billboard)
  else if (kind === 'line') geo = lineGeometry(el.points ?? [], billboard)
  if (!geo) return null
  // Later elements sit a hair above earlier ones so a pupil stays on a face.
  geo.translate(0, 0, order * 0.004)
  return geo
}

function polygonGeometry(flat: number[], filled: boolean, billboard: boolean): THREE.BufferGeometry | null {
  const pts = toLocal(flat)
  if (pts.length < 2) return null
  if (!filled) return strokeLoop(pts, true, billboard)
  if (pts.length < 3) return null
  const shape = shapeFrom(pts)
  if (!shape) return null
  return extrude(shape, billboard)
}

function circleGeometry(el: ShapeElement, billboard: boolean): THREE.BufferGeometry | null {
  const rx = Math.abs(el.w) / 2 / shapeBox
  const ry = Math.abs(el.h) / 2 / shapeBox
  if (rx < 1e-4 || ry < 1e-4) return null
  const cx = (el.x + el.w / 2 - 150) / shapeBox
  const cy = (150 - (el.y + el.h / 2)) / shapeBox
  if (!el.filled) {
    const steps = 20
    const pts: { x: number; y: number }[] = []
    for (let i = 0; i < steps; i++) {
      const a = (i / steps) * Math.PI * 2
      pts.push({ x: cx + Math.cos(a) * rx, y: cy + Math.sin(a) * ry })
    }
    return strokeLoop(pts, true, billboard)
  }
  const shape = new THREE.Shape()
  shape.absellipse(cx, cy, rx, ry, 0, Math.PI * 2, false, 0)
  return extrude(shape, billboard)
}

function lineGeometry(flat: number[], billboard: boolean): THREE.BufferGeometry | null {
  const pts = toLocal(flat).slice(0, 2)
  if (pts.length < 2) return null
  return strokeLoop(pts, false, billboard)
}

function toLocal(flat: number[]): { x: number; y: number }[] {
  const out: { x: number; y: number }[] = []
  for (let i = 0; i + 1 < flat.length; i += 2) {
    const x = (flat[i] - 150) / shapeBox
    const y = (150 - flat[i + 1]) / shapeBox
    const prev = out[out.length - 1]
    if (prev && Math.hypot(prev.x - x, prev.y - y) < 1e-6) continue
    out.push({ x, y })
  }
  if (out.length > 1 && Math.hypot(out[0].x - out[out.length - 1].x, out[0].y - out[out.length - 1].y) < 1e-6) out.pop()
  return out
}

function shapeFrom(pts: { x: number; y: number }[]): THREE.Shape | null {
  if (pts.length < 3) return null
  const shape = new THREE.Shape()
  shape.moveTo(pts[0].x, pts[0].y)
  for (let i = 1; i < pts.length; i++) shape.lineTo(pts[i].x, pts[i].y)
  shape.closePath()
  return shape
}

function extrude(shape: THREE.Shape, billboard: boolean): THREE.BufferGeometry | null {
  try {
    // A side-view shape has no thickness. A top view is a cookie cutter.
    if (billboard) return new THREE.ShapeGeometry(shape, 10)
    const geo = new THREE.ExtrudeGeometry(shape, { depth: extrusionDepth, bevelEnabled: false, curveSegments: 10 })
    geo.translate(0, 0, -extrusionDepth / 2)
    return geo
  } catch {
    return null
  }
}

function strokeLoop(pts: { x: number; y: number }[], close: boolean, billboard: boolean): THREE.BufferGeometry | null {
  const parts: THREE.BufferGeometry[] = []
  const n = close ? pts.length : pts.length - 1
  if (n < 1) return null
  const depth = billboard ? 0.001 : extrusionDepth * 0.45
  for (let i = 0; i < n; i++) {
    const a = pts[i]
    const b = pts[(i + 1) % pts.length]
    const dx = b.x - a.x
    const dy = b.y - a.y
    const len = Math.hypot(dx, dy)
    if (len < 1e-5) continue
    const geo = new THREE.BoxGeometry(lineWidth, len, depth)
    geo.rotateZ(-Math.atan2(dx, dy))
    geo.translate((a.x + b.x) / 2, (a.y + b.y) / 2, 0)
    parts.push(geo)
  }
  return mergeAll(parts)
}

// layFacingCamera turns a shape drawn in the turtle's xy plane into a
// flat card whose +y normal points at the camera and whose up is +z.
function layFacingCamera(geo: THREE.BufferGeometry) {
  geo.rotateX(Math.PI / 2)
  geo.scale(1, -1, 1)
}

function paint(geo: THREE.BufferGeometry, r: number, g: number, b: number) {
  const color = new THREE.Color().setRGB(r / 255, g / 255, b / 255, THREE.SRGBColorSpace)
  const pos = geo.getAttribute('position')
  const colors = new Float32Array(pos.count * 3)
  for (let i = 0; i < pos.count; i++) {
    colors[i * 3] = color.r
    colors[i * 3 + 1] = color.g
    colors[i * 3 + 2] = color.b
  }
  geo.setAttribute('color', new THREE.BufferAttribute(colors, 3))
}

function mergeAll(parts: THREE.BufferGeometry[]): THREE.BufferGeometry | null {
  const live = parts.filter((g) => g.getAttribute('position') && g.getAttribute('position').count > 0)
  if (live.length === 0) return null
  if (live.length === 1) return strip(live[0])
  const flat = live.map((g) => {
    const n = g.index ? g.toNonIndexed() : g
    if (n !== g) g.dispose()
    return strip(n)
  })
  const merged = mergeGeometries(flat, false)
  for (const g of flat) g.dispose()
  return merged
}

function strip(geo: THREE.BufferGeometry): THREE.BufferGeometry {
  for (const name of Object.keys(geo.attributes)) {
    if (name !== 'position' && name !== 'normal' && name !== 'color') geo.deleteAttribute(name)
  }
  return geo
}
