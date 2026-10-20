package nl

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// ViewSnap is an immutable picture of the world. The draw thread paints it
// while the model mutates the live maps on another goroutine.
type ViewSnap struct {
	Width, Height int
	minPx, maxPy  int
	patches       []snapPatch
	links         []snapLink
	turtles       []snapTurtle
	marks         []snapMark
	labels        []snapLabel
}

type snapPatch struct {
	x, y int
	c    color.RGBA
}

type snapLink struct {
	x1, y1, x2, y2 float64
	c              color.RGBA
}

type snapTurtle struct {
	x, y, heading, size float64
	color               float64
	shape               string
}

type snapMark struct {
	pen, erase           bool
	x, y, x2, y2         float64
	heading, size, color float64
	shape                string
}

type snapLabel struct {
	x, y, size float64
	patch      bool
	link       bool
	text       string
	c          color.RGBA
}

// CurrentView returns the last frame a painter asked for. A model command
// only marks the frame dirty; this copies the world if that flag is set
// or no frame has been stored yet.
func (w *World) CurrentView() *ViewSnap {
	if w == nil {
		return &ViewSnap{Width: 1, Height: 1}
	}
	if !w.dirty.Load() {
		if s := w.view.Load(); s != nil {
			return s
		}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.dirty.Load() {
		if s := w.view.Load(); s != nil {
			return s
		}
	}
	s := w.captureView()
	w.view.Store(s)
	w.dirty.Store(false)
	return s
}

// captureView copies the pixels the renderer needs. The caller holds w.mu,
// or the world is not yet shared.
func (w *World) captureView() *ViewSnap {
	s := &ViewSnap{
		Width:   w.width,
		Height:  w.height,
		minPx:   w.MinPxcor,
		maxPy:   w.MaxPycor,
		patches: make([]snapPatch, len(w.patchOrder)),
	}
	for i, p := range w.patchOrder {
		s.patches[i] = snapPatch{x: p.PX, y: p.PY, c: patchRGBA(p)}
		if text, col, ok := p.viewLabel(); ok {
			s.labels = append(s.labels, snapLabel{
				x: float64(p.PX), y: float64(p.PY), patch: true, text: text, c: col,
			})
		}
	}
	if len(w.marks) > 0 {
		s.marks = make([]snapMark, len(w.marks))
		for i, m := range w.marks {
			s.marks[i] = snapMark{
				pen: m.Pen, erase: m.Erase,
				x: m.X, y: m.Y, x2: m.X2, y2: m.Y2,
				heading: m.Heading, size: m.Size, color: m.Color, shape: m.Shape,
			}
		}
	}
	for _, l := range w.links {
		if l == nil || l.dead || l.Hidden || l.End1 == nil || l.End2 == nil || l.End1.dead || l.End2.dead {
			continue
		}
		s.links = append(s.links, snapLink{l.End1.X, l.End1.Y, l.End2.X, l.End2.Y, ColorRGBA(l.Color)})
		if text, col, ok := l.viewLabel(); ok {
			s.labels = append(s.labels, snapLabel{
				x: (l.End1.X + l.End2.X) / 2, y: (l.End1.Y + l.End2.Y) / 2,
				link: true, text: text, c: col,
			})
		}
	}
	if len(w.whoSeq) != len(w.turtles) {
		w.rebuildWhoSeq()
	}
	for _, id := range w.whoSeq {
		t := w.turtles[id]
		if t == nil || t.dead || t.Hidden {
			continue
		}
		shape := t.Shape
		if shape == "" {
			shape = "default"
		}
		s.turtles = append(s.turtles, snapTurtle{t.X, t.Y, t.Heading, t.Size, t.Color, shape})
		if text, col, ok := t.viewLabel(); ok {
			s.labels = append(s.labels, snapLabel{
				x: t.X, y: t.Y, size: t.Size, text: text, c: col,
			})
		}
	}
	return s
}

// RenderWorld paints patches, the drawing layer, links, turtles, and labels.
// patchPx is the width of one patch in pixels. The paint uses the latest
// published frame, so it does not read the live turtle map.
func RenderWorld(w *World, patchPx int) *image.RGBA {
	if w == nil {
		return image.NewRGBA(image.Rect(0, 0, 1, 1))
	}
	return w.CurrentView().Paint(patchPx)
}

// Paint draws this frame. It does not touch the live world.
func (s *ViewSnap) Paint(patchPx int) *image.RGBA {
	if s == nil {
		return image.NewRGBA(image.Rect(0, 0, 1, 1))
	}
	if patchPx < 1 {
		patchPx = 1
	}
	width := s.Width * patchPx
	height := s.Height * patchPx
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for _, p := range s.patches {
		x0 := (p.x - s.minPx) * patchPx
		y0 := (s.maxPy - p.y) * patchPx
		drawRect(img, x0, y0, x0+patchPx, y0+patchPx, p.c)
	}
	if layer := s.paintDrawing(patchPx); layer != nil {
		draw.Draw(img, img.Bounds(), layer, image.Point{}, draw.Over)
	}
	for _, l := range s.links {
		x1, y1 := worldToPx(s.minPx, s.maxPy, l.x1, l.y1, patchPx)
		x2, y2 := worldToPx(s.minPx, s.maxPy, l.x2, l.y2, patchPx)
		drawLine(img, x1, y1, x2, y2, l.c)
	}
	for _, t := range s.turtles {
		drawTurtle(img, s.minPx, s.maxPy, t, patchPx)
	}
	s.paintLabels(img, patchPx)
	return img
}

func (s *ViewSnap) paintDrawing(patchPx int) *image.RGBA {
	if s == nil || len(s.marks) == 0 {
		return nil
	}
	w := s.Width * patchPx
	h := s.Height * patchPx
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	layer := image.NewRGBA(image.Rect(0, 0, w, h))
	p := &painter{img: layer}
	for _, m := range s.marks {
		if m.pen {
			p.erase = false
			p.c = ColorRGBA(m.color)
			x1, y1 := worldToPx(s.minPx, s.maxPy, m.x, m.y, patchPx)
			x2, y2 := worldToPx(s.minPx, s.maxPy, m.x2, m.y2, patchPx)
			width := m.size * float64(patchPx) * 0.55
			if width < 1 {
				width = 1
			}
			p.thickLine(x1, y1, x2, y2, width)
			continue
		}
		p.erase = m.erase
		p.c = ColorRGBA(m.color)
		cx, cy := worldToPx(s.minPx, s.maxPy, m.x, m.y, patchPx)
		drawAgentShape(p, cx, cy, agentRadius(m.size, patchPx), m.heading, m.shape)
	}
	return layer
}

func (s *ViewSnap) paintLabels(img *image.RGBA, patchPx int) {
	scale := labelScale(patchPx)
	for _, lb := range s.labels {
		if lb.patch {
			cx := (int(lb.x)-s.minPx)*patchPx + patchPx/2
			cy := (s.maxPy-int(lb.y))*patchPx + patchPx/2
			paintLabel(img, cx, cy-6*scale, lb.text, lb.c, scale)
			continue
		}
		if lb.link {
			cx, cy := worldToPx(s.minPx, s.maxPy, lb.x, lb.y, patchPx)
			paintLabel(img, cx, cy-6*scale, lb.text, lb.c, scale)
			continue
		}
		cx, cy := worldToPx(s.minPx, s.maxPy, lb.x, lb.y, patchPx)
		r := agentRadius(lb.size, patchPx)
		paintLabel(img, cx+int(r*0.5), cy+int(r*0.2), lb.text, lb.c, scale)
	}
}

func labelScale(patchPx int) int {
	n := patchPx / 8
	if n < 1 {
		n = 1
	}
	if n > 4 {
		n = 4
	}
	return n
}

func paintLabel(img *image.RGBA, x, y int, text string, c color.Color, scale int) {
	if text == "" {
		return
	}
	if scale <= 1 {
		drawText(img, x, y, text, c)
		return
	}
	w := 7*len(text) + 1
	h := 13
	tmp := image.NewRGBA(image.Rect(0, 0, w, h))
	drawText(tmp, 0, 1, text, c)
	for yy := 0; yy < h; yy++ {
		for xx := 0; xx < w; xx++ {
			p := tmp.RGBAAt(xx, yy)
			if p.A == 0 {
				continue
			}
			ink := color.RGBA{p.R, p.G, p.B, 255}
			for dy := 0; dy < scale; dy++ {
				for dx := 0; dx < scale; dx++ {
					setPx(img, x+xx*scale+dx, y+yy*scale+dy, ink)
				}
			}
		}
	}
}

func patchRGBA(p *Patch) color.RGBA {
	if p.HasRGB {
		c := p.RGB
		c.A = 255
		return c
	}
	return ColorRGBA(p.PColor)
}

func worldToPx(minPx, maxPy int, x, y float64, patchPx int) (int, int) {
	minX := float64(minPx) - 0.5
	maxY := float64(maxPy) + 0.5
	px := int((x - minX) * float64(patchPx))
	py := int((maxY - y) * float64(patchPx))
	return px, py
}

func drawTurtle(img *image.RGBA, minPx, maxPy int, t snapTurtle, patchPx int) {
	cx, cy := worldToPx(minPx, maxPy, t.x, t.y, patchPx)
	p := &painter{img: img, c: ColorRGBA(t.color)}
	drawAgentShape(p, cx, cy, agentRadius(t.size, patchPx), t.heading, t.shape)
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
