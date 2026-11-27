import { useEffect, useRef, useState } from 'react'
import type { CSSProperties, ReactNode } from 'react'
import { layoutScale, placeInterface, readableFontPx, type Box, type LayoutChoice } from './interfaceLayout'

export function InterfaceCanvas({
  view,
  items,
  world,
  dropping,
}: {
  view: Box | null
  items: { key: string; box: Box; node: ReactNode }[]
  world: ReactNode
  dropping: boolean
}) {
  const ref = useRef<HTMLDivElement>(null)
  const [avail, setAvail] = useState({ w: 0, h: 0 })
  useEffect(() => {
    const el = ref.current
    if (!el) return
    const measure = () => {
      const w = el.clientWidth
      const h = el.clientHeight
      setAvail((prev) => (prev.w === w && prev.h === h ? prev : { w, h }))
    }
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(el)
    return () => observer.disconnect()
  }, [])

  const placed = placeInterface(
    view,
    items.map((item) => item.box),
  )
  const scale = layoutScale(placed.width, placed.height, avail.w, avail.h)
  const ready = scale > 0
  return (
    <div className="iface-stage" ref={ref}>
      {ready && (
        <div
          className="iface-sheet"
          style={{
            width: placed.width * scale,
            height: placed.height * scale,
            fontSize: readableFontPx(scale),
            ['--iface-scale' as string]: scale,
          }}
        >
          {placed.view && (
            <div className="iface-view" style={boxStyle(placed.view, scale)}>
              <div className="canvas-wrap">{world}</div>
            </div>
          )}
          {items.map((item, i) => {
            const box = placed.items[i]
            if (!box) return null
            return (
              <div key={item.key} className="iface-item" style={boxStyle(box, scale)}>
                {item.node}
              </div>
            )
          })}
        </div>
      )}
      {dropping && <div className="drop-hint">Drop a .nlogo, .nlogox, .nlogo3d, or .nlogox3d file</div>}
    </div>
  )
}

const layoutOptions: { id: LayoutChoice; label: string }[] = [
  { id: 'auto', label: 'Auto' },
  { id: 'netlogo', label: 'NetLogo' },
  { id: 'column', label: 'Column' },
]

export function LayoutToggle({
  choice,
  onChange,
  forcedColumn,
}: {
  choice: LayoutChoice
  onChange: (next: LayoutChoice) => void
  forcedColumn: boolean
}) {
  return (
    <div
      className="layout-toggle"
      role="group"
      aria-label="Layout"
      title={forcedColumn ? 'This model has no usable positions, so the column layout is shown.' : undefined}
    >
      <span>Layout</span>
      {layoutOptions.map((opt) => (
        <button
          key={opt.id}
          type="button"
          className={choice === opt.id ? 'on' : undefined}
          aria-pressed={choice === opt.id}
          onClick={() => onChange(opt.id)}
        >
          {opt.label}
        </button>
      ))}
    </div>
  )
}

function boxStyle(box: Box, scale: number): CSSProperties {
  return {
    left: box.x * scale,
    top: box.y * scale,
    width: box.w * scale,
    height: box.h * scale,
  }
}
