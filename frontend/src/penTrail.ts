import { wrapSegments, type Vec3, type WrapBounds } from './wrapSegment.ts'

// A pen stroke or stamp from the drawing layer. z is 0 in a 2D world.
export type TrailMark = {
  pen: boolean
  erase: boolean
  x: number
  y: number
  z: number
  x2: number
  y2: number
  z2: number
  size: number
  heading?: number
  pitch?: number
  roll?: number
  shape?: string
  r: number
  g: number
  b: number
}

// One straight piece of ink that is still visible. Wrapped strokes are
// already split, and pen-erase has already removed the part it covers.
export type TrailPiece = {
  ax: number
  ay: number
  az: number
  bx: number
  by: number
  bz: number
  size: number
  r: number
  g: number
  b: number
}

// penRadius is half the thickness of a pen-size, in world units. It matches
// the 2D stroke, which is 0.55 of a patch wide for pen-size 1.
export function penRadius(size: number): number {
  const s = size > 0 ? size : 1
  return s * 0.275
}

// A turtle stamp that is still on the drawing layer.
export type DrawnStamp = {
  x: number
  y: number
  z: number
  heading: number
  pitch: number
  roll: number
  size: number
  shape: string
  r: number
  g: number
  b: number
}

// stampRadius is how far a stamp's ink reaches. Size 1 fills one patch.
export function stampRadius(size: number): number {
  const s = Math.abs(size) > 0 ? Math.abs(size) : 1
  return s * 0.5
}

// visiblePenTrails replays the drawing layer and returns the pen ink that
// is still there. Stamps are not trails. clear-drawing is an empty list.
export function visiblePenTrails(marks: readonly TrailMark[], bounds: WrapBounds): TrailPiece[] {
  return visibleDrawing(marks, bounds).trails
}

// visibleDrawing replays pen strokes and stamps. pen-erase and stamp-erase
// remove earlier ink they cover. A stamp-erase does not leave a shape.
export function visibleDrawing(
  marks: readonly TrailMark[],
  bounds: WrapBounds,
): { trails: TrailPiece[]; stamps: DrawnStamp[] } {
  let trails: TrailPiece[] = []
  const stamps: DrawnStamp[] = []
  for (const m of marks) {
    if (m.pen) {
      const parts = splitMark(m, bounds)
      if (!m.erase) {
        trails.push(...parts)
        continue
      }
      for (const eraser of parts) {
        trails = eraseTrails(trails, eraser, penRadius(eraser.size))
        const keep: DrawnStamp[] = []
        for (const stamp of stamps) {
          const hit =
            pointSegmentDistance(vec(stamp.x, stamp.y, stamp.z), vec(eraser.ax, eraser.ay, eraser.az), vec(eraser.bx, eraser.by, eraser.bz)) <=
            penRadius(eraser.size) + stampRadius(stamp.size)
          if (!hit) keep.push(stamp)
        }
        stamps.length = 0
        stamps.push(...keep)
      }
      continue
    }
    const radius = stampRadius(m.size)
    if (m.erase) {
      const eraser: TrailPiece = {
        ax: m.x,
        ay: m.y,
        az: m.z,
        bx: m.x,
        by: m.y,
        bz: m.z,
        size: m.size,
        r: 0,
        g: 0,
        b: 0,
      }
      trails = eraseTrails(trails, eraser, radius)
      const keep: DrawnStamp[] = []
      for (const stamp of stamps) {
        const d = Math.hypot(stamp.x - m.x, stamp.y - m.y, stamp.z - m.z)
        if (d > radius + stampRadius(stamp.size)) keep.push(stamp)
      }
      stamps.length = 0
      stamps.push(...keep)
      continue
    }
    stamps.push({
      x: m.x,
      y: m.y,
      z: m.z,
      heading: m.heading ?? 0,
      pitch: m.pitch ?? 0,
      roll: m.roll ?? 0,
      size: m.size || 1,
      shape: m.shape && m.shape.length > 0 ? m.shape : 'default',
      r: m.r,
      g: m.g,
      b: m.b,
    })
  }
  return { trails, stamps }
}

function eraseTrails(trails: TrailPiece[], eraser: TrailPiece, radius: number): TrailPiece[] {
  const next: TrailPiece[] = []
  for (const piece of trails) next.push(...clipPiece(piece, eraser, radius))
  return next
}

function splitMark(m: TrailMark, bounds: WrapBounds): TrailPiece[] {
  const segs = wrapSegments({ x: m.x, y: m.y, z: m.z }, { x: m.x2, y: m.y2, z: m.z2 }, bounds)
  return segs.map((s) => ({
    ax: s.a.x,
    ay: s.a.y,
    az: s.a.z,
    bx: s.b.x,
    by: s.b.y,
    bz: s.b.z,
    size: m.size,
    r: m.r,
    g: m.g,
    b: m.b,
  }))
}

function clipPiece(piece: TrailPiece, eraser: TrailPiece, eraserRadius: number): TrailPiece[] {
  const a = vec(piece.ax, piece.ay, piece.az)
  const b = vec(piece.bx, piece.by, piece.bz)
  const ea = vec(eraser.ax, eraser.ay, eraser.az)
  const eb = vec(eraser.bx, eraser.by, eraser.bz)
  const len = dist(a, b)
  if (len < 1e-9) return []
  const reach = penRadius(piece.size) + eraserRadius
  const steps = Math.max(2, Math.min(48, Math.ceil(len / Math.max(reach * 0.5, 0.05))))
  const inside: boolean[] = []
  let anyIn = false
  let anyOut = false
  for (let i = 0; i <= steps; i++) {
    const hit = pointSegmentDistance(lerp(a, b, i / steps), ea, eb) <= reach + 1e-8
    inside.push(hit)
    if (hit) anyIn = true
    else anyOut = true
  }
  if (!anyIn) return [piece]
  if (!anyOut) return []
  const out: TrailPiece[] = []
  let start = -1
  const flush = (from: number, to: number) => {
    if (to <= from) return
    const p = lerp(a, b, from / steps)
    const q = lerp(a, b, to / steps)
    if (dist(p, q) < 1e-6) return
    out.push({ ...piece, ax: p.x, ay: p.y, az: p.z, bx: q.x, by: q.y, bz: q.z })
  }
  for (let i = 0; i <= steps; i++) {
    if (!inside[i]) {
      if (start < 0) start = i
    } else if (start >= 0) {
      flush(start, i - 1)
      start = -1
    }
  }
  if (start >= 0) flush(start, steps)
  return out
}

function vec(x: number, y: number, z: number): Vec3 {
  return { x, y, z }
}

function dist(a: Vec3, b: Vec3): number {
  return Math.hypot(a.x - b.x, a.y - b.y, a.z - b.z)
}

function lerp(a: Vec3, b: Vec3, t: number): Vec3 {
  return { x: a.x + (b.x - a.x) * t, y: a.y + (b.y - a.y) * t, z: a.z + (b.z - a.z) * t }
}

function pointSegmentDistance(p: Vec3, a: Vec3, b: Vec3): number {
  const abx = b.x - a.x
  const aby = b.y - a.y
  const abz = b.z - a.z
  const len2 = abx * abx + aby * aby + abz * abz
  if (len2 < 1e-18) return dist(p, a)
  let t = ((p.x - a.x) * abx + (p.y - a.y) * aby + (p.z - a.z) * abz) / len2
  if (t < 0) t = 0
  else if (t > 1) t = 1
  return dist(p, { x: a.x + abx * t, y: a.y + aby * t, z: a.z + abz * t })
}
