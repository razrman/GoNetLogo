// The Open Model dialog remembers the last library file and the size of
// the file list. Both live in localStorage, the same place as the
// sidebar and command-center sizes.

export const lastModelStorageKey = 'gonetlogo.open.lastModel'
export const openDialogWidthKey = 'gonetlogo.open.width'
export const openDialogHeightKey = 'gonetlogo.open.height'

export const openDialogMinWidth = 280
export const openDialogMinHeight = 280
export const openDialogDefaultWidth = 440
export const openDialogDefaultHeight = 480

export type KeyValueStore = {
  getItem(key: string): string | null
  setItem(key: string, value: string): void
}

// normalizeModelPath keeps a library path relative to the models
// directory. Empty, absolute, and .. paths are rejected.
export function normalizeModelPath(raw: string): string | null {
  const trimmed = raw.trim().replace(/\\/g, '/')
  // Absolute paths are not library paths. A leading slash would otherwise
  // disappear and look like a relative folder.
  if (!trimmed || trimmed.startsWith('/') || /^[A-Za-z]:/.test(trimmed)) return null
  const path = trimmed.replace(/\/+$/g, '')
  if (!path) return null
  const parts = path.split('/')
  if (parts.some((part) => part === '' || part === '.' || part === '..')) return null
  return path
}

// parentFolder is the directory to list so the saved file is on screen.
// A file in the models root has an empty parent, which lists that root.
// An unusable path returns null so the dialog opens its default folder.
export function parentFolder(filePath: string): string | null {
  const path = normalizeModelPath(filePath)
  if (path == null) return null
  const slash = path.lastIndexOf('/')
  if (slash < 0) return ''
  return path.slice(0, slash)
}

function browserStore(): KeyValueStore {
  return localStorage
}

export function readLastModelPath(store: KeyValueStore = browserStore()): string | null {
  try {
    const raw = store.getItem(lastModelStorageKey)
    if (raw == null) return null
    return normalizeModelPath(raw)
  } catch {
    return null
  }
}

export function writeLastModelPath(filePath: string, store: KeyValueStore = browserStore()): void {
  const path = normalizeModelPath(filePath)
  if (path == null) return
  try {
    store.setItem(lastModelStorageKey, path)
  } catch {
    // Private mode and disabled storage still open the file.
  }
}

// dialogSize fits a stored width and height in the viewport. The minimum
// shrinks when the window is smaller than the preferred minimum.
export function dialogSize(width: number, height: number, viewportW: number, viewportH: number) {
  const maxW = Math.max(160, viewportW - 32)
  const maxH = Math.max(160, viewportH - 32)
  const minW = Math.min(openDialogMinWidth, maxW)
  const minH = Math.min(openDialogMinHeight, maxH)
  return {
    width: clamp(width, minW, maxW),
    height: clamp(height, minH, maxH),
  }
}

function clamp(n: number, min: number, max: number) {
  if (max < min) return min
  return Math.min(max, Math.max(min, n))
}
