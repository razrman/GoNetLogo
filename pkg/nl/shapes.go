package nl

import (
	"image"
	"image/color"
	"math"
	"strings"
)

// canonicalShape maps a turtle shape name onto one this program can draw.
// Unknown names use the default arrow.
func canonicalShape(shape string) string {
	s := strings.ToLower(strings.TrimSpace(shape))
	switch s {
	case "", "default", "arrow", "triangle":
		return "default"
	case "circle", "dot", "square", "box", "person", "sheep", "wolf", "bug":
		return s
	case "face happy", "face-happy", "facehappy":
		return "face happy"
	default:
		return "default"
	}
}

func agentRadius(size float64, patchPx int) float64 {
	r := size * float64(patchPx) * 0.45
	if r < 2 {
		r = 2
	}
	return r
}

// stampCovers reports whether the center of patch (px, py) lies on a stamp.
// Model shapes use their real geometry and skip heading when they are
// not rotatable. Anything else uses the built-in approximations.
func (w *World) stampCovers(m Mark, px, py int) bool {
	var lib *ShapeList
	if w != nil {
		lib = w.Shapes
	}
	sh := lib.Get(m.Shape)
	if sh == nil {
		sh = lib.Get("default")
	}
	if sh != nil && len(sh.Elements) > 0 {
		return vectorCovers(sh, m, float64(px), float64(py))
	}
	return shapeCovers(m, px, py)
}

// shapeCovers reports whether the center of patch (px, py) lies inside a stamp.
func shapeCovers(m Mark, px, py int) bool {
	if m.Pen {
		return false
	}
	r := m.Size * 0.45
	if r < 0.05 {
		r = 0.05
	}
	dx := float64(px) - m.X
	dy := float64(py) - m.Y
	h := m.Heading * math.Pi / 180
	sin, cos := math.Sin(h), math.Cos(h)
	// Local space: -y is the heading (screen up at heading 0).
	lx := dx*cos - dy*sin
	ly := -dx*sin - dy*cos
	return localCovers(canonicalShape(m.Shape), lx, ly, r)
}

func localCovers(shape string, lx, ly, r float64) bool {
	switch shape {
	case "circle", "face happy":
		return lx*lx+ly*ly <= r*r
	case "dot":
		rr := r * 0.38
		return lx*lx+ly*ly <= rr*rr
	case "square":
		return math.Abs(lx) <= r && math.Abs(ly) <= r
	case "box":
		for _, face := range boxFaces(r) {
			if pointInPoly(lx, ly, face) {
				return true
			}
		}
		return false
	case "person":
		hx, hy, hr, polys := personParts(r)
		if (lx-hx)*(lx-hx)+(ly-hy)*(ly-hy) <= hr*hr {
			return true
		}
		for _, poly := range polys {
			if pointInPoly(lx, ly, poly) {
				return true
			}
		}
		return false
	case "sheep", "wolf", "bug":
		return lx*lx+ly*ly <= r*r*1.2
	default:
		return pointInPoly(lx, ly, arrowLocal(r))
	}
}

func arrowLocal(r float64) [][2]float64 {
	// Matches fillTri at heading 0: tip up, base about 0.55 radii down.
	const s, c = 0.675463180551151, -0.7373937155412456 // sin(2.4), cos(2.4)
	back := 0.75 * r
	return [][2]float64{
		{0, -r},
		{s * back, -c * back},
		{-s * back, -c * back},
	}
}

func boxFaces(r float64) [][][2]float64 {
	return [][][2]float64{
		scalePoly(r, [][2]float64{{-0.55, -0.15}, {0.35, -0.15}, {0.35, 0.70}, {-0.55, 0.70}}),
		scalePoly(r, [][2]float64{{-0.55, -0.15}, {-0.20, -0.70}, {0.70, -0.70}, {0.35, -0.15}}),
		scalePoly(r, [][2]float64{{0.35, -0.15}, {0.70, -0.70}, {0.70, 0.15}, {0.35, 0.70}}),
	}
}

func personParts(r float64) (hx, hy, hr float64, polys [][][2]float64) {
	hx, hy, hr = 0, -0.52*r, 0.26*r
	polys = [][][2]float64{
		scalePoly(r, [][2]float64{{-0.20, -0.28}, {0.20, -0.28}, {0.16, 0.18}, {-0.16, 0.18}}),
		scalePoly(r, [][2]float64{{-0.58, -0.24}, {0.58, -0.24}, {0.58, -0.06}, {-0.58, -0.06}}),
		scalePoly(r, [][2]float64{{-0.16, 0.12}, {-0.01, 0.12}, {-0.26, 0.84}, {-0.46, 0.80}}),
		scalePoly(r, [][2]float64{{0.01, 0.12}, {0.16, 0.12}, {0.46, 0.80}, {0.26, 0.84}}),
	}
	return hx, hy, hr, polys
}

func scalePoly(r float64, unit [][2]float64) [][2]float64 {
	out := make([][2]float64, len(unit))
	for i, p := range unit {
		out[i] = [2]float64{p[0] * r, p[1] * r}
	}
	return out
}

type painter struct {
	img   *image.RGBA
	c     color.RGBA
	erase bool
}

func (p *painter) set(x, y int) {
	if p.img == nil || !image.Pt(x, y).In(p.img.Bounds()) {
		return
	}
	if p.erase {
		i := p.img.PixOffset(x, y)
		p.img.Pix[i] = 0
		p.img.Pix[i+1] = 0
		p.img.Pix[i+2] = 0
		p.img.Pix[i+3] = 0
		return
	}
	c := p.c
	c.A = 255
	p.img.SetRGBA(x, y, c)
}

func (p *painter) fillCircle(cx, cy int, r float64) {
	if r <= 0 {
		return
	}
	ri := int(r + 1)
	rr := r * r
	for y := -ri; y <= ri; y++ {
		for x := -ri; x <= ri; x++ {
			if float64(x*x+y*y) <= rr {
				p.set(cx+x, cy+y)
			}
		}
	}
}

func (p *painter) fillEllipse(cx, cy, rx, ry float64) {
	if rx <= 0 || ry <= 0 {
		return
	}
	minX := int(math.Floor(cx - rx - 1))
	maxX := int(math.Ceil(cx + rx + 1))
	minY := int(math.Floor(cy - ry - 1))
	maxY := int(math.Ceil(cy + ry + 1))
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			dx := (float64(x) + 0.5 - cx) / rx
			dy := (float64(y) + 0.5 - cy) / ry
			if dx*dx+dy*dy <= 1 {
				p.set(x, y)
			}
		}
	}
}

func (p *painter) circleAt(cx, cy int, heading, lx, ly, r float64) {
	sx, sy := rotateLocal(cx, cy, heading, lx, ly)
	p.fillCircle(int(math.Round(sx)), int(math.Round(sy)), r)
}

func (p *painter) poly(cx, cy int, heading float64, local [][2]float64) {
	if len(local) < 3 {
		return
	}
	pts := make([][2]float64, len(local))
	for i, lp := range local {
		sx, sy := rotateLocal(cx, cy, heading, lp[0], lp[1])
		pts[i] = [2]float64{sx, sy}
	}
	p.fillPolygon(pts)
}

func (p *painter) fillPolygon(pts [][2]float64) {
	minX, maxX := int(math.Floor(pts[0][0])), int(math.Ceil(pts[0][0]))
	minY, maxY := int(math.Floor(pts[0][1])), int(math.Ceil(pts[0][1]))
	for _, pt := range pts[1:] {
		if x := int(math.Floor(pt[0])); x < minX {
			minX = x
		}
		if x := int(math.Ceil(pt[0])); x > maxX {
			maxX = x
		}
		if y := int(math.Floor(pt[1])); y < minY {
			minY = y
		}
		if y := int(math.Ceil(pt[1])); y > maxY {
			maxY = y
		}
	}
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			if pointInPoly(float64(x)+0.5, float64(y)+0.5, pts) {
				p.set(x, y)
			}
		}
	}
}

func (p *painter) fillTri(cx, cy int, r, heading float64) {
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
	for _, pt := range pts {
		if int(pt[0]) < minX {
			minX = int(pt[0])
		}
		if int(pt[0]) > maxX {
			maxX = int(pt[0])
		}
		if int(pt[1]) < minY {
			minY = int(pt[1])
		}
		if int(pt[1]) > maxY {
			maxY = int(pt[1])
		}
	}
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			if pointInTri(float64(x)+0.5, float64(y)+0.5, pts) {
				p.set(x, y)
			}
		}
	}
}

func (p *painter) line(x0, y0, x1, y1 int) {
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
		p.set(x0, y0)
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

func (p *painter) thickLine(x0, y0, x1, y1 int, width float64) {
	steps := int(math.Hypot(float64(x1-x0), float64(y1-y0)))
	if steps < 1 {
		steps = 1
	}
	rad := width / 2
	if rad < 0.6 {
		rad = 0.6
	}
	for i := 0; i <= steps; i++ {
		f := float64(i) / float64(steps)
		x := float64(x0) + (float64(x1-x0))*f
		y := float64(y0) + (float64(y1-y0))*f
		p.fillCircle(int(math.Round(x)), int(math.Round(y)), rad)
	}
}

func rotateLocal(cx, cy int, heading, lx, ly float64) (float64, float64) {
	h := heading * math.Pi / 180
	sin, cos := math.Sin(h), math.Cos(h)
	return float64(cx) + lx*cos - ly*sin, float64(cy) + lx*sin + ly*cos
}

func drawAgentShape(p *painter, cx, cy int, r, heading float64, shape string) {
	switch canonicalShape(shape) {
	case "square":
		p.poly(cx, cy, heading, [][2]float64{{-r, -r}, {r, -r}, {r, r}, {-r, r}})
	case "circle":
		p.fillCircle(cx, cy, r)
	case "dot":
		p.fillCircle(cx, cy, r*0.38)
	case "box":
		drawBox(p, cx, cy, r, heading)
	case "person":
		drawPerson(p, cx, cy, r, heading)
	case "face happy":
		drawFace(p, cx, cy, r, heading)
	case "sheep":
		p.fillCircle(cx, cy, r)
		p.fillCircle(cx, cy-int(r*0.7), r*0.45)
	case "wolf":
		p.fillTri(cx, cy, r*1.15, heading)
		old := p.c
		p.c = color.RGBA{40, 40, 40, 255}
		p.fillCircle(cx, cy, r*0.35)
		p.c = old
	case "bug":
		p.fillCircle(cx, cy, r*0.7)
		h := heading * math.Pi / 180
		ax := int(float64(cx) + math.Sin(h)*r)
		ay := int(float64(cy) - math.Cos(h)*r)
		old := p.c
		p.c = color.RGBA{20, 20, 20, 255}
		p.line(cx, cy, ax, ay)
		p.c = old
	default:
		p.fillTri(cx, cy, r, heading)
	}
}

func drawPerson(p *painter, cx, cy int, r, heading float64) {
	hx, hy, hr, polys := personParts(r)
	p.circleAt(cx, cy, heading, hx, hy, hr)
	for _, poly := range polys {
		p.poly(cx, cy, heading, poly)
	}
}

func drawBox(p *painter, cx, cy int, r, heading float64) {
	faces := boxFaces(r)
	base := p.c
	p.poly(cx, cy, heading, faces[0])
	if !p.erase {
		p.c = lighten(base, 0.38)
	}
	p.poly(cx, cy, heading, faces[1])
	if !p.erase {
		p.c = darken(base, 0.32)
	}
	p.poly(cx, cy, heading, faces[2])
	p.c = base
}

func drawFace(p *painter, cx, cy int, r, heading float64) {
	base := p.c
	p.fillCircle(cx, cy, r)
	if !p.erase {
		p.c = contrastInk(base)
	}
	p.circleAt(cx, cy, heading, -0.32*r, -0.18*r, math.Max(1, 0.12*r))
	p.circleAt(cx, cy, heading, 0.32*r, -0.18*r, math.Max(1, 0.12*r))
	steps := int(r*1.2) + 6
	smile := math.Max(0.8, 0.06*r)
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		a := math.Pi * (0.15 + 0.70*t)
		lx := math.Cos(a) * r * 0.46
		ly := math.Sin(a)*r*0.40 + r*0.12
		p.circleAt(cx, cy, heading, lx, ly, smile)
	}
	p.c = base
}

func contrastInk(c color.RGBA) color.RGBA {
	lum := 0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B)
	if lum > 150 {
		return color.RGBA{20, 20, 20, 255}
	}
	return color.RGBA{255, 255, 255, 255}
}

func lighten(c color.RGBA, t float64) color.RGBA {
	return color.RGBA{
		mixByte(c.R, 255, t),
		mixByte(c.G, 255, t),
		mixByte(c.B, 255, t),
		255,
	}
}

func darken(c color.RGBA, t float64) color.RGBA {
	return color.RGBA{
		uint8(clampByte(int(float64(c.R) * (1 - t)))),
		uint8(clampByte(int(float64(c.G) * (1 - t)))),
		uint8(clampByte(int(float64(c.B) * (1 - t)))),
		255,
	}
}

func mixByte(c, toward uint8, t float64) uint8 {
	return uint8(clampByte(int(float64(c) + (float64(toward)-float64(c))*t)))
}

func pointInTri(x, y float64, pts [3][2]float64) bool {
	s1 := edge(x, y, pts[0], pts[1])
	s2 := edge(x, y, pts[1], pts[2])
	s3 := edge(x, y, pts[2], pts[0])
	neg := s1 < 0 || s2 < 0 || s3 < 0
	pos := s1 > 0 || s2 > 0 || s3 > 0
	return !(neg && pos)
}

func edge(x, y float64, a, b [2]float64) float64 {
	return (x-b[0])*(a[1]-b[1]) - (a[0]-b[0])*(y-b[1])
}

// drawTurtleShape paints a model vector shape when the library has one.
// Unknown names use the library's default arrow, then the built-in
// approximations. A shape with rotatable false ignores heading.
func drawTurtleShape(p *painter, cx, cy int, size, heading float64, shape string, lib *ShapeList, patchPx int) {
	if size == 0 {
		size = 1
	}
	sh := lib.Get(shape)
	if sh == nil {
		sh = lib.Get("default")
	}
	if sh != nil && len(sh.Elements) > 0 {
		paintVectorShape(p, cx, cy, size, heading, sh, patchPx)
		return
	}
	drawAgentShape(p, cx, cy, agentRadius(size, patchPx), heading, shape)
}

func paintVectorShape(p *painter, cx, cy int, size, heading float64, sh *VectorShape, patchPx int) {
	scale := size * float64(patchPx) / shapeBox
	if scale <= 0 || sh == nil {
		return
	}
	rot := 0.0
	if sh.Rotatable {
		rot = heading
	}
	base := p.c
	for i := range sh.Elements {
		el := &sh.Elements[i]
		if el.Marked {
			p.c = base
		} else if !p.erase {
			p.c = color.RGBA{R: el.R, G: el.G, B: el.B, A: 255}
		}
		switch el.Kind {
		case "polygon":
			pts := projectShape(el.Points, scale, rot, cx, cy)
			if el.Filled {
				p.fillPolygon(pts)
			} else {
				p.strokeClosed(pts)
			}
		case "rectangle":
			flat := []float64{
				el.X, el.Y,
				el.X + el.W, el.Y,
				el.X + el.W, el.Y + el.H,
				el.X, el.Y + el.H,
			}
			pts := projectShape(flat, scale, rot, cx, cy)
			if el.Filled {
				p.fillPolygon(pts)
			} else {
				p.strokeClosed(pts)
			}
		case "circle":
			sx, sy := projectShapePoint(el.X+el.W/2, el.Y+el.H/2, scale, rot, cx, cy)
			rx := math.Abs(el.W) / 2 * scale
			ry := math.Abs(el.H) / 2 * scale
			if el.Filled {
				p.fillEllipse(sx, sy, rx, ry)
			} else {
				p.strokeEllipse(sx, sy, rx, ry)
			}
		case "line":
			if len(el.Points) >= 4 {
				x0, y0 := projectShapePoint(el.Points[0], el.Points[1], scale, rot, cx, cy)
				x1, y1 := projectShapePoint(el.Points[2], el.Points[3], scale, rot, cx, cy)
				p.thickLine(int(math.Round(x0)), int(math.Round(y0)), int(math.Round(x1)), int(math.Round(y1)), 1)
			}
		}
	}
	p.c = base
}

func projectShape(flat []float64, scale, rot float64, cx, cy int) [][2]float64 {
	out := make([][2]float64, 0, len(flat)/2)
	for i := 0; i+1 < len(flat); i += 2 {
		x, y := projectShapePoint(flat[i], flat[i+1], scale, rot, cx, cy)
		out = append(out, [2]float64{x, y})
	}
	return out
}

func projectShapePoint(sx, sy, scale, rot float64, cx, cy int) (float64, float64) {
	lx := (sx - 150) * scale
	ly := (sy - 150) * scale
	h := rot * math.Pi / 180
	cos, sin := math.Cos(h), math.Sin(h)
	return float64(cx) + lx*cos - ly*sin, float64(cy) + lx*sin + ly*cos
}

func (p *painter) strokeClosed(pts [][2]float64) {
	if len(pts) < 2 {
		return
	}
	for i := range pts {
		j := (i + 1) % len(pts)
		p.thickLine(int(math.Round(pts[i][0])), int(math.Round(pts[i][1])), int(math.Round(pts[j][0])), int(math.Round(pts[j][1])), 1)
	}
}

func (p *painter) strokeEllipse(cx, cy, rx, ry float64) {
	if rx <= 0 || ry <= 0 {
		return
	}
	steps := int(math.Max(rx, ry)*2) + 12
	if steps < 16 {
		steps = 16
	}
	var prevX, prevY float64
	for i := 0; i <= steps; i++ {
		a := 2 * math.Pi * float64(i) / float64(steps)
		x := cx + math.Cos(a)*rx
		y := cy + math.Sin(a)*ry
		if i > 0 {
			p.thickLine(int(math.Round(prevX)), int(math.Round(prevY)), int(math.Round(x)), int(math.Round(y)), 1)
		}
		prevX, prevY = x, y
	}
}

func pointInPoly(x, y float64, pts [][2]float64) bool {
	inside := false
	j := len(pts) - 1
	for i := 0; i < len(pts); i++ {
		xi, yi := pts[i][0], pts[i][1]
		xj, yj := pts[j][0], pts[j][1]
		if (yi > y) != (yj > y) && x < (xj-xi)*(y-yi)/(yj-yi)+xi {
			inside = !inside
		}
		j = i
	}
	return inside
}
