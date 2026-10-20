export type Widget = {
  index: number
  kind: string
  display: string
  variable: string | null
  forever: boolean
  disabled: boolean
  min: number | null
  max: number | null
  value: number | null
  step: number | null
  units: string | null
  on: boolean | null
  reading: string | null
  choices: string[] | null
  current: number | null
  text: string | null
}

export type Turtle = {
  who: number
  x: number
  y: number
  heading: number
  size: number
  shape: string
  hidden: boolean
  r: number
  g: number
  b: number
  label: string
}
export type Link = {
  x1: number
  y1: number
  x2: number
  y2: number
  hidden: boolean
  r: number
  g: number
  b: number
  label: string
}
export type PatchLabel = {
  x: number
  y: number
  label: string
}

export type WorldView = {
  minPxcor: number
  maxPxcor: number
  minPycor: number
  maxPycor: number
  patchSize: number
  wrapX: boolean
  wrapY: boolean
}

export type World = WorldView & {
  width: number
  height: number
  patchColors: string
  turtles: Turtle[]
  links: Link[]
  patchLabels: PatchLabel[]
}

export type Frame = {
  sessionId: string
  ticks: number | null
  ticksStarted: boolean
  tps: number
  timer: string
  frameRateCap: number
  droppedFrames: number
  droppedSlow: number
  publishedFps: number
  running: boolean
  speed: number
  log: string[]
  widgets: Widget[]
  world: World
}

export type SessionInfo = {
  id: string
  modelName: string
  code: string
  notes: string[]
  widgets: { index: number; kind: string; display: string; forever: boolean }[]
}
