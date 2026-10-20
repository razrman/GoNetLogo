import { useEffect, useRef } from 'react'
import type { World } from './types'

// Draw the patch grid, links, and turtles. NetLogo y grows upward and
// heading 0 points north, increasing clockwise. Patch colors arrive as
// one RGB image so a 100×100 world stays a single blit.
export function WorldCanvas({ world }: { world: World | null }) {
  const ref = useRef<HTMLCanvasElement>(null)
  const worldRef = useRef(world)
  worldRef.current = world

  useEffect(() => {
    const canvas = ref.current
    if (!canvas) return
    let raf = 0
    let painted: World | null | undefined
    const paintLatest = () => {
      const latest = worldRef.current
      if (latest === painted) return
      draw(canvas, latest)
      painted = latest
    }
    const loop = () => {
      paintLatest()
      raf = requestAnimationFrame(loop)
    }
    raf = requestAnimationFrame(loop)
    const observer = new ResizeObserver(() => {
      painted = undefined
      paintLatest()
    })
    observer.observe(canvas)
    return () => {
      cancelAnimationFrame(raf)
      observer.disconnect()
    }
  }, [])

  return (
    <canvas
      ref={ref}
      className="world"
      aria-label="World view"
    />
  )
}

const scratch = document.createElement('canvas')

function draw(canvas: HTMLCanvasElement, world: World | null) {
  const dpr = window.devicePixelRatio || 1
  const rect = canvas.getBoundingClientRect()
  const w = Math.max(1, Math.floor(rect.width * dpr))
  const h = Math.max(1, Math.floor(rect.height * dpr))
  if (canvas.width !== w || canvas.height !== h) {
    canvas.width = w
    canvas.height = h
  }
  const ctx = canvas.getContext('2d')
  if (!ctx) return
  ctx.imageSmoothingEnabled = false
  ctx.fillStyle = '#0c0e0a'
  ctx.fillRect(0, 0, w, h)
  if (!world || world.width === 0 || world.height === 0) return

  // Square patches, letterboxed into the canvas. A wide world stays wide.
  const scale = Math.min(w / world.width, h / world.height)
  const dw = world.width * scale
  const dh = world.height * scale
  const ox = Math.floor((w - dw) / 2)
  const oy = Math.floor((h - dh) / 2)

  const rgb = decodeRGB(world.patchColors)
  const need = world.width * world.height * 3
  if (rgb && rgb.length === need) {
    if (scratch.width !== world.width || scratch.height !== world.height) {
      scratch.width = world.width
      scratch.height = world.height
    }
    const sctx = scratch.getContext('2d')
    if (sctx) {
      const img = sctx.createImageData(world.width, world.height)
      const data = img.data
      for (let i = 0, j = 0; i < rgb.length; i += 3, j += 4) {
        data[j] = rgb[i]
        data[j + 1] = rgb[i + 1]
        data[j + 2] = rgb[i + 2]
        data[j + 3] = 255
      }
      sctx.putImageData(img, 0, 0)
      ctx.drawImage(scratch, ox, oy, dw, dh)
    }
  }

  const xOf = (x: number) => ox + (x - (world.minPxcor - 0.5)) * scale
  const yOf = (y: number) => oy + (world.maxPycor + 0.5 - y) * scale

  const marks = world.drawing ?? []
  if (marks.length > 0) {
    const lw = Math.max(1, Math.round(dw))
    const lh = Math.max(1, Math.round(dh))
    if (layer.width !== lw || layer.height !== lh) {
      layer.width = lw
      layer.height = lh
    }
    const lctx = layer.getContext('2d')
    if (lctx) {
      lctx.clearRect(0, 0, lw, lh)
      lctx.imageSmoothingEnabled = false
      drawDrawing(lctx, world, dw / world.width)
      ctx.drawImage(layer, ox, oy, dw, dh)
    }
  }

  ctx.lineWidth = Math.max(1, scale * 0.12)
  for (const link of world.links) {
    if (link.hidden) continue
    ctx.strokeStyle = rgbCss(link.r, link.g, link.b)
    ctx.beginPath()
    ctx.moveTo(xOf(link.x1), yOf(link.y1))
    ctx.lineTo(xOf(link.x2), yOf(link.y2))
    ctx.stroke()
  }

  for (const t of world.turtles) {
    if (t.hidden) continue
    const rad = agentRadius(t.size, scale)
    ctx.save()
    ctx.translate(xOf(t.x), yOf(t.y))
    ctx.rotate((t.heading * Math.PI) / 180)
    drawShape(ctx, t.shape, rad, t.r, t.g, t.b)
    ctx.restore()
  }

  const labels = world.labels ?? []
  if (labels.length > 0) {
    const byWho = new Map<number, (typeof world.turtles)[number]>()
    for (const t of world.turtles) byWho.set(t.who, t)
    ctx.font = `${Math.max(12, Math.round(scale * 0.7))}px ui-sans-serif, system-ui, sans-serif`
    ctx.textAlign = 'left'
    for (const label of labels) {
      ctx.fillStyle = rgbCss(label.r, label.g, label.b)
      if (label.who != null) {
        const t = byWho.get(label.who)
        if (!t || t.hidden) continue
        const rad = agentRadius(t.size, scale)
        ctx.textBaseline = 'top'
        ctx.fillText(label.text, xOf(t.x) + rad * 0.5, yOf(t.y) + rad * 0.2)
      } else if (label.px != null && label.py != null) {
        ctx.textBaseline = 'middle'
        ctx.fillText(label.text, xOf(label.px), yOf(label.py))
      }
    }
  }
}

const layer = document.createElement('canvas')

function agentRadius(size: number, scale: number) {
  return Math.max(2, size * scale * 0.45)
}

// Shapes are drawn with heading 0 pointing up. The caller has already rotated.
function drawShape(
  ctx: CanvasRenderingContext2D,
  shape: string,
  rad: number,
  r: number,
  g: number,
  b: number,
) {
  const name = shape.toLowerCase().trim()
  ctx.fillStyle = rgbCss(r, g, b)
  ctx.strokeStyle = ctx.fillStyle
  if (name === 'circle') {
    fillDisk(ctx, 0, 0, rad)
    return
  }
  if (name === 'dot') {
    fillDisk(ctx, 0, 0, rad * 0.38)
    return
  }
  if (name === 'square') {
    ctx.fillRect(-rad, -rad, rad * 2, rad * 2)
    return
  }
  if (name === 'box') {
    drawBox(ctx, rad, r, g, b)
    return
  }
  if (name === 'person') {
    drawPerson(ctx, rad)
    return
  }
  if (name === 'face happy' || name === 'face-happy' || name === 'facehappy') {
    drawFace(ctx, rad, r, g, b)
    return
  }
  drawArrow(ctx, rad)
}

function drawDrawing(ctx: CanvasRenderingContext2D, world: World, ps: number) {
  const xOf = (x: number) => (x - (world.minPxcor - 0.5)) * ps
  const yOf = (y: number) => (world.maxPycor + 0.5 - y) * ps
  for (const m of world.drawing ?? []) {
    ctx.save()
    if (m.pen) {
      ctx.strokeStyle = rgbCss(m.r, m.g, m.b)
      ctx.lineWidth = Math.max(1, m.size * ps * 0.55)
      ctx.lineCap = 'round'
      ctx.lineJoin = 'round'
      ctx.beginPath()
      ctx.moveTo(xOf(m.x), yOf(m.y))
      ctx.lineTo(xOf(m.x2), yOf(m.y2))
      ctx.stroke()
    } else {
      if (m.erase) ctx.globalCompositeOperation = 'destination-out'
      ctx.translate(xOf(m.x), yOf(m.y))
      ctx.rotate((m.heading * Math.PI) / 180)
      drawShape(ctx, m.shape, agentRadius(m.size, ps), m.r, m.g, m.b)
    }
    ctx.restore()
  }
}

function drawArrow(ctx: CanvasRenderingContext2D, rad: number) {
  ctx.beginPath()
  ctx.moveTo(0, -rad)
  ctx.lineTo(rad * 0.78, rad * 0.72)
  ctx.lineTo(-rad * 0.78, rad * 0.72)
  ctx.closePath()
  ctx.fill()
}

function drawPerson(ctx: CanvasRenderingContext2D, rad: number) {
  fillDisk(ctx, 0, -rad * 0.52, rad * 0.26)
  ctx.beginPath()
  ctx.moveTo(-rad * 0.2, -rad * 0.28)
  ctx.lineTo(rad * 0.2, -rad * 0.28)
  ctx.lineTo(rad * 0.16, rad * 0.18)
  ctx.lineTo(-rad * 0.16, rad * 0.18)
  ctx.closePath()
  ctx.fill()
  ctx.fillRect(-rad * 0.58, -rad * 0.24, rad * 1.16, rad * 0.18)
  ctx.beginPath()
  ctx.moveTo(-rad * 0.16, rad * 0.12)
  ctx.lineTo(-rad * 0.01, rad * 0.12)
  ctx.lineTo(-rad * 0.26, rad * 0.84)
  ctx.lineTo(-rad * 0.46, rad * 0.8)
  ctx.closePath()
  ctx.fill()
  ctx.beginPath()
  ctx.moveTo(rad * 0.01, rad * 0.12)
  ctx.lineTo(rad * 0.16, rad * 0.12)
  ctx.lineTo(rad * 0.46, rad * 0.8)
  ctx.lineTo(rad * 0.26, rad * 0.84)
  ctx.closePath()
  ctx.fill()
}

function drawBox(
  ctx: CanvasRenderingContext2D,
  rad: number,
  r: number,
  g: number,
  b: number,
) {
  ctx.fillStyle = rgbCss(r, g, b)
  ctx.beginPath()
  ctx.moveTo(-rad * 0.55, -rad * 0.15)
  ctx.lineTo(rad * 0.35, -rad * 0.15)
  ctx.lineTo(rad * 0.35, rad * 0.7)
  ctx.lineTo(-rad * 0.55, rad * 0.7)
  ctx.closePath()
  ctx.fill()
  ctx.fillStyle = rgbCss(mix(r, 255, 0.38), mix(g, 255, 0.38), mix(b, 255, 0.38))
  ctx.beginPath()
  ctx.moveTo(-rad * 0.55, -rad * 0.15)
  ctx.lineTo(-rad * 0.2, -rad * 0.7)
  ctx.lineTo(rad * 0.7, -rad * 0.7)
  ctx.lineTo(rad * 0.35, -rad * 0.15)
  ctx.closePath()
  ctx.fill()
  ctx.fillStyle = rgbCss(r * 0.68, g * 0.68, b * 0.68)
  ctx.beginPath()
  ctx.moveTo(rad * 0.35, -rad * 0.15)
  ctx.lineTo(rad * 0.7, -rad * 0.7)
  ctx.lineTo(rad * 0.7, rad * 0.15)
  ctx.lineTo(rad * 0.35, rad * 0.7)
  ctx.closePath()
  ctx.fill()
}

function drawFace(
  ctx: CanvasRenderingContext2D,
  rad: number,
  r: number,
  g: number,
  b: number,
) {
  fillDisk(ctx, 0, 0, rad)
  const ink = contrast(r, g, b)
  ctx.fillStyle = ink
  fillDisk(ctx, -rad * 0.32, -rad * 0.18, Math.max(1, rad * 0.12))
  fillDisk(ctx, rad * 0.32, -rad * 0.18, Math.max(1, rad * 0.12))
  ctx.strokeStyle = ink
  ctx.lineWidth = Math.max(1, rad * 0.1)
  ctx.beginPath()
  ctx.arc(0, rad * 0.12, rad * 0.42, Math.PI * 0.15, Math.PI * 0.85)
  ctx.stroke()
}

function fillDisk(ctx: CanvasRenderingContext2D, x: number, y: number, rad: number) {
  ctx.beginPath()
  ctx.arc(x, y, rad, 0, Math.PI * 2)
  ctx.fill()
}

function mix(c: number, toward: number, t: number) {
  return Math.round(c + (toward - c) * t)
}

function contrast(r: number, g: number, b: number) {
  const lum = 0.299 * r + 0.587 * g + 0.114 * b
  return lum > 150 ? 'rgb(20,20,20)' : 'rgb(255,255,255)'
}

function decodeRGB(b64: string): Uint8Array | null {
  if (!b64) return null
  try {
    const bin = atob(b64)
    const out = new Uint8Array(bin.length)
    for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i)
    return out
  } catch {
    return null
  }
}

function rgbCss(r: number, g: number, b: number) {
  return `rgb(${r},${g},${b})`
}
