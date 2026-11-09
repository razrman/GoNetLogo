import { useEffect, useRef, useState } from 'react'
import type { KeyboardEvent as ReactKeyboardEvent, MouseEvent as ReactMouseEvent, PointerEvent as ReactPointerEvent } from 'react'
import type { LibraryEntry, LibraryListing } from './api'
import {
  dialogSize,
  openDialogDefaultHeight,
  openDialogDefaultWidth,
  openDialogHeightKey,
  openDialogWidthKey,
} from './libraryMemory'
import { useStoredSize } from './split'

function libraryLabel(path: string) {
  if (path === '' || path === 'library') return 'Models Library'
  const rest = path.startsWith('library/') ? path.slice('library/'.length) : path
  return rest
}

export function LibraryBrowser({
  listing,
  loading,
  error,
  selectedPath,
  onOpen,
  onUp,
  onFile,
  onClose,
  onLocal,
}: {
  listing: LibraryListing | null
  loading: boolean
  error: string | null
  selectedPath: string | null
  onOpen: (path: string) => void
  onUp: (path: string) => void
  onFile: (entry: LibraryEntry) => void
  onClose: () => void
  onLocal: () => void
}) {
  const title = libraryLabel(listing?.path ?? 'library')
  const selectedRef = useRef<HTMLButtonElement>(null)
  const [boxW, setBoxW] = useStoredSize(openDialogWidthKey, openDialogDefaultWidth)
  const [boxH, setBoxH] = useStoredSize(openDialogHeightKey, openDialogDefaultHeight)
  const [viewport, setViewport] = useState(() => ({ w: window.innerWidth, h: window.innerHeight }))
  const shown = dialogSize(boxW, boxH, viewport.w, viewport.h)

  useEffect(() => {
    const onResize = () => setViewport({ w: window.innerWidth, h: window.innerHeight })
    window.addEventListener('resize', onResize)
    return () => window.removeEventListener('resize', onResize)
  }, [])

  useEffect(() => {
    selectedRef.current?.scrollIntoView({ block: 'nearest' })
  }, [listing?.path, selectedPath, loading])

  function applySize(width: number, height: number) {
    const next = dialogSize(width, height, window.innerWidth, window.innerHeight)
    setBoxW(next.width)
    setBoxH(next.height)
  }

  return (
    <div className="modal-back" onMouseDown={onClose}>
      <div
        className="modal library-modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="library-title"
        style={{ width: shown.width, height: shown.height }}
        onMouseDown={(e) => e.stopPropagation()}
        onKeyDown={(e) => {
          if (e.key === 'Escape') onClose()
        }}
      >
        <p id="library-title">{title}</p>
        {listing && listing.path !== '' && listing.path !== 'library' && (
          <div className="library-path">{listing.path}</div>
        )}
        {error && <div className="library-error">{error}</div>}
        <ul className="library-list" aria-label="Model files">
          {listing?.parent != null && (
            <li>
              <button type="button" onClick={() => onUp(listing.parent ?? '')}>
                ..
              </button>
            </li>
          )}
          {loading && <li className="library-empty">Opening the models library…</li>}
          {!loading && listing && listing.entries.length === 0 && (
            <li className="library-empty">This folder has no models.</li>
          )}
          {listing?.entries.map((entry) => {
            const selected = !entry.directory && entry.path === selectedPath
            return (
              <li key={entry.path}>
                <button
                  ref={selected ? selectedRef : undefined}
                  type="button"
                  className={selected ? 'selected' : undefined}
                  aria-current={selected ? 'true' : undefined}
                  onClick={() => (entry.directory ? onOpen(entry.path) : onFile(entry))}
                >
                  {entry.directory ? `${entry.name}/` : entry.name.replace(/\.nlogox3d$|\.nlogo3d$|\.nlogox$|\.nlogo$/i, '')}
                </button>
              </li>
            )
          })}
        </ul>
        <div className="modal-actions">
          <button type="button" onClick={onLocal}>
            On this computer…
          </button>
          <button type="button" className="apply" onClick={onClose}>
            Close
          </button>
        </div>
        <DialogResize
          axis="x"
          width={shown.width}
          height={shown.height}
          onResize={(width, height) => applySize(width, height)}
        />
        <DialogResize
          axis="y"
          width={shown.width}
          height={shown.height}
          onResize={(width, height) => applySize(width, height)}
        />
        <DialogResize
          axis="both"
          width={shown.width}
          height={shown.height}
          onResize={(width, height) => applySize(width, height)}
        />
      </div>
    </div>
  )
}

function DialogResize({
  axis,
  width,
  height,
  onResize,
}: {
  axis: 'x' | 'y' | 'both'
  width: number
  height: number
  onResize: (width: number, height: number) => void
}) {
  const drag = useRef<{ x: number; y: number; w: number; h: number } | null>(null)
  const [dragging, setDragging] = useState(false)

  function endDrag() {
    drag.current = null
    setDragging(false)
    document.body.classList.remove('pane-dragging', 'library-resizing', 'pane-col', 'pane-row')
  }

  function onPointerDown(event: ReactPointerEvent<HTMLDivElement>) {
    if (event.button !== 0) return
    event.preventDefault()
    event.stopPropagation()
    drag.current = { x: event.clientX, y: event.clientY, w: width, h: height }
    setDragging(true)
    document.body.classList.add('pane-dragging', axis === 'x' ? 'pane-col' : axis === 'y' ? 'pane-row' : 'library-resizing')
    event.currentTarget.setPointerCapture(event.pointerId)
  }

  function onMouseDown(event: ReactMouseEvent<HTMLDivElement>) {
    event.preventDefault()
    event.stopPropagation()
  }

  function onPointerMove(event: ReactPointerEvent<HTMLDivElement>) {
    const active = drag.current
    if (!active) return
    onResize(active.w + (event.clientX - active.x), active.h + (event.clientY - active.y))
  }

  function onKeyDown(event: ReactKeyboardEvent<HTMLDivElement>) {
    const step = event.shiftKey ? 48 : 16
    if (event.key === 'ArrowRight') onResize(width + step, height)
    else if (event.key === 'ArrowLeft') onResize(width - step, height)
    else if (event.key === 'ArrowDown') onResize(width, height + step)
    else if (event.key === 'ArrowUp') onResize(width, height - step)
    else return
    event.preventDefault()
    event.stopPropagation()
  }

  const label = axis === 'x' ? 'Resize file selector width' : axis === 'y' ? 'Resize file selector height' : 'Resize file selector'
  return (
    <div
      className={dragging ? `library-resize ${axis} dragging` : `library-resize ${axis}`}
      role="slider"
      aria-label={label}
      aria-valuemin={160}
      aria-valuemax={4000}
      aria-valuenow={Math.round(axis === 'y' ? height : width)}
      aria-valuetext={`${Math.round(width)} by ${Math.round(height)}`}
      tabIndex={0}
      onMouseDown={onMouseDown}
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={endDrag}
      onPointerCancel={endDrag}
      onLostPointerCapture={endDrag}
      onKeyDown={onKeyDown}
    />
  )
}
