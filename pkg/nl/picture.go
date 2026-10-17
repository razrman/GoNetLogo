package nl

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// RenderWorld paints patches, pen trails, links, and turtles.
// patchPx is the width of one patch in pixels.
func RenderWorld(w *World, patchPx int) *image.RGBA {
	if patchPx < 1 {
		patchPx = 1
	}
	width := w.width * patchPx
	height := w.height * patchPx
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for _, p := range w.patchOrder {
		c := patchRGBA(p)
		x0 := (p.PX - w.MinPxcor) * patchPx
		y0 := (w.MaxPycor - p.PY) * patchPx
		drawRect(img, x0, y0, x0+patchPx, y0+patchPx, c)
		if d, ok := w.DrawingAt(p.PX, p.PY); ok {
			m := patchPx / 3
			if m < 1 {
				m = 1
			}
			drawRect(img, x0+m, y0+m, x0+patchPx-m, y0+patchPx-m, color.RGBA{d.R, d.G, d.B, 255})
		}
	}
	for _, l := range w.links {
		if l == nil || l.dead || l.Hidden || l.End1 == nil || l.End2 == nil || l.End1.dead || l.End2.dead {
			continue
		}
		x1, y1 := worldToPx(w, l.End1.X, l.End1.Y, patchPx)
		x2, y2 := worldToPx(w, l.End2.X, l.End2.Y, patchPx)
		drawLine(img, x1, y1, x2, y2, ColorRGBA(l.Color))
	}
	for _, t := range w.liveTurtles() {
		if t.Hidden {
			continue
		}
		drawTurtle(img, w, t, patchPx)
	}
	return img
}

func patchRGBA(p *Patch) color.RGBA {
	if p.HasRGB {
		c := p.RGB
		c.A = 255
		return c
	}
	return ColorRGBA(p.PColor)
}

func worldToPx(w *World, x, y float64, patchPx int) (int, int) {
	minX := float64(w.MinPxcor) - 0.5
	maxY := float64(w.MaxPycor) + 0.5
	px := int((x - minX) * float64(patchPx))
	py := int((maxY - y) * float64(patchPx))
	return px, py
}

func drawTurtle(img *image.RGBA, w *World, t *Turtle, patchPx int) {
	cx, cy := worldToPx(w, t.X, t.Y, patchPx)
	r := t.Size * float64(patchPx) * 0.45
	if r < 2 {
		r = 2
	}
	col := ColorRGBA(t.Color)
	shape := t.Shape
	if shape == "" {
		shape = "default"
	}
	switch shape {
	case "square":
		rr := int(r)
		drawRect(img, cx-rr, cy-rr, cx+rr, cy+rr, col)
	case "circle", "dot":
		fillCircle(img, cx, cy, r, col)
	case "sheep":
		fillCircle(img, cx, cy, r, col)
		fillCircle(img, cx, cy-int(r*0.7), r*0.45, col)
	case "wolf":
		fillTri(img, cx, cy, r*1.15, t.Heading, col)
		fillCircle(img, cx, cy, r*0.35, color.RGBA{40, 40, 40, 255})
	case "bug":
		fillCircle(img, cx, cy, r*0.7, col)
		// two antennae along the heading
		h := t.Heading * math.Pi / 180
		ax := int(float64(cx) + math.Sin(h)*r)
		ay := int(float64(cy) - math.Cos(h)*r)
		drawLine(img, cx, cy, ax, ay, color.RGBA{20, 20, 20, 255})
	default:
		fillTri(img, cx, cy, r, t.Heading, col)
	}
}

func fillTri(img *image.RGBA, cx, cy int, r, heading float64, c color.RGBA) {
	h := heading * math.Pi / 180
	tipX := float64(cx) + math.Sin(h)*r
	tipY := float64(cy) - math.Cos(h)*r
	left := h + 2.4
	right := h - 2.4
	pts := [3][2]float64{
		{tipX, tipY},
		{float64(cx) + math.Sin(left)*r*0.75, float64(cy) - math.Cos(left)*r*0.75},
		{float64(cx) + math.Sin(right)*r*0.75, float64(cy) - math.Cos(right)*r*0.75},
	}
	minX, maxX := int(pts[0][0]), int(pts[0][0])
	minY, maxY := int(pts[0][1]), int(pts[0][1])
	for _, p := range pts {
		if int(p[0]) < minX {
			minX = int(p[0])
		}
		if int(p[0]) > maxX {
			maxX = int(p[0])
		}
		if int(p[1]) < minY {
			minY = int(p[1])
		}
		if int(p[1]) > maxY {
			maxY = int(p[1])
		}
	}
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			if pointInTri(float64(x)+0.5, float64(y)+0.5, pts) {
				setPx(img, x, y, c)
			}
		}
	}
}

func pointInTri(x, y float64, p [3][2]float64) bool {
	s1 := edge(x, y, p[0], p[1])
	s2 := edge(x, y, p[1], p[2])
	s3 := edge(x, y, p[2], p[0])
	neg := s1 < 0 || s2 < 0 || s3 < 0
	pos := s1 > 0 || s2 > 0 || s3 > 0
	return !(neg && pos)
}

func edge(x, y float64, a, b [2]float64) float64 {
	return (x-b[0])*(a[1]-b[1]) - (a[0]-b[0])*(y-b[1])
}

func fillCircle(img *image.RGBA, cx, cy int, r float64, c color.RGBA) {
	ri := int(r + 1)
	rr := r * r
	for y := -ri; y <= ri; y++ {
		for x := -ri; x <= ri; x++ {
			if float64(x*x+y*y) <= rr {
				setPx(img, cx+x, cy+y, c)
			}
		}
	}
}

func drawRect(img *image.RGBA, x0, y0, x1, y1 int, c color.RGBA) {
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
	if x1 > b.Max.X {
		x1 = b.Max.X
	}
	if y1 > b.Max.Y {
		y1 = b.Max.Y
	}
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

func drawLine(img *image.RGBA, x0, y0, x1, y1 int, c color.RGBA) {
	dx := abs(x1 - x0)
	dy := -abs(y1 - y0)
	sx, sy := 1, 1
	if x0 > x1 {
		sx = -1
	}
	if y0 > y1 {
		sy = -1
	}
	err := dx + dy
	for {
		setPx(img, x0, y0, c)
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

func setPx(img *image.RGBA, x, y int, c color.RGBA) {
	if image.Pt(x, y).In(img.Bounds()) {
		img.SetRGBA(x, y, c)
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// Snapshot writes a still of the interface with the world as it is now.
func Snapshot(m *FileModel, vm *VM, path string) error {
	img := ComposeInterface(m, vm)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// ComposeInterface paints a desktop-style frame around the world view.
func ComposeInterface(m *FileModel, vm *VM) *image.RGBA {
	const W, H = 1040, 720
	img := image.NewRGBA(image.Rect(0, 0, W, H))
	bg := color.RGBA{236, 234, 228, 255}
	draw.Draw(img, img.Bounds(), image.NewUniform(bg), image.Point{}, draw.Src)
	bar := color.RGBA{32, 92, 58, 255}
	drawRect(img, 0, 0, W, 46, bar)
	drawText(img, 16, 16, "GoNetLogo", color.RGBA{255, 255, 255, 255})
	title := m.Name
	if title == "" {
		title = "model"
	}
	drawText(img, 150, 16, title, color.RGBA{220, 235, 220, 255})

	drawRect(img, 0, 46, W, 86, color.RGBA{250, 250, 248, 255})
	drawRect(img, 16, 56, 92, 78, color.RGBA{46, 120, 72, 255})
	drawText(img, 28, 60, "setup", color.RGBA{255, 255, 255, 255})
	drawRect(img, 100, 56, 164, 78, color.RGBA{180, 70, 50, 255})
	drawText(img, 122, 60, "go", color.RGBA{255, 255, 255, 255})
	ticks := "ticks"
	if vm != nil && vm.World.TicksStarted() {
		n, _ := vm.World.Ticks()
		ticks = "ticks: " + formatNum(n)
	}
	drawText(img, 190, 60, ticks, color.RGBA{40, 40, 40, 255})
	drawText(img, 360, 60, "speed", color.RGBA{40, 40, 40, 255})
	drawRect(img, 420, 64, 620, 72, color.RGBA{180, 180, 180, 255})
	drawRect(img, 510, 60, 522, 76, color.RGBA{40, 40, 40, 255})

	world := RenderWorld(vm.World, 8)
	// Fit the world into the left pane.
	maxW, maxH := 620, 500
	scale := 1
	if world.Bounds().Dx() > 0 {
		for (scale+1)*world.Bounds().Dx() <= maxW && (scale+1)*world.Bounds().Dy() <= maxH && scale < 3 {
			scale++
		}
	}
	scaled := scaleImage(world, scale)
	ox, oy := 16, 100
	drawRect(img, ox-2, oy-2, ox+scaled.Bounds().Dx()+2, oy+scaled.Bounds().Dy()+2, color.RGBA{20, 20, 20, 255})
	draw.Draw(img, image.Rect(ox, oy, ox+scaled.Bounds().Dx(), oy+scaled.Bounds().Dy()), scaled, image.Point{}, draw.Src)

	sx := ox + scaled.Bounds().Dx() + 24
	sy := 100
	drawText(img, sx, sy, "Interface", color.RGBA{32, 92, 58, 255})
	sy += 24
	if m != nil {
		for _, w := range m.Widgets {
			if sy > 640 {
				break
			}
			switch w.Kind {
			case "slider":
				label := w.Display
				if w.Units != "" {
					label += " (" + w.Units + ")"
				}
				drawText(img, sx, sy, label+"  "+formatNum(w.Value), color.RGBA{30, 30, 30, 255})
				sy += 16
				drawRect(img, sx, sy, sx+220, sy+6, color.RGBA{190, 190, 190, 255})
				frac := 0.0
				if w.Max > w.Min {
					frac = (w.Value - w.Min) / (w.Max - w.Min)
				}
				if frac < 0 {
					frac = 0
				}
				if frac > 1 {
					frac = 1
				}
				drawRect(img, sx, sy, sx+int(220*frac), sy+6, color.RGBA{46, 120, 72, 255})
				sy += 18
			case "switch":
				state := "off"
				if w.On {
					state = "on"
				}
				drawText(img, sx, sy, w.Display+"  ["+state+"]", color.RGBA{30, 30, 30, 255})
				sy += 20
			case "button":
				drawRect(img, sx, sy, sx+120, sy+22, color.RGBA{230, 230, 226, 255})
				drawText(img, sx+8, sy+4, w.Display, color.RGBA{20, 20, 20, 255})
				sy += 28
			case "monitor":
				val := "N/A"
				if vm != nil && w.Source != "" {
					if out, err := vm.ExecuteLine(w.Source); err == nil {
						val = out
					}
				}
				drawText(img, sx, sy, w.Display, color.RGBA{90, 90, 90, 255})
				sy += 14
				drawText(img, sx, sy, val, color.RGBA{20, 20, 20, 255})
				sy += 22
			case "chooser":
				choice := ""
				if w.Current >= 0 && w.Current < len(w.Choices) {
					choice = w.Choices[w.Current].Format(false)
				}
				drawText(img, sx, sy, w.Display+": "+choice, color.RGBA{30, 30, 30, 255})
				sy += 20
			case "note":
				drawText(img, sx, sy, w.Display, color.RGBA{70, 70, 70, 255})
				sy += 18
			}
		}
	}
	drawRect(img, 0, H-72, W, H, color.RGBA{255, 255, 255, 255})
	drawText(img, 16, H-52, "Command center", color.RGBA{32, 92, 58, 255})
	drawRect(img, 16, H-32, W-16, H-10, color.RGBA{245, 245, 242, 255})
	drawText(img, 24, H-26, "observer>", color.RGBA{120, 120, 120, 255})
	return img
}

func scaleImage(src *image.RGBA, n int) *image.RGBA {
	if n <= 1 {
		return src
	}
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx()*n, b.Dy()*n))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			c := src.RGBAAt(x, y)
			for dy := 0; dy < n; dy++ {
				for dx := 0; dx < n; dx++ {
					dst.SetRGBA(x*n+dx, y*n+dy, c)
				}
			}
		}
	}
	return dst
}

func drawText(img *image.RGBA, x, y int, s string, c color.Color) {
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(c),
		Face: basicfont.Face7x13,
		Dot:  fixed.P(x, y+12),
	}
	d.DrawString(s)
}
