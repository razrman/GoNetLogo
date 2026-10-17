package nl

import (
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func loadModel(t *testing.T, name string) *FileModel {
	t.Helper()
	m, err := LoadFile(filepath.Join("..", "..", "models", name))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestLoadWidgets(t *testing.T) {
	m := loadModel(t, "fire.nlogo")
	if m.View.MinPxcor != -12 || m.View.MaxPxcor != 12 || m.View.WrapX || m.View.WrapY || m.View.PatchSize != 12 {
		t.Fatalf("view %+v", m.View)
	}
	fireVM, err := m.NewVM(1)
	if err != nil {
		t.Fatal(err)
	}
	fw := fireVM.World
	if fw.MinPxcor != -12 || fw.MaxPxcor != 12 || fw.MinPycor != -12 || fw.MaxPycor != 12 || fw.WrapX || fw.WrapY || fw.PatchSize != 12 {
		t.Fatalf("fire world %+v", fw)
	}
	density := false
	for _, w := range m.Widgets {
		if w.Kind == "slider" && w.Variable == "density" {
			density = true
			if w.Value != 60 || w.Min != 0 || w.Max != 100 {
				t.Fatalf("density slider %+v", w)
			}
		}
	}
	if !density {
		t.Fatal("missing density slider")
	}
	life := loadModel(t, "life.nlogo")
	if !life.View.WrapX || !life.View.WrapY {
		t.Fatal("life should wrap")
	}
	sheep := loadModel(t, "wolfsheep.nlogo")
	var grass *Widget
	for i := range sheep.Widgets {
		if sheep.Widgets[i].Variable == "grass?" {
			grass = &sheep.Widgets[i]
		}
	}
	if grass == nil || !grass.On {
		t.Fatalf("switch polarity: %+v", grass)
	}
}

func TestNLogoXRoundTripShape(t *testing.T) {
	src := `<?xml version="1.0"?>
<model version="NetLogo 7.0.0">
  <code><![CDATA[
globals [density]
to setup
  clear-all
  reset-ticks
end
to go
  tick
end
]]></code>
  <widgets>
    <view minPxcor="-3" maxPxcor="3" minPycor="-4" maxPycor="4" patchSize="9" wrappingAllowedX="false" wrappingAllowedY="true" fontSize="10" updateMode="1" showTickCounter="true" frameRate="30"/>
    <slider variable="density" min="0" max="99" default="40" step="1" direction="Horizontal"/>
    <switch variable="go?" on="false"/>
    <button forever="true" kind="Observer" disableUntilTicks="true">go</button>
    <monitor display="ticks" precision="1" fontSize="11">ticks</monitor>
  </widgets>
</model>`
	m, err := ParseModel("tiny.nlogox", src)
	if err != nil {
		t.Fatal(err)
	}
	if m.View.MinPxcor != -3 || m.View.MaxPycor != 4 || m.View.WrapX || !m.View.WrapY || m.View.PatchSize != 9 {
		t.Fatalf("view %+v", m.View)
	}
	vm, err := m.NewVM(1)
	if err != nil {
		t.Fatal(err)
	}
	if vm.World.MinPxcor != -3 || vm.World.MaxPxcor != 3 || vm.World.MinPycor != -4 || vm.World.MaxPycor != 4 || vm.World.WrapX || !vm.World.WrapY || vm.World.PatchSize != 9 {
		t.Fatalf("applied world min %d max %d py %d %d wrap %v %v size %v", vm.World.MinPxcor, vm.World.MaxPxcor, vm.World.MinPycor, vm.World.MaxPycor, vm.World.WrapX, vm.World.WrapY, vm.World.PatchSize)
	}
	if got := mustEval(t, vm, "count patches"); got != "63" {
		t.Fatalf("count patches = %s, want 63", got)
	}
	if got, _ := vm.World.Global("density"); got.N != 40 {
		t.Fatalf("density %v", got)
	}
	if got, _ := vm.World.Global("go?"); got.K != KindBool || got.B {
		t.Fatalf("switch %v", got)
	}
	if _, err := vm.ExecuteLine("setup"); err != nil {
		t.Fatal(err)
	}
}

func TestLifeBlinker(t *testing.T) {
	m := loadModel(t, "life.nlogo")
	vm, err := m.NewVM(1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := vm.Call("setup-blinker"); err != nil {
		t.Fatal(err)
	}
	live := func() string { return mustEval(t, vm, "count patches with [ living? ]") }
	if live() != "3" {
		t.Fatalf("blinker start %s", live())
	}
	// vertical bar becomes a horizontal bar
	if _, err := vm.Call("go"); err != nil {
		t.Fatal(err)
	}
	if got := mustEval(t, vm, "[ living? ] of patch 1 0"); got != "true" {
		t.Fatalf("expected birth at 1 0, got %s", got)
	}
	if got := mustEval(t, vm, "[ living? ] of patch 0 1"); got != "false" {
		t.Fatalf("expected death at 0 1, got %s", got)
	}
	if live() != "3" {
		t.Fatalf("still 3 cells, got %s", live())
	}
	if _, err := vm.Call("go"); err != nil {
		t.Fatal(err)
	}
	if got := mustEval(t, vm, "[ living? ] of patch 0 1"); got != "true" {
		t.Fatalf("period 2 failed: %s", got)
	}
	if got := mustEval(t, vm, "[ living? ] of patch 1 0"); got != "false" {
		t.Fatalf("period 2 side cell: %s", got)
	}
}

func TestRandomAlwaysUnder100(t *testing.T) {
	vm := newTestVM(t, `
globals [ density misses ]
to setup
  clear-all
  set density 100
  set misses 0
  ask patches [
    ifelse random 100 < density [
      set pcolor green
    ] [
      set misses misses + 1
    ]
  ]
end
`, -3, 3, false, 7)
	mustCall(t, vm, "setup")
	if mustEval(t, vm, "misses") != "0" {
		t.Fatal("random 100 was not always < 100")
	}
}

func TestFireBurnsAFullForest(t *testing.T) {
	m := loadModel(t, "fire.nlogo")
	for i := range m.Widgets {
		if m.Widgets[i].Variable == "density" {
			m.Widgets[i].Value = 100
		}
	}
	// A 7x7 forest is enough to prove the spread rule and stays fast.
	m.View.MinPxcor, m.View.MaxPxcor = -3, 3
	m.View.MinPycor, m.View.MaxPycor = -3, 3
	vm, err := m.NewVM(7)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := vm.Call("setup"); err != nil {
		t.Fatal(err)
	}
	patches := (3 - -3 + 1) * (3 - -3 + 1)
	stopped := false
	for i := 0; i < 80 && !stopped; i++ {
		stopped, err = vm.Call("go")
		if err != nil {
			t.Fatal(err)
		}
	}
	if !stopped {
		t.Fatal("fire did not burn out")
	}
	if got := mustEval(t, vm, "count patches with [ pcolor = green ]"); got != "0" {
		t.Fatalf("green left: %s", got)
	}
	if got := mustEval(t, vm, "count turtles"); got != "0" {
		t.Fatalf("turtles left: %s", got)
	}
	if got := mustEval(t, vm, "burned-trees"); got != itoa(patches) {
		t.Fatalf("burned %s want %d", got, patches)
	}
}

func TestTermitesConserveChips(t *testing.T) {
	m := loadModel(t, "termites.nlogo")
	vm, err := m.NewVM(3)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := vm.Call("setup"); err != nil {
		t.Fatal(err)
	}
	start := mustEval(t, vm, "chip-count")
	for i := 0; i < 25; i++ {
		if _, err := vm.Call("go"); err != nil {
			t.Fatal(err)
		}
		if got := mustEval(t, vm, "chip-count"); got != start {
			t.Fatalf("tick %d chips %s want %s", i, got, start)
		}
	}
	if mustEval(t, vm, "count turtles") == "0" {
		t.Fatal("termites vanished")
	}
}

// TestWolfSheepConcurrentRender runs the model while other goroutines paint.
// Reading the live turtle map from RenderWorld used to fatal on the desktop
// draw thread (concurrent map iteration during hatch/die).
func TestWolfSheepConcurrentRender(t *testing.T) {
	m := loadModel(t, "wolfsheep.nlogo")
	vm, err := m.NewVM(1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := vm.Call("setup"); err != nil {
		t.Fatal(err)
	}
	side := vm.World.Width()
	if side < 2 || vm.World.Height() != side {
		t.Fatalf("world %dx%d", side, vm.World.Height())
	}
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			if _, err := vm.Call("go"); err != nil {
				t.Errorf("go: %v", err)
				return
			}
		}
	}()
	for i := 0; i < 2; i++ {
		wg.Add(1)
		px := 2 + i
		go func(px int) {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				img := RenderWorld(vm.World, px)
				if img.Bounds().Dx() != side*px || img.Bounds().Dy() != side*px {
					t.Errorf("frame %v with patch %d", img.Bounds(), px)
					return
				}
			}
		}(px)
	}
	time.Sleep(300 * time.Millisecond)
	close(stop)
	wg.Wait()
}

func TestWolfSheepSteps(t *testing.T) {
	m := loadModel(t, "wolfsheep.nlogo")
	vm, err := m.NewVM(5)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := vm.Call("setup"); err != nil {
		t.Fatal(err)
	}
	sheep := mustEval(t, vm, "count sheep")
	wolves := mustEval(t, vm, "count wolves")
	if sheep == "0" || wolves == "0" {
		t.Fatalf("setup populations sheep %s wolves %s", sheep, wolves)
	}
	for i := 0; i < 15; i++ {
		if _, err := vm.Call("go"); err != nil {
			t.Fatalf("go %d: %v", i, err)
		}
	}
	if _, err := vm.World.Ticks(); err != nil {
		t.Fatal(err)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [16]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
