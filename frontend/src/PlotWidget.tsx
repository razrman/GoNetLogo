import { useEffect, useRef } from 'react'
import type { Frame, PlotPenDelta, PlotPoint, PlotView } from './types'

export type PenSeries = { epoch: number; seq: number; points: PlotPoint[] }
export type PlotSeries = Record<string, PenSeries>

export function absorbPlots(state: PlotSeries, frame: Frame): PlotSeries {
  const incoming = frame.plotAppend ?? []
  if (incoming.length === 0) return state
  let next = state
  const ensure = () => {
    if (next === state) next = { ...state }
  }
  for (const delta of incoming) {
    const key = penKey(delta.plotIndex, delta.penIndex)
    const prev = next[key]
    const points = applyDelta(prev, delta)
    if (prev && prev.epoch === delta.epoch && prev.seq === delta.seq && points === prev.points) continue
    ensure()
    next[key] = { epoch: delta.epoch, seq: delta.seq, points }
  }
  return next
}

function applyDelta(prev: PenSeries | undefined, delta: PlotPenDelta): PlotPoint[] {
  const incoming = delta.points ?? []
  if (!prev || delta.reset || delta.epoch !== prev.epoch || delta.seq < prev.seq) {
    return incoming
  }
  if (delta.seq === prev.seq) return prev.points
  const gained = delta.seq - prev.seq
  const extra = incoming.length - gained
  const slice = extra > 0 ? incoming.slice(extra) : incoming
  const points = prev.points.concat(slice)
  if (points.length > 8000) return points.slice(-8000)
  return points
}

function penKey(plot: number, pen: number) {
  return `${plot}:${pen}`
}

export function PlotCard({ plot, series }: { plot: PlotView; series: PlotSeries }) {
  const ref = useRef<HTMLCanvasElement>(null)
  const points = countPoints(plot, series)
  useEffect(() => {
    const canvas = ref.current
    if (!canvas) return
    const width = canvas.clientWidth || 240
    const height = canvas.clientHeight || 150
    const dpr = window.devicePixelRatio || 1
    canvas.width = Math.max(1, Math.round(width * dpr))
    canvas.height = Math.max(1, Math.round(height * dpr))
    const ctx = canvas.getContext('2d')
    if (!ctx) return
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
    paintPlot(ctx, width, height, plot, series)
  }, [plot, series, points])

  const yName = plot.yLabel || 'y'
  const xName = plot.xLabel || 'x'
  return (
    <div className="plot-card">
      <div className="label">{plot.name}</div>
      <div className="plot-axes">
        {yName} vs {xName}
        {plot.autoPlot ? '' : ' · fixed'}
      </div>
      <canvas ref={ref} aria-label={plot.name} data-points={points} />
      {plot.legend && plot.pens.length > 0 && (
        <div className="plot-legend">
          {plot.pens.map((pen) => (
            <span key={pen.name}>
              <i style={{ background: rgb(pen.r, pen.g, pen.b) }} />
              {pen.name}
            </span>
          ))}
        </div>
      )}
      {plot.error && <p className="err">{plot.error}</p>}
    </div>
  )
}

function countPoints(plot: PlotView, series: PlotSeries) {
  let n = 0
  for (let i = 0; i < plot.pens.length; i++) {
    n += series[penKey(plot.index, i)]?.points.length ?? 0
  }
  return n
}

function paintPlot(
  ctx: CanvasRenderingContext2D,
  w: number,
  h: number,
  plot: PlotView,
  series: PlotSeries,
) {
  ctx.clearRect(0, 0, w, h)
  ctx.fillStyle = '#fffdf8'
  ctx.fillRect(0, 0, w, h)
  const pad = { l: 38, r: 8, t: 8, b: 18 }
  let xMin = plot.xMin
  let xMax = plot.xMax
  let yMin = plot.yMin
  let yMax = plot.yMax
  if (!(xMax > xMin)) xMax = xMin + 1
  if (!(yMax > yMin)) yMax = yMin + 1
  const pw = Math.max(1, w - pad.l - pad.r)
  const ph = Math.max(1, h - pad.t - pad.b)
  const sx = (x: number) => pad.l + ((x - xMin) / (xMax - xMin)) * pw
  const sy = (y: number) => pad.t + ((yMax - y) / (yMax - yMin)) * ph

  ctx.strokeStyle = 'rgba(28, 25, 20, 0.18)'
  ctx.lineWidth = 1
  ctx.strokeRect(pad.l + 0.5, pad.t + 0.5, pw - 1, ph - 1)

  ctx.fillStyle = '#6e675c'
  ctx.font = '10px "Segoe UI", sans-serif'
  ctx.textAlign = 'right'
  ctx.textBaseline = 'top'
  ctx.fillText(formatAxis(yMax), pad.l - 4, pad.t)
  ctx.textBaseline = 'bottom'
  ctx.fillText(formatAxis(yMin), pad.l - 4, pad.t + ph)
  ctx.textAlign = 'left'
  ctx.textBaseline = 'top'
  ctx.fillText(formatAxis(xMin), pad.l, pad.t + ph + 2)
  ctx.textAlign = 'right'
  ctx.fillText(formatAxis(xMax), pad.l + pw, pad.t + ph + 2)

  ctx.save()
  ctx.beginPath()
  ctx.rect(pad.l, pad.t, pw, ph)
  ctx.clip()
  for (let i = 0; i < plot.pens.length; i++) {
    const pen = plot.pens[i]
    const pts = series[penKey(plot.index, i)]?.points ?? []
    const color = rgb(pen.r, pen.g, pen.b)
    ctx.strokeStyle = color
    ctx.fillStyle = color
    ctx.lineWidth = 1.6
    ctx.lineJoin = 'round'
    ctx.lineCap = 'round'
    if (pen.mode === 'bar') {
      const bar = Math.max(1, Math.abs(sx(xMin + pen.interval) - sx(xMin)))
      const y0 = sy(0)
      for (const p of pts) {
        if (!p.down) continue
        const y = sy(p.y)
        ctx.fillRect(sx(p.x), Math.min(y, y0), bar, Math.max(1, Math.abs(y0 - y)))
      }
      continue
    }
    if (pen.mode === 'point') {
      for (const p of pts) {
        if (!p.down) continue
        ctx.beginPath()
        ctx.arc(sx(p.x), sy(p.y), 2.2, 0, Math.PI * 2)
        ctx.fill()
      }
      continue
    }
    ctx.beginPath()
    let open = false
    for (const p of pts) {
      if (!p.down) {
        open = false
        continue
      }
      const x = sx(p.x)
      const y = sy(p.y)
      if (!open) {
        ctx.moveTo(x, y)
        open = true
      } else {
        ctx.lineTo(x, y)
      }
    }
    ctx.stroke()
  }
  ctx.restore()
}

function rgb(r: number, g: number, b: number) {
  return `rgb(${r}, ${g}, ${b})`
}

function formatAxis(n: number) {
  if (!Number.isFinite(n)) return ''
  const abs = Math.abs(n)
  if (abs !== 0 && (abs >= 10000 || abs < 0.01)) return n.toExponential(0)
  const digits = abs >= 100 ? 0 : abs >= 10 ? 1 : 2
  return n.toFixed(digits).replace(/(\.\d*?)0+$/, '$1').replace(/\.$/, '')
}
