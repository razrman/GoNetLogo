package nl

import (
	"strings"
	"testing"
)

func TestParsePlotWidget(t *testing.T) {
	src := `to setup
  clear-all
  reset-ticks
end
@#$#@#$#@
PLOT
11
357
312
557
Best Fitness
Generation
Best Fitness
0.0
5.0
-30.0
1.0
true
false
"" ""
PENS
"best" 1.0 0 -16777216 true "" "plot best-fitness"
"other" 2.0 1 -65536 false "plot 1" "plot 2"
`
	m, err := ParseModel("robby.nlogo", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Notes) != 0 {
		t.Fatalf("notes %v", m.Notes)
	}
	var w *Widget
	for i := range m.Widgets {
		if m.Widgets[i].Kind == "plot" {
			w = &m.Widgets[i]
		}
	}
	if w == nil {
		t.Fatal("no plot widget")
	}
	if w.Display != "Best Fitness" || w.XLabel != "Generation" || w.YLabel != "Best Fitness" {
		t.Fatalf("labels %+v", w)
	}
	if w.X != 11 || w.Y != 357 || w.W != 301 || w.H != 200 {
		t.Fatalf("box %+v", w)
	}
	if w.XMin != 0 || w.XMax != 5 || w.YMin != -30 || w.YMax != 1 || !w.AutoPlot || w.Legend {
		t.Fatalf("ranges %+v", w)
	}
	if len(w.Pens) != 2 {
		t.Fatalf("pens %+v", w.Pens)
	}
	best := w.Pens[0]
	if best.Name != "best" || best.Interval != 1 || best.Mode != 0 || best.Color != -16777216 || !best.InLegend || best.Setup != "" || best.Update != "plot best-fitness" {
		t.Fatalf("best %+v", best)
	}
	other := w.Pens[1]
	if other.Name != "other" || other.Interval != 2 || other.Mode != 1 || other.Color != -65536 || other.InLegend || other.Setup != "plot 1" || other.Update != "plot 2" {
		t.Fatalf("other %+v", other)
	}
	r, g, b := unpackPenColor(best.Color)
	if r != 0 || g != 0 || b != 0 {
		t.Fatalf("black rgb %d %d %d", r, g, b)
	}
	r, g, b = unpackPenColor(other.Color)
	if r != 255 || g != 0 || b != 0 {
		t.Fatalf("red rgb %d %d %d", r, g, b)
	}
}

func TestParsePlotWidgetXML(t *testing.T) {
	src := `<?xml version="1.0"?>
<model version="NetLogo 7.0.0">
  <code><![CDATA[to setup clear-all reset-ticks end]]></code>
  <widgets>
    <plot display="counts" x="4" y="8" width="200" height="120" xLabel="ticks" yLabel="n" xMin="0" xMax="10" yMin="0" yMax="4" autoPlot="false" legend="true">
      <pen name="turtles" interval="1" mode="0" color="-16776961" legend="true">
        <update>plot count turtles</update>
      </pen>
    </plot>
  </widgets>
</model>`
	m, err := ParseModel("counts.nlogox", src)
	if err != nil {
		t.Fatal(err)
	}
	var w *Widget
	for i := range m.Widgets {
		if m.Widgets[i].Kind == "plot" {
			w = &m.Widgets[i]
		}
	}
	if w == nil {
		t.Fatal("no plot")
	}
	if w.Display != "counts" || w.W != 200 || w.H != 120 || w.AutoPlot || !w.Legend || w.XLabel != "ticks" {
		t.Fatalf("widget %+v", w)
	}
	if len(w.Pens) != 1 || w.Pens[0].Update != "plot count turtles" || w.Pens[0].Color != -16776961 {
		t.Fatalf("pen %+v", w.Pens)
	}
}

func TestPlotAdvancesAndPenUp(t *testing.T) {
	vm := plotVM(t, onePlot("p", true, 0, 10, 0, 10, penSpec("default", "")))
	mustRun(t, vm, "plot 1")
	mustRun(t, vm, "plot 3")
	mustRun(t, vm, "plot-pen-up")
	mustRun(t, vm, "plot 30")
	mustRun(t, vm, "plot-pen-down")
	mustRun(t, vm, "plot 4")
	pts, ok := vm.PlotPoints("p", "default")
	if !ok {
		t.Fatal("missing pen")
	}
	want := []PlotPoint{{0, 1, true}, {1, 3, true}, {2, 30, false}, {3, 4, true}}
	if !samePoints(pts, want) {
		t.Fatalf("points %+v", pts)
	}
	view := mustPlot(t, vm, "p")
	if view.YMax != 10 {
		t.Fatalf("pen-up sample grew the range: %+v", view)
	}
}

func TestPlotXYSetsPenX(t *testing.T) {
	vm := plotVM(t, onePlot("p", true, 0, 10, 0, 10, penSpec("default", "")))
	mustRun(t, vm, "plot-pen-up")
	mustRun(t, vm, "plotxy -1 0")
	mustRun(t, vm, "plot 5")
	mustRun(t, vm, "plot-pen-down")
	mustRun(t, vm, "plot 6")
	pts, _ := vm.PlotPoints("p", "default")
	want := []PlotPoint{{-1, 0, false}, {0, 5, false}, {1, 6, true}}
	if !samePoints(pts, want) {
		t.Fatalf("points %+v", pts)
	}
}

func TestRobbyPlotSequence(t *testing.T) {
	code := `globals [ best-fitness ]
to setup
  clear-all
  reset-ticks
  set best-fitness 12.4
  plot-pen-up
  plotxy -1 0
  set-plot-y-range precision best-fitness 0 precision best-fitness 0 + 3
  plot best-fitness
  plot-pen-down
end
to go
  set best-fitness best-fitness + 1
  tick
end
`
	spec := onePlot("Best Fitness", true, 0, 5, -30, 1, PlotPenSpec{
		Name: "best", Interval: 1, Color: defaultPenColor, InLegend: true, Update: "plot best-fitness",
	})
	vm := plotProgram(t, code, spec)
	if _, err := vm.Call("setup"); err != nil {
		t.Fatal(err)
	}
	pts, _ := vm.PlotPoints("Best Fitness", "best")
	// reset-ticks plots the cleared global (0) with the pen down, then the
	// explicit pen-up plotxy / plot place generation 0 at x = 0.
	want := []PlotPoint{{0, 0, true}, {-1, 0, false}, {0, 12.4, false}}
	if !samePoints(pts, want) {
		t.Fatalf("setup points %+v", pts)
	}
	view := mustPlot(t, vm, "Best Fitness")
	if view.YMin != 12 || view.YMax != 15 {
		t.Fatalf("y range %+v", view)
	}
	if !view.AutoPlot {
		t.Fatal("set-plot-y-range turned autoplot off")
	}
	if _, err := vm.Call("go"); err != nil {
		t.Fatal(err)
	}
	pts, _ = vm.PlotPoints("Best Fitness", "best")
	if len(pts) != 4 || pts[3].X != 1 || pts[3].Y != 13.4 || !pts[3].Down {
		t.Fatalf("after go %+v", pts)
	}
}

func TestAutoPlotAndFixedRange(t *testing.T) {
	vm := plotVM(t, onePlot("p", true, 0, 10, 0, 10, penSpec("default", "")))
	mustRun(t, vm, "plotxy 12 20")
	view := mustPlot(t, vm, "p")
	if view.XMax <= 12 || view.YMax <= 20 {
		t.Fatalf("autoplot did not grow %+v", view)
	}
	mustRun(t, vm, "auto-plot-off")
	mustRun(t, vm, "set-plot-x-range 0 4")
	mustRun(t, vm, "set-plot-y-range -2 2")
	mustRun(t, vm, "plotxy 100 100")
	view = mustPlot(t, vm, "p")
	if view.AutoPlot || view.XMin != 0 || view.XMax != 4 || view.YMin != -2 || view.YMax != 2 {
		t.Fatalf("fixed range %+v", view)
	}
	if _, err := vm.ExecuteLine("set-plot-y-range 3 3"); err == nil {
		t.Fatal("equal range accepted")
	}
}

func TestPlotSelectionAndClear(t *testing.T) {
	a := onePlot("alpha", true, 0, 10, 0, 10, penSpec("a", ""), penSpec("b", ""))
	b := onePlot("beta", false, 0, 1, 0, 1, penSpec("c", ""))
	vm := plotVM(t, a, b)
	// The last loaded plot is current.
	if _, err := vm.ExecuteLine("plot 1"); err != nil {
		t.Fatal(err)
	}
	if pts, ok := vm.PlotPoints("beta", "c"); !ok || len(pts) != 1 {
		t.Fatalf("beta points %+v %v", pts, ok)
	}
	mustRun(t, vm, `set-current-plot "alpha"`)
	mustRun(t, vm, `set-current-plot-pen "b"`)
	mustRun(t, vm, "plot 2")
	mustRun(t, vm, "plot 3")
	mustRun(t, vm, "clear-plot")
	if pts, _ := vm.PlotPoints("alpha", "b"); len(pts) != 0 {
		t.Fatalf("clear-plot left %+v", pts)
	}
	mustRun(t, vm, "plot 4")
	mustRun(t, vm, "clear-all-plots")
	if pts, _ := vm.PlotPoints("beta", "c"); len(pts) != 0 {
		t.Fatalf("clear-all-plots left beta %+v", pts)
	}
	if pts, _ := vm.PlotPoints("alpha", "b"); len(pts) != 0 {
		t.Fatalf("clear-all-plots left alpha %+v", pts)
	}
	if _, err := vm.ExecuteLine(`set-current-plot "missing"`); err == nil || !strings.Contains(err.Error(), "no such plot") {
		t.Fatalf("missing plot: %v", err)
	}
	if _, err := vm.ExecuteLine(`set-current-plot-pen "nope"`); err == nil || !strings.Contains(err.Error(), "no pen") {
		t.Fatalf("missing pen: %v", err)
	}
}

func TestClearAllClearsPlots(t *testing.T) {
	vm := plotVM(t, onePlot("p", true, 0, 10, 0, 10, penSpec("default", "")))
	mustRun(t, vm, "plotxy 1 2")
	mustRun(t, vm, "clear-all")
	if pts, _ := vm.PlotPoints("p", "default"); len(pts) != 0 {
		t.Fatalf("points %+v", pts)
	}
}

func TestPenUpdateOnTick(t *testing.T) {
	code := `to setup
  clear-all
  create-turtles 2
  reset-ticks
end
to go
  tick
end
`
	spec := onePlot("p", true, 0, 10, 0, 10, PlotPenSpec{
		Name: "turtles", Interval: 1, Color: defaultPenColor, Update: "plot count turtles",
	})
	vm := plotProgram(t, code, spec)
	if _, err := vm.Call("setup"); err != nil {
		t.Fatal(err)
	}
	pts, _ := vm.PlotPoints("p", "turtles")
	if len(pts) != 1 || pts[0].X != 0 || pts[0].Y != 2 {
		t.Fatalf("setup %+v", pts)
	}
	if _, err := vm.Call("go"); err != nil {
		t.Fatal(err)
	}
	pts, _ = vm.PlotPoints("p", "turtles")
	if len(pts) != 2 || pts[1].X != 1 || pts[1].Y != 2 {
		t.Fatalf("tick %+v", pts)
	}
}

func TestPlotDeltasAreIncremental(t *testing.T) {
	vm := plotVM(t, onePlot("p", true, 0, 10, 0, 10, penSpec("default", "")))
	mustRun(t, vm, "plot 1")
	mustRun(t, vm, "plot 2")
	d1, cur := vm.PlotDeltas(nil)
	if len(d1) != 1 || !d1[0].Reset || len(d1[0].Points) != 2 || d1[0].Seq != 2 {
		t.Fatalf("first %+v", d1)
	}
	mustRun(t, vm, "plot 3")
	d2, cur := vm.PlotDeltas(cur)
	if len(d2) != 1 || d2[0].Reset || len(d2[0].Points) != 1 || d2[0].Points[0].Y != 3 || d2[0].Seq != 3 {
		t.Fatalf("second %+v", d2)
	}
	d3, _ := vm.PlotDeltas(cur)
	if len(d3) != 0 {
		t.Fatalf("idle %+v", d3)
	}
	mustRun(t, vm, "clear-plot")
	d4, cur := vm.PlotDeltas(cur)
	if len(d4) != 1 || !d4[0].Reset || len(d4[0].Points) != 0 || d4[0].Epoch == d1[0].Epoch {
		t.Fatalf("clear %+v", d4)
	}
	mustRun(t, vm, "plot 8")
	d5, _ := vm.PlotDeltas(cur)
	if len(d5) != 1 || d5[0].Reset || len(d5[0].Points) != 1 || d5[0].Points[0].Y != 8 {
		t.Fatalf("after clear %+v", d5)
	}
}

func TestNoCurrentPlot(t *testing.T) {
	vm := NewVM(mustParse(t, "to go end"), nil)
	if _, err := vm.ExecuteLine("plot 1"); err == nil || !strings.Contains(err.Error(), "no current plot") {
		t.Fatalf("got %v", err)
	}
}

func TestTemporaryPlotPen(t *testing.T) {
	vm := plotVM(t, onePlot("p", true, 0, 10, 0, 10, penSpec("default", "")))
	mustRun(t, vm, `create-temporary-plot-pen "temp"`)
	mustRun(t, vm, "plot 4")
	pts, ok := vm.PlotPoints("p", "temp")
	if !ok || len(pts) != 1 || pts[0].Y != 4 {
		t.Fatalf("temp %+v %v", pts, ok)
	}
	mustRun(t, vm, "clear-plot")
	if _, ok := vm.PlotPoints("p", "temp"); ok {
		t.Fatal("temporary pen survived clear-plot")
	}
}

func TestPlotModelFile(t *testing.T) {
	m := loadModel(t, "plot.nlogo")
	vm, err := m.NewVM(1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := vm.Call("setup"); err != nil {
		t.Fatal(err)
	}
	counts, ok := vm.PlotPoints("counts", "turtles")
	if !ok || len(counts) != 1 || counts[0].Y != 12 {
		t.Fatalf("counts %+v %v", counts, ok)
	}
	if _, err := vm.Call("go"); err != nil {
		t.Fatal(err)
	}
	counts, _ = vm.PlotPoints("counts", "turtles")
	near, ok := vm.PlotPoints("counts", "near")
	trace, tok := vm.PlotPoints("trace", "wave")
	if len(counts) != 2 || !ok || len(near) != 2 || !tok || len(trace) < 2 {
		t.Fatalf("counts %d near %d trace %d", len(counts), len(near), len(trace))
	}
	traceView := mustPlot(t, vm, "trace")
	if traceView.AutoPlot {
		t.Fatal("trace autoplot")
	}
	if traceView.XMax != 80 || traceView.YMin != -5 {
		t.Fatalf("trace range %+v", traceView)
	}
	countsView := mustPlot(t, vm, "counts")
	if !countsView.AutoPlot || len(countsView.Pens) != 2 {
		t.Fatalf("counts view %+v", countsView)
	}
}

func plotVM(t *testing.T, specs ...PlotSpec) *VM {
	t.Helper()
	return plotProgram(t, "to go end", specs...)
}

func plotProgram(t *testing.T, code string, specs ...PlotSpec) *VM {
	t.Helper()
	prog, err := Parse(code)
	if err != nil {
		t.Fatal(err)
	}
	vm := NewVM(prog, NewWorld(-5, 5, -5, 5, true, true))
	vm.InstallPlots(specs)
	return vm
}

func mustRun(t *testing.T, vm *VM, line string) {
	t.Helper()
	if _, err := vm.ExecuteLine(line); err != nil {
		t.Fatalf("%s: %v", line, err)
	}
}

func onePlot(name string, auto bool, xMin, xMax, yMin, yMax float64, pens ...PlotPenSpec) PlotSpec {
	return PlotSpec{
		Name: name, XMin: xMin, XMax: xMax, YMin: yMin, YMax: yMax, AutoPlot: auto, Pens: pens,
	}
}

func penSpec(name, update string) PlotPenSpec {
	return PlotPenSpec{Name: name, Interval: 1, Color: defaultPenColor, InLegend: true, Update: update}
}

func mustPlot(t *testing.T, vm *VM, name string) PlotView {
	t.Helper()
	for _, p := range vm.PlotViews() {
		if p.Name == name {
			return p
		}
	}
	t.Fatalf("plot %s missing in %+v", name, vm.PlotViews())
	return PlotView{}
}

func samePoints(got, want []PlotPoint) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i].X != want[i].X || got[i].Y != want[i].Y || got[i].Down != want[i].Down {
			return false
		}
	}
	return true
}
