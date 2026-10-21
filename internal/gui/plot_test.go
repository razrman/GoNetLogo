package gui

import (
	"image/color"
	"testing"

	"gonetlogo/pkg/nl"
)

func TestDrawPlotImageDrawsPenLine(t *testing.T) {
	img := drawPlotImage(nl.PlotSnap{
		Name: "counts", XMin: 0, XMax: 10, YMin: 0, YMax: 10,
		Pens: []nl.PlotPenSnap{{
			Name: "turtles", R: 0, G: 180, B: 40, Mode: "line", Interval: 1,
			Points: []nl.PlotPoint{{X: 0, Y: 0, Down: true}, {X: 10, Y: 10, Down: true}},
		}},
	}, 200, 120)
	if img.Bounds().Dx() != 200 || img.Bounds().Dy() != 120 {
		t.Fatalf("size %v", img.Bounds())
	}
	green := 0
	for y := 0; y < 120; y++ {
		for x := 0; x < 200; x++ {
			p := img.RGBAAt(x, y)
			if p.G > 140 && p.R < 40 {
				green++
			}
		}
	}
	if green < 20 {
		t.Fatalf("pen pixels = %d", green)
	}
	corner := img.RGBAAt(2, 2)
	if corner != (color.RGBA{255, 253, 248, 255}) {
		t.Fatalf("margin %+v", corner)
	}
}

func TestDrawPlotImageBreaksOnPenUp(t *testing.T) {
	img := drawPlotImage(nl.PlotSnap{
		Name: "trace", XMin: 0, XMax: 4, YMin: 0, YMax: 4, AutoPlot: false,
		Pens: []nl.PlotPenSnap{{
			Name: "wave", R: 200, G: 0, B: 0, Mode: "line", Interval: 1,
			Points: []nl.PlotPoint{
				{X: 0, Y: 0, Down: true},
				{X: 1, Y: 0, Down: false},
				{X: 2, Y: 4, Down: true},
				{X: 4, Y: 4, Down: true},
			},
		}},
	}, 180, 100)
	// A lone pen-down sample does not draw. The following down sample starts
	// a new segment along the top of the range.
	if img.RGBAAt(20, 90).R > 150 && img.RGBAAt(20, 90).G < 40 {
		t.Fatal("pen-up sample was drawn")
	}
}
