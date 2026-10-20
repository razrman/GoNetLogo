// Package gui is the desktop shell: a world view, an interface tab,
// a code tab, and a command center.
package gui

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"io"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"gonetlogo/pkg/nl"
)

// Run opens the desktop app. modelsFS provides the built-in .nlogo demos.
func Run(modelsFS fs.FS) {
	a := app.New()
	w := a.NewWindow("GoNetLogo")
	s := newSession(modelsFS, w)
	w.SetContent(s.layout)
	w.Resize(fyne.NewSize(1120, 760))
	s.anim.Start()
	w.ShowAndRun()
}

type control struct {
	widget  nl.Widget
	slider  *widget.Slider
	check   *widget.Check
	choice  *widget.Select
	entry   *widget.Entry
	button  *widget.Button
	readout *widget.Label
	stmts   []nl.Stmt
}

// outputView is the model's output area: a scrolling monospace log.
type outputView struct {
	text     *canvas.Text
	scroll   *container.Scroll
	box      fyne.CanvasObject
	fontSize float32
}

type session struct {
	win       fyne.Window
	modelsFS  fs.FS
	names     []string
	model     *nl.FileModel
	vm        *nl.VM
	code      *widget.Entry
	output    *widget.Entry
	ticks     *widget.Label
	tps       *widget.Label
	dropped   *widget.Label
	timer     *widget.Label
	clock     nl.RunClock
	pub       nl.FramePublisher
	frameRate int
	maxFPS    *widget.Entry
	notes     *widget.Label
	raster    *canvas.Raster
	side      *fyne.Container
	scroll    *container.Scroll
	split     *container.Split
	layout    fyne.CanvasObject
	minPx     *widget.Entry
	maxPx     *widget.Entry
	minPy     *widget.Entry
	maxPy     *widget.Entry
	patchSize *widget.Entry
	wrapX     *widget.Check
	wrapY     *widget.Check
	seenWorld string
	picker    *widget.Select
	speed     *widget.Slider
	setupBtn  *widget.Button
	goBtn     *widget.Button

	ctls    []control
	areas   []*outputView
	running int
	frame   int
	anim    *fyne.Animation
	busy    bool

	// mu keeps the go loop, command center, monitors, world Apply, and
	// model loads off the same world. The draw thread does not take it;
	// it paints view, the last immutable frame.
	mu    sync.Mutex
	view  atomic.Pointer[nl.ViewSnap]
	outMu sync.Mutex
}

func newSession(modelsFS fs.FS, win fyne.Window) *session {
	s := &session{win: win, modelsFS: modelsFS, running: -1, frameRate: nl.DefaultFrameRate}
	s.names = demoNames(modelsFS)
	s.ticks = widget.NewLabel("ticks")
	s.tps = widget.NewLabel("tps —")
	s.dropped = widget.NewLabel("drop 0")
	s.timer = widget.NewLabel("00:00.0")
	s.tps.TextStyle = fyne.TextStyle{Monospace: true}
	s.dropped.TextStyle = fyne.TextStyle{Monospace: true}
	s.timer.TextStyle = fyne.TextStyle{Monospace: true}
	s.timer.Alignment = fyne.TextAlignCenter
	s.maxFPS = widget.NewEntry()
	s.maxFPS.SetText(strconv.Itoa(nl.DefaultFrameRate))
	s.maxFPS.OnChanged = s.setMaxFPS
	s.notes = widget.NewLabel("")
	s.output = widget.NewMultiLineEntry()
	s.output.Disable()
	s.output.Wrapping = fyne.TextWrapWord
	s.code = widget.NewMultiLineEntry()
	s.code.TextStyle = fyne.TextStyle{Monospace: true}
	s.code.Wrapping = fyne.TextWrapOff
	s.speed = widget.NewSlider(0, 100)
	s.speed.Value = 40
	s.speed.Step = 1
	s.raster = canvas.NewRaster(s.drawWorld)
	s.raster.SetMinSize(fyne.NewSize(520, 520))
	s.side = container.NewVBox()
	s.scroll = container.NewVScroll(s.side)
	s.scroll.SetMinSize(fyne.NewSize(240, 160))
	worldForm := s.buildWorldForm()

	s.picker = widget.NewSelect(s.names, func(name string) {
		if s.busy || name == "" {
			return
		}
		b, err := fs.ReadFile(s.modelsFS, name)
		if err != nil {
			s.logf("%v", err)
			return
		}
		s.load(name, string(b))
	})
	s.setupBtn = widget.NewButton("setup", func() { s.pressNamed("setup") })
	s.setupBtn.Importance = widget.HighImportance
	s.goBtn = widget.NewButton("go", func() { s.pressNamed("go") })
	s.goBtn.Importance = widget.DangerImportance

	open := widget.NewButton("Open", func() { s.openFile() })
	apply := widget.NewButton("Apply code", func() { s.applyCode() })

	cmd := widget.NewEntry()
	cmd.SetPlaceHolder("observer command or reporter")
	cmd.OnSubmitted = func(text string) {
		s.runLine(text)
		cmd.SetText("")
	}

	// Timer sits in the middle of the bar. Ticks stay immediately right of
	// the speed slider; tps stays with that cluster. The web header centers
	// the timer on the full bar; Fyne's border layout centers it in the
	// slot between the left controls and this cluster.
	toolbar := container.NewBorder(nil, nil,
		container.NewHBox(s.picker, open, s.setupBtn, s.goBtn),
		container.NewHBox(widget.NewLabel("speed"), s.speedWrap(), s.ticks, s.tps, s.dropped),
		container.NewCenter(s.timer),
	)
	// Center keeps the raster at its min size so a new world resizes the view
	// instead of stretching into the whole split pane. drawWorld still
	// letterboxes when the pane is smaller than that size.
	worldCard := container.NewPadded(container.NewCenter(s.raster))
	s.split = container.NewHSplit(worldCard, container.NewBorder(worldForm, nil, nil, nil, s.scroll))
	s.split.Offset = 0.72
	iface := s.split
	codePage := container.NewBorder(nil, apply, nil, nil, s.code)
	tabs := container.NewAppTabs(
		container.NewTabItem("Interface", iface),
		container.NewTabItem("Code", codePage),
	)
	bottom := container.NewBorder(nil, nil, widget.NewLabel("Command center"), nil,
		container.NewVBox(cmd, container.NewGridWrap(fyne.NewSize(800, 72), s.output)),
	)
	s.layout = container.NewBorder(toolbar, bottom, nil, nil, container.NewBorder(s.notes, nil, nil, nil, tabs))

	s.anim = fyne.NewAnimation(30*time.Millisecond, func(float32) { s.tick() })
	s.anim.RepeatCount = fyne.AnimationRepeatForever

	if len(s.names) > 0 {
		s.picker.SetSelected(s.names[0])
	}
	return s
}

// buildWorldForm is the compact World panel: a 2×2 coordinate grid, patch
// size, wrap toggles, and Apply. It is pinned above the widget scroll so the
// controls stay on screen.
func (s *session) buildWorldForm() fyne.CanvasObject {
	s.minPx = widget.NewEntry()
	s.maxPx = widget.NewEntry()
	s.minPy = widget.NewEntry()
	s.maxPy = widget.NewEntry()
	s.patchSize = widget.NewEntry()
	s.wrapX = widget.NewCheck("wrap x", nil)
	s.wrapY = widget.NewCheck("wrap y", nil)
	apply := widget.NewButton("Apply", func() { s.applyWorld() })
	apply.Importance = widget.HighImportance

	fpsRow := container.NewHBox(
		widget.NewLabel("max fps"),
		fixedBox(84, 32, s.maxFPS),
	)
	title := widget.NewLabelWithStyle("World", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	head := container.NewHBox(
		fixedBox(48, 22, widget.NewLabel("")),
		fixedBox(56, 22, widget.NewLabel("min")),
		fixedBox(56, 22, widget.NewLabel("max")),
	)
	px := container.NewHBox(
		fixedBox(48, 32, widget.NewLabel("pxcor")),
		fixedBox(56, 32, s.minPx),
		fixedBox(56, 32, s.maxPx),
	)
	py := container.NewHBox(
		fixedBox(48, 32, widget.NewLabel("pycor")),
		fixedBox(56, 32, s.minPy),
		fixedBox(56, 32, s.maxPy),
	)
	extras := container.NewHBox(
		widget.NewLabel("patch"),
		fixedBox(56, 32, s.patchSize),
		s.wrapX,
		s.wrapY,
	)
	return container.NewVBox(fpsRow, container.NewBorder(nil, nil, title, apply, layout.NewSpacer()), head, px, py, extras)
}

// setMaxFPS applies a typed cap. Values outside 1..120 are clamped.
// A partial entry (empty, or not yet a number) leaves the current cap.
func (s *session) setMaxFPS(text string) {
	n, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil {
		return
	}
	clamped := nl.ClampFrameRate(n)
	s.mu.Lock()
	s.frameRate = clamped
	s.mu.Unlock()
	shown := strconv.Itoa(clamped)
	if shown != strings.TrimSpace(text) && s.maxFPS != nil {
		s.maxFPS.SetText(shown)
	}
}

func fixedBox(w, h float32, o fyne.CanvasObject) fyne.CanvasObject {
	return container.NewGridWrap(fyne.NewSize(w, h), o)
}

// parseWorldInputs checks the World panel text. Coordinates are integers,
// the rectangle must contain the origin and stay within MaxWorldSide, and
// patch size must be in (0, 100].
func parseWorldInputs(minPxS, maxPxS, minPyS, maxPyS, patchS string) (minPx, maxPx, minPy, maxPy int, patch float64, err error) {
	minPx, err = parseCoord("min pxcor", minPxS)
	if err != nil {
		return
	}
	maxPx, err = parseCoord("max pxcor", maxPxS)
	if err != nil {
		return
	}
	minPy, err = parseCoord("min pycor", minPyS)
	if err != nil {
		return
	}
	maxPy, err = parseCoord("max pycor", maxPyS)
	if err != nil {
		return
	}
	if err = nl.ValidWorld(minPx, maxPx, minPy, maxPy); err != nil {
		return
	}
	patch, err = strconv.ParseFloat(strings.TrimSpace(patchS), 64)
	if err != nil {
		err = fmt.Errorf("patch size must be a number")
		return
	}
	if patch <= 0 || patch > 100 {
		err = fmt.Errorf("patch size must be positive and at most 100")
	}
	return
}

func parseCoord(name, raw string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", name)
	}
	return n, nil
}

// viewPixels is the on-screen size of the world view. Patch size scales the
// grid, then the long side is kept between 280 and 520 so a 100×100 world
// fits and a tiny world stays readable.
func viewPixels(width, height int, patchSize float64) (float32, float32) {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	if patchSize < 1 {
		patchSize = 1
	}
	pw := float64(width) * patchSize
	ph := float64(height) * patchSize
	long := pw
	if ph > long {
		long = ph
	}
	const maxLong = 520.0
	const minLong = 280.0
	scale := 1.0
	if long > maxLong {
		scale = maxLong / long
	} else if long < minLong {
		scale = minLong / long
	}
	return float32(pw * scale), float32(ph * scale)
}

func (s *session) applyWorld() {
	s.mu.Lock()
	err := s.applyWorldLocked()
	s.mu.Unlock()
	if err != nil {
		dialog.ShowError(err, s.win)
	}
}

func (s *session) applyWorldLocked() error {
	if s.vm == nil || s.model == nil {
		return nil
	}
	minPx, maxPx, minPy, maxPy, ps, err := parseWorldInputs(
		s.minPx.Text, s.maxPx.Text, s.minPy.Text, s.maxPy.Text, s.patchSize.Text,
	)
	if err != nil {
		s.logf("%v", err)
		return err
	}
	wrapX, wrapY := false, false
	if s.wrapX != nil {
		wrapX = s.wrapX.Checked
	}
	if s.wrapY != nil {
		wrapY = s.wrapY.Checked
	}
	if err := s.vm.World.ApplyView(minPx, maxPx, minPy, maxPy, ps, wrapX, wrapY); err != nil {
		s.logf("%v", err)
		return err
	}
	s.model.View.MinPxcor = minPx
	s.model.View.MaxPxcor = maxPx
	s.model.View.MinPycor = minPy
	s.model.View.MaxPycor = maxPy
	s.model.View.PatchSize = ps
	s.model.View.WrapX = wrapX
	s.model.View.WrapY = wrapY
	s.model.View.Set = true
	s.seenWorld = ""
	s.noteWorld()
	s.publishViewLocked()
	s.logf("world %d..%d × %d..%d · patch size %s", minPx, maxPx, minPy, maxPy, trimNum(ps))
	return nil
}

// publishViewLocked hands the draw thread a frame that does not alias the
// live turtle map. Caller holds s.mu.
func (s *session) publishViewLocked() {
	if s.vm == nil || s.vm.World == nil {
		s.view.Store(nil)
		return
	}
	s.view.Store(s.vm.World.CurrentView())
}

// noteWorld copies the live world into the panel when resize-world,
// set-patch-size, or a newly loaded model changes it. In-progress edits are
// left alone while the signature stays the same.
func (s *session) noteWorld() {
	if s.vm == nil || s.minPx == nil {
		return
	}
	w := s.vm.World
	sig := fmt.Sprintf("%d,%d,%d,%d,%g,%t,%t",
		w.MinPxcor, w.MaxPxcor, w.MinPycor, w.MaxPycor, w.PatchSize, w.WrapX, w.WrapY)
	if sig == s.seenWorld {
		return
	}
	s.seenWorld = sig
	s.minPx.SetText(strconv.Itoa(w.MinPxcor))
	s.maxPx.SetText(strconv.Itoa(w.MaxPxcor))
	s.minPy.SetText(strconv.Itoa(w.MinPycor))
	s.maxPy.SetText(strconv.Itoa(w.MaxPycor))
	s.patchSize.SetText(trimNum(w.PatchSize))
	if s.wrapX != nil {
		s.wrapX.SetChecked(w.WrapX)
	}
	if s.wrapY != nil {
		s.wrapY.SetChecked(w.WrapY)
	}
	if s.raster != nil {
		pw, ph := viewPixels(w.Width(), w.Height(), w.PatchSize)
		s.raster.SetMinSize(fyne.NewSize(pw, ph))
		s.raster.Refresh()
	}
	if s.split != nil {
		s.split.Refresh()
	}
}

func (s *session) speedWrap() fyne.CanvasObject {
	box := container.NewGridWrap(fyne.NewSize(140, 28), s.speed)
	return box
}

func (s *session) drawWorld(w, h int) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, max(w, 1), max(h, 1)))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.RGBA{24, 24, 24, 255}), image.Point{}, draw.Src)
	// Paint the published frame only. This runs on the GL thread while go
	// may be hatching or killing turtles on the animation goroutine.
	snap := s.view.Load()
	if snap == nil || w < 2 || h < 2 || snap.Width < 1 || snap.Height < 1 {
		return dst
	}
	ww, hh := snap.Width, snap.Height
	px := w / ww
	if h/hh < px {
		px = h / hh
	}
	if px < 1 {
		px = 1
	}
	src := snap.Paint(px)
	ox := (w - src.Bounds().Dx()) / 2
	oy := (h - src.Bounds().Dy()) / 2
	draw.Draw(dst, image.Rect(ox, oy, ox+src.Bounds().Dx(), oy+src.Bounds().Dy()), src, image.Point{}, draw.Over)
	return dst
}

func (s *session) load(name, content string) {
	m, err := nl.ParseModel(name, content)
	if err != nil {
		s.logf("load: %v", err)
		dialog.ShowError(err, s.win)
		return
	}
	vm, err := m.NewVM(0)
	if err != nil {
		s.logf("compile: %v", err)
		dialog.ShowError(err, s.win)
		return
	}
	s.mu.Lock()
	s.running = -1
	s.clock = nl.RunClock{}
	s.pub.Reset()
	s.model = m
	s.vm = vm
	s.vm.OnPrint = func(line string) { s.log(line) }
	s.vm.OnClear = func() { s.clearLog() }
	s.code.SetText(m.Code)
	s.rebuildSide()
	s.publishViewLocked()
	s.noteWorld()
	if s.raster != nil {
		s.raster.Refresh()
	}
	s.refreshReadouts()
	s.mu.Unlock()
	if len(m.Notes) > 0 {
		s.notes.SetText(strings.Join(m.Notes, " · "))
	} else {
		s.notes.SetText("")
	}
	s.logf("loaded %s", name)
}

func (s *session) rebuildSide() {
	s.ctls = nil
	s.areas = nil
	var objs []fyne.CanvasObject
	if s.model == nil {
		s.side.Objects = nil
		s.side.Refresh()
		return
	}
	for i := range s.model.Widgets {
		w := s.model.Widgets[i]
		c := control{widget: w}
		idx := len(s.ctls)
		switch w.Kind {
		case "button":
			stmts, err := nl.CompileButton(s.vm.Prog, w.Source, w.Agent)
			if err != nil {
				s.logf("%s: %v", w.Display, err)
			}
			c.stmts = stmts
			label := w.Display
			if w.Forever {
				label = label + " ∞"
			}
			btn := widget.NewButton(label, func() { s.press(idx) })
			if w.Forever {
				btn.Importance = widget.DangerImportance
			}
			c.button = btn
			objs = append(objs, btn)
		case "slider":
			label := widget.NewLabel(sliderCaption(w))
			sl := widget.NewSlider(w.Min, w.Max)
			sl.Step = w.Step
			if sl.Step <= 0 {
				sl.Step = 1
			}
			sl.Value = w.Value
			variable := w.Variable
			sl.OnChanged = func(v float64) {
				s.mu.Lock()
				defer s.mu.Unlock()
				if s.busy || s.vm == nil || s.model == nil || idx >= len(s.model.Widgets) {
					return
				}
				s.vm.World.SetGlobal(variable, nl.Num(v))
				s.model.Widgets[idx].Value = v
				label.SetText(sliderCaption(s.model.Widgets[idx]))
			}
			c.slider = sl
			objs = append(objs, label, sl)
		case "switch":
			variable := w.Variable
			chk := widget.NewCheck(w.Display, func(on bool) {
				s.mu.Lock()
				defer s.mu.Unlock()
				if s.busy || s.vm == nil || s.model == nil || idx >= len(s.model.Widgets) {
					return
				}
				s.vm.World.SetGlobal(variable, nl.Bool(on))
				s.model.Widgets[idx].On = on
			})
			chk.Checked = w.On
			c.check = chk
			objs = append(objs, chk)
		case "monitor":
			title := widget.NewLabel(w.Display)
			title.TextStyle = fyne.TextStyle{Bold: true}
			val := widget.NewLabel("—")
			c.readout = val
			objs = append(objs, title, val)
		case "chooser":
			opts := make([]string, len(w.Choices))
			for i, ch := range w.Choices {
				opts[i] = ch.Format(false)
			}
			variable := w.Variable
			sel := widget.NewSelect(opts, nil)
			if w.Current >= 0 && w.Current < len(opts) {
				sel.SetSelected(opts[w.Current])
			}
			sel.OnChanged = func(picked string) {
				s.mu.Lock()
				defer s.mu.Unlock()
				if s.busy || s.vm == nil || s.model == nil || idx >= len(s.model.Widgets) {
					return
				}
				for i, ch := range s.model.Widgets[idx].Choices {
					if ch.Format(false) == picked {
						s.model.Widgets[idx].Current = i
						s.vm.World.SetGlobal(variable, ch)
						break
					}
				}
			}
			c.choice = sel
			objs = append(objs, widget.NewLabel(w.Display), sel)
		case "input":
			ent := widget.NewEntry()
			if w.Text != "" {
				ent.SetText(w.Text)
			} else {
				ent.SetText(trimNum(w.Value))
			}
			variable := w.Variable
			typ := w.InputType
			ent.OnChanged = func(text string) {
				s.mu.Lock()
				defer s.mu.Unlock()
				if s.busy || s.vm == nil || s.model == nil || idx >= len(s.model.Widgets) {
					return
				}
				s.model.Widgets[idx].Text = text
				if strings.EqualFold(typ, "number") || strings.EqualFold(typ, "color") || typ == "Number" || typ == "Color" {
					var n float64
					fmt.Sscan(text, &n)
					s.model.Widgets[idx].Value = n
					s.vm.World.SetGlobal(variable, nl.Num(n))
				} else {
					s.vm.World.SetGlobal(variable, nl.Str(text))
				}
			}
			c.entry = ent
			objs = append(objs, widget.NewLabel(w.Display), ent)
		case "note":
			objs = append(objs, widget.NewLabel(w.Display))
		case "output":
			view := newOutputView(w)
			s.areas = append(s.areas, view)
			objs = append(objs, view.box)
		case "plot":
			objs = append(objs, widget.NewLabel("plot not available: "+w.Display))
		}
		s.ctls = append(s.ctls, c)
		_ = idx
	}
	s.side.Objects = objs
	s.side.Refresh()
}

func (s *session) pressNamed(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, c := range s.ctls {
		if c.widget.Kind != "button" {
			continue
		}
		if strings.EqualFold(c.widget.Display, name) || strings.EqualFold(strings.TrimSpace(c.widget.Source), name) {
			s.pressLocked(i)
			return
		}
	}
	// Fall back to a procedure of that name.
	if s.vm != nil && s.vm.Prog.Proc(name) != nil {
		stopped, err := s.vm.Call(name)
		if err != nil {
			s.logf("%s: %v", name, err)
			return
		}
		if name == "go" && stopped {
			s.running = -1
		}
		if strings.EqualFold(name, "setup") {
			s.pub.Reset()
		}
		s.publishViewLocked()
		s.raster.Refresh()
		s.refreshReadouts()
	}
}

func (s *session) press(i int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pressLocked(i)
}

func (s *session) pressLocked(i int) {
	if i < 0 || i >= len(s.ctls) || s.vm == nil {
		return
	}
	c := &s.ctls[i]
	if c.widget.Kind != "button" {
		return
	}
	if c.widget.Forever {
		if s.running == i {
			s.running = -1
			if c.button != nil {
				c.button.SetText(c.widget.Display + " ∞")
			}
			s.goBtn.SetText("go")
			return
		}
		if c.widget.DisableUntilTicks && !s.vm.World.TicksStarted() {
			s.logf("%s is disabled until ticks start", c.widget.Display)
			return
		}
		s.running = i
		if c.button != nil {
			c.button.SetText("stop")
		}
		if strings.EqualFold(c.widget.Display, "go") || strings.EqualFold(strings.TrimSpace(c.widget.Source), "go") {
			s.goBtn.SetText("stop")
		}
		return
	}
	s.stepLocked(c)
	if strings.EqualFold(c.widget.Display, "setup") || strings.EqualFold(strings.TrimSpace(c.widget.Source), "setup") {
		s.pub.Reset()
	}
	s.presentLocked()
}

// stepLocked runs one button. Caller holds s.mu. It does not paint.
// A click paints afterwards, and the animation paints on the frame cap.
// RunCompiled keeps a concurrent RenderWorld off the live maps.
func (s *session) stepLocked(c *control) bool {
	if c.disableTicks() && !s.vm.World.TicksStarted() {
		s.logf("%s is disabled until ticks start", c.widget.Display)
		return false
	}
	stopped, err := s.vm.RunCompiled(c.stmts)
	if err != nil {
		s.logf("%s: %v", c.widget.Display, err)
		s.running = -1
		s.goBtn.SetText("go")
		return false
	}
	if stopped {
		s.running = -1
		s.goBtn.SetText("go")
		for i := range s.ctls {
			if s.ctls[i].button != nil && s.ctls[i].widget.Forever {
				s.ctls[i].button.SetText(s.ctls[i].widget.Display + " ∞")
			}
		}
	}
	return true
}

// presentLocked publishes one frame and refreshes monitors. Caller holds s.mu.
// The speed slider changes how many steps run per call, not how often this runs.
func (s *session) presentLocked() {
	if s.vm == nil {
		return
	}
	s.refreshReadouts()
	s.publishViewLocked()
	if s.vm.DisplayOn {
		s.raster.Refresh()
	}
}

func (c *control) disableTicks() bool { return c.widget.DisableUntilTicks }

func (s *session) tick() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.vm == nil {
		return
	}
	s.frame++
	if s.running >= 0 && s.running < len(s.ctls) {
		n := stepsFor(s.speed.Value, s.frame)
		interval := nl.FrameInterval(s.frameRate)
		for i := 0; i < n; i++ {
			if s.vm == nil || s.running < 0 || s.running >= len(s.ctls) {
				break
			}
			if !s.stepLocked(&s.ctls[s.running]) {
				break
			}
			// Count the step, then publish only when the cap interval has
			// elapsed. The snapshot is the latest world; steps in between
			// are drops. Refresh is scheduled without waiting for the draw.
			s.pub.Produce(1)
			if s.pub.Publish(time.Now(), interval) {
				s.publishViewLocked()
				s.refreshRunLabels()
				s.refreshMonitorsLocked()
				displayOn := s.vm.DisplayOn
				s.mu.Unlock()
				if displayOn && s.raster != nil {
					s.raster.Refresh()
				}
				s.mu.Lock()
			}
			if s.running < 0 {
				break
			}
		}
	}
	s.refreshRunLabels()
	s.refreshOutputLocked()
}

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

func (s *session) setLabel(label *widget.Label, text string) {
	if label != nil && label.Text != text {
		label.SetText(text)
	}
}

// refreshReadouts updates monitors and the world fields. Caller holds s.mu.
func (s *session) refreshReadouts() {
	if s.vm == nil {
		return
	}
	s.refreshRunLabels()
	s.refreshMonitorsLocked()
	s.refreshOutputLocked()
	s.noteWorld()
}

// refreshOutputLocked copies the output area into the sidebar panel.
// Caller holds s.mu.
func (s *session) refreshOutputLocked() {
	if s.vm == nil || len(s.areas) == 0 {
		return
	}
	body := s.vm.OutputText()
	for _, area := range s.areas {
		if area.text.Text == body {
			continue
		}
		area.text.Text = body
		area.text.Refresh()
		area.scroll.ScrollToBottom()
	}
}

func newOutputView(w nl.Widget) *outputView {
	fs := w.FontSize
	if fs <= 0 {
		fs = 12
	}
	if fs < 8 {
		fs = 8
	}
	if fs > 32 {
		fs = 32
	}
	h := w.H
	if h < 72 {
		h = 96
	}
	if h > 200 {
		h = 200
	}
	text := canvas.NewText("", color.NRGBA{R: 0xf4, G: 0xef, B: 0xe4, A: 0xff})
	text.TextStyle = fyne.TextStyle{Monospace: true}
	text.TextSize = float32(fs)
	text.Alignment = fyne.TextAlignLeading
	bg := canvas.NewRectangle(color.NRGBA{R: 0x24, G: 0x1f, B: 0x18, A: 0xff})
	scroll := container.NewScroll(container.NewPadded(text))
	// GridWrap gives the log a compact fixed box. The rectangle fills that
	// box behind the scroll so a short log still looks like a panel.
	box := container.NewGridWrap(fyne.NewSize(220, float32(h)), container.NewStack(bg, scroll))
	return &outputView{text: text, scroll: scroll, box: box, fontSize: text.TextSize}
}

// refreshRunLabels updates the toolbar clock and drop count. Caller holds s.mu.
// It does not copy the world, so the step loop can call it every frame.
func (s *session) refreshRunLabels() {
	if s.vm == nil {
		return
	}
	var ticks float64
	if s.vm.World.TicksStarted() {
		n, err := s.vm.World.Ticks()
		if err == nil {
			ticks = n
			s.setLabel(s.ticks, "ticks: "+trimNum(n))
		}
	} else {
		s.setLabel(s.ticks, "ticks")
	}
	running := s.running >= 0
	tps, timer := s.clock.Sync(time.Now(), running, s.vm.World.RunEpoch(), ticks)
	s.setLabel(s.tps, "tps "+nl.FormatTPS(tps, running))
	s.setLabel(s.timer, timer)
	s.setLabel(s.dropped, "drop "+strconv.Itoa(s.pub.Dropped()))
}

// refreshMonitorsLocked re-reads monitor widgets. Caller holds s.mu.
func (s *session) refreshMonitorsLocked() {
	if s.vm == nil {
		return
	}
	for i := range s.ctls {
		c := &s.ctls[i]
		if c.widget.Kind == "monitor" && c.readout != nil {
			out, err := s.vm.ExecuteLine(c.widget.Source)
			if err != nil {
				c.readout.SetText("N/A")
			} else {
				c.readout.SetText(out)
			}
		}
		if c.button != nil && c.widget.DisableUntilTicks {
			if s.vm.World.TicksStarted() {
				c.button.Enable()
			} else if s.running != i {
				c.button.Disable()
			}
		}
	}
}

func (s *session) runLine(text string) {
	text = strings.TrimSpace(text)
	s.mu.Lock()
	defer s.mu.Unlock()
	if text == "" || s.vm == nil {
		return
	}
	out, err := s.vm.ExecuteLine(text)
	if err != nil {
		s.logf("%v", err)
		return
	}
	if out != "" {
		s.log(out)
	}
	s.publishViewLocked()
	s.raster.Refresh()
	s.refreshReadouts()
}

func (s *session) applyCode() {
	s.mu.Lock()
	err := s.applyCodeLocked()
	s.mu.Unlock()
	if err != nil {
		dialog.ShowError(err, s.win)
	}
}

func (s *session) applyCodeLocked() error {
	if s.model == nil {
		return nil
	}
	s.model.Code = s.code.Text
	vm, err := s.model.NewVM(0)
	if err != nil {
		s.logf("apply: %v", err)
		return err
	}
	s.running = -1
	s.goBtn.SetText("go")
	s.clock = nl.RunClock{}
	s.vm = vm
	s.vm.OnPrint = func(line string) { s.log(line) }
	s.vm.OnClear = func() { s.clearLog() }
	s.rebuildSide()
	s.publishViewLocked()
	s.noteWorld()
	s.raster.Refresh()
	s.log("code applied — press setup")
	return nil
}

func (s *session) openFile() {
	dialog.NewFileOpen(func(r fyne.URIReadCloser, err error) {
		if err != nil {
			s.logf("%v", err)
			return
		}
		if r == nil {
			return
		}
		defer r.Close()
		b, err := io.ReadAll(r)
		if err != nil {
			s.logf("%v", err)
			return
		}
		s.load(r.URI().Name(), string(b))
	}, s.win).Show()
}

func (s *session) log(line string) {
	s.outMu.Lock()
	defer s.outMu.Unlock()
	if s.output.Text == "" {
		s.output.SetText(line)
	} else {
		s.output.SetText(s.output.Text + "\n" + line)
	}
	s.output.CursorRow = strings.Count(s.output.Text, "\n")
}

func (s *session) clearLog() {
	s.outMu.Lock()
	defer s.outMu.Unlock()
	s.output.SetText("")
	s.output.CursorRow = 0
}

func (s *session) logf(f string, args ...any) {
	s.log(fmt.Sprintf(f, args...))
}

func sliderCaption(w nl.Widget) string {
	text := fmt.Sprintf("%s  %s", w.Display, trimNum(w.Value))
	if w.Units != "" && w.Units != "NIL" {
		text += " " + w.Units
	}
	return text
}

func trimNum(n float64) string {
	s := fmt.Sprintf("%g", n)
	return s
}

func demoNames(fsys fs.FS) []string {
	if fsys == nil {
		return nil
	}
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if strings.HasSuffix(n, ".nlogo") || strings.HasSuffix(n, ".nlogox") {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	return names
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
