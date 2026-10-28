package nl

import (
	_ "embed"
	"encoding/xml"
	"math"
	"strconv"
	"strings"
	"sync"
)

// shapeBox is the width of NetLogo's turtle shape editor. The turtle sits
// at (150, 150). A turtle of size 1 draws that box as one patch.
const shapeBox = 300.0

//go:embed defaultshapes.txt
var defaultShapesText string

// ShapeElement is one polygon, rectangle, circle, or line in a turtle
// shape. Coordinates are in the 300 by 300 editor, y growing downward.
// Marked elements take the turtle's color. Other elements keep R, G, B.
type ShapeElement struct {
	Kind    string
	Filled  bool
	Marked  bool
	R, G, B uint8
	// Points is x,y pairs. Polygons store every vertex. Lines store two.
	Points []float64
	// X, Y, W, H bound a circle or a rectangle. W and H are the diameter
	// for a circle.
	X, Y, W, H float64
}

// VectorShape is one turtle shape from a model's TURTLE-SHAPES section.
// Rotatable is false for shapes that ignore heading, including square,
// dot, and person in the default library.
type VectorShape struct {
	Name      string
	Rotatable bool
	Elements  []ShapeElement
}

// ShapeList is the model's turtle shapes, looked up by name. Names are
// compared case-insensitively. The list is not mutated after it is built.
type ShapeList struct {
	order []string
	by    map[string]*VectorShape
}

func newShapeList() *ShapeList {
	return &ShapeList{by: map[string]*VectorShape{}}
}

func (s *ShapeList) add(sh *VectorShape) {
	if s == nil || sh == nil || strings.TrimSpace(sh.Name) == "" {
		return
	}
	key := strings.ToLower(strings.TrimSpace(sh.Name))
	if _, ok := s.by[key]; !ok {
		s.order = append(s.order, key)
	}
	sh.Name = key
	s.by[key] = sh
}

// Len is the number of shapes.
func (s *ShapeList) Len() int {
	if s == nil {
		return 0
	}
	return len(s.order)
}

// All returns shapes in file order.
func (s *ShapeList) All() []*VectorShape {
	if s == nil {
		return nil
	}
	out := make([]*VectorShape, 0, len(s.order))
	for _, key := range s.order {
		out = append(out, s.by[key])
	}
	return out
}

// Get returns the shape with this name, or nil.
func (s *ShapeList) Get(name string) *VectorShape {
	if s == nil {
		return nil
	}
	return s.by[strings.ToLower(strings.TrimSpace(name))]
}

var (
	defaultShapesOnce sync.Once
	defaultShapes     *ShapeList
)

// DefaultShapes is NetLogo's built-in turtle shape library. A model with
// its own TURTLE-SHAPES section replaces this list.
func DefaultShapes() *ShapeList {
	defaultShapesOnce.Do(func() {
		defaultShapes = ParseVectorShapes(defaultShapesText)
	})
	return defaultShapes
}

// ParseVectorShapes reads the classic .nlogo turtle-shapes section.
// Shapes are separated by blank lines. Each shape is a name, a rotatable
// flag, an editable color index, then Polygon, Circle, Rectangle, and
// Line elements. A bad element is skipped. An empty section returns an
// empty list.
func ParseVectorShapes(section string) *ShapeList {
	lib := newShapeList()
	lines := strings.Split(strings.ReplaceAll(section, "\r\n", "\n"), "\n")
	var block []string
	flush := func() {
		if len(block) == 0 {
			return
		}
		if sh := shapeFromBlock(block); sh != nil {
			lib.add(sh)
		}
		block = nil
	}
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		block = append(block, line)
	}
	flush()
	return lib
}

func shapeFromBlock(block []string) *VectorShape {
	if len(block) < 2 {
		return nil
	}
	sh := &VectorShape{
		Name:      strings.TrimSpace(block[0]),
		Rotatable: strings.EqualFold(strings.TrimSpace(block[1]), "true"),
	}
	start := 2
	if len(block) > 2 && !elementLine(block[2]) {
		start = 3
	}
	for _, line := range block[start:] {
		el, ok := parseShapeElement(line)
		if ok {
			sh.Elements = append(sh.Elements, el)
		}
	}
	if sh.Name == "" {
		return nil
	}
	return sh
}

func elementLine(line string) bool {
	kind, _, _ := strings.Cut(strings.TrimSpace(line), " ")
	switch strings.ToLower(kind) {
	case "polygon", "circle", "rectangle", "line":
		return true
	default:
		return false
	}
}

func parseShapeElement(line string) (ShapeElement, bool) {
	f := strings.Fields(line)
	if len(f) < 3 {
		return ShapeElement{}, false
	}
	kind := strings.ToLower(f[0])
	r, g, b, ok := unpackShapeRGB(f[1])
	if !ok {
		return ShapeElement{}, false
	}
	el := ShapeElement{Kind: kind, R: r, G: g, B: b}
	switch kind {
	case "polygon", "rectangle", "circle":
		if len(f) < 4 {
			return ShapeElement{}, false
		}
		el.Filled = strings.EqualFold(f[2], "true")
		el.Marked = strings.EqualFold(f[3], "true")
		nums := parseShapeNums(f[4:])
		switch kind {
		case "polygon":
			if len(nums) < 6 {
				return ShapeElement{}, false
			}
			el.Points = nums
		case "rectangle":
			if len(nums) < 4 {
				return ShapeElement{}, false
			}
			el.X, el.Y = nums[0], nums[1]
			el.W, el.H = nums[2]-nums[0], nums[3]-nums[1]
		case "circle":
			if len(nums) < 3 {
				return ShapeElement{}, false
			}
			el.X, el.Y, el.W, el.H = nums[0], nums[1], nums[2], nums[2]
		}
	case "line":
		el.Marked = strings.EqualFold(f[2], "true")
		nums := parseShapeNums(f[3:])
		if len(nums) < 4 {
			return ShapeElement{}, false
		}
		el.Points = nums[:4]
	default:
		return ShapeElement{}, false
	}
	return el, true
}

func parseShapeNums(f []string) []float64 {
	out := make([]float64, 0, len(f))
	for _, s := range f {
		n, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return out
		}
		out = append(out, n)
	}
	return out
}

// unpackShapeRGB reads a Java Color.getRGB() value. The low 24 bits are
// R, G, B. -7500403 is the default gray that marked elements replace
// with the turtle color.
func unpackShapeRGB(s string) (r, g, b uint8, ok bool) {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0, 0, 0, false
	}
	u := uint32(int32(n))
	return uint8(u >> 16), uint8(u >> 8), uint8(u), true
}

// vectorCovers reports whether the center of a patch lies on a stamp of
// this shape. Heading is ignored when the shape is not rotatable.
func vectorCovers(sh *VectorShape, m Mark, wx, wy float64) bool {
	if sh == nil {
		return false
	}
	sx, sy := worldToShape(sh, m, wx, wy)
	for i := range sh.Elements {
		if elementCovers(&sh.Elements[i], sx, sy) {
			return true
		}
	}
	return false
}

func worldToShape(sh *VectorShape, m Mark, wx, wy float64) (float64, float64) {
	size := m.Size
	if size == 0 {
		size = 1
	}
	rot := 0.0
	if sh.Rotatable {
		rot = m.Heading * math.Pi / 180
	}
	dx := wx - m.X
	dy := wy - m.Y
	// Screen y grows down, world y grows up.
	dxs, dys := dx, -dy
	cos, sin := math.Cos(rot), math.Sin(rot)
	lx := dxs*cos + dys*sin
	ly := -dxs*sin + dys*cos
	return 150 + lx*shapeBox/size, 150 + ly*shapeBox/size
}

func elementCovers(el *ShapeElement, sx, sy float64) bool {
	switch el.Kind {
	case "polygon":
		return polyCovers(el.Points, el.Filled, sx, sy)
	case "rectangle":
		return rectCovers(el.X, el.Y, el.W, el.H, el.Filled, sx, sy)
	case "circle":
		return ellipseCovers(el.X, el.Y, el.W, el.H, el.Filled, sx, sy)
	case "line":
		if len(el.Points) < 4 {
			return false
		}
		return distToSeg(sx, sy, el.Points[0], el.Points[1], el.Points[2], el.Points[3]) <= 3
	default:
		return false
	}
}

func polyCovers(flat []float64, filled bool, sx, sy float64) bool {
	pts := pairs(flat)
	if len(pts) < 3 {
		return false
	}
	if filled {
		return pointInPoly(sx, sy, pts)
	}
	for i := range pts {
		j := (i + 1) % len(pts)
		if distToSeg(sx, sy, pts[i][0], pts[i][1], pts[j][0], pts[j][1]) <= 2.5 {
			return true
		}
	}
	return false
}

func rectCovers(x, y, w, h float64, filled bool, sx, sy float64) bool {
	x0, x1 := x, x+w
	y0, y1 := y, y+h
	if x1 < x0 {
		x0, x1 = x1, x0
	}
	if y1 < y0 {
		y0, y1 = y1, y0
	}
	inside := sx >= x0 && sx <= x1 && sy >= y0 && sy <= y1
	if filled {
		return inside
	}
	if !inside {
		return false
	}
	const edge = 2.5
	return sx-x0 <= edge || x1-sx <= edge || sy-y0 <= edge || y1-sy <= edge
}

func ellipseCovers(x, y, w, h float64, filled bool, sx, sy float64) bool {
	rx, ry := math.Abs(w)/2, math.Abs(h)/2
	if rx < 0.5 || ry < 0.5 {
		return false
	}
	cx, cy := x+w/2, y+h/2
	dx := (sx - cx) / rx
	dy := (sy - cy) / ry
	d := dx*dx + dy*dy
	if filled {
		return d <= 1
	}
	// A stroke about 2.5 editor units wide.
	outer := (rx + 2.5) / rx
	inner := (rx - 2.5) / rx
	if inner < 0 {
		inner = 0
	}
	return d <= outer*outer && d >= inner*inner
}

func pairs(flat []float64) [][2]float64 {
	n := len(flat) / 2
	pts := make([][2]float64, 0, n)
	for i := 0; i+1 < len(flat); i += 2 {
		pts = append(pts, [2]float64{flat[i], flat[i+1]})
	}
	return pts
}

func distToSeg(px, py, x0, y0, x1, y1 float64) float64 {
	dx, dy := x1-x0, y1-y0
	l2 := dx*dx + dy*dy
	if l2 == 0 {
		return math.Hypot(px-x0, py-y0)
	}
	t := ((px-x0)*dx + (py-y0)*dy) / l2
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	return math.Hypot(px-(x0+t*dx), py-(y0+t*dy))
}

// readTurtleShapesXML consumes a <turtleShapes> element. The decoder has
// already returned that start tag.
func readTurtleShapesXML(dec *xml.Decoder, start xml.StartElement) (*ShapeList, error) {
	lib := newShapeList()
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "shape" {
				sh, err := readXMLShape(dec, t)
				if err != nil {
					return nil, err
				}
				lib.add(sh)
			} else if err := dec.Skip(); err != nil {
				return nil, err
			}
		case xml.EndElement:
			if t.Name.Local == start.Name.Local {
				return lib, nil
			}
		}
	}
}

func readXMLShape(dec *xml.Decoder, start xml.StartElement) (*VectorShape, error) {
	sh := &VectorShape{
		Name:      strings.TrimSpace(xmlAttr(start, "name")),
		Rotatable: strings.EqualFold(xmlAttr(start, "rotatable"), "true"),
	}
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			el, ok, err := readXMLElement(dec, t)
			if err != nil {
				return nil, err
			}
			if ok {
				sh.Elements = append(sh.Elements, el)
			}
		case xml.EndElement:
			if t.Name.Local == "shape" {
				return sh, nil
			}
		}
	}
}

func readXMLElement(dec *xml.Decoder, start xml.StartElement) (ShapeElement, bool, error) {
	kind := strings.ToLower(start.Name.Local)
	r, g, b, ok := unpackShapeRGB(xmlAttr(start, "color"))
	if !ok && kind != "point" {
		if err := dec.Skip(); err != nil {
			return ShapeElement{}, false, err
		}
		return ShapeElement{}, false, nil
	}
	el := ShapeElement{
		Kind:   kind,
		Filled: strings.EqualFold(xmlAttr(start, "filled"), "true"),
		Marked: strings.EqualFold(xmlAttr(start, "marked"), "true"),
		R:      r, G: g, B: b,
	}
	switch kind {
	case "circle":
		el.X = atof(xmlAttr(start, "x"))
		el.Y = atof(xmlAttr(start, "y"))
		el.W = atof(xmlAttr(start, "diameter"))
		el.H = el.W
		if err := dec.Skip(); err != nil {
			return ShapeElement{}, false, err
		}
		return el, true, nil
	case "rectangle":
		x0 := atof(xmlAttr(start, "startX"))
		y0 := atof(xmlAttr(start, "startY"))
		x1 := atof(xmlAttr(start, "endX"))
		y1 := atof(xmlAttr(start, "endY"))
		el.X, el.Y = x0, y0
		el.W, el.H = x1-x0, y1-y0
		if err := dec.Skip(); err != nil {
			return ShapeElement{}, false, err
		}
		return el, true, nil
	case "line":
		el.Points = []float64{
			atof(xmlAttr(start, "startX")), atof(xmlAttr(start, "startY")),
			atof(xmlAttr(start, "endX")), atof(xmlAttr(start, "endY")),
		}
		if err := dec.Skip(); err != nil {
			return ShapeElement{}, false, err
		}
		return el, true, nil
	case "polygon":
		for {
			tok, err := dec.Token()
			if err != nil {
				return ShapeElement{}, false, err
			}
			switch t := tok.(type) {
			case xml.StartElement:
				if t.Name.Local == "point" {
					el.Points = append(el.Points, atof(xmlAttr(t, "x")), atof(xmlAttr(t, "y")))
				}
				if err := dec.Skip(); err != nil {
					return ShapeElement{}, false, err
				}
			case xml.EndElement:
				if t.Name.Local == "polygon" {
					if len(el.Points) < 6 {
						return ShapeElement{}, false, nil
					}
					return el, true, nil
				}
			}
		}
	default:
		if err := dec.Skip(); err != nil {
			return ShapeElement{}, false, err
		}
		return ShapeElement{}, false, nil
	}
}

func xmlAttr(se xml.StartElement, name string) string {
	for _, a := range se.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}
