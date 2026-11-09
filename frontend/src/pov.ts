// Point of view for the 3D view. NetLogo tracks follow, ride, and watch
// on the world; this turns that, plus the subject turtle, into a camera
// pose. reset-perspective clears the mode. The view then uses the
// observer camera, which reset-perspective sends home.

export type PovMode = '' | 'follow' | 'ride' | 'watch'

export type Vec3 = { x: number; y: number; z: number }

export type PovTurtle = {
  x: number
  y: number
  z: number
  heading: number
  pitch: number
  roll: number
}

export type PovPose = {
  position: Vec3
  target: Vec3
  up: Vec3
  // Offset from the turtle to the camera while following. The view keeps
  // this across frames so orbiting around the turtle survives the next tick.
  followOffset: Vec3 | null
  // Ride locks orbit and pan. The camera is the turtle.
  lockOrbit: boolean
}

const worldUp: Vec3 = { x: 0, y: 0, z: 1 }

// turtleAxes is NetLogo 3D's forward and up. Heading 0 is +y, heading 90
// is +x, and positive pitch raises the nose.
export function turtleAxes(heading: number, pitch: number, roll: number): { forward: Vec3; up: Vec3 } {
  const rad = Math.PI / 180
  const sh = Math.sin(heading * rad)
  const ch = Math.cos(heading * rad)
  const sp = Math.sin(pitch * rad)
  const cp = Math.cos(pitch * rad)
  const sr = Math.sin(roll * rad)
  const cr = Math.cos(roll * rad)
  const forward = { x: cp * sh, y: cp * ch, z: sp }
  const ux = -sp * sh
  const uy = -sp * ch
  const uz = cp
  const rx = ch
  const ry = -sh
  return {
    forward,
    up: { x: ux * cr + rx * sr, y: uy * cr + ry * sr, z: uz * cr },
  }
}

// applyPov places the camera for one mode.
// follow orbits around the turtle, keeping followOffset (or the current
// orbit offset when that is null).
// ride puts the camera on the turtle, looking along its heading.
// watch leaves the camera where it is and aims it at the turtle.
// An empty mode, or a missing turtle, leaves the camera alone.
export function applyPov(
  mode: PovMode,
  turtle: PovTurtle | null,
  camera: Vec3,
  target: Vec3,
  followOffset: Vec3 | null,
): PovPose {
  if (!mode || !turtle) {
    return {
      position: { ...camera },
      target: { ...target },
      up: { ...worldUp },
      followOffset: null,
      lockOrbit: false,
    }
  }
  const here = { x: turtle.x, y: turtle.y, z: turtle.z }
  if (mode === 'watch') {
    return { position: { ...camera }, target: here, up: { ...worldUp }, followOffset: null, lockOrbit: false }
  }
  if (mode === 'ride') {
    const { forward, up } = turtleAxes(turtle.heading, turtle.pitch, turtle.roll)
    return {
      position: here,
      target: { x: here.x + forward.x, y: here.y + forward.y, z: here.z + forward.z },
      up,
      followOffset: null,
      lockOrbit: true,
    }
  }
  let offset = followOffset
  if (!offset) {
    offset = { x: camera.x - target.x, y: camera.y - target.y, z: camera.z - target.z }
    if (Math.hypot(offset.x, offset.y, offset.z) < 0.5) offset = { x: 0, y: -8, z: 6 }
  }
  return {
    position: { x: here.x + offset.x, y: here.y + offset.y, z: here.z + offset.z },
    target: here,
    up: { ...worldUp },
    followOffset: offset,
    lockOrbit: false,
  }
}
