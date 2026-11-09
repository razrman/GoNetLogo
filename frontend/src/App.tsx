import { Suspense, lazy, useCallback, useEffect, useRef, useState } from 'react'
import type { DragEvent, FormEvent } from 'react'
import { useMutation, useQuery, useSubscription } from 'urql'
import {
  CreateSession,
  Frames,
  Go,
  LibraryQuery,
  LoadModel,
  LoadModelFile,
  LoadModelSource,
  ModelsQuery,
  client,
  PressButton,
  RunCommand,
  SetChooser,
  SetCode,
  SetDynamics,
  SetInput,
  SetSlider,
  SetFrameRate,
  SetSpeed,
  SetSwitch,
  SetWorld,
  SetSeedOverride,
  GenerateRngSeed,
  Setup,
  Step,
  Stop,
  WorldSettingsQuery,
  hasProcedure,
  isDisconnectError,
  onConnection,
  reportDisconnect,
  type LibraryEntry,
  type LibraryListing,
} from './api'
import type { Frame, PlotView, SessionInfo, SystemDynamics, Widget, WorldView } from './types'
import {
  dynamicsApplyEnabled,
  dynamicsDefinitionReadOnly,
  dynamicsErrorTitle,
  dynamicsLockedMessage,
  generatedProceduresReadOnly,
  hasSystemDynamics,
  mainTabLabel,
  presentDynamicsError,
  settleDynamicsApply,
  tabAfterModelLoad,
  visibleMainTabs,
  type MainTab,
} from './dynamics'
import { InfoView } from './InfoTab'
import { LibraryBrowser } from './LibraryBrowser'
import { parentFolder, readLastModelPath, writeLastModelPath } from './libraryMemory'
import { WorldCanvas } from './WorldCanvas'
import { readSeedOverride, rngSeedError, rngSeedForApply, seedControlState, writeSeedOverride } from './rngSeed'
import { worldGeometryChanged, worldSizeLockedMessage } from './runLock'

// three.js is only needed for 3D worlds, so it loads in its own chunk the
// first time one is shown. 2D models never fetch it.
const WorldView3D = lazy(() => import('./WorldView3D'))
import { PlotCard, absorbPlots, type PlotSeries } from './PlotWidget'
import { HSplit, VSplit, commandStorageKey, sidebarStorageKey, useStoredSize } from './split'

type CreateResult = { createSession: SessionInfo }
type LoadResult = { loadModel: SessionInfo }
type FileResult = { loadModelFile: SessionInfo }
type SourceResult = { loadModelSource: SessionInfo }
type LibraryResult = { library: LibraryListing }
type CodeResult = { setCode: { ok: boolean; error: string | null; session?: { procedures: string[] } } }
type DynamicsApply = {
  setDynamics: {
    ok: boolean
    error: string | null
    session?: { procedures: string[]; dynamics: SystemDynamics | null }
  }
}
type CommandResult = { runCommand: { ok: boolean; output: string | null; error: string | null } }

export function App() {
  const [models] = useQuery<{ models: string[] }>({ query: ModelsQuery })
  const [, createSession] = useMutation<CreateResult>(CreateSession)
  const [, loadModel] = useMutation<LoadResult>(LoadModel)
  const [, loadModelFile] = useMutation<FileResult>(LoadModelFile)
  const [, loadModelSource] = useMutation<SourceResult>(LoadModelSource)
  const [, setCodeMut] = useMutation<CodeResult>(SetCode)
  const [, setDynamicsMut] = useMutation<DynamicsApply>(SetDynamics)
  const [, runCommand] = useMutation<CommandResult>(RunCommand)
  const [, setupMut] = useMutation(Setup)
  const [, goMut] = useMutation(Go)
  const [, stopMut] = useMutation(Stop)
  const [, stepMut] = useMutation(Step)
  const [, pressMut] = useMutation(PressButton)
  const [, speedMut] = useMutation(SetSpeed)
  const [, frameRateMut] = useMutation(SetFrameRate)
  const [, sliderMut] = useMutation(SetSlider)
  const [, switchMut] = useMutation(SetSwitch)
  const [, chooserMut] = useMutation(SetChooser)
  const [, inputMut] = useMutation(SetInput)
  const [, setWorldMut] = useMutation(SetWorld)
  const [, setSeedOverrideMut] = useMutation(SetSeedOverride)
  const [, generateSeedMut] = useMutation<{ generateRngSeed: string }>(GenerateRngSeed)

  const [sessionId, setSessionId] = useState<string | null>(null)
  const [modelName, setModelName] = useState('fire.nlogo')
  const [procedures, setProcedures] = useState<string[]>([])
  const [code, setCode] = useState('')
  const [dynamics, setDynamics] = useState<SystemDynamics | null>(null)
  const [definition, setDefinition] = useState('')
  const [dynamicsError, setDynamicsError] = useState<string | null>(null)
  const [info, setInfo] = useState('')
  const [modelDir, setModelDir] = useState<string | null>(null)
  const [notes, setNotes] = useState<string[]>([])
  const [frame, setFrame] = useState<Frame | null>(null)
  const [tab, setTab] = useState<MainTab>('interface')
  const [sidebarW, setSidebarW] = useStoredSize(sidebarStorageKey, 300)
  const [commandH, setCommandH] = useStoredSize(commandStorageKey, 156)
  const [worldDirty, setWorldDirty] = useState(false)
  const [leaveTo, setLeaveTo] = useState<MainTab | null>(null)
  const [leaving, setLeaving] = useState(false)
  const applyWorldRef = useRef<(() => Promise<boolean>) | null>(null)
  const flushSeedRef = useRef<(() => Promise<string | null>) | null>(null)
  const onWorldDirty = useCallback((dirty: boolean) => setWorldDirty(dirty), [])
  const registerWorldApply = useCallback((apply: (() => Promise<boolean>) | null) => {
    applyWorldRef.current = apply
  }, [])
  const registerFlushSeed = useCallback((flush: (() => Promise<string | null>) | null) => {
    flushSeedRef.current = flush
  }, [])
  const [banner, setBanner] = useState<string | null>(null)
  const [online, setOnline] = useState(true)
  const [compileError, setCompileError] = useState<string | null>(null)
  const [command, setCommand] = useState('')
  const [commandError, setCommandError] = useState<string | null>(null)
  const [active, setActive] = useState<number | null>(null)
  const [booting, setBooting] = useState(true)
  const [outputText, setOutputText] = useState('')
  const [dropping, setDropping] = useState(false)
  const logRef = useRef<HTMLPreElement>(null)
  const outputState = useRef({ epoch: -1, seq: 0, lines: [] as string[], partial: '' })
  const plotState = useRef<PlotSeries>({})
  const [plotSeries, setPlotSeries] = useState<PlotSeries>({})
  const resetPlots = () => {
    plotState.current = {}
    setPlotSeries({})
  }
  const fileRef = useRef<HTMLInputElement>(null)
  const [library, setLibrary] = useState<LibraryListing | null>(null)
  const [libraryError, setLibraryError] = useState<string | null>(null)
  const [libraryLoading, setLibraryLoading] = useState(false)

  const modelTitle = interfaceModelTitle(modelName)
  useEffect(() => {
    document.title = `${modelTitle} - GoNetLogo`
  }, [modelTitle])

  useEffect(() => onConnection(setOnline), [])
  useEffect(() => {
    if (models.error) reportDisconnect(models.error.message)
  }, [models.error])

  const [settingsQ] = useQuery<{ worldSettings: WorldView }>({
    query: WorldSettingsQuery,
    variables: { id: sessionId ?? '' },
    pause: !sessionId,
  })
  const seedPrefFor = useRef<string | null>(null)
  const worldReady = Boolean(frame?.world || settingsQ.data?.worldSettings)
  const serverOverride = (frame?.world ?? settingsQ.data?.worldSettings)?.seedOverride === true
  useEffect(() => {
    if (!sessionId || !worldReady) return
    if (seedPrefFor.current === sessionId) return
    seedPrefFor.current = sessionId
    const want = readSeedOverride()
    if (want === serverOverride) return
    void setSeedOverrideMut({ id: sessionId, on: want })
  }, [sessionId, worldReady, serverOverride, setSeedOverrideMut])

  useEffect(() => {
    let cancel = false
    ;(async () => {
      const created = await createSession({ model: 'fire.nlogo' })
      if (cancel) return
      if (created.error || !created.data) {
        const message = created.error?.message ?? 'Could not open a session'
        if (!reportDisconnect(message)) setBanner(message)
        setBooting(false)
        return
      }
      const info = created.data.createSession
      setSessionId(info.id)
      setModelName(info.modelName)
      setCode(info.code)
      setInfo(info.info ?? '')
      setModelDir(info.modelDir ?? null)
      setNotes(info.notes ?? [])
      setProcedures(info.procedures ?? [])
      rememberDynamics(info.dynamics ?? null)
      await speedMut({ id: info.id, speed: 30 })
      await autoSetup(info.id, info.procedures)
      const go = await goMut({ id: info.id })
      if (go.error && !reportDisconnect(go.error.message)) setBanner(go.error.message)
      else setActive(foreverIndex(info.widgets))
      if (!cancel) setBooting(false)
    })()
    return () => {
      cancel = true
    }
    // Boot once. The mutation executors are stable for this client.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    const el = logRef.current
    if (el) el.scrollTop = el.scrollHeight
  }, [frame?.log])

  useEffect(() => {
    if (frame && !frame.running) setActive(null)
  }, [frame?.running])

  useEffect(() => {
    if (tab === 'dynamics' && !hasSystemDynamics(dynamics)) setTab('interface')
  }, [tab, dynamics])

  async function onModel(name: string) {
    if (!sessionId || name === modelName) return
    setBanner(null)
    const res = await loadModel({ id: sessionId, model: name })
    if (res.error || !res.data) {
      const message = res.error?.message ?? 'Could not load that model'
      if (!reportDisconnect(message)) setBanner(message)
      return
    }
    const info = res.data.loadModel
    setModelName(info.modelName)
    setCode(info.code)
    setInfo(info.info ?? '')
    setModelDir(info.modelDir ?? null)
    setNotes(info.notes ?? [])
    setProcedures(info.procedures ?? [])
    rememberDynamics(info.dynamics ?? null)
    setCompileError(null)
    setActive(null)
    resetPlots()
    await autoSetup(sessionId, info.procedures)
  }

  async function onSetup() {
    if (!sessionId || !hasProcedure(procedures, 'setup')) return
    setBanner(null)
    const flushed = await flushSeedRef.current?.()
    if (flushed) {
      setBanner(flushed)
      return
    }
    const res = await setupMut({ id: sessionId })
    if (res.error && !reportDisconnect(res.error.message)) setBanner(res.error.message)
  }

  async function onGo() {
    if (!sessionId) return
    setBanner(null)
    if (frame?.running) {
      const res = await stopMut({ id: sessionId })
      if (res.error && !reportDisconnect(res.error.message)) setBanner(res.error.message)
      setActive(null)
      return
    }
    const res = await goMut({ id: sessionId })
    if (res.error && !reportDisconnect(res.error.message)) setBanner(res.error.message)
    else setActive(foreverIndex(frame?.widgets ?? []))
  }

  async function onStep() {
    if (!sessionId) return
    setBanner(null)
    const res = await stepMut({ id: sessionId })
    if (res.error && !reportDisconnect(res.error.message)) setBanner(res.error.message)
  }

  async function onPress(widget: Widget) {
    if (!sessionId) return
    setBanner(null)
    if (!widget.forever && widget.display.trim().toLowerCase() === 'setup') {
      const flushed = await flushSeedRef.current?.()
      if (flushed) {
        setBanner(flushed)
        return
      }
    }
    const res = await pressMut({ id: sessionId, index: widget.index })
    if (res.error) {
      if (!reportDisconnect(res.error.message)) setBanner(res.error.message)
      return
    }
    if (widget.forever) {
      setActive((cur) => (cur === widget.index ? null : widget.index))
    }
  }

  async function applyCode() {
    if (!sessionId) return
    const res = await setCodeMut({ id: sessionId, code })
    if (res.error) {
      setCompileError(presentCompileError(res.error.message, code))
      return
    }
    const message = res.data?.setCode.error
    setCompileError(message ? presentCompileError(message, code) : null)
    const nextProcedures = res.data?.setCode.session?.procedures
    if (res.data?.setCode.ok && nextProcedures) setProcedures(nextProcedures)
  }

  function rememberDynamics(next: SystemDynamics | null) {
    setDynamics(next)
    setDefinition(next?.definition ?? '')
    setDynamicsError(null)
    setTab((cur) => tabAfterModelLoad(cur, hasSystemDynamics(next)))
  }

  async function applyDynamics() {
    if (!sessionId || !dynamicsApplyEnabled(true, frame?.running ?? false)) return
    const res = await setDynamicsMut({ id: sessionId, definition })
    if (res.error) {
      const message = res.error.message.replace(/^\[GraphQL\]\s*/i, '').trim()
      if (!reportDisconnect(message)) setDynamicsError(message)
      return
    }
    const applied = res.data?.setDynamics
    if (!applied) {
      setDynamicsError('Could not apply')
      return
    }
    const settled = settleDynamicsApply(definition, {
      ok: applied.ok,
      error: applied.error,
      definition: applied.session?.dynamics?.definition ?? null,
    })
    setDefinition(settled.definition)
    setDynamicsError(settled.error)
    if (applied.session) setDynamics(applied.session.dynamics)
    if (applied.ok && applied.session?.procedures) setProcedures(applied.session.procedures)
  }

  async function openSource(filename: string, source: string) {
    if (!sessionId) return
    setBanner(null)
    const res = await loadModelSource({ id: sessionId, filename, source })
    if (res.error || !res.data) {
      setCompileError(presentCompileError(res.error?.message ?? 'Could not open that model', source, filename))
      return
    }
    const info = res.data.loadModelSource
    setModelName(info.modelName)
    setCode(info.code)
    setInfo(info.info ?? '')
    setModelDir(info.modelDir ?? null)
    setNotes(info.notes ?? [])
    setProcedures(info.procedures ?? [])
    rememberDynamics(info.dynamics ?? null)
    setCompileError(null)
    setActive(null)
    resetPlots()
    await autoSetup(sessionId, info.procedures)
  }

  async function onPickFile(file: File | undefined) {
    if (!file) return
    if (!sessionId) {
      setBanner('The session is still opening')
      return
    }
    if (!/\.nlogo(x3d|x|3d)?$/i.test(file.name)) {
      setCompileError(`Open a .nlogo, .nlogox, .nlogo3d, or .nlogox3d file. ${file.name} is not one of those.`)
      return
    }
    let text: string
    try {
      text = await file.text()
    } catch (err) {
      setCompileError(err instanceof Error ? err.message : 'Could not read that file')
      return
    }
    await openSource(file.name, text)
  }

  async function queryLibrary(path: string | null) {
    const res = await client.query<LibraryResult>(LibraryQuery, { path }, { requestPolicy: 'network-only' }).toPromise()
    if (res.error || !res.data) {
      const message = (res.error?.message ?? 'Could not list models').replace(/^\[GraphQL\]\s*/i, '').trim()
      return { listing: null as LibraryListing | null, message, disconnect: reportDisconnect(message) }
    }
    return { listing: res.data.library, message: null as string | null, disconnect: false }
  }

  async function openLibrary() {
    if (!sessionId) return
    setLibraryLoading(true)
    setLibraryError(null)
    const last = readLastModelPath()
    const folder = last == null ? null : parentFolder(last)
    let result = await queryLibrary(folder)
    if (result.disconnect) {
      setLibraryLoading(false)
      return
    }
    // A saved folder that is gone falls back to the usual library root.
    if (folder != null && (result.message || !result.listing?.available)) {
      result = await queryLibrary(null)
      if (result.disconnect) {
        setLibraryLoading(false)
        return
      }
    }
    setLibraryLoading(false)
    if (!result.listing?.available) {
      fileRef.current?.click()
      return
    }
    if (result.message) setLibraryError(result.message)
    if (result.listing) setLibrary(result.listing)
  }

  async function openLibraryPath(path: string) {
    setLibraryLoading(true)
    setLibraryError(null)
    const result = await queryLibrary(path)
    setLibraryLoading(false)
    if (result.disconnect) return
    if (result.message) {
      setLibraryError(result.message)
      return
    }
    if (result.listing) setLibrary(result.listing)
  }

  function applyOpenedModel(info: SessionInfo) {
    setModelName(info.modelName)
    setCode(info.code)
    setInfo(info.info ?? '')
    setModelDir(info.modelDir ?? null)
    setNotes(info.notes ?? [])
    setProcedures(info.procedures ?? [])
    rememberDynamics(info.dynamics ?? null)
    setCompileError(null)
    setActive(null)
    resetPlots()
    setLibrary(null)
    setLibraryError(null)
  }

  async function openLibraryFile(entry: LibraryEntry) {
    if (!sessionId) return
    setLibraryError(null)
    setBanner(null)
    const res = await loadModelFile({ id: sessionId, path: entry.path })
    if (res.error || !res.data) {
      const message = (res.error?.message ?? 'Could not open that model').replace(/^\[GraphQL\]\s*/i, '').trim()
      if (reportDisconnect(message)) return
      setLibraryError(message)
      setCompileError(presentCompileError(message, ''))
      return
    }
    const info = res.data.loadModelFile
    writeLastModelPath(entry.path)
    applyOpenedModel(info)
    await autoSetup(sessionId, info.procedures)
  }

  async function onCommand(event: FormEvent) {
    event.preventDefault()
    if (!sessionId || !command.trim()) return
    const res = await runCommand({ id: sessionId, text: command })
    if (res.error) {
      if (!reportDisconnect(res.error.message)) setCommandError(res.error.message)
      return
    }
    setCommandError(res.data?.runCommand.error ?? null)
    if (res.data?.runCommand.ok) setCommand('')
  }

  async function onWorld(next: WorldView) {
    if (!sessionId) return 'No session'
    // Size, wrapping, and patch size rebuild the grid. Leave that locked
    // while go is running and say so, instead of stopping and then racing
    // the tick that is still inside the interpreter. A new RNG seed is
    // random-seed: it applies during the run and does not call setup.
    const running = frame?.running ?? false
    const geometryChanged = frame != null && worldGeometryChanged(next, frame.world)
    const seedOnly = running && !geometryChanged
    if (running && geometryChanged) {
      return worldSizeLockedMessage
    }
    const applied = await sendWorld(sessionId, next, setWorldMut)
    if (applied.message) return applied.message
    setProcedures(applied.procedures)
    if (seedOnly) return null
    return autoSetup(sessionId, applied.procedures)
  }

  async function onFlushSeed(seed: string): Promise<string | null> {
    if (!sessionId) return 'No session'
    const live = frame?.world ?? settingsQ.data?.worldSettings
    if (!live) return 'No world'
    const applied = await sendWorld(sessionId, { ...worldGeometry(live), rngSeed: seed, seedOverride: true }, setWorldMut)
    return applied.message
  }

  async function onSeedOverride(on: boolean): Promise<string | null> {
    if (!sessionId) return 'No session'
    writeSeedOverride(on)
    const res = await setSeedOverrideMut({ id: sessionId, on })
    const message = (res.error?.message ?? '').replace(/^\[GraphQL\]\s*/i, '').trim()
    if (reportDisconnect(message)) return 'Server disconnected. Reconnecting…'
    return message || null
  }

  async function onPopulateSeed(): Promise<{ seed: string | null; message: string | null }> {
    if (!sessionId) return { seed: null, message: 'No session' }
    const res = await generateSeedMut({ id: sessionId })
    const message = (res.error?.message ?? '').replace(/^\[GraphQL\]\s*/i, '').trim()
    if (reportDisconnect(message)) return { seed: null, message: 'Server disconnected. Reconnecting…' }
    if (message) return { seed: null, message }
    const seed = res.data?.generateRngSeed ?? ''
    if (rngSeedError(seed)) return { seed: null, message: 'Could not generate an RNG seed' }
    return { seed, message: null }
  }

  async function autoSetup(id: string, procedures: readonly string[] | undefined): Promise<string | null> {
    if (!hasProcedure(procedures, 'setup')) return null
    const setup = await setupMut({ id })
    if (reportDisconnect(setup.error?.message)) return 'Server disconnected. Reconnecting…'
    if (setup.error) {
      setBanner(setup.error.message)
      return setup.error.message
    }
    return null
  }

  function requestTab(next: MainTab) {
    if (next === tab) return
    if (tab === 'world' && worldDirty) {
      setLeaveTo(next)
      return
    }
    setTab(next)
  }

  async function applyAndLeave() {
    if (!leaveTo || leaving) return
    setLeaving(true)
    const ok = (await applyWorldRef.current?.()) ?? false
    setLeaving(false)
    if (!ok) {
      setLeaveTo(null)
      return
    }
    const next = leaveTo
    setLeaveTo(null)
    setTab(next)
  }

  const onFrame = useCallback((next: Frame) => {
    absorbOutput(outputState.current, next, setOutputText)
    const plots = absorbPlots(plotState.current, next)
    if (plots !== plotState.current) {
      plotState.current = plots
      setPlotSeries(plots)
    }
    setFrame(next)
  }, [])

  const widgets = frame?.widgets ?? []
  const libraryNames = models.data?.models ?? []
  const modelOptions = libraryNames.includes(modelName) || !modelName ? libraryNames : [modelName, ...libraryNames]
  const goWidget = widgets.find((w) => w.kind === 'button' && w.display.toLowerCase() === 'go')
  const running = frame?.running ?? false
  const showDynamics = hasSystemDynamics(dynamics)
  // A non-SDM model never keeps the Dynamics tab on screen, even for the
  // render before the tab state catches up.
  const shownTab: MainTab = tab === 'dynamics' && !showDynamics ? 'interface' : tab
  const ticksLabel = frame?.ticksStarted && frame.ticks != null ? String(frame.ticks) : '—'
  const tpsLabel = frame?.running ? frame.tps.toFixed(1) : '—'
  const timerLabel = frame?.timer || '00:00.0'
  const dropLabel = frame ? String(frame.droppedFrames) : '—'
  return (
    <div className="app">
      <header className="topbar">
        <div className="topbar-left">
          <div className="brand">
            <strong>GoNetLogo</strong>
            <span>GraphQL client</span>
          </div>
          <label className="model-pick">
            <span>Models</span>
            <select
              className="model"
              aria-label="Models"
              value={modelOptions.includes(modelName) ? modelName : ''}
              disabled={!sessionId || modelOptions.length === 0}
              onChange={(e) => void onModel(e.target.value)}
            >
            {modelOptions.length === 0 && <option value="">Loading models…</option>}
            {modelOptions.map((name) => (
              <option key={name} value={name}>
                {name.replace(/\.nlogo(x3d|x|3d)?$/i, '')}
              </option>
            ))}
            </select>
          </label>
          <button
            className="open-model"
            type="button"
            disabled={!sessionId}
            onClick={() => void openLibrary()}
          >
            Open model…
          </button>
          <input
            ref={fileRef}
            type="file"
            accept=".nlogo,.nlogox,.nlogo3d,.nlogox3d"
            aria-label="Open model file"
            hidden
            onChange={(e) => {
              const file = e.target.files?.[0]
              e.target.value = ''
              void onPickFile(file)
            }}
          />
          <div className="controls">
            <button
              className="act"
              type="button"
              disabled={!sessionId || !hasProcedure(procedures, 'setup')}
              title={sessionId && !hasProcedure(procedures, 'setup') ? 'This model has no setup procedure' : undefined}
              onClick={() => void onSetup()}
            >
              setup
            </button>
            <button
              className={running ? 'act stop' : 'act go'}
              type="button"
              disabled={!sessionId || (!running && (goWidget?.disabled ?? true))}
              onClick={() => void onGo()}
            >
              {running ? 'stop' : 'go'}
            </button>
            <button
              className="act step"
              type="button"
              disabled={!sessionId || (goWidget?.disabled ?? true)}
              onClick={() => void onStep()}
            >
              step
            </button>
          </div>
          <label className="speed">
            speed
            <input
              type="range"
              min={0}
              max={100}
              step={1}
              aria-label="Speed"
              value={frame?.speed ?? 30}
              disabled={!sessionId}
              onChange={(e) => {
                if (sessionId) void speedMut({ id: sessionId, speed: Number(e.target.value) })
              }}
            />
          </label>
          <div className="ticks tick-count" data-ticks={ticksLabel}>
            <span>ticks <b>{ticksLabel}</b></span>
          </div>
        </div>
        <div className="topbar-right">
          <div className="run-timer" data-timer={timerLabel}><b>{timerLabel}</b></div>
          <div
            className="ticks"
            data-tps={tpsLabel}
            data-dropped={dropLabel}
            data-running={running ? 'true' : 'false'}
          >
            <span>tps <b>{tpsLabel}</b></span>
            <span title={frame ? `${frame.publishedFps.toFixed(1)} published fps, ${frame.droppedSlow} slow` : undefined}>drop <b>{dropLabel}</b></span>
          </div>
        </div>
      </header>

      {sessionId && (
        <FrameBridge
          sessionId={sessionId}
          onFrame={onFrame}
          onError={(message) => {
            if (!reportDisconnect(message)) setBanner(message)
          }}
        />
      )}

      {!online && (
        <div className="banner reconnect" role="status">
          Server disconnected. Reconnecting…
        </div>
      )}
      {online && models.error && !isDisconnectError(models.error.message) && (
        <div className="banner">{models.error.message}</div>
      )}
      {online && banner && <div className="banner">{banner}</div>}
      {compileError && (
        <div className="compile-error" role="alert">
          <strong>{/line \d+/i.test(compileError) ? 'Compile error' : 'Could not load model'}</strong>
          <pre>{compileError}</pre>
        </div>
      )}

      <VSplit
        trailingSize={commandH}
        onTrailingSize={setCommandH}
        minLeading={240}
        minTrailing={120}
        label="Resize command center"
        leading={
      <main className="stage">
        {shownTab === 'dynamics' ? (
          <section className="code-panel dynamics-panel" aria-label="System dynamics">
            <MainTabs tab={shownTab} showDynamics={showDynamics} onChange={requestTab} />
            <p className="dynamics-note">
              Stocks, flows, and converters. Apply recompiles the model. The procedures below are what the engine adds; the Code tab does not list them.
            </p>
            {dynamicsError && (
              <div className="compile-error" role="alert">
                <strong>{dynamicsErrorTitle(dynamicsError)}</strong>
                <pre>{presentDynamicsError(dynamicsError, definition)}</pre>
              </div>
            )}
            <div className="dynamics-split">
              <label className="dynamics-block">
                Definition
                <textarea
                  aria-label="System dynamics definition"
                  spellCheck={false}
                  readOnly={dynamicsDefinitionReadOnly(running)}
                  value={definition}
                  onChange={(e) => {
                    if (!dynamicsDefinitionReadOnly(running)) setDefinition(e.target.value)
                  }}
                />
              </label>
              <label className="dynamics-block">
                Generated procedures
                <textarea
                  aria-label="Generated system dynamics procedures"
                  spellCheck={false}
                  readOnly={generatedProceduresReadOnly()}
                  value={dynamics?.source ?? ''}
                />
              </label>
            </div>
            <div className="code-actions">
              <button
                className="act"
                type="button"
                disabled={!dynamicsApplyEnabled(!!sessionId, running)}
                onClick={() => void applyDynamics()}
              >
                Apply
              </button>
              {running && <span className="err">{dynamicsLockedMessage}</span>}
            </div>
          </section>
        ) : shownTab === 'code' ? (
          <section className="code-panel">
            <MainTabs tab={shownTab} showDynamics={showDynamics} onChange={requestTab} />
            <textarea
              aria-label="Model code"
              spellCheck={false}
              value={code}
              onChange={(e) => setCode(e.target.value)}
            />
            <div className="code-actions">
              <button className="act" type="button" disabled={!sessionId} onClick={() => void applyCode()}>
                Apply code
              </button>
            </div>
          </section>
        ) : shownTab === 'info' ? (
          <section className="info-panel" aria-label="Model info">
            <MainTabs tab={shownTab} showDynamics={showDynamics} onChange={requestTab} />
            <InfoView
              key={`${modelName}\0${modelDir ?? ''}`}
              markdown={info}
              modelDir={modelDir}
              loading={booting && info === ''}
            />
          </section>
        ) : shownTab === 'world' ? (
          <section className="world-panel" aria-label="World settings">
            <MainTabs tab={shownTab} showDynamics={showDynamics} onChange={requestTab} />
            <FrameCapControl
              fps={frame?.frameRateCap ?? 24}
              disabled={!sessionId}
              onChange={(fps) => {
                if (sessionId) void frameRateMut({ id: sessionId, fps })
              }}
            />
            <WorldSettingsForm
              world={frame?.world ?? settingsQ.data?.worldSettings ?? null}
              disabled={!sessionId || !(frame?.world || settingsQ.data?.worldSettings)}
              onApply={onWorld}
              onPopulateSeed={onPopulateSeed}
              onSeedOverride={onSeedOverride}
              onFlushSeed={onFlushSeed}
              onDirtyChange={onWorldDirty}
              registerApply={registerWorldApply}
              registerFlushSeed={registerFlushSeed}
            />
          </section>
        ) : (
          <HSplit
            trailingSize={sidebarW}
            onTrailingSize={setSidebarW}
            minLeading={320}
            minTrailing={220}
            label="Resize widgets panel"
            leading={
            <section
              className={dropping ? 'view drop' : 'view'}
              onDragEnter={(e) => {
                if (!dragHasFiles(e)) return
                e.preventDefault()
                setDropping(true)
              }}
              onDragOver={(e) => {
                if (!dragHasFiles(e)) return
                e.preventDefault()
                e.dataTransfer.dropEffect = 'copy'
                setDropping(true)
              }}
              onDragLeave={(e) => {
                const next = e.relatedTarget
                if (next instanceof Node && e.currentTarget.contains(next)) return
                setDropping(false)
              }}
              onDrop={(e) => {
                e.preventDefault()
                setDropping(false)
                void onPickFile(e.dataTransfer.files?.[0])
              }}
            >
              <div className="view-bar">
                <MainTabs tab={shownTab} showDynamics={showDynamics} onChange={requestTab} />
                <h1 className="model-title" data-model-title={modelTitle} title={modelTitle}>
                  {modelTitle}
                </h1>
              </div>
              <div className="canvas-wrap">
                {booting && !frame ? (
                  <div className="loading">Opening the forest fire…</div>
                ) : frame && (frame.world.depth ?? 1) > 1 ? (
                  <Suspense fallback={<WorldCanvas world={frame.world} />}>
                    <WorldView3D world={frame.world} />
                  </Suspense>
                ) : (
                  <WorldCanvas world={frame?.world ?? null} />
                )}
                {dropping && <div className="drop-hint">Drop a .nlogo, .nlogox, .nlogo3d, or .nlogox3d file</div>}
              </div>
              <div className="view-meta">
                {frame
                  ? `${frame.world.width} × ${frame.world.height}${(frame.world.depth ?? 1) > 1 ? ` × ${frame.world.depth}` : ''} patches · patch size ${frame.world.patchSize} · wrap ${frame.world.wrapX ? 'x' : '–'}${frame.world.wrapY ? ' y' : ''}`
                  : modelName}
              </div>
            </section>
            }
            trailing={
            <aside className="side" aria-label="Interface widgets">
              {widgets.length === 0 && (
                <div className="empty">{booting ? 'Loading widgets…' : 'This model has no interface widgets.'}</div>
              )}
              {widgets.map((w) => (
                <WidgetView
                  key={`${modelName}-${w.index}`}
                  widget={w}
                  live={running && active === w.index}
                  outputText={outputText}
                  plot={plotFor(frame, w)}
                  series={plotSeries}
                  onPress={() => void onPress(w)}
                  onSlider={(value) => sessionId && void sliderMut({ id: sessionId, index: w.index, value })}
                  onSwitch={(on) => sessionId && void switchMut({ id: sessionId, index: w.index, on })}
                  onChooser={(current) => sessionId && void chooserMut({ id: sessionId, index: w.index, current })}
                  onInput={(text) => sessionId && void inputMut({ id: sessionId, index: w.index, text })}
                />
              ))}
              {notes.map((note) => (
                <p className="note" key={note}>{note}</p>
              ))}
            </aside>
            }
          />
        )}
      </main>
        }
        trailing={
      <section className="command" aria-label="Command center">
        <pre ref={logRef}>{frame?.log.length ? frame.log.join('\n') : 'Command center. A reporter prints its result here.'}</pre>
        <form onSubmit={(e) => void onCommand(e)}>
          <input
            aria-label="Command"
            placeholder="show count turtles"
            value={command}
            onChange={(e) => setCommand(e.target.value)}
            disabled={!sessionId}
          />
          <button className="act" type="submit" disabled={!sessionId || !command.trim()}>
            Run
          </button>
          <div className="hint">Observer commands and reporters, one line at a time.</div>
          {commandError && <div className="err">{commandError}</div>}
        </form>
      </section>
        }
      />

      {library && (
        <LibraryBrowser
          listing={library}
          loading={libraryLoading}
          error={libraryError}
          selectedPath={readLastModelPath()}
          onOpen={(path) => void openLibraryPath(path)}
          onUp={(path) => void openLibraryPath(path)}
          onFile={(entry) => void openLibraryFile(entry)}
          onClose={() => {
            setLibrary(null)
            setLibraryError(null)
          }}
          onLocal={() => {
            setLibrary(null)
            setLibraryError(null)
            fileRef.current?.click()
          }}
        />
      )}
      {leaveTo && (
        <div className="modal-back" onMouseDown={() => { if (!leaving) setLeaveTo(null) }}>
          <div
            className="modal"
            role="dialog"
            aria-modal="true"
            aria-labelledby="apply-changes-title"
            onMouseDown={(e) => e.stopPropagation()}
            onKeyDown={(e) => {
              if (e.key === 'Escape' && !leaving) setLeaveTo(null)
            }}
          >
            <p id="apply-changes-title">Apply changes?</p>
            <div className="modal-actions">
              <button type="button" disabled={leaving} onClick={() => setLeaveTo(null)}>Cancel</button>
              <button type="button" className="apply" autoFocus disabled={leaving} onClick={() => void applyAndLeave()}>
                Apply
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

// interfaceModelTitle is the name NetLogo puts in the window title:
// the file name without its extension. A missing name, or a file
// named Untitled, is a blank model.
function interfaceModelTitle(modelName: string): string {
  const base = modelName.replace(/\.nlogo(?:x3d|x|3d)?$/i, '').trim()
  if (base === '' || /^untitled$/i.test(base)) return 'Untitled'
  return base
}

function MainTabs({
  tab,
  showDynamics,
  onChange,
}: {
  tab: MainTab
  showDynamics: boolean
  onChange: (next: MainTab) => void
}) {
  return (
    <div className="tabs">
      {visibleMainTabs(showDynamics).map((id) => (
        <button type="button" key={id} className={tab === id ? 'on' : undefined} onClick={() => onChange(id)}>
          {mainTabLabel(id)}
        </button>
      ))}
    </div>
  )
}

function FrameCapControl({
  fps,
  disabled,
  onChange,
}: {
  fps: number
  disabled: boolean
  onChange: (fps: number) => void
}) {
  const [text, setText] = useState(String(fps || 24))
  const focused = useRef(false)
  useEffect(() => {
    if (!focused.current) setText(String(fps || 24))
  }, [fps])
  return (
    <label className="frame-cap">
      max fps
      <input
        type="number"
        min={1}
        max={120}
        step={1}
        aria-label="max fps"
        value={text}
        disabled={disabled}
        onFocus={() => {
          focused.current = true
        }}
        onBlur={() => {
          focused.current = false
          const n = Number(text)
          if (Number.isInteger(n) && n >= 1 && n <= 120) {
            onChange(n)
            setText(String(n))
          } else {
            setText(String(fps || 24))
          }
        }}
        onChange={(e) => {
          const next = e.target.value
          setText(next)
          const n = Number(next)
          if (next !== '' && Number.isInteger(n) && n >= 1 && n <= 120) onChange(n)
        }}
      />
    </label>
  )
}

function WorldSettingsForm({
  world,
  disabled,
  onApply,
  onPopulateSeed,
  onSeedOverride,
  onFlushSeed,
  onDirtyChange,
  registerApply,
  registerFlushSeed,
}: {
  world: WorldView | null
  disabled: boolean
  onApply: (next: WorldView) => Promise<string | null>
  onPopulateSeed: () => Promise<{ seed: string | null; message: string | null }>
  onSeedOverride: (on: boolean) => Promise<string | null>
  onFlushSeed: (seed: string) => Promise<string | null>
  onDirtyChange: (dirty: boolean) => void
  registerApply: (apply: (() => Promise<boolean>) | null) => void
  registerFlushSeed: (flush: (() => Promise<string | null>) | null) => void
}) {
  const [draft, setDraft] = useState<WorldDraft>({
    minPxcor: '0',
    maxPxcor: '99',
    minPycor: '0',
    maxPycor: '99',
    minPzcor: '0',
    maxPzcor: '0',
    patchSize: '12',
    wrapX: true,
    wrapY: true,
    wrapZ: false,
    rngSeed: '',
  })
  const [error, setError] = useState<string | null>(null)
  const [synced, setSynced] = useState(false)
  const showZ = world?.threeD === true
  const signature = world
    ? `${world.minPxcor},${world.maxPxcor},${world.minPycor},${world.maxPycor},${world.minPzcor ?? 0},${world.maxPzcor ?? 0},${world.patchSize},${world.wrapX},${world.wrapY},${world.wrapZ ?? false},${world.threeD ?? false}`
    : ''
  // Keep the seed box on the server value unless the user has edited it.
  // A geometry change still replaces the whole draft below.
  const serverSeed = world?.rngSeed ?? ''
  const [seenSeed, setSeenSeed] = useState(serverSeed)
  if (serverSeed !== seenSeed) {
    setSeenSeed(serverSeed)
    setDraft((d) => (d.rngSeed === seenSeed ? { ...d, rngSeed: serverSeed } : d))
  }
  const serverOverride = world?.seedOverride === true
  const [override, setOverride] = useState(serverOverride)
  const [seenOverride, setSeenOverride] = useState(serverOverride)
  if (serverOverride !== seenOverride) {
    setSeenOverride(serverOverride)
    setOverride(serverOverride)
  }
  const controls = seedControlState(override, disabled)

  useEffect(() => {
    if (!world) return
    setDraft({
      minPxcor: String(world.minPxcor),
      maxPxcor: String(world.maxPxcor),
      minPycor: String(world.minPycor),
      maxPycor: String(world.maxPycor),
      minPzcor: String(world.minPzcor ?? 0),
      maxPzcor: String(world.maxPzcor ?? 0),
      patchSize: String(world.patchSize),
      wrapX: world.wrapX,
      wrapY: world.wrapY,
      wrapZ: world.wrapZ ?? false,
      rngSeed: world.rngSeed ?? '',
    })
    setError(null)
    setSynced(true)
  }, [signature]) // eslint-disable-line react-hooks/exhaustive-deps

  const dirty = synced && world != null && worldDraftDirty(draft, world, override)
  useEffect(() => {
    onDirtyChange(dirty)
  }, [dirty, onDirtyChange])
  useEffect(() => () => onDirtyChange(false), [onDirtyChange])

  async function submitDraft() {
    const parsed = parseWorldDraft(draft, showZ, override)
    if (typeof parsed === 'string') {
      setError(parsed)
      return false
    }
    // Override on always sends the seed, even when it matches the server,
    // so a second Apply cannot skip the reseed. Override off omits it.
    const next: WorldView = {
      ...parsed,
      rngSeed: rngSeedForApply(override, draft.rngSeed),
      seedOverride: override,
    }
    setError(null)
    const message = await onApply(next)
    setError(message)
    return message == null
  }

  async function toggleOverride(on: boolean) {
    setOverride(on)
    const message = await onSeedOverride(on)
    if (message) setError(message)
  }

  async function populate() {
    const result = await onPopulateSeed()
    if (result.message) {
      setError(result.message)
      return
    }
    if (result.seed) setDraft((d) => ({ ...d, rngSeed: result.seed ?? '' }))
    setError(null)
  }

  useEffect(() => {
    registerApply(submitDraft)
    return () => registerApply(null)
  }, [registerApply, draft, showZ, onApply, override]) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    registerFlushSeed(async () => {
      if (!override || !world) return null
      const err = rngSeedError(draft.rngSeed)
      if (err) return err
      if (draft.rngSeed === (world.rngSeed ?? '') && world.seedOverride === true) return null
      return onFlushSeed(draft.rngSeed)
    })
    return () => registerFlushSeed(null)
  }, [registerFlushSeed, override, world, draft.rngSeed, onFlushSeed])

  async function apply(event: FormEvent) {
    event.preventDefault()
    await submitDraft()
  }

  return (
    <form className="world-settings" onSubmit={(e) => void apply(e)}>
      <h2>World</h2>
      <div className="seed">
        <label className="check">
          <input
            type="checkbox"
            aria-label="Seed override"
            checked={override}
            disabled={controls.overrideDisabled}
            onChange={(e) => void toggleOverride(e.target.checked)}
          />
          Seed override
        </label>
        <span>RNG Seed</span>
        <input
          aria-label="RNG Seed"
          size={12}
          inputMode="numeric"
          spellCheck={false}
          autoCapitalize="off"
          autoCorrect="off"
          autoComplete="off"
          value={draft.rngSeed}
          disabled={controls.seedDisabled}
          onChange={(e) => setDraft({ ...draft, rngSeed: e.target.value })}
        />
        <button className="iface" type="button" aria-label="Random RNG seed" disabled={controls.randomDisabled} onClick={() => void populate()}>
          Random
        </button>
      </div>
      <p className="hint">
        Check Seed override, enter a seed, then press setup and go. Press setup again to repeat the run.
        A random-seed command in the model still runs during that setup.
      </p>
      <div className="axes">
        <span />
        <span className="col">min</span>
        <span className="col">max</span>
        <span className="axis">pxcor</span>
        <input aria-label="min pxcor" size={4} inputMode="numeric" value={draft.minPxcor} disabled={disabled} onChange={(e) => setDraft({ ...draft, minPxcor: e.target.value })} />
        <input aria-label="max pxcor" size={4} inputMode="numeric" value={draft.maxPxcor} disabled={disabled} onChange={(e) => setDraft({ ...draft, maxPxcor: e.target.value })} />
        <span className="axis">pycor</span>
        <input aria-label="min pycor" size={4} inputMode="numeric" value={draft.minPycor} disabled={disabled} onChange={(e) => setDraft({ ...draft, minPycor: e.target.value })} />
        <input aria-label="max pycor" size={4} inputMode="numeric" value={draft.maxPycor} disabled={disabled} onChange={(e) => setDraft({ ...draft, maxPycor: e.target.value })} />
        {showZ && (
          <>
            <span className="axis">pzcor</span>
            <input aria-label="min pzcor" size={4} inputMode="numeric" value={draft.minPzcor} disabled={disabled} onChange={(e) => setDraft({ ...draft, minPzcor: e.target.value })} />
            <input aria-label="max pzcor" size={4} inputMode="numeric" value={draft.maxPzcor} disabled={disabled} onChange={(e) => setDraft({ ...draft, maxPzcor: e.target.value })} />
          </>
        )}
      </div>
      <div className="extras">
        <label className="patch">
          patch
          <input aria-label="patch size" size={4} inputMode="decimal" value={draft.patchSize} disabled={disabled} onChange={(e) => setDraft({ ...draft, patchSize: e.target.value })} />
        </label>
        <label className="check">
          <input type="checkbox" aria-label="wrap x" checked={draft.wrapX} disabled={disabled} onChange={(e) => setDraft({ ...draft, wrapX: e.target.checked })} />
          wrap x
        </label>
        <label className="check">
          <input type="checkbox" aria-label="wrap y" checked={draft.wrapY} disabled={disabled} onChange={(e) => setDraft({ ...draft, wrapY: e.target.checked })} />
          wrap y
        </label>
        {showZ && (
          <label className="check">
            <input type="checkbox" aria-label="wrap z" checked={draft.wrapZ} disabled={disabled} onChange={(e) => setDraft({ ...draft, wrapZ: e.target.checked })} />
            wrap z
          </label>
        )}
      </div>
      <button className="iface" type="submit" disabled={disabled}>Apply</button>
      {error && <p className="err">{error}</p>}
    </form>
  )
}

type WorldDraft = {
  minPxcor: string
  maxPxcor: string
  minPycor: string
  maxPycor: string
  minPzcor: string
  maxPzcor: string
  patchSize: string
  wrapX: boolean
  wrapY: boolean
  wrapZ: boolean
  rngSeed: string
}

function worldDraftDirty(draft: WorldDraft, world: WorldView, seedOverride: boolean) {
  const xy =
    !sameNumber(draft.minPxcor, world.minPxcor) ||
    !sameNumber(draft.maxPxcor, world.maxPxcor) ||
    !sameNumber(draft.minPycor, world.minPycor) ||
    !sameNumber(draft.maxPycor, world.maxPycor) ||
    !sameNumber(draft.patchSize, world.patchSize) ||
    draft.wrapX !== world.wrapX ||
    draft.wrapY !== world.wrapY ||
    (seedOverride && draft.rngSeed !== (world.rngSeed ?? ''))
  if (world.threeD !== true) return xy
  return (
    xy ||
    !sameNumber(draft.minPzcor, world.minPzcor ?? 0) ||
    !sameNumber(draft.maxPzcor, world.maxPzcor ?? 0) ||
    draft.wrapZ !== (world.wrapZ ?? false)
  )
}

function sameNumber(raw: string, live: number) {
  const text = raw.trim()
  if (text === '') return false
  const n = Number(text)
  return Number.isFinite(n) && n === live
}

function parseWorldDraft(draft: WorldDraft, threeD: boolean, seedOverride: boolean): WorldView | string {
  if (seedOverride) {
    const seedError = rngSeedError(draft.rngSeed)
    if (seedError) return seedError
  }
  const minPxcor = Number(draft.minPxcor)
  const maxPxcor = Number(draft.maxPxcor)
  const minPycor = Number(draft.minPycor)
  const maxPycor = Number(draft.maxPycor)
  const patchSize = Number(draft.patchSize)
  const coords = [minPxcor, maxPxcor, minPycor, maxPycor]
  let minPzcor = 0
  let maxPzcor = 0
  if (threeD) {
    minPzcor = Number(draft.minPzcor)
    maxPzcor = Number(draft.maxPzcor)
    coords.push(minPzcor, maxPzcor)
  }
  if (!coords.every((n) => Number.isInteger(n))) {
    return 'Coordinates must be integers.'
  }
  const missesOrigin =
    minPxcor > 0 || maxPxcor < 0 || minPycor > 0 || maxPycor < 0 || (threeD && (minPzcor > 0 || maxPzcor < 0))
  if (missesOrigin) {
    return threeD ? 'The world must contain the origin (0, 0, 0).' : 'The world must contain the origin (0, 0).'
  }
  if (!(patchSize > 0) || patchSize > 100) {
    return 'Patch size must be greater than 0 and at most 100.'
  }
  const next: WorldView = {
    minPxcor,
    maxPxcor,
    minPycor,
    maxPycor,
    patchSize,
    wrapX: draft.wrapX,
    wrapY: draft.wrapY,
    rngSeed: draft.rngSeed,
  }
  if (!threeD) return next
  return { ...next, minPzcor, maxPzcor, wrapZ: draft.wrapZ, threeD: true }
}

type WorldApply = { message: string | null; procedures: string[] }

function worldGeometry(live: WorldView): WorldView {
  return {
    minPxcor: live.minPxcor,
    maxPxcor: live.maxPxcor,
    minPycor: live.minPycor,
    maxPycor: live.maxPycor,
    patchSize: live.patchSize,
    wrapX: live.wrapX,
    wrapY: live.wrapY,
    minPzcor: live.minPzcor,
    maxPzcor: live.maxPzcor,
    wrapZ: live.wrapZ,
    threeD: live.threeD,
  }
}

async function sendWorld(
  sessionId: string,
  next: WorldView,
  setWorldMut: (
    vars: Omit<WorldView, 'minPzcor' | 'maxPzcor' | 'wrapZ' | 'rngSeed' | 'seedOverride'> & {
      id: string
      minPzcor: number | null
      maxPzcor: number | null
      wrapZ: boolean | null
      rngSeed: string | null
      seedOverride: boolean | null
    },
  ) => Promise<{
    error?: { message: string }
    data?: { setWorld: { procedures?: string[] } }
  }>,
): Promise<WorldApply> {
  const res = await setWorldMut({
    id: sessionId,
    ...next,
    minPzcor: next.threeD ? (next.minPzcor ?? 0) : null,
    maxPzcor: next.threeD ? (next.maxPzcor ?? 0) : null,
    wrapZ: next.threeD ? (next.wrapZ ?? false) : null,
    rngSeed: next.rngSeed ?? null,
    seedOverride: next.seedOverride ?? null,
  })
  const message = (res.error?.message ?? '').replace(/^\[GraphQL\]\s*/i, '').trim()
  if (reportDisconnect(message)) return { message: 'Server disconnected. Reconnecting…', procedures: [] }
  if (message) return { message, procedures: [] }
  return { message: null, procedures: res.data?.setWorld.procedures ?? [] }
}

function dragHasFiles(event: DragEvent) {
  return Array.from(event.dataTransfer?.types ?? []).includes('Files')
}

function presentCompileError(message: string, source: string, filename?: string) {
  const cleaned = message.replace(/^\[GraphQL\]\s*/i, '').trim()
  const match = /line (\d+)/i.exec(cleaned)
  if (!match) return cleaned
  const n = Number(match[1])
  const lines = filename ? codeLines(filename, source) : source.replace(/\r\n/g, '\n').replace(/\r/g, '\n').split('\n')
  const text = lines[n - 1]
  if (text == null) return cleaned
  return `${cleaned}\n${String(n).padStart(4, ' ')} | ${text}`
}

function codeLines(filename: string, source: string) {
  let code = source.replace(/\r\n/g, '\n').replace(/\r/g, '\n')
  const lower = filename.toLowerCase()
  const xml = lower.endsWith('.nlogox') || lower.endsWith('.nlogox3d') || /^\s*<\?xml/.test(code) || /^\s*<model[\s>]/.test(code)
  if (xml) {
    const found = code.match(/<code[^>]*>([\s\S]*?)<\/code>/i)
    if (found) {
      code = found[1].replace(/^\s*<!\[CDATA\[/, '').replace(/\]\]>\s*$/, '')
    }
  } else {
    const sep = code.indexOf('@#$#@#$#@')
    if (sep >= 0) code = code.slice(0, sep)
  }
  return code.split('\n')
}

function foreverIndex(widgets: { index: number; kind: string; display: string; forever: boolean }[]) {
  const go = widgets.find((w) => w.kind === 'button' && w.forever && w.display.toLowerCase() === 'go')
  return go?.index ?? widgets.find((w) => w.kind === 'button' && w.forever)?.index ?? null
}

type OutputCursor = { epoch: number; seq: number; lines: string[]; partial: string }

function absorbOutput(state: OutputCursor, frame: Frame, setText: (text: string) => void) {
  let lines = state.lines
  if (frame.outputReset || frame.outputEpoch !== state.epoch || frame.outputSeq < state.seq) {
    lines = frame.outputAppend ?? []
  } else if (frame.outputSeq !== state.seq) {
    const append = frame.outputAppend ?? []
    const delta = frame.outputSeq - state.seq
    const extra = append.length - delta
    lines = state.lines.concat(extra > 0 ? append.slice(extra) : append)
  }
  if (lines.length > 2000) lines = lines.slice(-2000)
  const partial = frame.outputPartial ?? ''
  const same =
    lines === state.lines &&
    frame.outputEpoch === state.epoch &&
    frame.outputSeq === state.seq &&
    partial === state.partial
  state.epoch = frame.outputEpoch
  state.seq = frame.outputSeq
  state.lines = lines
  state.partial = partial
  if (same) return
  setText(joinOutput(lines, partial))
}

function joinOutput(lines: string[], partial: string) {
  if (!partial) return lines.join('\n')
  if (lines.length === 0) return partial
  return lines.join('\n') + '\n' + partial
}

function plotFor(frame: Frame | null, widget: Widget): PlotView | null {
  if (!frame || widget.kind !== 'plot') return null
  const name = widget.display.toLowerCase()
  return (frame.plots ?? []).find((plot) => plot.name.toLowerCase() === name) ?? null
}

function WidgetView({
  widget,
  live,
  outputText,
  plot,
  series,
  onPress,
  onSlider,
  onSwitch,
  onChooser,
  onInput,
}: {
  widget: Widget
  live: boolean
  outputText: string
  plot: PlotView | null
  series: PlotSeries
  onPress: () => void
  onSlider: (value: number) => void
  onSwitch: (on: boolean) => void
  onChooser: (current: number) => void
  onInput: (text: string) => void
}) {
  const [slider, setSlider] = useState<number | null>(null)
  const [text, setText] = useState<string | null>(null)

  if (widget.kind === 'button') {
    const label = live ? 'stop' : widget.display
    return (
      <div className="widget">
        <button
          type="button"
          className={live ? 'iface live' : widget.forever ? 'iface forever' : 'iface'}
          disabled={widget.disabled && !live}
          onClick={onPress}
        >
          {label}
          {widget.forever && !live ? ' ∞' : ''}
        </button>
      </div>
    )
  }

  if (widget.kind === 'slider') {
    const value = slider ?? widget.value ?? widget.min ?? 0
    const units = widget.units ? ` ${widget.units}` : ''
    return (
      <div className="widget">
        <div className="label">{widget.display}</div>
        <input
          type="range"
          min={widget.min ?? 0}
          max={widget.max ?? 100}
          step={widget.step || 1}
          aria-label={widget.display}
          value={value}
          onChange={(e) => {
            const next = Number(e.target.value)
            setSlider(next)
            onSlider(next)
          }}
        />
        <div className="slider-val">{value}{units}</div>
      </div>
    )
  }

  if (widget.kind === 'switch') {
    return (
      <label className="switch">
        <input
          type="checkbox"
          checked={widget.on ?? false}
          aria-label={widget.display}
          onChange={(e) => onSwitch(e.target.checked)}
        />
        {widget.display}
      </label>
    )
  }

  if (widget.kind === 'monitor') {
    return (
      <div className="widget">
        <div className="label">{widget.display}</div>
        <div className="readout">{widget.reading ?? '—'}</div>
      </div>
    )
  }

  if (widget.kind === 'chooser') {
    return (
      <div className="widget">
        <div className="label">{widget.display}</div>
        <select
          aria-label={widget.display}
          value={widget.current ?? 0}
          onChange={(e) => onChooser(Number(e.target.value))}
        >
          {(widget.choices ?? []).map((choice, i) => (
            <option key={choice} value={i}>{choice}</option>
          ))}
        </select>
      </div>
    )
  }

  if (widget.kind === 'input') {
    const shown = text ?? widget.text ?? ''
    return (
      <div className="widget">
        <div className="label">{widget.display}</div>
        <input
          type="text"
          aria-label={widget.display}
          value={shown}
          onChange={(e) => {
            setText(e.target.value)
            onInput(e.target.value)
          }}
        />
      </div>
    )
  }

  if (widget.kind === 'output') {
    return <OutputPanel widget={widget} text={outputText} />
  }

  if (widget.kind === 'note') {
    return <p className="note">{widget.display}</p>
  }

  if (widget.kind === 'plot') {
    if (!plot) {
      return (
        <div className="plot-card">
          <div className="label">{widget.display}</div>
        </div>
      )
    }
    return <PlotCard plot={plot} series={series} />
  }

  return null
}

function OutputPanel({ widget, text }: { widget: Widget; text: string }) {
  const ref = useRef<HTMLPreElement>(null)
  useEffect(() => {
    const el = ref.current
    if (el) el.scrollTop = el.scrollHeight
  }, [text])
  const fontSize = widget.fontSize > 0 ? Math.min(28, Math.max(10, widget.fontSize)) : 12
  const height = widget.height > 0 ? Math.min(200, Math.max(72, widget.height)) : 120
  return (
    <div className="output-panel" aria-label="Output">
      <div className="label">Output</div>
      <pre ref={ref} style={{ fontSize, maxHeight: height }}>
        {text}
      </pre>
    </div>
  )
}

function FrameBridge({
  sessionId,
  onFrame,
  onError,
}: {
  sessionId: string
  onFrame: (frame: Frame) => void
  onError: (message: string) => void
}) {
  const [res] = useSubscription<{ frames: Frame }>({
    query: Frames,
    variables: { id: sessionId },
  })
  useEffect(() => {
    if (res.data?.frames) onFrame(res.data.frames)
  }, [res.data, onFrame])
  useEffect(() => {
    if (res.error) onError(res.error.message)
  }, [res.error, onError])
  return null
}
