package nl

import "testing"

// BenchmarkRobbyGenerations times one compiled generation after setup.
// A generation is one GA step of the unmodified Robby model.
func BenchmarkRobbyGenerations(b *testing.B) {
	m := loadModel(b, "Robby the Robot.nlogo")
	vm, err := m.NewVM(1)
	if err != nil {
		b.Fatal(err)
	}
	if _, err := vm.Call("setup"); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := vm.Call("go"); err != nil {
			b.Fatal(err)
		}
	}
}
