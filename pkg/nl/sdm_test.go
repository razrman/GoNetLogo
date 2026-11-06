package nl

import (
	"math"
	"strings"
	"testing"
)

func TestIsSDConstant(t *testing.T) {
	yes := []string{"1", "0.1", ".04", "3.0E-4", ".8", "true", "FALSE", "nobody", `"hi"`, "[1 2]", "[]", "[1 [2 nobody]]", "  0.6  ", ";; note\n0.6"}
	no := []string{"", "growth-rate-slider", "stock * 2", "sheep-reproduce / 100", "initial-number-sheep ;; slider", "1 2", "[1 sheep]"}
	for _, expr := range yes {
		if !isSDConstant(expr) {
			t.Errorf("want constant: %q", expr)
		}
	}
	for _, expr := range no {
		if isSDConstant(expr) {
			t.Errorf("want non-constant: %q", expr)
		}
	}
}

func TestSystemDynamicsNLogoSection(t *testing.T) {
	diagram := `<systemDynamics dt="1.0">
  <stock name="zz" allowNegative="true">1</stock>
  <stock name="aa" allowNegative="false">zz</stock>
  <stock name="s" allowNegative="true">k</stock>
  <binding startFigure="0" endFigure="1"></binding>
  <reservoir></reservoir>
  <rate name="out" bivalent="false" startFigure="1" endFigure="4">100</rate>
  <converter name="k">4</converter>
</systemDynamics>`
	// The info tab also contains a systemDynamics tag. Only section 6 is the diagram.
	parts := []string{
		"to setup\n  clear-all\n  system-dynamics-setup\nend\nto go\n  system-dynamics-go\nend\n",
		"\n",
		"\nThe info mentions <systemDynamics dt=\"9\"></systemDynamics> and is not the diagram.\n",
		"\n",
		"\nNetLogo 6.4.0\n",
		"\n",
		"\n" + diagram + "\n",
		"\n",
	}
	m, err := ParseModel("diagram.nlogo", strings.Join(parts, sectionSep))
	if err != nil {
		t.Fatal(err)
	}
	sd := m.Dynamics
	if sd == nil {
		t.Fatal("missing system dynamics section")
	}
	if sd.DT != "1.0" {
		t.Fatalf("dt %q", sd.DT)
	}
	if len(sd.Stocks) != 3 || len(sd.Rates) != 1 || len(sd.Converters) != 1 {
		t.Fatalf("stocks %d rates %d converters %d", len(sd.Stocks), len(sd.Rates), len(sd.Converters))
	}
	if sd.Rates[0].Name != "out" || sd.Rates[0].Source != "aa" || sd.Rates[0].Sink != "" {
		t.Fatalf("rate %+v", sd.Rates[0])
	}
	aa := stockByName(t, sd, "aa")
	if aa.AllowNegative || strings.TrimSpace(aa.Initial) != "zz" {
		t.Fatalf("aa %+v", aa)
	}
	if !sd.Converters[0].Constant || sd.Converters[0].Name != "k" {
		t.Fatalf("converter %+v", sd.Converters[0])
	}
	vm, err := m.NewVM(1)
	if err != nil {
		t.Fatalf("compile: %v\n%s", err, sd.Source())
	}
	if vm.Prog.Proc("k") != nil {
		t.Fatal("constant converter was compiled as a procedure")
	}
	if p := vm.Prog.Proc("out"); p == nil || !p.IsReporter {
		t.Fatal("flow reporter missing")
	}
	if p := vm.Prog.Proc("system-dynamics-setup"); p == nil || p.IsReporter {
		t.Fatal("system-dynamics-setup missing")
	}
	mustCall(t, vm, "setup")
	// Literal stock zz is initialized before aa, whose initial value reads zz.
	// Constant converter k is set before any stock.
	if got := numGlobal(t, vm, "zz"); got != 1 {
		t.Fatalf("zz = %v", got)
	}
	if got := numGlobal(t, vm, "aa"); got != 1 {
		t.Fatalf("aa = %v, want 1 (zz already set)", got)
	}
	if got := numGlobal(t, vm, "s"); got != 4 {
		t.Fatalf("s = %v, want 4 (constant k)", got)
	}
	if got := numGlobal(t, vm, "k"); got != 4 {
		t.Fatalf("k = %v", got)
	}
	if got := numGlobal(t, vm, "dt"); got != 1 {
		t.Fatalf("dt = %v", got)
	}
	mustCall(t, vm, "system-dynamics-do-plot")
	mustCall(t, vm, "go")
	if got := numGlobal(t, vm, "aa"); got != 0 {
		t.Fatalf("aa = %v, want 0 (clamped)", got)
	}
	if got := numGlobal(t, vm, "zz"); got != 1 {
		t.Fatalf("zz = %v", got)
	}
	if got := numGlobal(t, vm, "s"); got != 4 {
		t.Fatalf("s = %v", got)
	}
	ticks, err := vm.World.Ticks()
	if err != nil {
		t.Fatal(err)
	}
	if ticks != 1 {
		t.Fatalf("ticks = %v", ticks)
	}
}

func TestSystemDynamicsTransfer(t *testing.T) {
	src := `<?xml version="1.0"?>
<model>
  <code><![CDATA[to setup
  clear-all
  system-dynamics-setup
end
to go
  system-dynamics-go
end
]]></code>
  <systemDynamics dt="1">
    <stock name="a" allowNegative="true">10</stock>
    <stock name="b" allowNegative="true">1</stock>
    <rate name="move" bivalent="false" startFigure="0" endFigure="1">4</rate>
  </systemDynamics>
</model>`
	m, err := ParseModel("transfer.nlogox", src)
	if err != nil {
		t.Fatal(err)
	}
	if m.Dynamics == nil || m.Dynamics.Rates[0].Source != "a" || m.Dynamics.Rates[0].Sink != "b" {
		t.Fatalf("rate %+v", m.Dynamics)
	}
	vm, err := m.NewVM(1)
	if err != nil {
		t.Fatal(err)
	}
	mustCall(t, vm, "setup")
	mustCall(t, vm, "go")
	if got := numGlobal(t, vm, "a"); got != 6 {
		t.Fatalf("a = %v, want 6", got)
	}
	if got := numGlobal(t, vm, "b"); got != 5 {
		t.Fatalf("b = %v, want 5", got)
	}
}

func TestNoSystemDynamicsProcedures(t *testing.T) {
	m, err := ParseModel("plain.nlogo", "to setup\n  clear-all\nend\n"+sectionSep+"\n")
	if err != nil {
		t.Fatal(err)
	}
	if m.Dynamics != nil {
		t.Fatal("plain model has a diagram")
	}
	prog, err := m.Compile()
	if err != nil {
		t.Fatal(err)
	}
	if prog.Proc("system-dynamics-setup") != nil {
		t.Fatal("plain model defined system-dynamics-setup")
	}
}

func TestExponentialGrowth(t *testing.T) {
	m := loadModel(t, "library/Sample Models/System Dynamics/Exponential Growth.nlogox")
	sd := m.Dynamics
	if sd == nil || sd.DT != "0.001" {
		t.Fatalf("dynamics %+v", sd)
	}
	if len(sd.Stocks) != 1 || sd.Stocks[0].Name != "stock" || strings.TrimSpace(sd.Stocks[0].Initial) != "1" || !sd.Stocks[0].AllowNegative {
		t.Fatalf("stock %+v", sd.Stocks)
	}
	growth, ok := converterByName(sd, "growth-rate")
	if !ok || growth.Constant || strings.TrimSpace(growth.Expression) != "growth-rate-slider" {
		t.Fatalf("growth-rate %+v", growth)
	}
	in, ok := rateByName(sd, "inflow")
	if !ok || in.Source != "" || in.Sink != "stock" {
		t.Fatalf("inflow %+v", in)
	}
	vm, err := m.NewVM(1)
	if err != nil {
		t.Fatalf("compile: %v\n%s", err, sd.Source())
	}
	if p := vm.Prog.Proc("growth-rate"); p == nil || !p.IsReporter {
		t.Fatal("growth-rate reporter missing")
	}
	mustCall(t, vm, "setup")
	if got := numGlobal(t, vm, "stock"); got != 1 {
		t.Fatalf("stock after setup %v", got)
	}
	if ticks, err := vm.World.Ticks(); err != nil || ticks != 0 {
		t.Fatalf("ticks after setup %v %v", ticks, err)
	}
	stock := 1.0
	const dt = 0.001
	const g = 0.1
	for i := 0; i < 3; i++ {
		mustCall(t, vm, "go")
		stock = stock + (stock*g)*dt
	}
	if got := numGlobal(t, vm, "stock"); math.Abs(got-stock) > 1e-12 {
		t.Fatalf("stock %v want %v", got, stock)
	}
	ticks, err := vm.World.Ticks()
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(ticks-0.003) > 1e-12 {
		t.Fatalf("ticks %v", ticks)
	}
}

func TestWolfSheepSystemDynamics(t *testing.T) {
	m := loadModel(t, "library/Sample Models/System Dynamics/Wolf Sheep Predation (System Dynamics).nlogox")
	vm, err := m.NewVM(1)
	if err != nil {
		t.Fatal(err)
	}
	if vm.Prog.Proc("sheep-birth-rate") != nil {
		t.Fatal("constant sheep-birth-rate compiled as a procedure")
	}
	mustCall(t, vm, "setup")
	if got := numGlobal(t, vm, "sheep"); got != 100 {
		t.Fatalf("sheep %v", got)
	}
	if got := numGlobal(t, vm, "wolves"); got != 30 {
		t.Fatalf("wolves %v", got)
	}
	if got := numGlobal(t, vm, "sheep-birth-rate"); got != 0.04 {
		t.Fatalf("sheep-birth-rate %v", got)
	}
	if got := numGlobal(t, vm, "predation-rate"); math.Abs(got-3.0e-4) > 1e-15 {
		t.Fatalf("predation-rate %v", got)
	}
	mustCall(t, vm, "go")
	const dt = 0.001
	const sheep0 = 100.0
	const wolves0 = 30.0
	br := numGlobal(t, vm, "sheep-birth-rate")
	pr := numGlobal(t, vm, "predation-rate")
	eff := numGlobal(t, vm, "predator-efficiency")
	wdr := numGlobal(t, vm, "wolf-death-rate")
	sheep := sheep0 + ((br * sheep0) * dt) - (((sheep0 * pr) * wolves0) * dt)
	wolves := wolves0 + ((((wolves0 * eff) * pr) * sheep0) * dt) - ((wolves0 * wdr) * dt)
	if got := numGlobal(t, vm, "sheep"); math.Abs(got-sheep) > 1e-9 {
		t.Fatalf("sheep %v want %v", got, sheep)
	}
	if got := numGlobal(t, vm, "wolves"); math.Abs(got-wolves) > 1e-9 {
		t.Fatalf("wolves %v want %v", got, wolves)
	}
	if wolves >= wolves0 {
		t.Fatalf("wolves did not decline: %v", wolves)
	}
}

func TestSystemDynamicsLibraryModels(t *testing.T) {
	models := []string{
		"library/Sample Models/System Dynamics/Exponential Growth.nlogox",
		"library/Sample Models/System Dynamics/Logistic Growth.nlogox",
		"library/Sample Models/System Dynamics/Wolf Sheep Predation (System Dynamics).nlogox",
		"library/Sample Models/System Dynamics/Wolf Sheep Predation (Docked Hybrid).nlogox",
		"library/Sample Models/System Dynamics/Unverified/Tabonuco Yagrumo.nlogox",
		"library/Sample Models/System Dynamics/Unverified/Tabonuco Yagrumo Hybrid.nlogox",
	}
	for _, name := range models {
		t.Run(name, func(t *testing.T) {
			m := loadModel(t, name)
			if m.Dynamics == nil {
				t.Fatal("no system dynamics diagram")
			}
			vm, err := m.NewVM(1)
			if err != nil {
				t.Fatalf("compile: %v\n%s", err, m.Dynamics.Source())
			}
			if vm.Prog.Proc("setup") != nil {
				if _, err := vm.Call("setup"); err != nil {
					t.Fatalf("setup: %v", err)
				}
			}
			if vm.Prog.Proc("go") != nil {
				for i := 0; i < 3; i++ {
					if _, err := vm.Call("go"); err != nil {
						t.Fatalf("go %d: %v", i, err)
					}
				}
			}
		})
	}
}

func TestDockedHybridAggregate(t *testing.T) {
	m := loadModel(t, "library/Sample Models/System Dynamics/Wolf Sheep Predation (Docked Hybrid).nlogox")
	vm, err := m.NewVM(1)
	if err != nil {
		t.Fatalf("compile: %v\n%s", err, m.Dynamics.Source())
	}
	mustCall(t, vm, "setup-aggregate")
	if got := numGlobal(t, vm, "sheepstock"); got != 148 {
		t.Fatalf("sheepStock %v", got)
	}
	if got := numGlobal(t, vm, "wolfstock"); got != 30 {
		t.Fatalf("wolfStock %v", got)
	}
	mustCall(t, vm, "system-dynamics-go")
	const (
		dt     = 0.001
		sheep0 = 148.0
		wolf0  = 30.0
		br     = 4.0 / 100
		pr     = 3.0e-4
		eff    = 1.0
		death  = 0.15
	)
	sheep := sheep0 + (sheep0*br)*dt - (sheep0*(pr*wolf0))*dt
	wolves := wolf0 + ((wolf0*(pr*eff))*sheep0)*dt - (wolf0*death)*dt
	if sheep < 0 {
		sheep = 0
	}
	if got := numGlobal(t, vm, "sheepstock"); math.Abs(got-sheep) > 1e-9 {
		t.Fatalf("sheepStock %v want %v", got, sheep)
	}
	if got := numGlobal(t, vm, "wolfstock"); math.Abs(got-wolves) > 1e-9 {
		t.Fatalf("wolfStock %v want %v", got, wolves)
	}
}

func TestTabonucoStocksAndLogistic(t *testing.T) {
	tab := loadModel(t, "library/Sample Models/System Dynamics/Unverified/Tabonuco Yagrumo.nlogox")
	vm, err := tab.NewVM(1)
	if err != nil {
		t.Fatal(err)
	}
	mustCall(t, vm, "setup")
	if got := numGlobal(t, vm, "gaps"); got != 0.6 {
		t.Fatalf("gaps %v", got)
	}
	if got := numGlobal(t, vm, "tabonuco"); got != 0.2 {
		t.Fatalf("tabonuco %v", got)
	}
	if got := numGlobal(t, vm, "yagrumo"); got != 0.2 {
		t.Fatalf("yagrumo %v", got)
	}
	if got := numGlobal(t, vm, "dt"); got != 1 {
		t.Fatalf("dt %v", got)
	}

	logm := loadModel(t, "library/Sample Models/System Dynamics/Logistic Growth.nlogox")
	lvm, err := logm.NewVM(1)
	if err != nil {
		t.Fatal(err)
	}
	mustCall(t, lvm, "setup")
	stock := 0.1
	const dt = 0.01
	for i := 0; i < 3; i++ {
		mustCall(t, lvm, "go")
		stock = stock + (stock*(1-stock))*dt
	}
	if got := numGlobal(t, lvm, "stock"); math.Abs(got-stock) > 1e-12 {
		t.Fatalf("logistic stock %v want %v", got, stock)
	}
}

func stockByName(t *testing.T, sd *SystemDynamics, name string) SDStock {
	t.Helper()
	for _, s := range sd.Stocks {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("no stock %s", name)
	return SDStock{}
}

func converterByName(sd *SystemDynamics, name string) (SDConverter, bool) {
	for _, c := range sd.Converters {
		if c.Name == name {
			return c, true
		}
	}
	return SDConverter{}, false
}

func rateByName(sd *SystemDynamics, name string) (SDRate, bool) {
	for _, r := range sd.Rates {
		if r.Name == name {
			return r, true
		}
	}
	return SDRate{}, false
}
