package nl

import (
	"testing"
)

func TestWolfSheepBenchDeterministic(t *testing.T) {
	m := loadModel(t, "wolfsheep.nlogo")
	a, err := BenchWolfSheep(m, 20, 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if a.Width != 101 || a.Height != 101 || a.PatchSize != 5 {
		t.Fatalf("world %dx%d patch %v", a.Width, a.Height, a.PatchSize)
	}
	b, err := BenchWolfSheep(m, 20, 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Points) != 1 || len(b.Points) != 1 {
		t.Fatalf("points %d %d", len(a.Points), len(b.Points))
	}
	if a.Points[0].Sheep != b.Points[0].Sheep || a.Points[0].Wolves != b.Points[0].Wolves || a.Points[0].Turtles != b.Points[0].Turtles {
		t.Fatalf("populations differ %+v %+v", a.Points[0], b.Points[0])
	}
	if a.Points[0].Sheep == 0 || a.Points[0].Wolves == 0 {
		t.Fatalf("empty population %+v", a.Points[0])
	}
}

func BenchmarkWolfSheep101(b *testing.B) {
	m := loadModel(b, "wolfsheep.nlogo")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := BenchWolfSheep(m, 50, 1, 50); err != nil {
			b.Fatal(err)
		}
	}
}
