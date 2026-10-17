import { useEffect, useRef } from 'react'
import type { World } from './types'

// Draw the patch grid, links, and turtles. NetLogo y grows upward and
// heading 0 points north, increasing clockwise.
export function WorldCanvas({ world }: { world: World | null }) {
  const ref = useRef<HTMLCanvasElement>(null)

  useEffect(() => {
    const canvas = ref.current
    if (!canvas) return
    const paint = () => draw(canvas, world)
    paint()
    const observer = new ResizeObserver(paint)
    observer.observe(canvas)
    return () => observer.disconnect()
  }, [world])

  return (
    <canvas
      ref={ref}
      className="world"
      aria-label="World view"
    />
  )
}

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

  // Keep patches square. Extra canvas space stays the well color.
  const side = Math.min(w, h)
  const ox = Math.floor((w - side) / 2)
  const oy = Math.floor((h - side) / 2)
  const pw = side / world.width
  const ph = side / world.height
  for (const p of world.patches) {
    ctx.fillStyle = rgb(p.r, p.g, p.b)
    const col = p.px - world.minPxcor
    const row = world.maxPycor - p.py
    ctx.fillRect(ox + Math.floor(col * pw), oy + Math.floor(row * ph), Math.ceil(pw) + 1, Math.ceil(ph) + 1)
  }

  const xOf = (x: number) => ox + (x - (world.minPxcor - 0.5)) * pw
  const yOf = (y: number) => oy + (world.maxPycor + 0.5 - y) * ph
  ctx.lineWidth = Math.max(1, Math.min(pw, ph) * 0.12)
  for (const link of world.links) {
    if (link.hidden) continue
    ctx.strokeStyle = rgb(link.r, link.g, link.b)
    ctx.beginPath()
    ctx.moveTo(xOf(link.x1), yOf(link.y1))
    ctx.lineTo(xOf(link.x2), yOf(link.y2))
    ctx.stroke()
  }

  for (const t of world.turtles) {
    if (t.hidden) continue
    const rad = Math.max(Math.min(pw, ph) * 0.28, (t.size * Math.min(pw, ph)) / 2)
    ctx.save()
    ctx.translate(xOf(t.x), yOf(t.y))
    ctx.rotate((t.heading * Math.PI) / 180)
    ctx.fillStyle = rgb(t.r, t.g, t.b)
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
  }
}

function rgb(r: number, g: number, b: number) {
  return `rgb(${r},${g},${b})`
}
