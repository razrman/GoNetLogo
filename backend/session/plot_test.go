package session

import (
	"testing"

	"gonetlogo/backend/graph/model"
)

func TestPlotFramesStayIncremental(t *testing.T) {
	st := NewStore()
	t.Cleanup(st.Close)
	s, err := st.Create("plot.nlogo")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Setup(); err != nil {
		t.Fatal(err)
	}
	first, _, _, cur := s.FrameSince(-1, 0, nil)
	if len(first.Plots) != 2 {
		t.Fatalf("plots %+v", first.Plots)
	}
	if first.Plots[0].Name != "counts" || !first.Plots[0].AutoPlot || len(first.Plots[0].Pens) != 2 {
		t.Fatalf("counts %+v", first.Plots[0])
	}
	if first.Plots[0].Pens[0].Name != "turtles" || first.Plots[0].Pens[0].G == 0 {
		t.Fatalf("pen color %+v", first.Plots[0].Pens[0])
	}
	if first.Plots[1].Name != "trace" || first.Plots[1].AutoPlot {
		t.Fatalf("trace %+v", first.Plots[1])
	}
	turtles := penDelta(t, first.PlotAppend, 0, 0)
	if !turtles.Reset || len(turtles.Points) != 1 || turtles.Points[0].Y != 12 {
		t.Fatalf("setup delta %+v", turtles)
	}
	if err := s.Step(); err != nil {
		t.Fatal(err)
	}
	second, _, _, cur := s.FrameSince(0, 0, cur)
	again := penDelta(t, second.PlotAppend, 0, 0)
	if again.Reset || len(again.Points) != 1 || again.Seq != turtles.Seq+1 {
		t.Fatalf("step delta %+v (setup seq %d)", again, turtles.Seq)
	}
	third, _, _, _ := s.FrameSince(0, 0, cur)
	for _, d := range third.PlotAppend {
		if d.PlotIndex == 0 && d.PenIndex == 0 {
			t.Fatalf("idle frame repeated the turtles pen: %+v", d)
		}
	}
	full, _, _, _ := s.FrameSince(-1, 0, nil)
	all := penDelta(t, full.PlotAppend, 0, 0)
	if !all.Reset || len(all.Points) != 2 {
		t.Fatalf("fresh subscriber got %+v", all)
	}
}

func penDelta(t *testing.T, deltas []model.PlotPenDelta, plot, pen int) model.PlotPenDelta {
	t.Helper()
	for _, d := range deltas {
		if d.PlotIndex == plot && d.PenIndex == pen {
			return d
		}
	}
	t.Fatalf("no delta for plot %d pen %d in %+v", plot, pen, deltas)
	return model.PlotPenDelta{}
}
