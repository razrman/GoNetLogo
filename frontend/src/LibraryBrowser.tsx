import type { LibraryEntry, LibraryListing } from './api'

function libraryLabel(path: string) {
  if (path === '' || path === 'library') return 'Models Library'
  const rest = path.startsWith('library/') ? path.slice('library/'.length) : path
  return rest
}

export function LibraryBrowser({
  listing,
  loading,
  error,
  onOpen,
  onUp,
  onFile,
  onClose,
  onLocal,
}: {
  listing: LibraryListing | null
  loading: boolean
  error: string | null
  onOpen: (path: string) => void
  onUp: (path: string) => void
  onFile: (entry: LibraryEntry) => void
  onClose: () => void
  onLocal: () => void
}) {
  const title = libraryLabel(listing?.path ?? 'library')
  return (
    <div className="modal-back" onMouseDown={onClose}>
      <div
        className="modal library-modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="library-title"
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
        <ul className="library-list">
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
          {listing?.entries.map((entry) => (
            <li key={entry.path}>
              <button
                type="button"
                onClick={() => (entry.directory ? onOpen(entry.path) : onFile(entry))}
              >
                {entry.directory ? `${entry.name}/` : entry.name.replace(/\.nlogox3d$|\.nlogox$|\.nlogo$/i, '')}
              </button>
            </li>
          ))}
        </ul>
        <div className="modal-actions">
          <button type="button" onClick={onLocal}>
            On this computer…
          </button>
          <button type="button" className="apply" onClick={onClose}>
            Close
          </button>
        </div>
      </div>
    </div>
  )
}
