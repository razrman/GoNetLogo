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
  x2: number
  y2: number
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
  heading: number
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
  x2: number
  y2: number
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

export type World = WorldView & {
  width: number
  height: number
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
  notes: string[]
  widgets: { index: number; kind: string; display: string; forever: boolean }[]
}
