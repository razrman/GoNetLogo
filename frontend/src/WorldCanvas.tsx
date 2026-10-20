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
  ctx.lineWidth = Math.max(1, scale * 0.12)
  ctx.textBaseline = 'middle'
  ctx.font = `${Math.max(10, scale * 0.42)}px sans-serif`
  for (const p of world.patchLabels ?? []) {
    if (!p.label) continue
    drawLabel(ctx, xOf(p.x), yOf(p.y), p.label)
  }
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
    const rad = Math.max(scale * 0.28, (t.size * scale) / 2)
    ctx.save()
    ctx.translate(xOf(t.x), yOf(t.y))
    ctx.rotate((t.heading * Math.PI) / 180)
    ctx.fillStyle = rgbCss(t.r, t.g, t.b)
    const shape = t.shape.toLowerCase()
    if (shape === 'circle' || shape === 'dot') {
      ctx.beginPath()
      ctx.arc(0, 0, rad, 0, Math.PI * 2)
      ctx.fill()
    } else if (shape === 'square') {
      ctx.fillRect(-rad, -rad, rad * 2, rad * 2)
    } else {
      ctx.beginPath()
      ctx.moveTo(0, -rad)
      ctx.lineTo(rad * 0.78, rad * 0.72)
      ctx.lineTo(-rad * 0.78, rad * 0.72)
      ctx.closePath()
      ctx.fill()
    }
    ctx.restore()
    if (t.label) drawLabel(ctx, xOf(t.x) + rad * 0.35, yOf(t.y), t.label)
  }
  for (const link of world.links) {
    if (!link.label || link.hidden) continue
    drawLabel(ctx, (xOf(link.x1) + xOf(link.x2)) / 2, (yOf(link.y1) + yOf(link.y2)) / 2, link.label)
  }
}

function drawLabel(ctx: CanvasRenderingContext2D, x: number, y: number, text: string) {
  ctx.save()
  ctx.lineWidth = 3
  ctx.strokeStyle = 'rgba(0,0,0,0.85)'
  ctx.strokeText(text, x, y)
  ctx.fillStyle = '#fff'
  ctx.fillText(text, x, y)
  ctx.restore()
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
