package gui

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"gonetlogo/pkg/nl"
)

// plotView is one plot widget. The raster reads the latest snapshot, which
// the step loop replaces only when a frame is published.
type plotView struct {
	name   string
	raster *canvas.Raster
	box    fyne.CanvasObject
}

type plotSnapshot struct {
	Plots []nl.PlotSnap
}

func newPlotView(s *session, w nl.Widget) *plotView {
	name := w.Display
	h := w.H
	if h < 120 {
		h = 150
	}
	if h > 220 {
		h = 220
	}
	raster := canvas.NewRaster(func(width, height int) image.Image {
		return s.renderPlot(name, width, height)
	})
	raster.SetMinSize(fyne.NewSize(220, float32(h)))
	title := widget.NewLabel(name)
	title.TextStyle = fyne.TextStyle{Bold: true}
	return &plotView{
		name:   name,
		raster: raster,
		box:    container.NewVBox(title, raster),
	}
}

func (s *session) renderPlot(name string, w, h int) image.Image {
	if snap := s.plotSnap.Load(); snap != nil {
		for i := range snap.Plots {
			if strings.EqualFold(snap.Plots[i].Name, name) {
				return drawPlotImage(snap.Plots[i], w, h)
			}
		}
	}
	return drawPlotImage(nl.PlotSnap{Name: name, XMin: 0, XMax: 10, YMin: 0, YMax: 10}, w, h)
}

func (s *session) refreshPlotRasters() {
	for _, p := range s.plotViews {
		if p.raster != nil {
			p.raster.Refresh()
		}
	}
}

// drawPlotImage paints axes and pen samples. A pen-up sample breaks the
// line. The range is the plot's current range, already grown when autoplot
// is on.
func drawPlotImage(plot nl.PlotSnap, w, h int) *image.RGBA {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	bg := color.RGBA{255, 253, 248, 255}
	draw.Draw(img, img.Bounds(), image.NewUniform(bg), image.Point{}, draw.Src)
	padL, padR, padT, padB := 36, 8, 8, 16
	xMin, xMax := plot.XMin, plot.XMax
	yMin, yMax := plot.YMin, plot.YMax
	if !(xMax > xMin) {
		xMax = xMin + 1
	}
	if !(yMax > yMin) {
		yMax = yMin + 1
	}
	pw := w - padL - padR
	ph := h - padT - padB
	if pw < 1 {
		pw = 1
	}
	if ph < 1 {
		ph = 1
	}
	sx := func(x float64) int {
		return padL + int(math.Round((x-xMin)/(xMax-xMin)*float64(pw-1)))
	}
	sy := func(y float64) int {
		return padT + int(math.Round((yMax-y)/(yMax-yMin)*float64(ph-1)))
	}
	ink := color.RGBA{28, 25, 20, 255}
	grid := color.RGBA{210, 202, 188, 255}
	strokeRect(img, padL, padT, padL+pw-1, padT+ph-1, grid)
	drawString(img, padL-34, padT+10, formatAxis(yMax), ink)
	drawString(img, padL-34, padT+ph, formatAxis(yMin), ink)
	drawString(img, padL, padT+ph+12, formatAxis(xMin), ink)

	for _, pen := range plot.Pens {
		c := color.RGBA{pen.R, pen.G, pen.B, 255}
		if pen.Mode == "bar" {
			bar := sx(xMin+pen.Interval) - sx(xMin)
			if bar < 1 {
				bar = 1
			}
			y0 := sy(0)
			for _, p := range pen.Points {
				if !p.Down {
					continue
				}
				fillRect(img, sx(p.X), sy(p.Y), sx(p.X)+bar, y0, c)
			}
			continue
		}
		if pen.Mode == "point" {
			for _, p := range pen.Points {
				if !p.Down {
					continue
				}
				fillRect(img, sx(p.X)-1, sy(p.Y)-1, sx(p.X)+2, sy(p.Y)+2, c)
			}
			continue
		}
		var prev nl.PlotPoint
		have := false
		for _, p := range pen.Points {
			if !p.Down {
				have = false
				continue
			}
			if have {
				drawLine(img, sx(prev.X), sy(prev.Y), sx(p.X), sy(p.Y), c)
			}
			prev = p
			have = true
		}
	}
	return img
}

func strokeRect(img *image.RGBA, x0, y0, x1, y1 int, c color.RGBA) {
	drawLine(img, x0, y0, x1, y0, c)
	drawLine(img, x0, y1, x1, y1, c)
	drawLine(img, x0, y0, x0, y1, c)
	drawLine(img, x1, y0, x1, y1, c)
}

func fillRect(img *image.RGBA, x0, y0, x1, y1 int, c color.RGBA) {
	if x1 < x0 {
		x0, x1 = x1, x0
	}
	if y1 < y0 {
		y0, y1 = y1, y0
	}
	b := img.Bounds()
	if x0 < b.Min.X {
		x0 = b.Min.X
	}
	if y0 < b.Min.Y {
		y0 = b.Min.Y
	}
	if x1 >= b.Max.X {
		x1 = b.Max.X - 1
	}
	if y1 >= b.Max.Y {
		y1 = b.Max.Y - 1
	}
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

func drawLine(img *image.RGBA, x0, y0, x1, y1 int, c color.RGBA) {
	dx := absInt(x1 - x0)
	dy := -absInt(y1 - y0)
	sx := 1
	if x0 > x1 {
		sx = -1
	}
	sy := 1
	if y0 > y1 {
		sy = -1
	}
	err := dx + dy
	for {
		put(img, x0, y0, c)
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

func put(img *image.RGBA, x, y int, c color.RGBA) {
	if !image.Pt(x, y).In(img.Bounds()) {
		return
	}
	img.SetRGBA(x, y, c)
}

func drawString(img *image.RGBA, x, y int, s string, c color.Color) {
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(c),
		Face: basicfont.Face7x13,
		Dot:  fixed.P(x, y),
	}
	d.DrawString(s)
}

func formatAxis(n float64) string {
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return ""
	}
	s := strconv.FormatFloat(n, 'f', 2, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	return s
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
