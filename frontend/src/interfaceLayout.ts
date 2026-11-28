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
// layoutPaneWidth is the width Auto compares with the breakpoint. A phone
// window and a narrow interface pane both count, so the column is used
// when either one is the smaller.
export function layoutPaneWidth(slotWidth: number, viewportWidth: number) {
  const slot = Number.isFinite(slotWidth) && slotWidth > 0 ? slotWidth : viewportWidth
  const viewport = Number.isFinite(viewportWidth) && viewportWidth > 0 ? viewportWidth : slot
  if (!Number.isFinite(slot) || slot <= 0) return viewport
  if (!Number.isFinite(viewport) || viewport <= 0) return slot
  return Math.min(slot, viewport)
}

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

// The NetLogo layout is a virtual sheet of this many model pixels. It
// grows when a widget sits outside that rectangle, and the whole sheet
// then scales to the pane.
export const virtualSheetWidth = 800
export const virtualSheetHeight = 600

export const interfaceGridKey = 'gonetlogo.layout.grid'
export const gridChoices = [0, 5, 10, 20, 40] as const
export type GridSpacing = (typeof gridChoices)[number]

// A snapped widget is never thinner or shorter than this, in model pixels.
export const minWidgetSize = 20
export const minViewSize = 80

export function parseGridSpacing(raw: string | null): GridSpacing {
  if (raw == null || raw.trim() === '') return 10
  const n = Number(raw)
  if (n === 0 || n === 5 || n === 10 || n === 20 || n === 40) return n
  return 10
}

export function readGridSpacing(): GridSpacing {
  try {
    return parseGridSpacing(localStorage.getItem(interfaceGridKey))
  } catch {
    return 10
  }
}

export function writeGridSpacing(spacing: GridSpacing) {
  try {
    localStorage.setItem(interfaceGridKey, String(spacing))
  } catch {
    // Private mode keeps the spacing for this tab only.
  }
}

export type PlacedInterface = {
  width: number
  height: number
  originX: number
  originY: number
  view: Box | null
  items: (Box | null)[]
}

// placeInterface keeps model coordinates on an 800×600 sheet. A widget
// past that rectangle, or left of the origin, grows the sheet to the
// bounding box. A widget with no size is given a stand-in only when it
// sits near the other boxes. The result order matches the input.
export function placeInterface(view: Box | null, widgets: Box[]): PlacedInterface {
  const real: Box[] = []
  if (finiteBox(view) && positiveBox(view)) real.push(view)
  for (const widget of widgets) {
    if (positiveBox(widget)) real.push(widget)
  }
  let minX = 0
  let minY = 0
  let maxX = virtualSheetWidth
  let maxY = virtualSheetHeight
  for (const box of real) {
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
  const shift = (box: Box): Box => ({ x: box.x - minX, y: box.y - minY, w: box.w, h: box.h })
  return {
    width: maxX - minX,
    height: maxY - minY,
    originX: minX,
    originY: minY,
    view: finiteBox(view) && positiveBox(view) ? shift(view) : null,
    items: items.map((box) => (box ? shift(box) : null)),
  }
}

export function snapToGrid(value: number, grid: number): number {
  if (!(grid > 0) || !Number.isFinite(value)) return value
  return Math.round(value / grid) * grid
}

export function snapSize(value: number, grid: number): number {
  if (!(grid > 0) || !Number.isFinite(value)) return value
  const snapped = Math.round(value / grid) * grid
  return Math.max(Math.max(minWidgetSize, grid), snapped)
}

export function snapBox(box: Box, grid: number): Box {
  if (!(grid > 0)) return { ...box }
  return {
    x: snapToGrid(box.x, grid),
    y: snapToGrid(box.y, grid),
    w: snapSize(box.w, grid),
    h: snapSize(box.h, grid),
  }
}

const boxKeys = ['x', 'y', 'width', 'height'] as const

// snapWidgetYAML rounds each entry's x, y, width, and height to the grid.
// Off leaves the text unchanged. Width and height stay at least minWidgetSize.
export function snapWidgetYAML(text: string, grid: number): string {
  if (!(grid > 0)) return text
  return text.replace(
    /^([ \t]*)(x|y|width|height):[ \t]*(-?\d+(?:\.\d+)?)[ \t]*$/gm,
    (line, indent: string, key: string, raw: string) => {
      const n = Number(raw)
      if (!Number.isFinite(n)) return line
      const next = key === 'x' || key === 'y' ? snapToGrid(n, grid) : snapSize(n, grid)
      return `${indent}${key}: ${next}`
    },
  )
}

const arrangeOrder = ['button', 'slider', 'switch', 'chooser', 'input', 'monitor', 'plot', 'output', 'note']

const arrangeDefaults: Record<string, [number, number]> = {
  button: [110, 40],
  slider: [180, 50],
  switch: [140, 40],
  chooser: [170, 50],
  input: [170, 40],
  monitor: [140, 50],
  plot: [240, 160],
  output: [240, 140],
  note: [160, 48],
}

export type ArrangeWidget = { kind: string; box: Box }

// autoArrange packs the view and widgets onto the 800×600 sheet without
// overlaps. Controls form columns on the left, in kind order. The view
// sits on the right, or along the top when the columns need the width.
// Sizes snap to the grid and are not shrunk below the minimum.
export function autoArrange(view: Box | null, widgets: ArrangeWidget[], grid: number): { view: Box; widgets: Box[] } {
  const gap = grid > 0 ? grid : 10
  const indexed = widgets.map((widget, index) => ({ widget, index }))
  const grouped = arrangeOrder.flatMap((kind) => indexed.filter((item) => item.widget.kind === kind))
  const rest = indexed.filter((item) => !arrangeOrder.includes(item.widget.kind))
  const sequence = [...grouped, ...rest]
  const sized = sequence.map((item) => sizeForArrange(item.widget.kind, item.widget.box, grid, 260, 200))
  const viewSized = sizeForArrange('view', view ?? { x: 0, y: 0, w: 300, h: 300 }, grid, 380, 520)
  const right = tryRight(sized, viewSized, gap, grid)
  const placed = right ?? tryTop(sized, viewSized, gap, grid) ?? packOverflow(sized, viewSized, gap, grid)
  const widgetsOut = widgets.map(() => ({ x: 0, y: 0, w: 0, h: 0 }))
  sequence.forEach((item, index) => {
    widgetsOut[item.index] = placed.widgets[index]!
  })
  return { view: placed.view, widgets: widgetsOut }
}

function sizeForArrange(kind: string, box: Box, grid: number, maxW: number, maxH: number): Box {
  const fallback = kind === 'view' ? [300, 300] : (arrangeDefaults[kind] ?? [140, 40])
  const w = box.w > 0 ? box.w : fallback[0]
  const h = box.h > 0 ? box.h : fallback[1]
  const capped = { x: 0, y: 0, w: Math.min(maxW, w), h: Math.min(maxH, h) }
  if (!(grid > 0)) {
    return { ...capped, w: Math.max(kind === 'view' ? minViewSize : minWidgetSize, capped.w), h: Math.max(kind === 'view' ? minViewSize : minWidgetSize, capped.h) }
  }
  const snapped = snapBox(capped, grid)
  const floor = kind === 'view' ? Math.max(minViewSize, grid) : Math.max(minWidgetSize, grid)
  return { ...snapped, w: Math.max(floor, snapped.w), h: Math.max(floor, snapped.h) }
}

function tryRight(items: Box[], view: Box, gap: number, grid: number) {
  const viewX = snapToGrid(virtualSheetWidth - gap - view.w, grid)
  const viewY = snapToGrid(gap, grid)
  const packed = packColumns(items, gap, gap, viewX - gap, virtualSheetHeight - gap, gap, grid)
  if (!packed) return null
  const placedView = { x: viewX, y: viewY, w: view.w, h: view.h }
  if (placedView.x < gap || placedView.y + placedView.h > virtualSheetHeight) return null
  if (overlapsAny(placedView, packed)) return null
  return { view: placedView, widgets: packed }
}

function tryTop(items: Box[], view: Box, gap: number, grid: number) {
  const viewH = Math.min(view.h, 220)
  const viewW = Math.min(view.w, virtualSheetWidth - 2 * gap)
  const sized = sizeForArrange('view', { x: 0, y: 0, w: viewW, h: viewH }, grid, virtualSheetWidth - 2 * gap, 220)
  const viewBox = { x: snapToGrid(gap, grid), y: snapToGrid(gap, grid), w: sized.w, h: sized.h }
  const top = viewBox.y + viewBox.h + gap
  const packed = packColumns(items, gap, top, virtualSheetWidth - gap, virtualSheetHeight - gap, gap, grid)
  if (!packed || overlapsAny(viewBox, packed)) return null
  return { view: viewBox, widgets: packed }
}

function packOverflow(items: Box[], view: Box, gap: number, grid: number) {
  const packed = packColumns(items, gap, gap, Number.POSITIVE_INFINITY, virtualSheetHeight - gap, gap, grid) ?? []
  let right = gap
  for (const box of packed) right = Math.max(right, box.x + box.w)
  const viewBox = { x: snapToGrid(right + gap, grid), y: snapToGrid(gap, grid), w: view.w, h: view.h }
  return { view: viewBox, widgets: packed }
}

function packColumns(items: Box[], left: number, top: number, right: number, bottom: number, gap: number, grid: number): Box[] | null {
  const out: Box[] = []
  let x = snapToGrid(left, grid)
  let y = snapToGrid(top, grid)
  let colW = 0
  const startY = y
  for (const item of items) {
    if (out.length > 0 && y + item.h > bottom) {
      x = snapToGrid(x + colW + gap, grid)
      y = startY
      colW = 0
    }
    if (x + item.w > right || y + item.h > bottom) return null
    out.push({ x, y, w: item.w, h: item.h })
    y = snapToGrid(y + item.h + gap, grid)
    colW = Math.max(colW, item.w)
  }
  return out
}

function overlapsAny(box: Box, others: Box[]) {
  return others.some((other) => overlaps(box, other))
}

function overlaps(a: Box, b: Box) {
  return a.x < b.x + b.w && b.x < a.x + a.w && a.y < b.y + b.h && b.y < a.y + a.h
}

export function gridDotOffset(origin: number, grid: number, scale: number) {
  if (!(grid > 0) || !(scale > 0)) return 0
  const mod = ((origin % grid) + grid) % grid
  const delta = mod === 0 ? 0 : grid - mod
  return delta * scale
}

function splitWidgetEntries(text: string): string[] {
  const lines = text.replace(/\r\n/g, '\n').split('\n')
  const entries: string[][] = []
  let current: string[] | null = null
  for (const line of lines) {
    if (/^-(\s|$)/.test(line)) {
      if (current) entries.push(current)
      current = [line]
    } else if (current) {
      current.push(line)
    }
  }
  if (current) entries.push(current)
  return entries.map((entry) => entry.join('\n'))
}

function writeBox(entry: string, box: Box) {
  const values = { x: box.x, y: box.y, width: box.w, height: box.h }
  let next = entry
  for (const key of boxKeys) {
    const line = new RegExp(`^([ \\t]*${key}:)[ \\t]*.*$`, 'm')
    if (line.test(next)) next = next.replace(line, `$1 ${values[key]}`)
    else next = next.replace(/\s*$/, `\n  ${key}: ${values[key]}\n`)
  }
  return next
}

// arrangeWidgetDocument rewrites the view and widget boxes in a Widgets
// YAML list. The view entry is kind: view. Other entries follow the
// widget list order. The result is snapped to the grid.
export function arrangeWidgetDocument(text: string, widgets: ArrangeWidget[], view: Box | null, grid: number): { text: string; error: string | null } {
  const entries = splitWidgetEntries(text)
  const viewAt = entries.findIndex((entry) => /^-\s+kind:\s*view\b/m.test(entry))
  const widgetEntries = entries.filter((_, index) => index !== viewAt)
  if (viewAt < 0) return { text, error: 'the document has no view' }
  if (widgetEntries.length !== widgets.length) return { text, error: 'the document does not match the widgets on screen' }
  const arranged = autoArrange(view, widgets, grid)
  let widgetIndex = 0
  const next = entries.map((entry, index) => {
    if (index === viewAt) return writeBox(entry, snapBox(arranged.view, grid))
    const box = arranged.widgets[widgetIndex++]
    return box ? writeBox(entry, snapBox(box, grid)) : entry
  })
  return { text: next.join('\n'), error: null }
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
// noteFontPx is the note's own point size, scaled with the canvas.
// NetLogo sizes the text box for that font, so the text fits the box.
export function noteFontPx(fontSize: number, scale: number): number {
  const base = fontSize > 0 ? fontSize : 12
  const factor = Number.isFinite(scale) && scale > 0 ? scale : 1
  return base * factor
}

export function readableFontPx(scale: number): number {
  if (!Number.isFinite(scale) || scale <= 0) return 12
  return Math.round(Math.min(18, Math.max(12, 13 * scale)))
}
