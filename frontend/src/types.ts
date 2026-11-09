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
  x: number
  y: number
  width: number
  height: number
  fontSize: number
}

export type DrawingMark = {
  pen: boolean
  erase: boolean
  x: number
  y: number
  // zcor of the stroke. 0 in a 2D world.
  z: number
  x2: number
  y2: number
  z2: number
  heading: number
  size: number
  shape: string
  r: number
  g: number
  b: number
}

export type AgentLabel = {
  who: number | null
  px: number | null
  py: number | null
  lx: number | null
  ly: number | null
  text: string
  r: number
  g: number
  b: number
}

export type Turtle = {
  who: number
  x: number
  y: number
  // zcor, pitch, and roll; 0 in a 2D world.
  z?: number
  heading: number
  pitch?: number
  roll?: number
  size: number
  shape: string
  hidden: boolean
  r: number
  g: number
  b: number
}
export type Link = {
  x1: number
  y1: number
  z1?: number
  x2: number
  y2: number
  z2?: number
  hidden: boolean
  r: number
  g: number
  b: number
}

export type WorldView = {
  minPxcor: number
  maxPxcor: number
  minPycor: number
  maxPycor: number
  patchSize: number
  wrapX: boolean
  wrapY: boolean
  // Present on a 3D world. A 2D world omits them or leaves them at 0 / false.
  minPzcor?: number
  maxPzcor?: number
  wrapZ?: boolean
  // True for a NetLogo 3D world, including one whose Z bounds are both 0.
  threeD?: boolean
  // 6-digit base64 RNG seed. Omitted when this change does not reseed.
  rngSeed?: string
}

export type ShapeElement = {
  kind: string
  filled: boolean
  marked: boolean
  r: number
  g: number
  b: number
  points: number[]
  x: number
  y: number
  w: number
  h: number
}

export type TurtleShape = {
  name: string
  rotatable: boolean
  elements: ShapeElement[]
}

// The NetLogo 3D observer camera. revision changes only on observer
// camera commands.
export type Observer3D = {
  x: number
  y: number
  z: number
  targetX: number
  targetY: number
  targetZ: number
  revision: number
}

export type World = WorldView & {
  width: number
  height: number
  // Patch layers: 1 for a 2D world. A 3D world (depth > 1) is drawn by
  // WorldView3D from voxels, turtle z/pitch/roll, and link z.
  depth?: number
  // Non-black 3D patches, base64: int16 LE pxcor, pycor, pzcor, then r, g, b.
  voxels?: string
  observer?: Observer3D | null
  labelFont: number
  shapes: TurtleShape[]
  patchColors: string
  drawing: DrawingMark[]
  labels: AgentLabel[]
  turtles: Turtle[]
  links: Link[]
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
  hasOutput: boolean
  outputAppend: string[]
  outputPartial: string
  outputReset: boolean
  outputEpoch: number
  outputSeq: number
  widgets: Widget[]
  world: World
  plots: PlotView[]
  plotAppend: PlotPenDelta[]
}

export type PlotPoint = {
  x: number
  y: number
  down: boolean
}

export type PlotPenView = {
  name: string
  r: number
  g: number
  b: number
  mode: string
  interval: number
}

export type PlotView = {
  index: number
  name: string
  xLabel: string
  yLabel: string
  xMin: number
  xMax: number
  yMin: number
  yMax: number
  autoPlot: boolean
  legend: boolean
  error: string
  pens: PlotPenView[]
}

export type PlotPenDelta = {
  plotIndex: number
  penIndex: number
  epoch: number
  seq: number
  reset: boolean
  points: PlotPoint[]
}

export type SessionInfo = {
  id: string
  modelName: string
  code: string
  info: string
  modelDir: string | null
  notes: string[]
  procedures: string[]
  widgets: { index: number; kind: string; display: string; forever: boolean }[]
}
