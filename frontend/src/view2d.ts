// Camera for the flat world view.
//
// follow and ride center on the turtle and rotate the world so its
// heading points up. In 2D those two are the same view; ride's
// first-person camera is a 3D-only pose. watch centers on a turtle,
// patch, or link and leaves the world unrotated, with a spotlight
// around the subject. An empty perspective shows the whole world.

export type View2DPose = {
  x: number
  y: number
  // Radians, counterclockwise in world space, so this heading points up.
  rotation: number
  spotlight: boolean
}

export type View2DWorld = {
  minPxcor: number
  maxPxcor: number
  minPycor: number
  maxPycor: number
  perspective?: string
  subject?: number | null
  subjectX?: number | null
  subjectY?: number | null
  turtles: { who: number; x: number; y: number; heading: number }[]
}

export function view2dPose(world: View2DWorld): View2DPose {
  const free: View2DPose = {
    x: (world.minPxcor + world.maxPxcor) / 2,
    y: (world.minPycor + world.maxPycor) / 2,
    rotation: 0,
    spotlight: false,
  }
  const mode = world.perspective ?? ''
  if (mode !== 'follow' && mode !== 'ride' && mode !== 'watch') return free
  const turtle = world.subject == null ? undefined : world.turtles.find((t) => t.who === world.subject)
  if ((mode === 'follow' || mode === 'ride') && turtle) {
    return { x: turtle.x, y: turtle.y, rotation: (turtle.heading * Math.PI) / 180, spotlight: false }
  }
  if (mode === 'watch') {
    if (turtle) return { x: turtle.x, y: turtle.y, rotation: 0, spotlight: true }
    if (world.subjectX != null && world.subjectY != null) {
      return { x: world.subjectX, y: world.subjectY, rotation: 0, spotlight: true }
    }
  }
  return free
}

// worldToScreen maps a world point into a canvas whose y axis grows down.
// scale is pixels per patch. The pose center lands on the canvas center,
// and rotation turns the subject's heading toward the top of the screen.
export function worldToScreen(
  x: number,
  y: number,
  pose: View2DPose,
  scale: number,
  width: number,
  height: number,
): { x: number; y: number } {
  const m = canvasTransform(pose, scale, width, height)
  return { x: m[0] * x + m[2] * y + m[4], y: m[1] * x + m[3] * y + m[5] }
}

// canvasTransform is the 2D canvas matrix (a, b, c, d, e, f) for setTransform.
export function canvasTransform(
  pose: View2DPose,
  scale: number,
  width: number,
  height: number,
): [number, number, number, number, number, number] {
  const cos = Math.cos(pose.rotation)
  const sin = Math.sin(pose.rotation)
  const a = scale * cos
  const b = -scale * sin
  const c = -scale * sin
  const d = -scale * cos
  const e = width / 2 - a * pose.x - c * pose.y
  const f = height / 2 - b * pose.x - d * pose.y
  return [a, b, c, d, e, f]
}

// patchImageRect is where the patch bitmap sits in world coordinates.
// Row 0 of the bitmap is maxPycor, so the image height is negative.
export function patchImageRect(world: { minPxcor: number; maxPxcor: number; minPycor: number; maxPycor: number }) {
  return {
    x: world.minPxcor - 0.5,
    y: world.maxPycor + 0.5,
    w: world.maxPxcor - world.minPxcor + 1,
    h: -(world.maxPycor - world.minPycor + 1),
  }
}
