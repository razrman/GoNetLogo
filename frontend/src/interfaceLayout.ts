// The Interface tab places the view and the widgets on one canvas when
// the model stored positions. The canvas scales with its pane. A narrow
// pane, or a model with no usable box, keeps the column of widgets.

export const interfaceLayoutKey = 'gonetlogo.layout.interface'

// Below this width the automatic choice is the column. It matches the
// rest of the page's narrow layout.
export const interfaceBreakpoint = 860

export type LayoutChoice = 'auto' | 'netlogo' | 'column'
export type LayoutMode = 'netlogo' | 'column'

export type Box = { x: number; y: number; w: number; h: number }

export function parseLayoutChoice(raw: string | null): LayoutChoice {
  if (raw === 'netlogo' || raw === 'column') return raw
  return 'auto'
}

export function readLayoutChoice(): LayoutChoice {
  try {
    return parseLayoutChoice(localStorage.getItem(interfaceLayoutKey))
  } catch {
    return 'auto'
  }
}

export function writeLayoutChoice(choice: LayoutChoice) {
  try {
    localStorage.setItem(interfaceLayoutKey, choice)
  } catch {
    // Private mode keeps the choice for this tab only.
  }
}

// resolveLayout picks the canvas or the column. An unusable model stays
// in the column even when the user asked for the NetLogo layout. Until
// the first frame arrives, the automatic choice follows the pane width
// so the page does not flash the other layout.
export function resolveLayout(
  choice: LayoutChoice,
  paneWidth: number,
  usable: boolean,
  pending: boolean,
): LayoutMode {
  if (!pending && !usable) return 'column'
  if (choice === 'column') return 'column'
  if (choice === 'netlogo') return 'netlogo'
  if (!Number.isFinite(paneWidth) || paneWidth < interfaceBreakpoint) return 'column'
  return 'netlogo'
}

function finiteBox(box: Box | null | undefined): box is Box {
  return !!box && [box.x, box.y, box.w, box.h].every((n) => Number.isFinite(n))
}

function positiveBox(box: Box | null | undefined): boolean {
  return finiteBox(box) && box.w > 0 && box.h > 0
}

// geometryUsable is false when the view has no box, a coordinate is not
// a number, or the interface is so large that scaling it would make the
// controls unreadably small. Overlap and a widget sitting just outside
// the view are still usable.
export function geometryUsable(view: Box | null, widgets: Box[]): boolean {
  if (!finiteBox(view) || view.w <= 0 || view.h <= 0) return false
  for (const widget of widgets) {
    if (!finiteBox(widget) || widget.w < 0 || widget.h < 0) return false
    if (Math.abs(widget.x) > 20000 || Math.abs(widget.y) > 20000) return false
  }
  const boxes = [view, ...widgets.filter(positiveBox)]
  let minX = Infinity
  let minY = Infinity
  let maxX = -Infinity
  let maxY = -Infinity
  for (const box of boxes) {
    minX = Math.min(minX, box.x)
    minY = Math.min(minY, box.y)
    maxX = Math.max(maxX, box.x + box.w)
    maxY = Math.max(maxY, box.y + box.h)
  }
  const width = maxX - minX
  const height = maxY - minY
  return width > 0 && height > 0 && width <= 4000 && height <= 4000
}

const interfacePad = 12

export type PlacedInterface = {
  width: number
  height: number
  view: Box | null
  items: (Box | null)[]
}

// placeInterface shifts the model boxes so the union, plus a small pad,
// starts at the origin. A widget with no size is given a stand-in only
// when it sits near the other boxes. The result order matches the input.
export function placeInterface(view: Box | null, widgets: Box[]): PlacedInterface {
  const placed: Box[] = []
  if (finiteBox(view) && positiveBox(view)) placed.push(view)
  for (const widget of widgets) {
    if (positiveBox(widget)) placed.push(widget)
  }
  if (placed.length === 0) return { width: 0, height: 0, view: null, items: widgets.map(() => null) }
  let minX = Infinity
  let minY = Infinity
  let maxX = -Infinity
  let maxY = -Infinity
  for (const box of placed) {
    minX = Math.min(minX, box.x)
    minY = Math.min(minY, box.y)
    maxX = Math.max(maxX, box.x + box.w)
    maxY = Math.max(maxY, box.y + box.h)
  }
  const near = (box: Box) =>
    box.x >= minX - 200 && box.x <= maxX + 200 && box.y >= minY - 200 && box.y <= maxY + 200
  const items = widgets.map((widget) => {
    if (positiveBox(widget)) return widget
    if (!finiteBox(widget) || !near(widget)) return null
    const filled = { x: widget.x, y: widget.y, w: widget.w > 0 ? widget.w : 160, h: widget.h > 0 ? widget.h : 40 }
    minX = Math.min(minX, filled.x)
    minY = Math.min(minY, filled.y)
    maxX = Math.max(maxX, filled.x + filled.w)
    maxY = Math.max(maxY, filled.y + filled.h)
    return filled
  })
  const originX = minX - interfacePad
  const originY = minY - interfacePad
  const shift = (box: Box): Box => ({ x: box.x - originX, y: box.y - originY, w: box.w, h: box.h })
  return {
    width: maxX - minX + interfacePad * 2,
    height: maxY - minY + interfacePad * 2,
    view: finiteBox(view) && positiveBox(view) ? shift(view) : null,
    items: items.map((box) => (box ? shift(box) : null)),
  }
}

// layoutScale fits the model's box in the pane and keeps the aspect
// ratio. It grows and shrinks with the pane. Zero means the pane has
// not been measured yet.
export function layoutScale(contentW: number, contentH: number, availW: number, availH: number): number {
  if (!(contentW > 0) || !(contentH > 0) || !(availW > 0) || !(availH > 0)) return 0
  return Math.min(availW / contentW, availH / contentH)
}

// readableFontPx tracks the scale and stays in a range that can be read
// inside a widget. The boxes themselves use the full scale.
export function readableFontPx(scale: number): number {
  if (!Number.isFinite(scale) || scale <= 0) return 12
  return Math.round(Math.min(18, Math.max(12, 13 * scale)))
}
