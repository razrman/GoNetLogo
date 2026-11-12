// Bitmap sizing for the on-screen plot and the view's offscreen buffers.
//
// Assigning canvas.width or canvas.height throws the bitmap away and resets
// the context, even when the number did not change. The plot widget used to
// do that on every frame. While the world view was uploading its patch image
// and drawing layer in the same frames, an accelerated canvas would present
// those pixels inside the fitness plot for one frame: the cans and grid
// flashed on the chart. Resize only when the CSS size or device pixel ratio
// actually changed, and keep the view's scratch buffers off the GPU.

export function bitmapPixels(cssWidth: number, cssHeight: number, dpr: number) {
  const scale = Number.isFinite(dpr) && dpr > 0 ? dpr : 1
  const width = cssWidth > 0 ? cssWidth : 1
  const height = cssHeight > 0 ? cssHeight : 1
  return {
    width: Math.max(1, Math.round(width * scale)),
    height: Math.max(1, Math.round(height * scale)),
  }
}

// assignBitmapSize writes the drawing-buffer size. False means the bitmap
// was left in place, so a repaint must clear it itself instead of relying
// on the reset that a width assignment would have done.
export function assignBitmapSize(canvas: { width: number; height: number }, width: number, height: number) {
  if (canvas.width === width && canvas.height === height) return false
  canvas.width = width
  canvas.height = height
  return true
}

export function syncBitmap(
  canvas: { width: number; height: number },
  cssWidth: number,
  cssHeight: number,
  dpr: number,
) {
  const width = cssWidth > 0 ? cssWidth : 240
  const height = cssHeight > 0 ? cssHeight : 150
  const px = bitmapPixels(width, height, dpr)
  assignBitmapSize(canvas, px.width, px.height)
  return { width, height }
}

export type SoftwareSurface = {
  canvas: HTMLCanvasElement | OffscreenCanvas
  ctx: CanvasRenderingContext2D | OffscreenCanvasRenderingContext2D
}

// softwareSurface is a scratch bitmap for putImageData and stamp replay.
// It is not an element in the document. willReadFrequently keeps it in
// software so its storage is not a GPU texture the plot canvas can be
// handed for a frame. Null when this runtime has no 2D canvas at all.
export function softwareSurface(width: number, height: number): SoftwareSurface | null {
  const w = Math.max(1, Math.floor(width))
  const h = Math.max(1, Math.floor(height))
  if (typeof OffscreenCanvas !== 'undefined') {
    const canvas = new OffscreenCanvas(w, h)
    const ctx = canvas.getContext('2d', { willReadFrequently: true })
    if (ctx) return { canvas, ctx }
  }
  if (typeof document === 'undefined') return null
  const canvas = document.createElement('canvas')
  canvas.width = w
  canvas.height = h
  const ctx = canvas.getContext('2d', { willReadFrequently: true })
  if (!ctx) return null
  return { canvas, ctx }
}

// blitSource is the image to draw. The on-screen canvas is never a valid
// source for itself: that draw would be the view painting into the widget
// that happens to hold this context.
export function blitSource<T extends object>(dest: object, source: T): T | null {
  if (dest === source) return null
  return source
}
