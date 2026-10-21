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
	"encoding/hex"
	"fmt"
	"image/color"
	"io/fs"
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
type Store struct {
	mu       sync.Mutex
	sessions map[string]*Session
}

// NewStore starts an empty store.
func NewStore() *Store {
	return &Store{sessions: map[string]*Session{}}
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

// Create loads a model into a new isolated session.
func (st *Store) Create(modelName string) (*Session, error) {
	name, content, err := readModel(modelName)
	if err != nil {
		return nil, err
	}
	s := newSession()
	if err := s.useModel(name, content); err != nil {
		s.Close()
		return nil, err
	}
	go s.loop()
	st.mu.Lock()
	st.sessions[s.id] = s
	st.mu.Unlock()
	return s, nil
}

// Get returns a session by id.
func (st *Store) Get(id string) (*Session, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	s, ok := st.sessions[id]
	return s, ok
}

// Close stops every session loop.
func (st *Store) Close() {
	st.mu.Lock()
	all := make([]*Session, 0, len(st.sessions))
	for _, s := range st.sessions {
		all = append(all, s)
	}
	st.mu.Unlock()
	for _, s := range all {
		s.Close()
	}
}

// Session is one interpreter, one world, and the widgets that drive it.
type Session struct {
	mu        sync.Mutex
	id        string
	modelName string
	model     *nl.FileModel
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
	rateNote  chan struct{}
	stop      chan struct{}
	closed    bool
}

func newSession() *Session {
	return &Session{
		id:        newID(),
		running:   -1,
		speed:     40,
		frameRate: nl.DefaultFrameRate,
		buttons:   map[int][]nl.Stmt{},
		listeners: map[int]chan struct{}{},
		rateNote:  make(chan struct{}, 1),
		stop:      make(chan struct{}),
	}
}

func (s *Session) ID() string { return s.id }

// Close stops the forever loop. It is safe to call more than once.
func (s *Session) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	close(s.stop)
}

func (s *Session) loop() {
	// Steps stay on the speed-slider cadence. A second goroutine publishes
	// on the frame-rate interval so it can take the latest snapshot between
	// steps without the step loop waiting on a subscriber.
	go s.publishLoop()
	step := time.NewTicker(30 * time.Millisecond)
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
func (s *Session) runStepSlot() {
	s.mu.Lock()
	if s.closed || s.model == nil || s.running < 0 || s.running >= len(s.model.Widgets) {
		s.mu.Unlock()
		return
	}
	s.frame++
	n := stepsFor(s.speed, s.frame)
	s.mu.Unlock()
	for i := 0; i < n; i++ {
		s.mu.Lock()
		if s.closed || s.model == nil || s.running < 0 || s.running >= len(s.model.Widgets) {
			s.syncClockLocked(time.Now())
			s.mu.Unlock()
			return
		}
		_, err := s.runIndexLocked(s.running)
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
	if s.model == nil || s.running < 0 || s.running >= len(s.model.Widgets) {
		return
	}
	s.frame++
	n := stepsFor(s.speed, s.frame)
	done := 0
	for i := 0; i < n; i++ {
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
	if !s.pub.Publish(now, nl.FrameInterval(s.frameRate)) {
		return
	}
	s.broadcastLocked()
}

// Subscribe registers a wake-up channel. The first value is not sent
// here; the caller snapshots once, then waits. The channel is removed
// when ctx ends. A slow subscriber misses intermediate frames and
// still observes the latest state on the next wake-up.
func (s *Session) Subscribe() (wake <-chan struct{}, cancel func()) {
	s.mu.Lock()
	ch := make(chan struct{}, 1)
	id := s.nextSub
	s.nextSub++
	s.listeners[id] = ch
	s.mu.Unlock()
	return ch, func() {
		s.mu.Lock()
		delete(s.listeners, id)
		s.mu.Unlock()
	}
}

func (s *Session) useModel(name, content string) error {
	m, err := nl.ParseModel(name, content)
	if err != nil {
		return err
	}
	vm, err := m.NewVM(0)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.log = nil
	s.installLocked(name, m, vm)
	s.broadcastLocked()
	return nil
}

func (s *Session) installLocked(name string, m *nl.FileModel, vm *nl.VM) {
	s.modelName = name
	s.model = m
	s.vm = vm
	s.running = -1
	s.frame = 0
	s.clock = nl.RunClock{}
	s.pub.Reset()
	vm.OnPrint = func(line string) {
		// Called while s.mu is held by the command that is running.
		s.appendLogLocked(line)
	}
	vm.OnClear = func() {
		s.log = nil
	}
	s.compileButtonsLocked()
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
			continue
		}
		s.buttons[i] = stmts
	}
}

// LoadModel replaces the program with an embedded demo and rebuilds the world.
func (s *Session) LoadModel(modelName string) error {
	name, content, err := readModel(modelName)
	if err != nil {
		return err
	}
	return s.useModel(name, content)
}

// LoadModelSource replaces the program with the contents of a .nlogo or
// .nlogox file. filename is the original name; it is not a filesystem path.
// A compile error leaves the current model in place.
func (s *Session) LoadModelSource(filename, source string) error {
	if len(source) > maxModelBytes {
		return fmt.Errorf("model file is too large")
	}
	if strings.TrimSpace(source) == "" {
		return fmt.Errorf("model file is empty")
	}
	return s.useModel(modelDisplayName(filename), source)
}

// LoadModelFile reads one model from the server models directory
// (MODELS_DIR, default ./models). Paths that leave that directory are rejected.
func (s *Session) LoadModelFile(path string) error {
	name, content, err := readModelFile(path)
	if err != nil {
		return err
	}
	return s.useModel(name, content)
}

// SetCode recompiles the editor buffer. A failed compile keeps the
// previous world and returns the error; the buffer itself is kept so
// the editor does not jump back.
func (s *Session) SetCode(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.model == nil {
		return fmt.Errorf("no model is loaded")
	}
	s.model.Code = code
	vm, err := s.model.NewVM(0)
	if err != nil {
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
	if s.vm == nil {
		return "", fmt.Errorf("no model is loaded")
	}
	output, err = s.vm.ExecuteLine(text)
	if err != nil {
		s.appendLogLocked(err.Error())
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
func (s *Session) Setup() error {
	return s.runNamed("setup", false)
}

// Go starts the forever button.
func (s *Session) Go() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.foreverButtonLocked()
	if i < 0 {
		return fmt.Errorf("this model has no go button")
	}
	if err := s.ensureRunnableLocked(i); err != nil {
		return err
	}
	s.running = i
	s.syncClockLocked(time.Now())
	s.broadcastLocked()
	return nil
}

// Stop halts the forever button.
func (s *Session) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = -1
	s.syncClockLocked(time.Now())
	s.broadcastLocked()
}

// Step runs the go button a single time.
func (s *Session) Step() error {
	s.mu.Lock()
	defer s.mu.Unlock()
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
			s.running = -1
		} else {
			if err := s.ensureRunnableLocked(index); err != nil {
				return err
			}
			s.running = index
		}
		s.syncClockLocked(time.Now())
		s.broadcastLocked()
		return nil
	}
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
func (s *Session) SetWorld(minPx, maxPx, minPy, maxPy int, patchSize float64, wrapX, wrapY bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.vm == nil || s.vm.World == nil {
		return fmt.Errorf("no world")
	}
	if err := s.vm.World.ApplyView(minPx, maxPx, minPy, maxPy, patchSize, wrapX, wrapY); err != nil {
		return err
	}
	s.broadcastLocked()
	return nil
}

// WorldSettings is the current size, patch size, and wrapping.
func (s *Session) WorldSettings() (*model.WorldSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.vm == nil || s.vm.World == nil {
		return nil, fmt.Errorf("no world")
	}
	w := s.vm.World
	return &model.WorldSettings{
		FrameRateCap: s.frameRateCap(),
		MinPxcor:     w.MinPxcor,
		MaxPxcor:     w.MaxPxcor,
		MinPycor:     w.MinPycor,
		MaxPycor:     w.MaxPycor,
		PatchSize:    w.PatchSize,
		WrapX:        w.WrapX,
		WrapY:        w.WrapY,
	}, nil
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
	return s.editWidget(index, "slider", func(w *nl.Widget) error {
		if value < w.Min {
			value = w.Min
		}
		if value > w.Max {
			value = w.Max
		}
		w.Value = value
		if w.Variable != "" {
			s.vm.World.SetGlobal(w.Variable, nl.Num(value))
		}
		return nil
	})
}

// SetSwitch updates a switch and its global.
func (s *Session) SetSwitch(index int, on bool) error {
	return s.editWidget(index, "switch", func(w *nl.Widget) error {
		w.On = on
		if w.Variable != "" {
			s.vm.World.SetGlobal(w.Variable, nl.Bool(on))
		}
		return nil
	})
}

// SetChooser selects a chooser row and copies that value into the global.
func (s *Session) SetChooser(index, current int) error {
	return s.editWidget(index, "chooser", func(w *nl.Widget) error {
		if current < 0 || current >= len(w.Choices) {
			return fmt.Errorf("chooser %s has no choice %d", w.Display, current)
		}
		w.Current = current
		if w.Variable != "" {
			s.vm.World.SetGlobal(w.Variable, w.Choices[current])
		}
		return nil
	})
}

// SetInput updates an input box. Number and color inputs become globals of that type.
func (s *Session) SetInput(index int, text string) error {
	return s.editWidget(index, "input", func(w *nl.Widget) error {
		w.Text = text
		if w.Variable == "" {
			return nil
		}
		if numberInput(w.InputType) {
			n, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
			if err != nil {
				return fmt.Errorf("%s: %w", w.Display, err)
			}
			w.Value = n
			s.vm.World.SetGlobal(w.Variable, nl.Num(n))
			return nil
		}
		s.vm.World.SetGlobal(w.Variable, nl.Str(text))
		return nil
	})
}

func (s *Session) editWidget(index int, kind string, fn func(*nl.Widget) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.model == nil || s.vm == nil || index < 0 || index >= len(s.model.Widgets) {
		return fmt.Errorf("no %s at index %d", kind, index)
	}
	w := &s.model.Widgets[index]
	if w.Kind != kind {
		return fmt.Errorf("widget %d is a %s", index, w.Kind)
	}
	if err := fn(w); err != nil {
		return err
	}
	s.broadcastLocked()
	return nil
}

func (s *Session) runNamed(name string, forever bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.findButtonLocked(name)
	if i < 0 {
		if s.vm != nil && s.vm.Prog != nil && s.vm.Prog.Proc(name) != nil {
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
		s.running = i
		s.broadcastLocked()
		return nil
	}
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

func (s *Session) runIndexLocked(i int) (bool, error) {
	if err := s.ensureRunnableLocked(i); err != nil {
		s.running = -1
		return false, err
	}
	w := s.model.Widgets[i]
	stopped, err := s.vm.RunCompiled(s.buttons[i])
	if err != nil {
		s.appendLogLocked(w.Display + ": " + err.Error())
		s.running = -1
		return false, err
	}
	if stopped {
		s.running = -1
	}
	return stopped, nil
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
	if s.vm != nil {
		reset, e, q, lines, partial := s.vm.OutputDelta(epoch, seq)
		upd = outputUpdate{reset: reset, epoch: e, seq: q, lines: lines, partial: partial}
	} else if epoch == 0 && seq == 0 {
		upd.reset = false
	}
	if upd.lines == nil {
		upd.lines = []string{}
	}
	plotViews := []model.PlotView{}
	plotAppend := []model.PlotPenDelta{}
	nextPlots = map[string]nl.PlotCursor{}
	if s.vm != nil {
		plotViews = plotViewsOf(s.vm.PlotViews())
		deltas, next := s.vm.PlotDeltas(plots)
		plotAppend = plotDeltasOf(deltas)
		nextPlots = next
	}
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

// syncClockLocked samples the shared run clock. Caller holds s.mu.
func (s *Session) syncClockLocked(now time.Time) (tps float64, timer string) {
	var ticks float64
	var epoch uint64
	if s.vm != nil && s.vm.World != nil {
		w := s.vm.World
		epoch = w.RunEpoch()
		if w.TicksStarted() {
			if n, err := w.Ticks(); err == nil {
				ticks = n
			}
		}
	}
	return s.clock.Sync(now, s.running >= 0, epoch, ticks)
}

func (s *Session) fillRunStats(out *model.Session) {
	out.FrameRateCap = s.frameRateCap()
	out.DroppedFrames = s.pub.Dropped()
	out.DroppedSlow = s.pub.DroppedSlow()
	out.PublishedFps = s.pub.PublishedFPS()
}

func (s *Session) sessionLocked() *model.Session {
	out := &model.Session{
		ID:        s.id,
		ModelName: s.modelName,
		Speed:     s.speed,
		Running:   s.running >= 0,
		Log:       append([]string(nil), s.log...),
		Notes:     []string{},
		Output:    []string{},
		Widgets:   []model.Widget{},
		World: model.World{
			Turtles: []model.Turtle{},
			Links:   []model.Link{},
			Drawing: []model.DrawingMark{},
			Labels:  []model.AgentLabel{},
		},
	}
	if s.vm != nil {
		lines, partial := s.vm.OutputArea()
		out.HasOutput = s.vm.HasOutput()
		out.Output = lines
		out.OutputPartial = partial
	}
	if s.model != nil {
		out.Code = s.model.Code
		out.Notes = append([]string(nil), s.model.Notes...)
		if out.Notes == nil {
			out.Notes = []string{}
		}
	}
	s.fillRunStats(out)
	if s.vm == nil || s.vm.World == nil {
		out.Tps, out.Timer = s.syncClockLocked(time.Now())
		return out
	}
	w := s.vm.World
	out.TicksStarted = w.TicksStarted()
	if w.TicksStarted() {
		if t, err := w.Ticks(); err == nil {
			out.Ticks = &t
		}
	}
	out.Tps, out.Timer = s.syncClockLocked(time.Now())
	s.fillRunStats(out)
	out.Widgets = s.widgetsLocked()
	out.World = worldLocked(w)
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
				got, err := s.vm.ExecuteLine(w.Source)
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

func worldLocked(w *nl.World) model.World {
	out := model.World{
		MinPxcor:    w.MinPxcor,
		MaxPxcor:    w.MaxPxcor,
		MinPycor:    w.MinPycor,
		MaxPycor:    w.MaxPycor,
		Width:       w.Width(),
		Height:      w.Height(),
		PatchSize:   w.PatchSize,
		WrapX:       w.WrapX,
		WrapY:       w.WrapY,
		PatchColors: patchColors(w),
		Turtles:     []model.Turtle{},
		Links:       []model.Link{},
		Drawing:     drawingMarks(w),
		Labels:      viewLabels(w),
	}
	if turtles := w.Turtles(); turtles != nil {
		for _, a := range turtles.Alive() {
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
				Who: t.Who, X: t.X, Y: t.Y, Heading: t.Heading,
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
				X1: l.End1.X, Y1: l.End1.Y, X2: l.End2.X, Y2: l.End2.Y,
				Hidden: l.Hidden, R: r, G: g, B: b,
			})
		}
	}
	return out
}

// patchColors packs one RGB pixel per patch. Row 0 is maxPycor so the
// bytes can be drawn as an image without flipping.
func patchColors(w *nl.World) string {
	width, height := w.Width(), w.Height()
	if width < 1 || height < 1 {
		return ""
	}
	raw := make([]byte, width*height*3)
	i := 0
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
			X: m.X, Y: m.Y, X2: m.X2, Y2: m.Y2,
			Heading: m.Heading, Size: m.Size, Shape: m.Shape,
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

// stepsFor matches the desktop speed slider.
// At 50 and above, several model steps run per frame.
// Below 50, frames are skipped so the model slows down.
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
