package nl

import "testing"

func TestCurrentViewRefreshesOnce(t *testing.T) {
	m := loadModel(t, "wolfsheep.nlogo")
	vm, err := m.NewVM(1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := vm.Call("setup"); err != nil {
		t.Fatal(err)
	}
	first := vm.World.CurrentView()
	if first == nil || len(first.patches) != 101*101 {
		t.Fatalf("setup view patches %d", len(first.patches))
	}
	if again := vm.World.CurrentView(); again != first {
		t.Fatal("idle CurrentView copied the world again")
	}
	if _, err := vm.Call("go"); err != nil {
		t.Fatal(err)
	}
	second := vm.World.CurrentView()
	if second == first {
		t.Fatal("go left the previous frame in place")
	}
	if third := vm.World.CurrentView(); third != second {
		t.Fatal("second idle read copied the world again")
	}
	img := second.Paint(5)
	if img.Bounds().Dx() != 101*5 || img.Bounds().Dy() != 101*5 {
		t.Fatalf("paint %v", img.Bounds())
	}
}
