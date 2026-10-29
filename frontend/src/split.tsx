import { useEffect, useRef, useState } from 'react'
import type { KeyboardEvent as ReactKeyboardEvent, PointerEvent as ReactPointerEvent, ReactNode } from 'react'

export const sidebarStorageKey = 'gonetlogo.layout.sidebar'
export const commandStorageKey = 'gonetlogo.layout.command'

export function useStoredSize(key: string, fallback: number) {
  const [size, setSize] = useState(() => readStoredSize(key, fallback))
  const write = (next: number) => {
    const rounded = Math.round(next)
    setSize(rounded)
    try {
      localStorage.setItem(key, String(rounded))
    } catch {
      // Private mode and disabled storage still keep the size for this tab.
    }
  }
  return [size, write] as const
}

function readStoredSize(key: string, fallback: number) {
  try {
    const raw = localStorage.getItem(key)
    if (raw == null) return fallback
    const n = Number(raw)
    if (!Number.isFinite(n)) return fallback
    return n
  } catch {
    return fallback
  }
}

type SplitProps = {
  leading: ReactNode
  trailing: ReactNode
  trailingSize: number
  onTrailingSize: (next: number) => void
  minLeading: number
  minTrailing: number
  label: string
}

export function HSplit(props: SplitProps) {
  return (
    <Split
      {...props}
      axis="x"
      rootClass="h-split"
      leadClass="h-split-lead"
      trailClass="h-split-trail"
    />
  )
}

export function VSplit(props: SplitProps) {
  return (
    <Split
      {...props}
      axis="y"
      rootClass="v-split workspace"
      leadClass="v-split-lead"
      trailClass="v-split-trail"
    />
  )
}

function Split({
  leading,
  trailing,
  trailingSize,
  onTrailingSize,
  minLeading,
  minTrailing,
  label,
  axis,
  rootClass,
  leadClass,
  trailClass,
}: SplitProps & {
  axis: 'x' | 'y'
  rootClass: string
  leadClass: string
  trailClass: string
}) {
  const ref = useRef<HTMLDivElement>(null)
  const [span, setSpan] = useState(0)
  useEffect(() => {
    const el = ref.current
    if (!el) return
    const measure = () => setSpan(axis === 'x' ? el.clientWidth : el.clientHeight)
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(el)
    return () => observer.disconnect()
  }, [axis])
  const bounds = paneBounds(span, minTrailing, minLeading, 14)
  const shown = clamp(trailingSize, bounds.min, bounds.max)
  const trailStyle = axis === 'x' ? { width: shown } : { height: shown }
  return (
    <div className={rootClass} ref={ref}>
      <div className={leadClass}>{leading}</div>
      <Splitter
        orientation={axis === 'x' ? 'vertical' : 'horizontal'}
        label={label}
        value={shown}
        min={bounds.min}
        max={bounds.max}
        onChange={onTrailingSize}
      />
      <div className={trailClass} style={trailStyle}>{trailing}</div>
    </div>
  )
}

function Splitter({
  orientation,
  label,
  value,
  min,
  max,
  onChange,
}: {
  orientation: 'vertical' | 'horizontal'
  label: string
  value: number
  min: number
  max: number
  onChange: (next: number) => void
}) {
  const drag = useRef<{ start: number; size: number } | null>(null)
  const [dragging, setDragging] = useState(false)

  function coord(event: { clientX: number; clientY: number }) {
    return orientation === 'vertical' ? event.clientX : event.clientY
  }

  function endDrag() {
    drag.current = null
    setDragging(false)
    document.body.classList.remove('pane-dragging', 'pane-col', 'pane-row')
  }

  function onPointerDown(event: ReactPointerEvent<HTMLDivElement>) {
    if (event.button !== 0) return
    event.preventDefault()
    drag.current = { start: coord(event), size: value }
    setDragging(true)
    document.body.classList.add('pane-dragging', orientation === 'vertical' ? 'pane-col' : 'pane-row')
    event.currentTarget.setPointerCapture(event.pointerId)
  }

  function onPointerMove(event: ReactPointerEvent<HTMLDivElement>) {
    const active = drag.current
    if (!active) return
    const next = active.size - (coord(event) - active.start)
    onChange(clamp(next, min, max))
  }

  function onKeyDown(event: ReactKeyboardEvent<HTMLDivElement>) {
    const step = event.shiftKey ? 48 : 16
    const grow = orientation === 'vertical' ? 'ArrowLeft' : 'ArrowUp'
    const shrink = orientation === 'vertical' ? 'ArrowRight' : 'ArrowDown'
    if (event.key === grow) {
      onChange(clamp(value + step, min, max))
      event.preventDefault()
    } else if (event.key === shrink) {
      onChange(clamp(value - step, min, max))
      event.preventDefault()
    }
  }

  return (
    <div
      className={
        orientation === 'vertical'
          ? dragging ? 'splitter vertical dragging' : 'splitter vertical'
          : dragging ? 'splitter horizontal dragging' : 'splitter horizontal'
      }
      role="separator"
      aria-orientation={orientation}
      aria-label={label}
      aria-valuemin={Math.round(min)}
      aria-valuemax={Math.round(max)}
      aria-valuenow={Math.round(value)}
      tabIndex={0}
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={endDrag}
      onPointerCancel={endDrag}
      onLostPointerCapture={endDrag}
      onKeyDown={onKeyDown}
    />
  )
}

function paneBounds(container: number, wantMin: number, otherMin: number, gutter: number) {
  if (container <= 0) return { min: wantMin, max: 4000 }
  const max = Math.max(80, container - otherMin - gutter)
  const min = Math.min(wantMin, max)
  return { min, max }
}

function clamp(n: number, min: number, max: number) {
  if (max < min) return min
  return Math.min(max, Math.max(min, n))
}
