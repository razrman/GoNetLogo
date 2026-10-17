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
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
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

type session struct {
	win      fyne.Window
	modelsFS fs.FS
	names    []string
	model    *nl.FileModel
	vm       *nl.VM
	code     *widget.Entry
	output   *widget.Entry
	ticks    *widget.Label
	notes    *widget.Label
	raster   *canvas.Raster
	side     *fyne.Container
	scroll   *container.Scroll
	layout   fyne.CanvasObject
	picker   *widget.Select
	speed    *widget.Slider
	setupBtn *widget.Button
	goBtn    *widget.Button

	ctls    []control
	running int
	frame   int
	anim    *fyne.Animation
	busy    bool
}

func newSession(modelsFS fs.FS, win fyne.Window) *session {
	s := &session{win: win, modelsFS: modelsFS, running: -1}
	s.names = demoNames(modelsFS)
	s.ticks = widget.NewLabel("ticks")
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
	s.scroll.SetMinSize(fyne.NewSize(280, 400))

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

	toolbar := container.NewBorder(nil, nil, nil,
		container.NewHBox(widget.NewLabel("speed"), s.speedWrap(), s.ticks),
		container.NewHBox(s.picker, open, s.setupBtn, s.goBtn),
	)
	worldCard := container.NewPadded(s.raster)
	iface := container.NewHSplit(worldCard, s.scroll)
	iface.Offset = 0.68
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

func (s *session) speedWrap() fyne.CanvasObject {
	box := container.NewGridWrap(fyne.NewSize(140, 28), s.speed)
	return box
}

func (s *session) drawWorld(w, h int) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, max(w, 1), max(h, 1)))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.RGBA{24, 24, 24, 255}), image.Point{}, draw.Src)
	if s.vm == nil || w < 2 || h < 2 {
		return dst
	}
	ww, hh := s.vm.World.Width(), s.vm.World.Height()
	if ww < 1 || hh < 1 {
		return dst
	}
	px := w / ww
	if h/hh < px {
		px = h / hh
	}
	if px < 1 {
		px = 1
	}
	src := nl.RenderWorld(s.vm.World, px)
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
	s.running = -1
	s.model = m
	s.vm = vm
	s.vm.OnPrint = func(line string) { s.log(line) }
	s.code.SetText(m.Code)
	s.rebuildSide()
	if s.raster != nil {
		s.raster.Refresh()
	}
	s.refreshReadouts()
	if len(m.Notes) > 0 {
		s.notes.SetText(strings.Join(m.Notes, " · "))
	} else {
		s.notes.SetText("")
	}
	s.logf("loaded %s", name)
}

func (s *session) rebuildSide() {
	s.ctls = nil
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
				if s.busy || s.vm == nil {
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
				if s.busy || s.vm == nil {
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
			sel := widget.NewSelect(opts, func(picked string) {
				if s.busy || s.vm == nil {
					return
				}
				for i, ch := range s.model.Widgets[idx].Choices {
					if ch.Format(false) == picked {
						s.model.Widgets[idx].Current = i
						s.vm.World.SetGlobal(variable, ch)
						break
					}
				}
			})
			if w.Current >= 0 && w.Current < len(opts) {
				sel.SetSelected(opts[w.Current])
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
				if s.busy || s.vm == nil {
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
	for i, c := range s.ctls {
		if c.widget.Kind != "button" {
			continue
		}
		if strings.EqualFold(c.widget.Display, name) || strings.EqualFold(strings.TrimSpace(c.widget.Source), name) {
			s.press(i)
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
		s.raster.Refresh()
		s.refreshReadouts()
	}
}

func (s *session) press(i int) {
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
	s.step(c)
}

func (s *session) step(c *control) {
	if c.disableTicks() && !s.vm.World.TicksStarted() {
		s.logf("%s is disabled until ticks start", c.widget.Display)
		return
	}
	stopped, err := s.vm.RunCompiled(c.stmts)
	if err != nil {
		s.logf("%s: %v", c.widget.Display, err)
		s.running = -1
		s.goBtn.SetText("go")
		return
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
	s.raster.Refresh()
	s.refreshReadouts()
}

func (c *control) disableTicks() bool { return c.widget.DisableUntilTicks }

func (s *session) tick() {
	if s.vm == nil {
		return
	}
	s.frame++
	if s.running >= 0 && s.running < len(s.ctls) {
		n := stepsFor(s.speed.Value, s.frame)
		for i := 0; i < n; i++ {
			s.step(&s.ctls[s.running])
			if s.running < 0 {
				break
			}
		}
	}
	s.refreshReadouts()
	if s.vm.DisplayOn {
		s.raster.Refresh()
	}
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

func (s *session) refreshReadouts() {
	if s.vm == nil {
		return
	}
	if s.vm.World.TicksStarted() {
		n, err := s.vm.World.Ticks()
		if err == nil {
			s.ticks.SetText("ticks: " + trimNum(n))
		}
	} else {
		s.ticks.SetText("ticks")
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
	s.raster.Refresh()
	s.refreshReadouts()
}

func (s *session) applyCode() {
	if s.model == nil {
		return
	}
	s.model.Code = s.code.Text
	vm, err := s.model.NewVM(0)
	if err != nil {
		s.logf("apply: %v", err)
		dialog.ShowError(err, s.win)
		return
	}
	s.running = -1
	s.goBtn.SetText("go")
	s.vm = vm
	s.vm.OnPrint = func(line string) { s.log(line) }
	s.rebuildSide()
	s.raster.Refresh()
	s.log("code applied — press setup")
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
	if s.output.Text == "" {
		s.output.SetText(line)
	} else {
		s.output.SetText(s.output.Text + "\n" + line)
	}
	s.output.CursorRow = strings.Count(s.output.Text, "\n")
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
