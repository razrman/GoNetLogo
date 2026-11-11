// Package session runs one NetLogo model per client.
//
// The interpreter, the world, and the widgets live here. GraphQL
// resolvers call into this package and never touch the language core
// on their own. Each session has its own VM, so two clients do not
// share turtles, ticks, or slider values.
package session

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"image/color"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gonetlogo/backend/graph/model"
	"gonetlogo/models"
	"gonetlogo/pkg/nl"
)

// Store is the set of live sessions.
//
// mu guards the map and every session's client lease (clients, leaseGen,
// closing, leaseTimer, lastSocket, lastHeartbeat). Session.mu is a
// different lock. Never take Session.mu while holding Store.mu: Close
// and the step loop take Session.mu and must not wait on the store.
type Store struct {
	mu         sync.Mutex
	sessions   map[string]*Session
	grace      time.Duration
	idleTTL    time.Duration
	sweepEvery time.Duration
	heartbeat  time.Duration
	// maxSessions caps Create. 0 means unlimited. Guarded by mu.
	maxSessions int
	stop        chan struct{}
	sweepDone   chan struct{}
	closed      bool
	closeOnce   sync.Once
}

// NewStore starts an empty store with the default grace period and idle sweep.
func NewStore() *Store {
	return NewStoreWith(DefaultLifecycle())
}

// Models lists the embedded demo programs.
func (st *Store) Models() ([]string, error) {
	entries, err := fs.ReadDir(models.FS, ".")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".nlogo") || strings.HasSuffix(name, ".nlogox") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}

// ErrTooManySessions is returned by Create when the store is already at
// its cap. The text is the GraphQL error returned to API clients.
var ErrTooManySessions = errors.New("too many sessions: this server is hosting as many as it can")

// SetMaxSessions caps how many sessions Create will keep. n <= 0 means
// no cap, which is the local-development default. The count is every
// session still in the store, including ones waiting out a disconnect.
func (st *Store) SetMaxSessions(n int) {
	if n < 0 {
		n = 0
	}
	st.mu.Lock()
	st.maxSessions = n
	st.mu.Unlock()
}

func (st *Store) atCapLocked() bool {
	return st.maxSessions > 0 && len(st.sessions) >= st.maxSessions
}

// errAtCap rejects a create that is already over the cap, before a
// scratch directory or an interpreter is allocated.
func (st *Store) errAtCap(modelName string) error {
	st.mu.Lock()
	closed := st.closed
	atCap := st.atCapLocked()
	max := st.maxSessions
	st.mu.Unlock()
	if closed {
		return fmt.Errorf("session store is closed")
	}
	if !atCap {
		return nil
	}
	slog.Warn("session create failed", "model", modelName, "err", ErrTooManySessions, "maxSessions", max)
	return ErrTooManySessions
}

// Create loads a model into a new isolated session.
func (st *Store) Create(modelName string) (*Session, error) {
	if err := st.errAtCap(modelName); err != nil {
		return nil, err
	}
	name, content, err := readModel(modelName)
	if err != nil {
		slog.Warn("session create failed", "model", modelName, "err", err)
		return nil, err
	}
	s := newSession()
	root := ""
	if err := s.useModelFrom(name, content, &root); err != nil {
		slog.Warn("session create failed", "session", s.id, "model", name, "err", err)
		s.Close()
		return nil, err
	}
	// loop steps the model. publishLoop snapshots on the frame-rate cap.
	// Both exit when Close closes s.stop. The WaitGroup is how tests
	// know those goroutines are gone.
	s.trackLoops = true
	s.loops.Add(2)
	go s.loop()
	st.mu.Lock()
	if st.closed {
		st.mu.Unlock()
		s.Close()
		return nil, fmt.Errorf("session store is closed")
	}
	if st.atCapLocked() {
		max := st.maxSessions
		st.mu.Unlock()
		s.Close()
		slog.Warn("session create failed", "model", name, "err", ErrTooManySessions, "maxSessions", max)
		return nil, ErrTooManySessions
	}
	s.lastSocket = time.Now()
	st.sessions[s.id] = s
	st.mu.Unlock()
	if debugEnabled() {
		slog.Debug("session created", "session", s.id, "model", name)
	}
	return s, nil
}

// Get returns a session by id. It does not count as client activity, so a
// test poll or a log line cannot postpone the idle sweep.
func (st *Store) Get(id string) (*Session, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	s, ok := st.sessions[id]
	return s, ok
}

// Use returns a session for a resolver. A lookup is not websocket
// activity and does not move the idle or heartbeat clocks, so a poll
// cannot postpone teardown or cancel a pending disconnect.
func (st *Store) Use(id string) (*Session, bool) {
	return st.Get(id)
}

// Session is one interpreter, one world, and the widgets that drive it.
type Session struct {
	mu        sync.Mutex
	logMu     sync.Mutex
	id        string
	modelName string
	model     *nl.FileModel
	// modelDir is the loaded file's folder relative to the models
	// directory. Nil when the text was not read from that directory.
	// A pointer to an empty string is the models root.
	modelDir  *string
	vm        *nl.VM
	buttons   map[int][]nl.Stmt
	running   int
	speed     float64
	log       []string
	listeners map[int]chan struct{}
	nextSub   int
	frame     int
	clock     nl.RunClock
	frameRate int
	pub       nl.FramePublisher
	// busy is true while a button is inside the interpreter with s.mu released.
	busy bool
	// pendingOnce is the once-buttons waiting to run between steps of go,
	// oldest first. Every click is kept. NetLogo runs a once-button during
	// a forever button; the current step is interrupted and go continues.
	pendingOnce []int
	// cancelRun asks the button in progress to stop at the next checkpoint.
	// A one-shot clears it so setup is not aborted by the stop that ended go.
	cancelRun bool
	// shown* is the last tick sample taken under the world lock.
	// Stop and the speed slider read it instead of the live world, so they
	// do not wait out a generation.
	shownStarted bool
	shownTicks   float64
	shownEpoch   uint64
	rateNote     chan struct{}
	stop         chan struct{}
	closed       bool
	// drop* summarizes cap skips and slow subscribers. The step loop
	// logs at most one line per second.
	dropWindow time.Time
	dropCap    int
	dropSlow   int
	// ignored* rate-limits the "run active" warning.
	ignored   int
	ignoredAt time.Time
	// seedOverride applies overrideSeed before every setup and after a
	// model load. The World tab uses it to repeat a run. A model's own
	// random-seed still runs after that seed and wins for that setup.
	// overrideSeed is the unsigned 32-bit value, stored as a non-negative int64.
	seedOverride bool
	overrideSeed int64
	// progress* and run* feed the debug progress line and the stop log.
	progressAt time.Time
	rngMark    uint64
	rngMarkAt  time.Time
	runStart   time.Time
	runRNG     uint64
	capDrawing bool
	capPlot    bool
	// scratch is a private directory for this session's uploaded model
	// text. Close deletes it. Empty when the directory could not be made.
	// Guarded by s.mu.
	scratch string
	// loops counts loop and publishLoop when trackLoops is set. Create adds
	// 2 before starting them. Sessions started with a bare go s.loop()
	// leave the counter alone.
	trackLoops bool
	loops      sync.WaitGroup
	// Client lease, guarded by Store.mu and never by s.mu.
	// clients is the number of live frames subscriptions. Teardown checks
	// this under Store.mu and never removes a session while it is positive.
	// leaseGen changes whenever a kill timer is armed or cancelled, so a
	// timer that fires late is a no-op.
	// closing is set by the end-session beacon so the log can name it.
	// It does not bypass the client count.
	// lastSocket is the last create, attach, or detach.
	// lastHeartbeat is the last UI heartbeat. The idle clock is the later
	// of the two.
	clients       int
	leaseGen      uint64
	closing       bool
	leaseTimer    *time.Timer
	lastSocket    time.Time
	lastHeartbeat time.Time
}

func newSession() *Session {
	s := &Session{
		id:          newID(),
		running:     -1,
		pendingOnce: nil,
		speed:       40,
		frameRate:   nl.DefaultFrameRate,
		buttons:     map[int][]nl.Stmt{},
		listeners:   map[int]chan struct{}{},
		rateNote:    make(chan struct{}, 1),
		stop:        make(chan struct{}),
	}
	dir, err := os.MkdirTemp("", "gonetlogo-session-")
	if err != nil {
		slog.Warn("session storage", "session", s.id, "err", err)
	} else {
		s.scratch = dir
	}
	return s
}

func (s *Session) ID() string { return s.id }

// Running reports whether a forever button is stepping.
// A closed session is not running.
func (s *Session) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.closed && s.running >= 0
}

// Close stops the forever loop, releases the interpreter, and deletes this
// session's private storage. It is safe to call more than once.
func (s *Session) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.cancelRun = true
	s.flushDropsLocked()
	s.setRunningLocked(-1, "close")
	vm := s.vm
	id := s.id
	scratch := s.scratch
	s.scratch = ""
	listeners := s.listeners
	s.listeners = map[int]chan struct{}{}
	close(s.stop)
	s.mu.Unlock()
	// Wake subscribers after releasing s.mu. broadcastLocked sends only
	// while it holds s.mu and only on channels still in the map, so this
	// close cannot race a send.
	for _, ch := range listeners {
		close(ch)
	}
	s.haltVM(vm, "close")
	if vm != nil {
		vm.Close()
	}
	if scratch != "" {
		if err := os.RemoveAll(scratch); err != nil {
			slog.Warn("session storage", "session", id, "err", err)
		}
	}
	if debugEnabled() {
		slog.Debug("session closed", "session", id)
	}
}

// stepSlot is one forever-button turn. Speeds below 100 run a fixed
// number of steps in the slot. Speed 100 fills the slot, then yields so
// stop, widget edits, and the frame publisher can take the mutex.
const stepSlot = 30 * time.Millisecond

func (s *Session) loop() {
	// Steps stay on the speed-slider cadence. publishLoop, started here,
	// publishes on the frame-rate interval so it can snapshot between
	// steps without the step loop waiting on a subscriber.
	if s.trackLoops {
		defer s.loops.Done()
	}
	go s.publishLoop()
	step := time.NewTicker(stepSlot)
	defer step.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-step.C:
			s.runStepSlot()
		}
	}
}

func (s *Session) publishLoop() {
	if s.trackLoops {
		defer s.loops.Done()
	}
	s.mu.Lock()
	interval := nl.FrameInterval(s.frameRate)
	s.mu.Unlock()
	publish := time.NewTimer(interval)
	defer publish.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-s.rateNote:
			s.mu.Lock()
			interval = nl.FrameInterval(s.frameRate)
			s.mu.Unlock()
			if !publish.Stop() {
				select {
				case <-publish.C:
				default:
				}
			}
			publish.Reset(interval)
		case now := <-publish.C:
			s.mu.Lock()
			if !s.closed {
				s.publishDueLocked(now)
			}
			interval = nl.FrameInterval(s.frameRate)
			wait := s.pub.NextWait(now, interval)
			closed := s.closed
			s.mu.Unlock()
			if closed {
				return
			}
			publish.Reset(wait)
		}
	}
}

// runStepSlot runs one speed-slider slot, releasing the lock between
// steps so the publish timer can snapshot the latest world without
// waiting for the rest of the slot or for a subscriber.
// A once-button pressed during go runs at the start of the slot, and
// again between steps, before another go iteration.
func (s *Session) runStepSlot() {
	s.mu.Lock()
	if s.closed || s.model == nil {
		s.mu.Unlock()
		return
	}
	if s.busy {
		s.mu.Unlock()
		return
	}
	s.runPendingOnceLocked()
	if s.closed || s.model == nil || s.running < 0 || s.running >= len(s.model.Widgets) {
		s.mu.Unlock()
		return
	}
	s.frame++
	speed := s.speed
	n := stepsFor(speed, s.frame)
	warp := speed >= 100
	s.mu.Unlock()
	// Speed 100 is NetLogo warp: step until the slot is over instead of
	// stopping after a handful of gos. The lock is dropped around every
	// step, and snapshots stay on the frame-rate cap.
	var deadline time.Time
	if warp {
		deadline = time.Now().Add(stepSlot)
	}
	for steps := 0; warp || steps < n; steps++ {
		if warp && steps > 0 && !time.Now().Before(deadline) {
			return
		}
		s.mu.Lock()
		if !s.busy {
			s.runPendingOnceLocked()
		}
		if s.closed || s.model == nil || s.running < 0 || s.running >= len(s.model.Widgets) || s.cancelRun {
			s.syncClockLocked(time.Now())
			s.mu.Unlock()
			return
		}
		_, err := s.runIndexLocked(s.running)
		if errors.Is(err, errModelBusy) {
			s.mu.Unlock()
			return
		}
		stopped := s.running < 0
		now := time.Now()
		if err == nil {
			s.pub.Produce(1)
		}
		s.syncClockLocked(now)
		s.publishDueLocked(now)
		s.mu.Unlock()
		if err != nil || stopped {
			return
		}
	}
}

// frameTickLocked runs the speed-slider steps for one 30ms slot.
// It does not publish. Caller holds s.mu for the whole slot.
func (s *Session) frameTickLocked() {
	if !s.busy {
		s.runPendingOnceLocked()
	}
	if s.model == nil || s.running < 0 || s.running >= len(s.model.Widgets) {
		return
	}
	s.frame++
	n := stepsFor(s.speed, s.frame)
	warp := s.speed >= 100
	var deadline time.Time
	if warp {
		deadline = time.Now().Add(stepSlot)
	}
	done := 0
	for i := 0; warp || i < n; i++ {
		if warp && i > 0 && !time.Now().Before(deadline) {
			break
		}
		_, err := s.runIndexLocked(s.running)
		stopped := s.running < 0
		if err != nil {
			break
		}
		done++
		if stopped {
			break
		}
	}
	s.syncClockLocked(time.Now())
	s.pub.Produce(done)
}

// publishDueLocked sends the latest snapshot when the cap interval fires.
// Caller holds s.mu. Intermediate steps are counted, not queued.
func (s *Session) publishDueLocked(now time.Time) {
	before := s.pub.Dropped()
	if !s.pub.Publish(now, nl.FrameInterval(s.frameRate)) {
		return
	}
	s.noteDropsLocked(s.pub.Dropped()-before, 0)
	s.broadcastLocked()
}

// refreshDueLocked publishes the in-progress world without counting a step.
// A long button calls it from the yield callback so the view keeps moving.
func (s *Session) refreshDueLocked(now time.Time) {
	before := s.pub.Dropped()
	if !s.pub.Refresh(now, nl.FrameInterval(s.frameRate)) {
		return
	}
	s.noteDropsLocked(s.pub.Dropped()-before, 0)
	s.broadcastLocked()
}

// errModelBusy is returned when a second command arrives while a button is
// inside the interpreter. User-facing entry points reject that request.
var errModelBusy = errors.New("the model is running a command")

// armYield lets a long button publish frames and observe stop without
// holding s.mu. Caller holds s.mu. The VM must not already be inside a run.
func (s *Session) armYield(vm *nl.VM) {
	if vm == nil {
		return
	}
	vm.ClearHalt()
	if s.cancelRun || s.closed || len(s.pendingOnce) > 0 {
		vm.RequestHalt()
	}
	vm.SetYield(func() {
		lockAt := time.Now()
		s.mu.Lock()
		waited := time.Since(lockAt)
		now := time.Now()
		s.sampleTicksLocked()
		if s.closed || s.cancelRun || len(s.pendingOnce) > 0 {
			vm.RequestHalt()
		}
		if !s.closed {
			s.syncClockLocked(now)
			s.refreshDueLocked(now)
		}
		s.mu.Unlock()
		if waited > 250*time.Millisecond {
			slog.Warn("lock wait", "session", s.id, "where", "yield", "wait", waited)
		}
		s.noteProgress(vm)
	}, nl.DefaultYieldInterval)
}

// Subscribe registers a wake-up channel. The first value is not sent
// here; the caller snapshots once, then waits. The channel is removed
// when ctx ends. A slow subscriber misses intermediate frames and
// still observes the latest state on the next wake-up.
func (s *Session) Subscribe() (wake <-chan struct{}, cancel func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		ch := make(chan struct{})
		close(ch)
		return ch, func() {}
	}
	ch := make(chan struct{}, 1)
	id := s.nextSub
	s.nextSub++
	s.listeners[id] = ch
	return ch, func() {
		s.mu.Lock()
		delete(s.listeners, id)
		s.mu.Unlock()
	}
}

func (s *Session) useModel(name, content string) error {
	return s.useModelFrom(name, content, nil)
}

// useModelFrom installs a parsed model. modelDir is the folder relative
// to the models directory, copied so the caller can reuse its variable.
// nil clears it: the text did not come from that directory.
func (s *Session) useModelFrom(name, content string, modelDir *string) error {
	n := len(content)
	m, err := nl.ParseModel(name, content)
	if err != nil {
		logModel("model compile failed", s.id, name, n, err)
		return err
	}
	vm, err := m.NewVM(0)
	if err != nil {
		logModel("model compile failed", s.id, name, n, err)
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.rejectIfActiveLocked("load model"); err != nil {
		return err
	}
	s.logMu.Lock()
	s.log = nil
	s.logMu.Unlock()
	s.installLocked(name, m, vm)
	if modelDir == nil {
		s.modelDir = nil
	} else {
		dir := *modelDir
		s.modelDir = &dir
	}
	s.broadcastLocked()
	return nil
}

func (s *Session) installLocked(name string, m *nl.FileModel, vm *nl.VM) {
	old := s.vm
	s.modelName = name
	s.model = m
	s.vm = vm
	if old != nil && old != vm {
		old.Close()
	}
	s.pendingOnce = nil
	s.setRunningLocked(-1, "load")
	s.frame = 0
	s.clock = nl.RunClock{}
	s.pub.Reset()
	vm.OnPrint = func(line string) {
		// Print runs with the world lock held and s.mu released. The log
		// has its own lock so this does not wait on the session and the
		// session does not take the world lock to read the log.
		s.logMu.Lock()
		s.appendLogBody(line)
		s.logMu.Unlock()
	}
	vm.OnClear = func() {
		s.logMu.Lock()
		s.log = nil
		s.logMu.Unlock()
	}
	s.compileButtonsLocked()
	logModel("model loaded", s.id, name, len(m.Code), nil)
	widgets, plots := 0, 0
	for _, w := range m.Widgets {
		widgets++
		if w.Kind == "plot" {
			plots++
		}
	}
	slog.Info("model loaded", "session", s.id, "model", name, "widgets", widgets, "plots", plots,
		"min_px", m.View.MinPxcor, "max_px", m.View.MaxPxcor, "min_py", m.View.MinPycor, "max_py", m.View.MaxPycor)
	// A freshly built world is clock-seeded. The override replaces that
	// before the next setup, so a reload repeats the same run.
	if s.seedOverride && vm.World != nil {
		vm.World.Seed(s.overrideSeed)
	}
}

func (s *Session) compileButtonsLocked() {
	s.buttons = map[int][]nl.Stmt{}
	if s.vm == nil || s.vm.Prog == nil {
		return
	}
	for i, w := range s.model.Widgets {
		if w.Kind != "button" {
			continue
		}
		stmts, err := nl.CompileButton(s.vm.Prog, w.Source, w.Agent)
		if err != nil {
			s.appendLogLocked(w.Display + ": " + err.Error())
			slog.Warn("button compile failed", "session", s.id, "model", s.modelName, "button", w.Display, "err", err)
			continue
		}
		s.buttons[i] = stmts
	}
	if debugEnabled() {
		slog.Debug("model compiled", "session", s.id, "model", s.modelName, "buttons", len(s.buttons))
	}
}

// LoadModel replaces the program with an embedded demo and rebuilds the world.
func (s *Session) LoadModel(modelName string) error {
	name, content, err := readModel(modelName)
	if err != nil {
		slog.Warn("model load failed", "session", s.id, "model", modelName, "err", err)
		return err
	}
	root := ""
	return s.useModelFrom(name, content, &root)
}

// LoadModelSource replaces the program with the contents of a .nlogo or
// .nlogox file. filename is the original name; it is not a filesystem path.
// A compile error leaves the current model in place.
func (s *Session) LoadModelSource(filename, source string) error {
	if len(source) > maxModelBytes {
		err := fmt.Errorf("model file is too large")
		slog.Warn("model load failed", "session", s.id, "model", filename, "bytes", len(source), "err", err)
		return err
	}
	if strings.TrimSpace(source) == "" {
		err := fmt.Errorf("model file is empty")
		slog.Warn("model load failed", "session", s.id, "model", filename, "bytes", len(source), "err", err)
		return err
	}
	if err := s.useModelFrom(modelDisplayName(filename), source, nil); err != nil {
		return err
	}
	s.keepUpload(filename, source)
	return nil
}

// keepUpload stores the uploaded model text in the session directory so
// Close can delete it with the rest of the session's private files.
// A disk error leaves the in-memory model in place.
func (s *Session) keepUpload(filename, source string) {
	s.mu.Lock()
	dir := s.scratch
	s.mu.Unlock()
	if dir == "" {
		return
	}
	name := filepath.Base(strings.ReplaceAll(filename, "\\", "/"))
	if name == "." || name == ".." || name == "" || name == string(filepath.Separator) {
		name = "upload.nlogo"
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		slog.Warn("session storage", "session", s.id, "err", err)
	}
}

// LoadModelFile reads one model from the server models directory
// (MODELS_DIR, default ./models). Paths that leave that directory are rejected.
func (s *Session) LoadModelFile(path string) error {
	name, content, err := readModelFile(path)
	if err != nil {
		slog.Warn("model load failed", "session", s.id, "model", path, "err", err)
		return err
	}
	dir := modelDirectory(path)
	return s.useModelFrom(name, content, &dir)
}

// SetCode recompiles the editor buffer. A failed compile keeps the
// previous world and returns the error; the buffer itself is kept so
// the editor does not jump back.
func (s *Session) SetCode(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.rejectIfActiveLocked("set code"); err != nil {
		return err
	}
	if s.model == nil {
		return fmt.Errorf("no model is loaded")
	}
	s.model.Code = code
	vm, err := s.model.NewVM(0)
	if err != nil {
		slog.Warn("model compile failed", "session", s.id, "model", s.modelName, "bytes", len(code), "err", err)
		s.broadcastLocked()
		return err
	}
	s.installLocked(s.modelName, s.model, vm)
	s.broadcastLocked()
	return nil
}

// RunCommand executes one command-center line.
func (s *Session) RunCommand(text string) (output string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.rejectIfActiveLocked("command"); err != nil {
		return "", err
	}
	if s.vm == nil {
		return "", fmt.Errorf("no model is loaded")
	}
	output, err = s.vm.ExecuteLine(text)
	if err != nil {
		s.appendLogLocked(err.Error())
		slog.Warn("command failed", "session", s.id, "err", err)
		s.broadcastLocked()
		return "", err
	}
	if output != "" {
		s.appendLogLocked(output)
	}
	s.broadcastLocked()
	return output, nil
}

// Setup runs the setup button, or the setup procedure when there is no button.
// While go is running the setup button is queued and runs between steps,
// the same as pressing it on the interface. Go keeps running afterward.
// After stop, a tick may still be unwinding; this waits for it and then
// runs setup to completion.
func (s *Session) Setup() error {
	s.mu.Lock()
	if s.running >= 0 {
		i := s.findButtonLocked("setup")
		if i >= 0 {
			if err := s.ensureRunnableLocked(i); err != nil {
				s.mu.Unlock()
				return err
			}
			s.queueOnceLocked(i)
			s.broadcastLocked()
			s.mu.Unlock()
			return nil
		}
	}
	s.mu.Unlock()
	s.waitUntilQuiet()
	return s.runNamed("setup", false)
}

// waitUntilQuiet waits out a halted step. Stop returns before that step
// leaves the interpreter. A following setup or go waits so it does not
// race the tick. A model that is still running is not waited on.
func (s *Session) waitUntilQuiet() {
	deadline := time.Now().Add(2 * time.Second)
	for {
		s.mu.Lock()
		quiet := !s.busy || s.running >= 0
		s.mu.Unlock()
		if quiet || !time.Now().Before(deadline) {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// HasProcedure reports whether the compiled model defines a procedure
// with this name. The match is case-insensitive, as NetLogo procedure
// names are. The compiled spelling is lowercase.
func (s *Session) HasProcedure(name string) bool {
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.vm != nil && s.vm.Prog != nil && s.vm.Prog.Proc(key) != nil
}

// procedureNames lists compiled procedures. Names are lowercase.
// A nil VM yields an empty list, never nil, so the GraphQL field stays non-null.
func procedureNames(vm *nl.VM) []string {
	if vm == nil || vm.Prog == nil || len(vm.Prog.Procedures) == 0 {
		return []string{}
	}
	names := make([]string, 0, len(vm.Prog.Procedures))
	for _, pr := range vm.Prog.Procedures {
		if pr == nil || pr.Name == "" {
			continue
		}
		names = append(names, pr.Name)
	}
	return names
}

// Go starts the forever button.
func (s *Session) Go() error {
	// Stop returns before the halted step leaves the interpreter. Wait
	// that out so go does not race the tick that is still unwinding.
	s.waitUntilQuiet()
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.foreverButtonLocked()
	if i < 0 {
		return fmt.Errorf("this model has no go button")
	}
	if s.running == i {
		return nil
	}
	if err := s.rejectIfActiveLocked("go"); err != nil {
		return err
	}
	if err := s.ensureRunnableLocked(i); err != nil {
		slog.Warn("go rejected", "session", s.id, "err", err)
		return err
	}
	s.cancelRun = false
	s.setRunningLocked(i, "")
	s.syncClockLocked(time.Now())
	s.broadcastLocked()
	return nil
}

// Stop halts the forever button. It does not wait for the generation.
func (s *Session) Stop() {
	s.mu.Lock()
	s.cancelRun = true
	s.setRunningLocked(-1, "stop")
	vm := s.vm
	s.syncClockLocked(time.Now())
	s.broadcastLocked()
	s.mu.Unlock()
	s.haltVM(vm, "stop")
}

// Step runs the go button a single time.
func (s *Session) Step() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.rejectIfActiveLocked("step"); err != nil {
		return err
	}
	i := s.foreverButtonLocked()
	if i < 0 {
		if s.vm != nil && s.vm.Prog != nil && s.vm.Prog.Proc("go") != nil {
			_, err := s.vm.Call("go")
			if err != nil {
				s.appendLogLocked("go: " + err.Error())
			}
			s.broadcastLocked()
			return err
		}
		return fmt.Errorf("this model has no go button")
	}
	if _, err := s.runIndexLocked(i); err != nil {
		s.broadcastLocked()
		return err
	}
	s.broadcastLocked()
	return nil
}

// PressButton runs a button once, or toggles it when it is a forever button.
func (s *Session) PressButton(index int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.model == nil || index < 0 || index >= len(s.model.Widgets) {
		return fmt.Errorf("no button at index %d", index)
	}
	w := s.model.Widgets[index]
	if w.Kind != "button" {
		return fmt.Errorf("widget %d is a %s", index, w.Kind)
	}
	if w.Forever {
		if s.running == index {
			s.cancelRun = true
			s.setRunningLocked(-1, "stop")
			vm := s.vm
			s.syncClockLocked(time.Now())
			s.broadcastLocked()
			s.mu.Unlock()
			s.haltVM(vm, "button")
			s.mu.Lock()
			return nil
		}
		if s.busy || s.running >= 0 {
			s.noteIgnoredLocked(w.Display)
			return fmt.Errorf("Stop the model before starting another forever button.")
		}
		if err := s.ensureRunnableLocked(index); err != nil {
			slog.Warn("button rejected", "session", s.id, "button", w.Display, "err", err)
			return err
		}
		s.cancelRun = false
		s.setRunningLocked(index, "")
		s.syncClockLocked(time.Now())
		s.broadcastLocked()
		return nil
	}
	// A once-button during go is queued. The step loop interrupts the
	// current iteration and runs it, then go continues. NetLogo does this
	// for setup and for every other once-button.
	if s.busy || s.running >= 0 {
		if err := s.ensureRunnableLocked(index); err != nil {
			slog.Warn("button rejected", "session", s.id, "button", w.Display, "err", err)
			return err
		}
		s.queueOnceLocked(index)
		s.broadcastLocked()
		return nil
	}
	s.cancelRun = false
	if _, err := s.runIndexLocked(index); err != nil {
		s.broadcastLocked()
		return err
	}
	if strings.EqualFold(w.Display, "setup") || strings.EqualFold(strings.TrimSpace(w.Source), "setup") {
		s.pub.Reset()
	}
	s.broadcastLocked()
	return nil
}

// SetWorld resizes the patch grid and sets patch size and wrapping.
// Turtles and links die and patches are reset, as with resize-world.
// minPz, maxPz, and wrapZ change the Z axis of a 3D world. Pass both Z
// bounds or neither. A nil wrapZ leaves Z wrapping as it is. On a 2D
// world the Z arguments are ignored. A non-nil rngSeed reseeds the world
// RNG after a successful resize, including while go is running when the
// geometry is unchanged. It is an unsigned decimal integer in
// 0..4294967295, the 32 bits setSeed uses. An invalid seed is rejected
// first, so the grid is left alone. seedOverride turns the session
// override on or off. That stored seed is applied again before setup;
// this call does not wait for setup to reseed an explicit rngSeed.
func (s *Session) SetWorld(minPx, maxPx, minPy, maxPy int, patchSize float64, wrapX, wrapY bool, minPz, maxPz *int, wrapZ *bool, rngSeed *string, seedOverride *bool) error {
	var seed *int64
	if rngSeed != nil {
		n, err := nl.ParseRNGSeed(*rngSeed)
		if err != nil {
			return err
		}
		seed = &n
	}
	s.mu.Lock()
	if s.vm == nil || s.vm.World == nil {
		s.mu.Unlock()
		return fmt.Errorf("no world")
	}
	active := s.busy || s.running >= 0
	vm := s.vm
	s.mu.Unlock()

	var same bool
	var geomErr error
	vm.DoLocked(func() {
		w := vm.World
		if w.Is3D() && (minPz != nil || maxPz != nil || wrapZ != nil) {
			if (minPz == nil) != (maxPz == nil) {
				geomErr = fmt.Errorf("min-pzcor and max-pzcor must both be set")
				return
			}
		}
		same = worldGeometrySame(w, minPx, maxPx, minPy, maxPy, patchSize, wrapX, wrapY, minPz, maxPz, wrapZ)
	})
	if geomErr != nil {
		return geomErr
	}
	// Resizing, wrapping, and patch size rebuild the grid. That stays
	// locked during a tick. A seed by itself is random-seed: it is safe
	// while go is running and does not touch the grid. When the model is
	// stopped, an unchanged grid still goes through ApplyView, which
	// resets patches the way it always has.
	if active && !same {
		s.mu.Lock()
		s.noteIgnoredLocked("set world")
		s.mu.Unlock()
		return fmt.Errorf("%s", busyMessage("set world"))
	}
	if active && same {
		// A seed-only change stays legal during go. Resizing still returns
		// the stop-the-model error above. The explicit seed is applied now,
		// the same as random-seed. The override flag is only stored; the
		// automatic reseed waits for setup.
		if seed == nil && seedOverride == nil {
			return nil
		}
		s.mu.Lock()
		capture := s.noteSeedLocked(seed, seedOverride)
		s.mu.Unlock()
		if seed != nil {
			vm.DoLocked(func() { vm.World.Seed(*seed) })
		} else if capture {
			s.captureOverrideSeed(vm)
		}
		s.mu.Lock()
		s.broadcastLocked()
		s.mu.Unlock()
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy || s.running >= 0 {
		s.noteIgnoredLocked("set world")
		return fmt.Errorf("%s", busyMessage("set world"))
	}
	if s.vm == nil || s.vm.World == nil {
		return fmt.Errorf("no world")
	}
	w := s.vm.World
	if w.Is3D() && (minPz != nil || maxPz != nil || wrapZ != nil) {
		if (minPz == nil) != (maxPz == nil) {
			return fmt.Errorf("min-pzcor and max-pzcor must both be set")
		}
		minZ, maxZ := w.MinPzcor, w.MaxPzcor
		if minPz != nil {
			minZ, maxZ = *minPz, *maxPz
		}
		zWrap := w.WrapZ
		if wrapZ != nil {
			zWrap = *wrapZ
		}
		if err := w.ApplyView3D(minPx, maxPx, minPy, maxPy, minZ, maxZ, patchSize, wrapX, wrapY, zWrap); err != nil {
			return err
		}
		s.finishSeedLocked(seed, seedOverride)
		s.broadcastLocked()
		return nil
	}
	if err := w.ApplyView(minPx, maxPx, minPy, maxPy, patchSize, wrapX, wrapY); err != nil {
		return err
	}
	s.finishSeedLocked(seed, seedOverride)
	s.broadcastLocked()
	return nil
}

// SetSeedOverride turns the session seed override on or off. The world is
// not resized, and this is allowed while go is running. Turning it on
// records the low 32 bits of the current world seed. The generator is not
// reseeded here; the next setup applies the stored seed.
func (s *Session) SetSeedOverride(on bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.vm == nil || s.vm.World == nil {
		return fmt.Errorf("no world")
	}
	turningOn := on && !s.seedOverride
	s.seedOverride = on
	if turningOn {
		s.captureOverrideSeedLocked()
	}
	s.broadcastLocked()
	return nil
}

// noteSeedLocked stores the override flag and an explicit seed. Caller
// holds s.mu. It returns true when the override just turned on and no seed
// was supplied, so the caller copies the world's current low 32 bits.
func (s *Session) noteSeedLocked(seed *int64, override *bool) bool {
	turningOn := override != nil && *override && !s.seedOverride
	if override != nil {
		s.seedOverride = *override
	}
	if seed != nil && s.seedOverride {
		s.overrideSeed = *seed
		return false
	}
	return turningOn && seed == nil
}

// finishSeedLocked records the seed preference after a successful resize
// and reseeds when an explicit seed was sent. Caller holds s.mu.
func (s *Session) finishSeedLocked(seed *int64, override *bool) {
	capture := s.noteSeedLocked(seed, override)
	s.seedWorldLocked(seed)
	if capture {
		s.captureOverrideSeedLocked()
	}
}

// captureOverrideSeed copies the live seed's low 32 bits without holding
// the session lock across the world lock.
func (s *Session) captureOverrideSeed(vm *nl.VM) {
	if vm == nil || vm.World == nil {
		return
	}
	var n int64
	vm.DoLocked(func() { n = int64(uint32(vm.World.RNGSeed())) })
	s.mu.Lock()
	s.overrideSeed = n
	s.mu.Unlock()
}

// captureOverrideSeedLocked copies the live seed's low 32 bits.
// Caller holds s.mu.
func (s *Session) captureOverrideSeedLocked() {
	if s.vm == nil || s.vm.World == nil {
		return
	}
	var n int64
	s.withWorldLocked(func() { n = int64(uint32(s.vm.World.RNGSeed())) })
	s.overrideSeed = n
}

// worldGeometrySame reports whether applying these settings would leave
// the patch grid, wrapping, and patch size as they are. Caller holds the
// world lock.
func worldGeometrySame(w *nl.World, minPx, maxPx, minPy, maxPy int, patchSize float64, wrapX, wrapY bool, minPz, maxPz *int, wrapZ *bool) bool {
	if w == nil {
		return false
	}
	if w.MinPxcor != minPx || w.MaxPxcor != maxPx || w.MinPycor != minPy || w.MaxPycor != maxPy {
		return false
	}
	if w.PatchSize != patchSize || w.WrapX != wrapX || w.WrapY != wrapY {
		return false
	}
	if !w.Is3D() {
		return true
	}
	if minPz != nil && (maxPz == nil || *minPz != w.MinPzcor || *maxPz != w.MaxPzcor) {
		return false
	}
	if wrapZ != nil && *wrapZ != w.WrapZ {
		return false
	}
	return true
}

// GenerateRNGSeed draws an unsigned 32-bit seed with crypto/rand, seeds
// the world RNG with it, and returns the decimal text. The patch grid is
// not resized.
// random-seed is legal while go is running, so this does not wait for stop.
// The session lock is not held across the world update: a running button
// yields by taking the session lock after it drops the world lock.
func (s *Session) GenerateRNGSeed() (string, error) {
	text, err := nl.GenerateRNGSeed()
	if err != nil {
		return "", err
	}
	n, err := nl.ParseRNGSeed(text)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	if s.vm == nil || s.vm.World == nil {
		s.mu.Unlock()
		return "", fmt.Errorf("no world")
	}
	vm := s.vm
	s.mu.Unlock()
	vm.DoLocked(func() { vm.World.Seed(n) })
	s.mu.Lock()
	// Random updates the stored override when that mode is on, so the next
	// setup repeats this draw. The generator was already reseeded above.
	if s.seedOverride {
		s.overrideSeed = n
	}
	s.broadcastLocked()
	s.mu.Unlock()
	return text, nil
}

// reseedForSetupLocked applies the session seed override before setup.
// Caller holds s.mu. Model code runs afterward, so a random-seed command
// inside setup still wins for that setup. With the override off, only the
// draw counter is cleared: clear-all does not reseed, and neither does setup.
func (s *Session) reseedForSetupLocked() {
	if s.vm == nil || s.vm.World == nil {
		return
	}
	if !s.seedOverride {
		s.vm.World.ResetRNGDraws()
		return
	}
	n := s.overrideSeed
	s.withWorldLocked(func() {
		s.vm.World.Seed(n)
	})
}

// applySeedView reports the override on a world snapshot. Caller holds s.mu.
func (s *Session) applySeedView(w *model.World) {
	if w == nil {
		return
	}
	w.SeedOverride = s.seedOverride
	if s.seedOverride {
		w.RngSeed = nl.FormatRNGSeed(s.overrideSeed)
	}
}

// seedWorldLocked reseeds when seed is set. Caller holds s.mu.
func (s *Session) seedWorldLocked(seed *int64) {
	if seed == nil || s.vm == nil || s.vm.World == nil {
		return
	}
	n := *seed
	s.withWorldLocked(func() {
		s.vm.World.Seed(n)
	})
}

// WorldSettings is the current size, patch size, and wrapping.
func (s *Session) WorldSettings() (*model.WorldSettings, error) {
	s.mu.Lock()
	vm := s.vm
	fps := s.frameRateCap()
	override := s.seedOverride
	overrideSeed := s.overrideSeed
	s.mu.Unlock()
	if vm == nil || vm.World == nil {
		return nil, fmt.Errorf("no world")
	}
	var out *model.WorldSettings
	vm.DoLocked(func() {
		w := vm.World
		out = &model.WorldSettings{
			FrameRateCap: fps,
			MinPxcor:     w.MinPxcor,
			MaxPxcor:     w.MaxPxcor,
			MinPycor:     w.MinPycor,
			MaxPycor:     w.MaxPycor,
			MinPzcor:     w.MinPzcor,
			MaxPzcor:     w.MaxPzcor,
			PatchSize:    w.PatchSize,
			WrapX:        w.WrapX,
			WrapY:        w.WrapY,
			WrapZ:        w.Is3D() && w.WrapZ,
			ThreeD:       w.Is3D(),
			RngSeed:      displayedRNGSeed(w.RNGSeed(), override, overrideSeed),
			SeedOverride: override,
		}
	})
	return out, nil
}

// displayedRNGSeed is the text the World tab shows. While the override is
// on, that is the stored seed, not a random-seed the model ran afterward.
func displayedRNGSeed(live int64, override bool, overrideSeed int64) string {
	if override {
		return nl.FormatRNGSeed(overrideSeed)
	}
	return nl.FormatRNGSeed(live)
}

// SetFrameRate sets the publish cap. Values outside 1..120 are clamped.
// The step loop is not blocked; the next publish interval uses the new cap.
func (s *Session) SetFrameRate(fps int) int {
	fps = nl.ClampFrameRate(fps)
	s.mu.Lock()
	s.frameRate = fps
	s.mu.Unlock()
	select {
	case s.rateNote <- struct{}{}:
	default:
	}
	return fps
}

func (s *Session) frameRateCap() int {
	if s.frameRate == 0 {
		return nl.DefaultFrameRate
	}
	return s.frameRate
}

// SetSpeed sets the forever-button speed. 0 is slowest, 100 is fastest.
func (s *Session) SetSpeed(speed float64) {
	if speed < 0 {
		speed = 0
	}
	if speed > 100 {
		speed = 100
	}
	s.mu.Lock()
	s.speed = speed
	s.broadcastLocked()
	s.mu.Unlock()
}

// SetSlider updates a slider and the interface global it owns.
func (s *Session) SetSlider(index int, value float64) error {
	return s.editWidget(index, "slider", func(w *nl.Widget) (*widgetGlobal, error) {
		if value < w.Min {
			value = w.Min
		}
		if value > w.Max {
			value = w.Max
		}
		w.Value = value
		if w.Variable == "" {
			return nil, nil
		}
		return &widgetGlobal{name: w.Variable, value: nl.Num(value)}, nil
	})
}

// SetSwitch updates a switch and its global.
func (s *Session) SetSwitch(index int, on bool) error {
	return s.editWidget(index, "switch", func(w *nl.Widget) (*widgetGlobal, error) {
		w.On = on
		if w.Variable == "" {
			return nil, nil
		}
		return &widgetGlobal{name: w.Variable, value: nl.Bool(on)}, nil
	})
}

// SetChooser selects a chooser row and copies that value into the global.
func (s *Session) SetChooser(index, current int) error {
	return s.editWidget(index, "chooser", func(w *nl.Widget) (*widgetGlobal, error) {
		if current < 0 || current >= len(w.Choices) {
			return nil, fmt.Errorf("chooser %s has no choice %d", w.Display, current)
		}
		w.Current = current
		if w.Variable == "" {
			return nil, nil
		}
		return &widgetGlobal{name: w.Variable, value: w.Choices[current]}, nil
	})
}

// SetInput updates an input box. Number and color inputs become globals of that type.
func (s *Session) SetInput(index int, text string) error {
	return s.editWidget(index, "input", func(w *nl.Widget) (*widgetGlobal, error) {
		w.Text = text
		if w.Variable == "" {
			return nil, nil
		}
		if numberInput(w.InputType) {
			n, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", w.Display, err)
			}
			w.Value = n
			return &widgetGlobal{name: w.Variable, value: nl.Num(n)}, nil
		}
		return &widgetGlobal{name: w.Variable, value: nl.Str(text)}, nil
	})
}

// editWidget changes one interface widget and the global it owns.
// Go may be running. NetLogo applies slider, switch, chooser, and input
// edits during a forever button, and the next read in that run sees the
// new value. The session lock is not held across the world update: a
// running button yields by taking the session lock after it drops the
// world lock. Buttons, commands, code edits, and model loads stay
// rejected while a run is active.
func (s *Session) editWidget(index int, kind string, fn func(*nl.Widget) (*widgetGlobal, error)) error {
	s.mu.Lock()
	if s.model == nil || s.vm == nil || index < 0 || index >= len(s.model.Widgets) {
		s.mu.Unlock()
		return fmt.Errorf("no %s at index %d", kind, index)
	}
	w := &s.model.Widgets[index]
	if w.Kind != kind {
		s.mu.Unlock()
		return fmt.Errorf("widget %d is a %s", index, w.Kind)
	}
	global, err := fn(w)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	vm := s.vm
	s.mu.Unlock()
	if global != nil && global.name != "" && vm != nil && vm.World != nil {
		vm.World.SetGlobal(global.name, global.value)
	}
	s.mu.Lock()
	s.broadcastLocked()
	s.mu.Unlock()
	return nil
}

// widgetGlobal is the interface variable a widget edit writes. The world
// lock is taken only for this write, after the session lock is released.
type widgetGlobal struct {
	name  string
	value nl.Value
}

func (s *Session) runNamed(name string, forever bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !forever {
		if err := s.rejectIfActiveLocked(name); err != nil {
			return err
		}
	}
	i := s.findButtonLocked(name)
	if i < 0 {
		if s.vm != nil && s.vm.Prog != nil && s.vm.Prog.Proc(name) != nil {
			if strings.EqualFold(name, "setup") && s.vm.World != nil {
				s.reseedForSetupLocked()
			}
			_, err := s.vm.Call(name)
			if err != nil {
				s.appendLogLocked(name + ": " + err.Error())
			}
			if strings.EqualFold(name, "setup") {
				s.pub.Reset()
			}
			s.broadcastLocked()
			return err
		}
		return fmt.Errorf("nothing named %s", name)
	}
	if forever {
		if s.running == i {
			return nil
		}
		if err := s.rejectIfActiveLocked(name); err != nil {
			return err
		}
		s.cancelRun = false
		s.setRunningLocked(i, "")
		s.broadcastLocked()
		return nil
	}
	s.cancelRun = false
	if _, err := s.runIndexLocked(i); err != nil {
		s.broadcastLocked()
		return err
	}
	if strings.EqualFold(name, "setup") {
		s.pub.Reset()
	}
	s.broadcastLocked()
	return nil
}

func (s *Session) runIndexLocked(i int) (stopped bool, err error) {
	if err = s.ensureRunnableLocked(i); err != nil {
		s.setRunningLocked(-1, "not runnable")
		warnNetLogo("button failed", err, "session", s.id, "button", s.buttonNameLocked(i))
		return false, err
	}
	if s.busy {
		return false, errModelBusy
	}
	w := s.model.Widgets[i]
	stmts := s.buttons[i]
	vm := s.vm
	if isSetupName(w.Display, w.Source) && vm != nil && vm.World != nil {
		s.reseedForSetupLocked()
	}
	s.busy = true
	s.armYield(vm)
	s.mu.Unlock()
	var halted bool
	defer func() {
		s.mu.Lock()
		if vm != nil {
			vm.ClearYield()
			if vm.Halted() {
				halted = true
				stopped = true
				vm.ClearHalt()
			}
		}
		if err != nil {
			s.appendLogLocked(w.Display + ": " + err.Error())
			warnNetLogo("button failed", err, "session", s.id, "button", w.Display)
			// A once-button error is logged and does not cancel go.
			if s.running == i {
				s.setRunningLocked(-1, "error")
				s.cancelRun = false
			}
			stopped = false
		} else if stopped {
			reason := "stop"
			if halted {
				reason = "halt"
			}
			// A halt that only makes room for a queued once-button leaves
			// go running. stop inside a once-button does not cancel a
			// different forever button.
			keep := halted && len(s.pendingOnce) > 0 && !s.cancelRun && s.running == i
			if s.running == i && !keep {
				s.setRunningLocked(-1, reason)
			}
			if !keep {
				s.cancelRun = false
			}
		}
		if vm != nil {
			s.sampleTicksLocked()
		}
		s.busy = false
	}()
	stopped, err = vm.RunCompiled(stmts)
	return
}

// queueOnceLocked remembers a once-button and asks the current step to
// stop at the next checkpoint. Caller holds s.mu. Clicks queue in order;
// a later click does not replace an earlier one that has not started.
func (s *Session) queueOnceLocked(index int) {
	s.pendingOnce = append(s.pendingOnce, index)
	if s.vm != nil {
		s.vm.RequestHalt()
	}
}

// runPendingOnceLocked runs the oldest queued once-button. Caller holds
// s.mu and the interpreter is not already inside a button. The lock is
// dropped while the button runs and held again on return. One button
// runs per call so go can continue between them; the rest stay queued.
func (s *Session) runPendingOnceLocked() {
	if len(s.pendingOnce) == 0 || s.busy || s.model == nil {
		return
	}
	idx := s.pendingOnce[0]
	s.pendingOnce = s.pendingOnce[1:]
	if len(s.pendingOnce) == 0 {
		s.pendingOnce = nil
	}
	if idx >= len(s.model.Widgets) || s.model.Widgets[idx].Kind != "button" {
		return
	}
	if s.vm != nil {
		s.vm.ClearHalt()
	}
	// The halt that opened this gap must not abort the once-button itself.
	s.cancelRun = false
	w := s.model.Widgets[idx]
	_, err := s.runIndexLocked(idx)
	if err == nil && isSetupName(w.Display, w.Source) {
		s.pub.Reset()
	}
	s.syncClockLocked(time.Now())
	s.broadcastLocked()
}

func (s *Session) ensureRunnableLocked(i int) error {
	if s.vm == nil || s.model == nil || i < 0 || i >= len(s.model.Widgets) {
		return fmt.Errorf("no button at index %d", i)
	}
	w := s.model.Widgets[i]
	if w.DisableUntilTicks && !s.vm.World.TicksStarted() {
		return fmt.Errorf("%s is disabled until ticks start", w.Display)
	}
	if _, ok := s.buttons[i]; !ok {
		return fmt.Errorf("%s did not compile", w.Display)
	}
	return nil
}

func (s *Session) findButtonLocked(name string) int {
	if s.model == nil {
		return -1
	}
	for i, w := range s.model.Widgets {
		if w.Kind != "button" {
			continue
		}
		if strings.EqualFold(w.Display, name) || strings.EqualFold(strings.TrimSpace(w.Source), name) {
			return i
		}
	}
	return -1
}

func (s *Session) foreverButtonLocked() int {
	if i := s.findButtonLocked("go"); i >= 0 && s.model.Widgets[i].Forever {
		return i
	}
	if s.model == nil {
		return -1
	}
	for i, w := range s.model.Widgets {
		if w.Kind == "button" && w.Forever {
			return i
		}
	}
	return s.findButtonLocked("go")
}

func (s *Session) appendLogLocked(line string) {
	s.logMu.Lock()
	s.appendLogBody(line)
	s.logMu.Unlock()
}

func (s *Session) appendLogBody(line string) {
	if debugEnabled() {
		msg := line
		if len(msg) > 200 {
			msg = msg[:200] + "…"
		}
		slog.Debug("command center", "session", s.id, "line", msg)
	}
	s.log = append(s.log, line)
	if len(s.log) > 80 {
		s.log = append([]string(nil), s.log[len(s.log)-80:]...)
	}
}

// broadcastLocked wakes subscribers at most once each. An unread wake is
// replaced, and a send that does not fit is counted as a slow drop. The
// step loop never waits on a subscriber.
func (s *Session) broadcastLocked() {
	dropped := 0
	for _, ch := range s.listeners {
		unread := false
		select {
		case <-ch:
			unread = true
		default:
		}
		select {
		case ch <- struct{}{}:
			if unread {
				dropped++
			}
		default:
			dropped++
		}
	}
	s.pub.NoteSlow(dropped)
	s.noteDropsLocked(0, dropped)
}

// Session builds the GraphQL session object, including monitor readings.
func (s *Session) Session() *model.Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessionLocked()
}

// Frame builds one subscription payload from a fresh subscriber.
// It omits the source code so a running model does not reset the editor
// on every tick. Output lines and plot samples are the full retained buffers.
func (s *Session) Frame() *model.Frame {
	frame, _, _, _ := s.FrameSince(-1, 0, nil)
	return frame
}

// FrameSince builds one payload whose outputAppend and plotAppend are the
// lines and samples after the subscriber's cursors. The returned cursors
// are what the caller should pass on the next frame it actually delivers.
// Caller does not hold s.mu.
func (s *Session) FrameSince(epoch, seq int, plots map[string]nl.PlotCursor) (frame *model.Frame, nextEpoch, nextSeq int, nextPlots map[string]nl.PlotCursor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap := s.sessionLocked()
	upd := outputUpdate{reset: true, lines: []string{}}
	plotViews := []model.PlotView{}
	plotAppend := []model.PlotPenDelta{}
	nextPlots = map[string]nl.PlotCursor{}
	s.withWorldLocked(func() {
		if s.vm == nil {
			if epoch == 0 && seq == 0 {
				upd.reset = false
			}
			return
		}
		reset, e, q, lines, partial := s.vm.OutputDelta(epoch, seq)
		upd = outputUpdate{reset: reset, epoch: e, seq: q, lines: lines, partial: partial}
		plotViews = plotViewsOf(s.vm.PlotViews())
		deltas, next := s.vm.PlotDeltas(plots)
		plotAppend = plotDeltasOf(deltas)
		nextPlots = next
	})
	frame = &model.Frame{
		SessionID:     snap.ID,
		Ticks:         snap.Ticks,
		TicksStarted:  snap.TicksStarted,
		Tps:           snap.Tps,
		Timer:         snap.Timer,
		FrameRateCap:  snap.FrameRateCap,
		DroppedFrames: snap.DroppedFrames,
		DroppedSlow:   snap.DroppedSlow,
		PublishedFps:  snap.PublishedFps,
		Running:       snap.Running,
		Speed:         snap.Speed,
		Log:           snap.Log,
		HasOutput:     snap.HasOutput,
		OutputAppend:  upd.lines,
		OutputPartial: upd.partial,
		OutputReset:   upd.reset,
		OutputEpoch:   upd.epoch,
		OutputSeq:     upd.seq,
		Widgets:       snap.Widgets,
		World:         snap.World,
		Plots:         plotViews,
		PlotAppend:    plotAppend,
	}
	return frame, upd.epoch, upd.seq, nextPlots
}

func plotViewsOf(in []nl.PlotView) []model.PlotView {
	out := make([]model.PlotView, len(in))
	for i, p := range in {
		pens := make([]model.PlotPenView, len(p.Pens))
		for j, pen := range p.Pens {
			pens[j] = model.PlotPenView{
				Name: pen.Name, R: pen.R, G: pen.G, B: pen.B,
				Mode: pen.Mode, Interval: pen.Interval,
			}
		}
		if pens == nil {
			pens = []model.PlotPenView{}
		}
		out[i] = model.PlotView{
			Index: p.Index, Name: p.Name, XLabel: p.XLabel, YLabel: p.YLabel,
			XMin: p.XMin, XMax: p.XMax, YMin: p.YMin, YMax: p.YMax,
			AutoPlot: p.AutoPlot, Legend: p.Legend, Error: p.Error, Pens: pens,
		}
	}
	if out == nil {
		out = []model.PlotView{}
	}
	return out
}

func plotDeltasOf(in []nl.PlotDelta) []model.PlotPenDelta {
	out := make([]model.PlotPenDelta, len(in))
	for i, d := range in {
		pts := make([]model.PlotPoint, len(d.Points))
		for j, p := range d.Points {
			pts[j] = model.PlotPoint{X: p.X, Y: p.Y, Down: p.Down}
		}
		if pts == nil {
			pts = []model.PlotPoint{}
		}
		out[i] = model.PlotPenDelta{
			PlotIndex: d.PlotIndex, PenIndex: d.PenIndex,
			Epoch: d.Epoch, Seq: d.Seq, Reset: d.Reset, Points: pts,
		}
	}
	if out == nil {
		out = []model.PlotPenDelta{}
	}
	return out
}

type outputUpdate struct {
	reset   bool
	epoch   int
	seq     int
	lines   []string
	partial string
}

// sampleTicksLocked copies the tick counter. Caller holds s.mu.
// The world lock is taken and released here; the caller must not already
// hold it. A running button has released it before this yield.
func (s *Session) sampleTicksLocked() {
	vm := s.vm
	if vm == nil || vm.World == nil {
		s.shownStarted = false
		s.shownTicks = 0
		s.shownEpoch = 0
		return
	}
	var started bool
	var ticks float64
	var epoch uint64
	vm.DoLocked(func() {
		w := vm.World
		epoch = w.RunEpoch()
		started = w.TicksStarted()
		if started {
			if n, err := w.Ticks(); err == nil {
				ticks = n
			}
		}
	})
	s.shownStarted = started
	s.shownTicks = ticks
	s.shownEpoch = epoch
}

// syncClockLocked samples the shared run clock. Caller holds s.mu.
// While a button is inside the interpreter the last yield's sample is used,
// so this does not take the world lock.
func (s *Session) syncClockLocked(now time.Time) (tps float64, timer string) {
	if !s.busy {
		s.sampleTicksLocked()
	}
	ticks := 0.0
	if s.shownStarted {
		ticks = s.shownTicks
	}
	return s.clock.Sync(now, s.running >= 0, s.shownEpoch, ticks)
}

func (s *Session) fillRunStats(out *model.Session) {
	out.FrameRateCap = s.frameRateCap()
	out.DroppedFrames = s.pub.Dropped()
	out.DroppedSlow = s.pub.DroppedSlow()
	out.PublishedFps = s.pub.PublishedFPS()
}

// withWorldLocked runs fn while the world lock is held. Caller holds s.mu
// and must not already hold the world lock. The running button drops the
// world lock before it takes s.mu, so this waits out at most one yield gap.
func (s *Session) withWorldLocked(fn func()) {
	if fn == nil {
		return
	}
	if s.vm != nil && s.vm.World != nil {
		s.vm.DoLocked(fn)
		return
	}
	fn()
}

func (s *Session) sessionLocked() *model.Session {
	out := &model.Session{
		ID:         s.id,
		ModelName:  s.modelName,
		Speed:      s.speed,
		Running:    s.running >= 0,
		Procedures: procedureNames(s.vm),
		Notes:      []string{},
		Output:     []string{},
		Widgets:    []model.Widget{},
		World: model.World{
			LabelFont: 10,
			Shapes:    []model.TurtleShape{},
			Turtles:   []model.Turtle{},
			Links:     []model.Link{},
			Drawing:   []model.DrawingMark{},
			Labels:    []model.AgentLabel{},
		},
	}
	s.logMu.Lock()
	out.Log = append([]string(nil), s.log...)
	s.logMu.Unlock()
	if out.Log == nil {
		out.Log = []string{}
	}
	if s.model != nil {
		out.Code = s.model.Code
		out.Info = s.model.Info
		out.Dynamics = dynamicsView(s.model.Dynamics)
		out.Notes = append([]string(nil), s.model.Notes...)
		if out.Notes == nil {
			out.Notes = []string{}
		}
	}
	if s.modelDir != nil {
		dir := *s.modelDir
		out.ModelDir = &dir
	}
	s.fillRunStats(out)
	var started bool
	var ticks float64
	var epoch uint64
	s.withWorldLocked(func() {
		if s.vm == nil || s.vm.World == nil {
			return
		}
		lines, partial := s.vm.OutputArea()
		out.HasOutput = s.vm.HasOutput()
		out.Output = lines
		out.OutputPartial = partial
		w := s.vm.World
		epoch = w.RunEpoch()
		started = w.TicksStarted()
		out.TicksStarted = started
		if started {
			if t, err := w.Ticks(); err == nil {
				ticks = t
				out.Ticks = &t
			}
		}
		out.Widgets = s.widgetsLocked()
		out.World = worldLocked(w)
		s.applySeedView(&out.World)
	})
	s.shownStarted = started
	s.shownTicks = ticks
	s.shownEpoch = epoch
	tickArg := 0.0
	if started {
		tickArg = ticks
	}
	out.Tps, out.Timer = s.clock.Sync(time.Now(), s.running >= 0, epoch, tickArg)
	s.fillRunStats(out)
	return out
}

func (s *Session) widgetsLocked() []model.Widget {
	if s.model == nil {
		return []model.Widget{}
	}
	started := s.vm != nil && s.vm.World != nil && s.vm.World.TicksStarted()
	out := make([]model.Widget, 0, len(s.model.Widgets))
	for i, w := range s.model.Widgets {
		item := model.Widget{
			Index:    i,
			Kind:     w.Kind,
			Display:  w.Display,
			Forever:  w.Forever,
			X:        w.X,
			Y:        w.Y,
			Width:    w.W,
			Height:   w.H,
			FontSize: w.FontSize,
		}
		if w.Variable != "" {
			v := w.Variable
			item.Variable = &v
		}
		if w.Kind == "button" && w.DisableUntilTicks && !started {
			item.Disabled = true
		}
		switch w.Kind {
		case "slider":
			min, max, value, step := w.Min, w.Max, w.Value, w.Step
			item.Min, item.Max, item.Value, item.Step = &min, &max, &value, &step
			if w.Units != "" {
				u := w.Units
				item.Units = &u
			}
		case "switch":
			on := w.On
			item.On = &on
		case "chooser":
			choices := make([]string, len(w.Choices))
			for j, c := range w.Choices {
				choices[j] = c.Format(false)
			}
			item.Choices = choices
			cur := w.Current
			item.Current = &cur
		case "input":
			text := w.Text
			if text == "" && numberInput(w.InputType) {
				text = strconv.FormatFloat(w.Value, 'f', -1, 64)
			}
			item.Text = &text
		case "monitor":
			reading := "—"
			if s.vm != nil && strings.TrimSpace(w.Source) != "" {
				got, err := s.vm.LineUnlocked(w.Source)
				if err != nil {
					reading = "N/A"
				} else if got == "" {
					reading = "—"
				} else {
					reading = got
				}
			}
			item.Reading = &reading
		}
		out = append(out, item)
	}
	return out
}

// setPerspectiveSubject publishes who for a turtle and the world point
// for a turtle, patch, or link. follow and ride are turtles only.
// watch can sit on any of the three.
func setPerspectiveSubject(out *model.World, agent nl.Agent) {
	if agent == nil || agent.IsDead() {
		return
	}
	switch a := agent.(type) {
	case *nl.Turtle:
		who := a.Who
		out.Subject = &who
		x, y, z := a.X, a.Y, a.Z
		out.SubjectX, out.SubjectY, out.SubjectZ = &x, &y, &z
	case *nl.Patch:
		x, y, z := float64(a.PX), float64(a.PY), float64(a.PZ)
		out.SubjectX, out.SubjectY, out.SubjectZ = &x, &y, &z
	case *nl.Link:
		if a.End1 == nil || a.End2 == nil || a.End1.IsDead() || a.End2.IsDead() {
			return
		}
		x := (a.End1.X + a.End2.X) / 2
		y := (a.End1.Y + a.End2.Y) / 2
		z := (a.End1.Z + a.End2.Z) / 2
		out.SubjectX, out.SubjectY, out.SubjectZ = &x, &y, &z
	}
}

func labelFontOf(w *nl.World) int {
	if w == nil || w.LabelFont <= 0 {
		return 10
	}
	return w.LabelFont
}

// shapeViews copies the model's turtle shapes for the view. The list is
// static for a loaded model. An empty slice stays non-null for GraphQL.
func shapeViews(w *nl.World) []model.TurtleShape {
	out := []model.TurtleShape{}
	if w == nil || w.Shapes == nil {
		return out
	}
	for _, sh := range w.Shapes.All() {
		if sh == nil {
			continue
		}
		els := make([]model.ShapeElement, 0, len(sh.Elements))
		for _, el := range sh.Elements {
			pts := el.Points
			if pts == nil {
				pts = []float64{}
			}
			els = append(els, model.ShapeElement{
				Kind: el.Kind, Filled: el.Filled, Marked: el.Marked,
				R: int(el.R), G: int(el.G), B: int(el.B),
				Points: pts, X: el.X, Y: el.Y, W: el.W, H: el.H,
			})
		}
		out = append(out, model.TurtleShape{
			Name: sh.Name, Rotatable: sh.Rotatable, Elements: els,
		})
	}
	return out
}

func worldLocked(w *nl.World) model.World {
	out := model.World{
		MinPxcor:    w.MinPxcor,
		MaxPxcor:    w.MaxPxcor,
		MinPycor:    w.MinPycor,
		MaxPycor:    w.MaxPycor,
		Width:       w.Width(),
		Height:      w.Height(),
		Depth:       w.Depth(),
		MinPzcor:    w.MinPzcor,
		MaxPzcor:    w.MaxPzcor,
		PatchSize:   w.PatchSize,
		WrapX:       w.WrapX,
		WrapY:       w.WrapY,
		WrapZ:       w.Is3D() && w.WrapZ,
		ThreeD:      w.Is3D(),
		RngSeed:     nl.FormatRNGSeed(w.RNGSeed()),
		LabelFont:   labelFontOf(w),
		Shapes:      shapeViews(w),
		PatchColors: patchColors(w),
		Voxels:      voxels(w),
		Turtles:     []model.Turtle{},
		Links:       []model.Link{},
		Drawing:     drawingMarks(w),
		Labels:      viewLabels(w),
	}
	if w.Is3D() {
		o := w.Observer()
		out.Observer = &model.Observer3d{
			X: o.X, Y: o.Y, Z: o.Z,
			TargetX: o.TX, TargetY: o.TY, TargetZ: o.TZ,
			Revision: int(o.Revision),
		}
	}
	mode, agent := w.Perspective()
	out.Perspective = mode
	setPerspectiveSubject(&out, agent)
	if turtles := w.Turtles(); turtles != nil {
		alive := turtles.Alive()
		if w.Is3D() {
			// Top-down view of a 3D world: paint low turtles first so the
			// highest one on a column ends up on top.
			alive = append([]nl.Agent(nil), alive...)
			sort.SliceStable(alive, func(i, j int) bool {
				return alive[i].(*nl.Turtle).Z < alive[j].(*nl.Turtle).Z
			})
		}
		for _, a := range alive {
			t, ok := a.(*nl.Turtle)
			if !ok {
				continue
			}
			r, g, b := colorOf(false, color.RGBA{}, t.Color)
			shape := t.Shape
			if shape == "" {
				shape = "default"
			}
			size := t.Size
			if size == 0 {
				size = 1
			}
			out.Turtles = append(out.Turtles, model.Turtle{
				Who: t.Who, X: t.X, Y: t.Y, Z: t.Z, Heading: t.Heading,
				Pitch: t.Pitch, Roll: t.Roll,
				Size: size, Shape: shape, Hidden: t.Hidden, R: r, G: g, B: b,
			})
		}
	}
	if links := w.Links(); links != nil {
		for _, a := range links.Alive() {
			l, ok := a.(*nl.Link)
			if !ok || l.End1 == nil || l.End2 == nil || l.End1.IsDead() || l.End2.IsDead() {
				continue
			}
			r, g, b := colorOf(false, color.RGBA{}, l.Color)
			out.Links = append(out.Links, model.Link{
				X1: l.End1.X, Y1: l.End1.Y, Z1: l.End1.Z,
				X2: l.End2.X, Y2: l.End2.Y, Z2: l.End2.Z,
				Hidden: l.Hidden, R: r, G: g, B: b,
			})
		}
	}
	return out
}

// voxelBytes is the size of one packed voxel: three int16 coordinates and
// an RGBA color. Alpha is the color's own alpha; the 3D view turns a
// fully opaque NetLogo color into a translucent cube.
const voxelBytes = 10

// voxels packs the non-black patches of a 3D world for the 3D view:
// pxcor, pycor, pzcor as little-endian int16, then r, g, b, a. A world is
// at most nl.MaxWorldSide patches on a side, so coordinates fit in int16.
// A 2D world sends an empty string; its patches are in patchColors.
// Alpha 0 is omitted: that patch would not be drawn.
func voxels(w *nl.World) string {
	if !w.Is3D() {
		return ""
	}
	var raw []byte
	var rec [voxelBytes]byte
	w.EachVoxel(func(v nl.Voxel) {
		if v.C.A == 0 {
			return
		}
		if raw == nil {
			// Most colored 3D worlds color a good share of their patches;
			// start at a quarter of the grid to skip early regrowth.
			raw = make([]byte, 0, len(w.Patches().Agents)/4*voxelBytes+voxelBytes)
		}
		binary.LittleEndian.PutUint16(rec[0:], uint16(int16(v.X)))
		binary.LittleEndian.PutUint16(rec[2:], uint16(int16(v.Y)))
		binary.LittleEndian.PutUint16(rec[4:], uint16(int16(v.Z)))
		rec[6], rec[7], rec[8], rec[9] = v.C.R, v.C.G, v.C.B, v.C.A
		raw = append(raw, rec[:]...)
	})
	if len(raw) == 0 {
		return ""
	}
	return base64.StdEncoding.EncodeToString(raw)
}

// patchColors packs one RGB pixel per patch. Row 0 is maxPycor so the
// bytes can be drawn as an image without flipping. A 3D world is
// projected top-down: each column shows its highest non-black patch.
func patchColors(w *nl.World) string {
	width, height := w.Width(), w.Height()
	if width < 1 || height < 1 {
		return ""
	}
	raw := make([]byte, width*height*3)
	i := 0
	if w.Is3D() {
		for py := w.MaxPycor; py >= w.MinPycor; py-- {
			for px := w.MinPxcor; px <= w.MaxPxcor; px++ {
				c, _ := w.TopColor(px, py)
				raw[i], raw[i+1], raw[i+2] = c.R, c.G, c.B
				i += 3
			}
		}
		return base64.StdEncoding.EncodeToString(raw)
	}
	for py := w.MaxPycor; py >= w.MinPycor; py-- {
		for px := w.MinPxcor; px <= w.MaxPxcor; px++ {
			p := w.GetPatch(px, py)
			if p == nil {
				i += 3
				continue
			}
			r, g, b := colorOf(p.HasRGB, p.RGB, p.PColor)
			raw[i] = byte(r)
			raw[i+1] = byte(g)
			raw[i+2] = byte(b)
			i += 3
		}
	}
	return base64.StdEncoding.EncodeToString(raw)
}

func drawingMarks(w *nl.World) []model.DrawingMark {
	marks := w.Marks()
	out := make([]model.DrawingMark, 0, len(marks))
	for _, m := range marks {
		c := nl.ColorRGBA(m.Color)
		out = append(out, model.DrawingMark{
			Pen: m.Pen, Erase: m.Erase,
			X: m.X, Y: m.Y, Z: m.Z, X2: m.X2, Y2: m.Y2, Z2: m.Z2,
			Heading: m.Heading, Pitch: m.Pitch, Roll: m.Roll,
			Size: m.Size, Shape: m.Shape,
			R: int(c.R), G: int(c.G), B: int(c.B),
		})
	}
	return out
}

func viewLabels(w *nl.World) []model.AgentLabel {
	labels := w.ViewLabels()
	out := make([]model.AgentLabel, 0, len(labels))
	for _, lb := range labels {
		item := model.AgentLabel{
			Text: lb.Text,
			Z:    lb.Z,
			R:    int(lb.R), G: int(lb.G), B: int(lb.B),
		}
		switch {
		case lb.Turtle:
			who := lb.Who
			item.Who = &who
		case lb.Link:
			x, y := lb.X, lb.Y
			item.Lx = &x
			item.Ly = &y
		default:
			px, py := lb.PX, lb.PY
			item.Px = &px
			item.Py = &py
		}
		out = append(out, item)
	}
	return out
}

func colorOf(hasRGB bool, rgb color.RGBA, colorNum float64) (int, int, int) {
	if !hasRGB {
		rgb = nl.ColorRGBA(colorNum)
	}
	return int(rgb.R), int(rgb.G), int(rgb.B)
}

func numberInput(typ string) bool {
	switch strings.ToLower(typ) {
	case "number", "color":
		return true
	default:
		return false
	}
}

func readModel(name string) (string, string, error) {
	name = strings.TrimSpace(name)
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, "..") {
		return "", "", fmt.Errorf("unknown model %q", name)
	}
	if !strings.Contains(name, ".") {
		name += ".nlogo"
	}
	b, err := models.FS.ReadFile(name)
	if err != nil {
		return "", "", fmt.Errorf("unknown model %s", name)
	}
	return name, string(b), nil
}

func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("s%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// stepsFor matches the desktop speed slider for speeds below 100.
// At 50 and above, several model steps run per frame.
// Below 50, frames are skipped so the model slows down.
// Speed 100 is not used as a step count: the slot runner treats it as
// warp and fills the 30ms slot instead.
func stepsFor(speed float64, frame int) int {
	if speed >= 50 {
		return 1 + int((speed-50)/10)
	}
	gap := 1 + int((50-speed)/5)
	if gap < 1 {
		gap = 1
	}
	if frame%gap == 0 {
		return 1
	}
	return 0
}
