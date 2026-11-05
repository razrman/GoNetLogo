// Split a segment that crosses a wrapping world wall into the pieces
// NetLogo draws: from end 1 to the wall, then from the opposite wall
// onward. Segments stay in order from end 1 to end 2, so a direction
// marker on the last point sits on end 2.
//
// Walls are the half-patch edges, [minPcor - 0.5, maxPcor + 0.5].
// The short way around matches shortestWrap in pkg/nl: a wrapping
// delta is folded into [-size/2, size/2), so a halfway tie goes
// toward the negative direction. An axis with wrapping off keeps
// the straight difference.

export type AxisBounds = {
  min: number
  max: number
  wrap: boolean
}

export type WrapBounds = {
  x: AxisBounds
  y: AxisBounds
  z: AxisBounds
}

export type Vec3 = { x: number; y: number; z: number }

export type Segment = { a: Vec3; b: Vec3 }

const axes = ['x', 'y', 'z'] as const
type Axis = (typeof axes)[number]

const eps = 1e-9

function shortestDelta(from: number, to: number, axis: AxisBounds): number {
  const raw = to - from
  if (!axis.wrap) return raw
  const size = axis.max - axis.min
  if (!(size > 0)) return raw
  if (raw >= size / 2) return raw - size
  if (raw < -size / 2) return raw + size
  return raw
}

function samePoint(p: Vec3, q: Vec3): boolean {
  return Math.abs(p.x - q.x) <= eps && Math.abs(p.y - q.y) <= eps && Math.abs(p.z - q.z) <= eps
}

// wrapSegments returns the pieces of the shortest path from a to b.
// One piece when the path does not cross a wall.
export function wrapSegments(a: Vec3, b: Vec3, bounds: WrapBounds): Segment[] {
  const delta: Vec3 = {
    x: shortestDelta(a.x, b.x, bounds.x),
    y: shortestDelta(a.y, b.y, bounds.y),
    z: shortestDelta(a.z, b.z, bounds.z),
  }
  if (delta.x === b.x - a.x && delta.y === b.y - a.y && delta.z === b.z - a.z) {
    return [{ a: { ...a }, b: { ...b } }]
  }

  const hits: { t: number; axis: Axis }[] = []
  for (const axis of axes) {
    const d = delta[axis]
    const bound = bounds[axis]
    if (!bound.wrap || d === 0) continue
    const wall = d > 0 ? bound.max : bound.min
    const tRaw = (wall - a[axis]) / d
    // t in (0, 1] means the short path reaches this wall, including a
    // path that ends exactly on it. Past the wall (t > 1) the segment
    // stays inside and is not split here.
    if (tRaw < -eps || tRaw > 1 + eps) continue
    const t = tRaw < eps ? 0 : tRaw > 1 ? 1 : tRaw
    hits.push({ t, axis })
  }
  hits.sort((p, q) => p.t - q.t || axes.indexOf(p.axis) - axes.indexOf(q.axis))

  const groups: { t: number; axes: Axis[] }[] = []
  for (const hit of hits) {
    const last = groups[groups.length - 1]
    if (last && Math.abs(last.t - hit.t) <= 1e-8) last.axes.push(hit.axis)
    else groups.push({ t: hit.t, axes: [hit.axis] })
  }

  const shift: Vec3 = { x: 0, y: 0, z: 0 }
  const at = (t: number): Vec3 => ({
    x: a.x + delta.x * t + shift.x,
    y: a.y + delta.y * t + shift.y,
    z: a.z + delta.z * t + shift.z,
  })
  const opposite = (p: Vec3, wrapped: Axis[]) => {
    for (const axis of wrapped) {
      const bound = bounds[axis]
      p[axis] = delta[axis] > 0 ? bound.min : bound.max
    }
  }

  const segments: Segment[] = []
  let t0 = 0
  let entered: Axis[] = []
  for (const group of groups) {
    const start = at(t0)
    opposite(start, entered)
    const end = at(group.t)
    for (const axis of group.axes) {
      const bound = bounds[axis]
      end[axis] = delta[axis] > 0 ? bound.max : bound.min
    }
    if (!samePoint(start, end)) segments.push({ a: start, b: end })
    for (const axis of group.axes) {
      const size = bounds[axis].max - bounds[axis].min
      shift[axis] += delta[axis] > 0 ? -size : size
    }
    entered = group.axes
    t0 = group.t
  }

  const start = at(t0)
  opposite(start, entered)
  // End on the real end-2 coordinate. A directed arrowhead belongs there.
  if (!samePoint(start, b)) segments.push({ a: start, b: { ...b } })
  if (segments.length === 0) return [{ a: { ...a }, b: { ...b } }]
  return segments
}
