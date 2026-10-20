package nl

import (
	"fmt"
	"math"
	"strings"
	"sync/atomic"
)

// PlotPointCap is how many samples one pen keeps. Older samples drop off
// the front. Seq still counts every sample in the current epoch, so a
// subscriber behind the window is sent a reset instead of a gap.
const PlotPointCap = 8192

// PlotPoint is one sample. Down is false when the pen was up, which breaks
// the line before this sample. The sample is kept either way: a pen-up
// plot still moves the pen, matching NetLogo.
type PlotPoint struct {
	X, Y float64
	Down bool
}

// PlotCursor is the last sample a subscriber has applied for one pen.
type PlotCursor struct {
	Epoch int
	Seq   int
}

// PlotPenView is the pen description sent with every frame. Points are not
// included; those travel as PlotDelta.
type PlotPenView struct {
	Name     string
	R, G, B  int
	Mode     string
	Interval float64
}

// PlotView is one plot's axes and pens, without the point history.
type PlotView struct {
	Index            int
	Name             string
	XLabel, YLabel   string
	XMin, XMax       float64
	YMin, YMax       float64
	AutoPlot, Legend bool
	Error            string
	Pens             []PlotPenView
}

// PlotDelta is the samples one pen added since a subscriber's cursor.
// Reset means the subscriber should drop that pen's previous samples.
type PlotDelta struct {
	PlotIndex int
	PenIndex  int
	Epoch     int
	Seq       int
	Reset     bool
	Points    []PlotPoint
}

// PlotSnap is a full copy of one plot for an in-process painter.
type PlotSnap struct {
	Name             string
	XLabel, YLabel   string
	XMin, XMax       float64
	YMin, YMax       float64
	AutoPlot, Legend bool
	Pens             []PlotPenSnap
}

// PlotPenSnap is one pen and its retained samples.
type PlotPenSnap struct {
	Name     string
	R, G, B  uint8
	Mode     string
	Interval float64
	Points   []PlotPoint
}

// PlotPenSpec is a pen read from a PLOT widget.
type PlotPenSpec struct {
	Name     string
	Interval float64
	Mode     int
	Color    int32
	InLegend bool
	Setup    string
	Update   string
}

// PlotSpec is a plot read from a PLOT widget.
type PlotSpec struct {
	Name             string
	XLabel, YLabel   string
	XMin, XMax       float64
	YMin, YMax       float64
	AutoPlot, Legend bool
	Setup, Update    string
	Pens             []PlotPenSpec
}

const (
	penLine  = 0
	penBar   = 1
	penPoint = 2
)

const (
	autoXFactor = 1.25
	autoYFactor = 1.10
)

// defaultPenColor is Java's Color.BLACK.getRGB(), the .nlogo default.
const defaultPenColor int32 = -16777216

var plotEpochs atomic.Int64

type plotPen struct {
	name      string
	interval  float64
	mode      int
	color     int32
	r, g, b   uint8
	inLegend  bool
	setupSrc  string
	updateSrc string
	setup     []Stmt
	update    []Stmt
	temp      bool

	x      float64
	down   bool
	points []PlotPoint
	epoch  int
	seq    int
	err    string

	defX        float64
	defInterval float64
	defMode     int
	defColor    int32
	defDown     bool
}

type plot struct {
	name             string
	xLabel, yLabel   string
	xMin, xMax       float64
	yMin, yMax       float64
	auto             bool
	legend           bool
	setupSrc         string
	updateSrc        string
	setup            []Stmt
	update           []Stmt
	pens             []*plotPen
	current          int
	err              string
	defXMin, defXMax float64
	defYMin, defYMax float64
	defAuto          bool
}

type plotMgr struct {
	plots   []*plot
	current int
}

// InstallPlots builds the runtime plots from widget specs and compiles
// each setup and update command list. A compile error is kept on the plot
// and does not reject the model. The last plot becomes current, which is
// what loading widgets in order does.
func (vm *VM) InstallPlots(specs []PlotSpec) {
	mgr := &plotMgr{current: -1}
	for _, spec := range specs {
		mgr.plots = append(mgr.plots, newPlot(vm, spec))
		mgr.current = len(mgr.plots) - 1
	}
	vm.plots = mgr
}

func (vm *VM) plotsOrEmpty() *plotMgr {
	if vm.plots == nil {
		vm.plots = &plotMgr{current: -1}
	}
	return vm.plots
}

func newPlot(vm *VM, spec PlotSpec) *plot {
	name := strings.TrimSpace(spec.Name)
	if name == "" {
		name = "plot"
	}
	// A zero-width range is kept. Robby's Best Fitness plot is saved as
	// 0 to 0 on y so the first pen-down sample grows it. Only an inverted
	// range is replaced.
	xMin, xMax := axisSpan(spec.XMin, spec.XMax)
	yMin, yMax := axisSpan(spec.YMin, spec.YMax)
	p := &plot{
		name: name, xLabel: spec.XLabel, yLabel: spec.YLabel,
		xMin: xMin, xMax: xMax, yMin: yMin, yMax: yMax,
		auto: spec.AutoPlot, legend: spec.Legend,
		setupSrc: spec.Setup, updateSrc: spec.Update,
		defXMin: xMin, defXMax: xMax, defYMin: yMin, defYMax: yMax,
		defAuto: spec.AutoPlot,
	}
	p.setup, p.err = compilePlotCode(vm, spec.Setup, name, "", true)
	if upd, err := compilePlotCode(vm, spec.Update, name, "", false); err != "" && p.err == "" {
		p.err = err
		p.update = upd
	} else {
		p.update = upd
	}
	pens := spec.Pens
	if len(pens) == 0 {
		pens = []PlotPenSpec{{Name: "default", Interval: 1, Mode: penLine, Color: defaultPenColor, InLegend: true}}
	}
	for _, ps := range pens {
		p.pens = append(p.pens, newPen(vm, name, ps))
	}
	return p
}

func newPen(vm *VM, plotName string, spec PlotPenSpec) *plotPen {
	name := strings.TrimSpace(spec.Name)
	if name == "" {
		name = "default"
	}
	interval := spec.Interval
	if interval == 0 {
		interval = 1
	}
	color := spec.Color
	if color == 0 {
		color = defaultPenColor
	}
	r, g, b := unpackPenColor(color)
	pen := &plotPen{
		name: name, interval: interval, mode: spec.Mode, color: color,
		r: r, g: g, b: b, inLegend: spec.InLegend,
		setupSrc: spec.Setup, updateSrc: spec.Update,
		x: 0, down: true,
		epoch:       int(plotEpochs.Add(1)),
		defX:        0,
		defInterval: interval,
		defMode:     spec.Mode,
		defColor:    color,
		defDown:     true,
	}
	var err string
	pen.setup, err = compilePlotCode(vm, spec.Setup, plotName, name, true)
	if err != "" {
		pen.err = err
	}
	pen.update, err = compilePlotCode(vm, spec.Update, plotName, name, false)
	if err != "" && pen.err == "" {
		pen.err = err
	}
	return pen
}

func compilePlotCode(vm *VM, src, plotName, penName string, setup bool) ([]Stmt, string) {
	src = strings.TrimSpace(src)
	if src == "" || vm == nil || vm.Prog == nil {
		return nil, ""
	}
	stmts, err := ParseCommands(src, vm.Prog)
	if err != nil {
		which := "update"
		if setup {
			which = "setup"
		}
		where := "plot '" + plotName + "'"
		if penName != "" {
			where += " pen '" + penName + "'"
		}
		return nil, where + " " + which + " code: " + err.Error()
	}
	return stmts, ""
}

func (vm *VM) currentPlot() (*plot, error) {
	mgr := vm.plots
	if mgr == nil || mgr.current < 0 || mgr.current >= len(mgr.plots) {
		return nil, fmt.Errorf("no current plot")
	}
	return mgr.plots[mgr.current], nil
}

func (p *plot) currentPen() (*plotPen, error) {
	if p.current < 0 || p.current >= len(p.pens) {
		return nil, fmt.Errorf("plot %q has no pens", p.name)
	}
	return p.pens[p.current], nil
}

func (vm *VM) plotY(y float64) error {
	p, pen, err := vm.plotTarget()
	if err != nil {
		return err
	}
	x := pen.x
	if len(pen.points) > 0 {
		x += pen.interval
	}
	pen.add(x, y)
	if pen.down {
		p.grow(pen, x, y)
	}
	return nil
}

func (vm *VM) plotXY(x, y float64) error {
	p, pen, err := vm.plotTarget()
	if err != nil {
		return err
	}
	pen.add(x, y)
	if pen.down {
		p.grow(pen, x, y)
	}
	return nil
}

func (vm *VM) plotTarget() (*plot, *plotPen, error) {
	p, err := vm.currentPlot()
	if err != nil {
		return nil, nil, err
	}
	pen, err := p.currentPen()
	if err != nil {
		return nil, nil, err
	}
	return p, pen, nil
}

func (pen *plotPen) add(x, y float64) {
	pen.x = x
	pen.points = append(pen.points, PlotPoint{X: x, Y: y, Down: pen.down})
	pen.seq++
	if len(pen.points) > PlotPointCap {
		extra := len(pen.points) - PlotPointCap
		pen.points = append([]PlotPoint(nil), pen.points[extra:]...)
	}
}

func (p *plot) grow(pen *plotPen, x, y float64) {
	if !p.auto {
		return
	}
	if pen.mode == penBar {
		p.growAxis(x+pen.interval, y)
	}
	p.growAxis(x, y)
}

func (p *plot) growAxis(x, y float64) {
	if x > p.xMax {
		span := (x - p.xMin) * autoXFactor
		next := niceBound(p.xMin+span, span)
		if next <= x {
			next = x
		}
		p.xMax = next
	}
	if x < p.xMin {
		span := (p.xMax - x) * autoXFactor
		next := niceBound(p.xMax-span, span)
		if next >= x {
			next = x
		}
		p.xMin = next
	}
	if y > p.yMax {
		span := (y - p.yMin) * autoYFactor
		next := niceBound(p.yMin+span, span)
		if next <= y {
			next = y
		}
		p.yMax = next
	}
	if y < p.yMin {
		span := (p.yMax - y) * autoYFactor
		next := niceBound(p.yMax-span, span)
		if next >= y {
			next = y
		}
		p.yMin = next
	}
}

// axisSpan keeps a range from a plot widget, including a zero-width one.
// An inverted or non-finite range becomes 0 to 10.
func axisSpan(min, max float64) (float64, float64) {
	if math.IsNaN(min) || math.IsNaN(max) || math.IsInf(min, 0) || math.IsInf(max, 0) || max < min {
		return 0, 10
	}
	return min, max
}

func niceBound(bound, span float64) float64 {
	if !(span > 0) || math.IsNaN(bound) || math.IsInf(bound, 0) || math.IsNaN(span) || math.IsInf(span, 0) {
		return bound
	}
	places := 2 - math.Floor(math.Log10(span))
	if places > 8 {
		places = 8
	}
	if places < -6 {
		places = -6
	}
	pow := math.Pow(10, places)
	return math.Round(bound*pow) / pow
}

func (vm *VM) plotPenDown(down bool) error {
	_, pen, err := vm.plotTarget()
	if err != nil {
		return err
	}
	pen.down = down
	return nil
}

func (vm *VM) setPlotRange(minV, maxV Value, xAxis bool) error {
	p, err := vm.currentPlot()
	if err != nil {
		return err
	}
	what := "set-plot-y-range"
	if xAxis {
		what = "set-plot-x-range"
	}
	min, err := plotNum(minV, what)
	if err != nil {
		return err
	}
	max, err := plotNum(maxV, what)
	if err != nil {
		return err
	}
	if min >= max {
		return fmt.Errorf("%s: the minimum must be less than the maximum", what)
	}
	if xAxis {
		p.xMin, p.xMax = min, max
	} else {
		p.yMin, p.yMax = min, max
	}
	return nil
}

func (vm *VM) setAutoPlot(on bool) error {
	p, err := vm.currentPlot()
	if err != nil {
		return err
	}
	p.auto = on
	return nil
}

func (vm *VM) setCurrentPlot(v Value) error {
	if v.K != KindString {
		return fmt.Errorf("set-current-plot expected a string")
	}
	mgr := vm.plotsOrEmpty()
	name := v.S
	for i, p := range mgr.plots {
		if strings.EqualFold(p.name, name) {
			mgr.current = i
			return nil
		}
	}
	return fmt.Errorf("no such plot: %q", name)
}

func (vm *VM) setCurrentPlotPen(v Value) error {
	if v.K != KindString {
		return fmt.Errorf("set-current-plot-pen expected a string")
	}
	p, err := vm.currentPlot()
	if err != nil {
		return err
	}
	for i, pen := range p.pens {
		if strings.EqualFold(pen.name, v.S) {
			p.current = i
			return nil
		}
	}
	return fmt.Errorf("there is no pen named %q in the current plot", v.S)
}

func (vm *VM) createTempPen(v Value) error {
	if v.K != KindString || strings.TrimSpace(v.S) == "" {
		return fmt.Errorf("create-temporary-plot-pen expected a name")
	}
	p, err := vm.currentPlot()
	if err != nil {
		return err
	}
	for i, pen := range p.pens {
		if strings.EqualFold(pen.name, v.S) {
			p.current = i
			return nil
		}
	}
	pen := newPen(vm, p.name, PlotPenSpec{
		Name: v.S, Interval: 1, Mode: penLine, Color: defaultPenColor, InLegend: false,
	})
	pen.temp = true
	p.pens = append(p.pens, pen)
	p.current = len(p.pens) - 1
	return nil
}

func (vm *VM) clearPlot() error {
	p, err := vm.currentPlot()
	if err != nil {
		return err
	}
	p.clear()
	return nil
}

func (vm *VM) clearAllPlots() {
	if vm.plots == nil {
		return
	}
	for _, p := range vm.plots.plots {
		p.clear()
	}
}

func (p *plot) clear() {
	kept := p.pens[:0]
	for _, pen := range p.pens {
		if pen.temp {
			continue
		}
		pen.hardReset()
		kept = append(kept, pen)
	}
	p.pens = kept
	p.current = 0
	if len(p.pens) == 0 {
		p.current = -1
	}
	p.xMin, p.xMax = p.defXMin, p.defXMax
	p.yMin, p.yMax = p.defYMin, p.defYMax
	p.auto = p.defAuto
	p.err = ""
}

func (pen *plotPen) hardReset() {
	pen.x = pen.defX
	pen.down = pen.defDown
	pen.interval = pen.defInterval
	pen.mode = pen.defMode
	pen.color = pen.defColor
	pen.r, pen.g, pen.b = unpackPenColor(pen.color)
	pen.points = nil
	pen.seq = 0
	pen.epoch = int(plotEpochs.Add(1))
	pen.err = ""
}

// setupPlots runs every plot's setup commands, then each permanent pen's.
// A stop in the plot's own setup skips that plot's pens. Errors are stored
// on the plot or pen and do not abort the caller: reset-ticks still starts
// the counter when a pen's setup is wrong.
func (vm *VM) setupPlots() {
	vm.runPlotCode(true)
}

// updatePlots runs every plot's update commands, then each permanent pen's.
// tick and reset-ticks both call this. The current plot and pen are restored.
func (vm *VM) updatePlots() {
	vm.runPlotCode(false)
}

func (vm *VM) runPlotCode(setup bool) {
	mgr := vm.plots
	if mgr == nil || len(mgr.plots) == 0 {
		return
	}
	savedPlot := mgr.current
	for i, p := range mgr.plots {
		mgr.current = i
		code := p.update
		if setup {
			code = p.setup
		}
		stopped := false
		if len(code) > 0 {
			st, err := vm.runStmts(code)
			stopped = st
			if err != nil {
				p.err = err.Error()
			} else if setup {
				p.err = ""
			}
		}
		if stopped {
			continue
		}
		savedPen := p.current
		for j, pen := range p.pens {
			if pen.temp {
				continue
			}
			p.current = j
			penCode := pen.update
			if setup {
				penCode = pen.setup
			}
			if len(penCode) == 0 {
				continue
			}
			if _, err := vm.runStmts(penCode); err != nil {
				pen.err = err.Error()
			} else {
				pen.err = ""
			}
		}
		if savedPen >= 0 && savedPen < len(p.pens) {
			p.current = savedPen
		}
	}
	if savedPlot >= -1 && savedPlot < len(mgr.plots) {
		mgr.current = savedPlot
	}
}

// PlotViews copies axis state for a frame. The point history is not copied.
func (vm *VM) PlotViews() []PlotView {
	if vm.plots == nil || len(vm.plots.plots) == 0 {
		return []PlotView{}
	}
	out := make([]PlotView, len(vm.plots.plots))
	for i, p := range vm.plots.plots {
		out[i] = p.view(i)
	}
	return out
}

func (p *plot) view(index int) PlotView {
	pens := make([]PlotPenView, len(p.pens))
	err := p.err
	for i, pen := range p.pens {
		pens[i] = PlotPenView{
			Name: pen.name, R: int(pen.r), G: int(pen.g), B: int(pen.b),
			Mode: penModeName(pen.mode), Interval: pen.interval,
		}
		if pen.err != "" && err == "" {
			err = pen.err
		}
	}
	return PlotView{
		Index: index, Name: p.name, XLabel: p.xLabel, YLabel: p.yLabel,
		XMin: p.xMin, XMax: p.xMax, YMin: p.yMin, YMax: p.yMax,
		AutoPlot: p.auto, Legend: p.legend, Error: err, Pens: pens,
	}
}

// PlotDeltas returns samples added since cursors, and the cursor to keep
// for the next call. A pen with nothing new is omitted. Its cursor still
// advances in the returned map. A nil cursors map means the caller has
// never seen the pen, so the retained samples are sent with Reset set.
func (vm *VM) PlotDeltas(cursors map[string]PlotCursor) ([]PlotDelta, map[string]PlotCursor) {
	next := map[string]PlotCursor{}
	if vm.plots == nil {
		return []PlotDelta{}, next
	}
	var out []PlotDelta
	for i, p := range vm.plots.plots {
		for j, pen := range p.pens {
			key := fmt.Sprintf("%d:%d", i, j)
			c, ok := cursors[key]
			d, ncur := pen.delta(c, ok)
			d.PlotIndex = i
			d.PenIndex = j
			next[key] = ncur
			if d.Reset || len(d.Points) > 0 {
				out = append(out, d)
			}
		}
	}
	if out == nil {
		out = []PlotDelta{}
	}
	return out, next
}

func (pen *plotPen) delta(c PlotCursor, ok bool) (PlotDelta, PlotCursor) {
	next := PlotCursor{Epoch: pen.epoch, Seq: pen.seq}
	full := PlotDelta{
		Epoch: pen.epoch, Seq: pen.seq, Reset: true,
		Points: append([]PlotPoint(nil), pen.points...),
	}
	if full.Points == nil {
		full.Points = []PlotPoint{}
	}
	if !ok || c.Epoch != pen.epoch || c.Seq > pen.seq {
		return full, next
	}
	if c.Seq == pen.seq {
		return PlotDelta{Epoch: pen.epoch, Seq: pen.seq, Points: []PlotPoint{}}, next
	}
	n := len(pen.points)
	if n == 0 {
		return full, next
	}
	first := pen.seq - n + 1
	if c.Seq+1 < first {
		return full, next
	}
	idx := c.Seq - first + 1
	if idx < 0 {
		idx = 0
	}
	if idx > n {
		idx = n
	}
	pts := append([]PlotPoint(nil), pen.points[idx:]...)
	if pts == nil {
		pts = []PlotPoint{}
	}
	return PlotDelta{Epoch: pen.epoch, Seq: pen.seq, Points: pts}, next
}

// PlotSnapshot copies every retained sample for an in-process widget.
func (vm *VM) PlotSnapshot() []PlotSnap {
	if vm.plots == nil || len(vm.plots.plots) == 0 {
		return nil
	}
	out := make([]PlotSnap, len(vm.plots.plots))
	for i, p := range vm.plots.plots {
		pens := make([]PlotPenSnap, len(p.pens))
		for j, pen := range p.pens {
			pens[j] = PlotPenSnap{
				Name: pen.name, R: pen.r, G: pen.g, B: pen.b,
				Mode: penModeName(pen.mode), Interval: pen.interval,
				Points: append([]PlotPoint(nil), pen.points...),
			}
		}
		out[i] = PlotSnap{
			Name: p.name, XLabel: p.xLabel, YLabel: p.yLabel,
			XMin: p.xMin, XMax: p.xMax, YMin: p.yMin, YMax: p.yMax,
			AutoPlot: p.auto, Legend: p.legend, Pens: pens,
		}
	}
	return out
}

// Plots returns the live plot list for tests. The slice is a snapshot of
// the plot headers; point slices alias the pen until the next plot command.
func (vm *VM) Plots() []PlotView {
	return vm.PlotViews()
}

// PlotPoints returns a copy of one pen's samples. ok is false when the
// plot or pen is missing.
func (vm *VM) PlotPoints(plotName, penName string) ([]PlotPoint, bool) {
	if vm.plots == nil {
		return nil, false
	}
	for _, p := range vm.plots.plots {
		if !strings.EqualFold(p.name, plotName) {
			continue
		}
		for _, pen := range p.pens {
			if strings.EqualFold(pen.name, penName) {
				return append([]PlotPoint(nil), pen.points...), true
			}
		}
		return nil, false
	}
	return nil, false
}

func penModeName(mode int) string {
	switch mode {
	case penBar:
		return "bar"
	case penPoint:
		return "point"
	default:
		return "line"
	}
}

func plotNum(v Value, what string) (float64, error) {
	n, err := v.Num()
	if err != nil {
		return 0, fmt.Errorf("%s expected a number, got %s", what, v.TypeName())
	}
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, fmt.Errorf("%s expected a finite number", what)
	}
	return n, nil
}

func unpackPenColor(c int32) (r, g, b uint8) {
	u := uint32(c)
	// A small non-negative value with no alpha byte is a NetLogo color
	// number. Widget files store Java ARGB, whose high byte is 255.
	if u>>24 == 0 && c >= 0 && float64(c) < maxColor {
		rgb := ColorRGBA(float64(c))
		return rgb.R, rgb.G, rgb.B
	}
	return uint8(u >> 16), uint8(u >> 8), uint8(u)
}
