package nl

import (
	"fmt"
	"image/color"
	"math"
)

// The palette extension. Infix forms of with-* are also reporters, so
// `red palette:with-hue 100` and `(palette:with-hue red 100)` both work.
// Names are lowercase because the lexer folds case.

var paletteCommands = map[string]primSpec{
	"palette:set-alpha":        {argc: 1},
	"palette:set-transparency": {argc: 1},
	"palette:set-hue":          {argc: 1},
	"palette:set-saturation":   {argc: 1},
	"palette:set-brightness":   {argc: 1},
	"palette:set-r":            {argc: 1},
	"palette:set-g":            {argc: 1},
	"palette:set-b":            {argc: 1},
	"palette:scheme-dialog":    {argc: 0},
}

var paletteReporters = map[string]primSpec{
	"palette:alpha-of":           {argc: 1, prec: precNormal},
	"palette:transparency-of":    {argc: 1, prec: precNormal},
	"palette:hue-of":             {argc: 1, prec: precNormal},
	"palette:saturation-of":      {argc: 1, prec: precNormal},
	"palette:brightness-of":      {argc: 1, prec: precNormal},
	"palette:r-of":               {argc: 1, prec: precNormal},
	"palette:g-of":               {argc: 1, prec: precNormal},
	"palette:b-of":               {argc: 1, prec: precNormal},
	"palette:with-alpha":         {argc: 2, prec: precNormal},
	"palette:with-transparency":  {argc: 2, prec: precNormal},
	"palette:with-hue":           {argc: 2, prec: precNormal},
	"palette:with-saturation":    {argc: 2, prec: precNormal},
	"palette:with-brightness":    {argc: 2, prec: precNormal},
	"palette:with-r":             {argc: 2, prec: precNormal},
	"palette:with-g":             {argc: 2, prec: precNormal},
	"palette:with-b":             {argc: 2, prec: precNormal},
	"palette:scale-gradient":     {argc: 4, prec: precNormal},
	"palette:scale-gradient-hsb": {argc: 4, prec: precNormal},
	"palette:scheme-colors":      {argc: 3, prec: precNormal},
	"palette:scale-scheme":       {argc: 6, prec: precNormal},
}

var paletteZero = map[string]bool{
	"palette:alpha":        true,
	"palette:transparency": true,
	"palette:hue":          true,
	"palette:saturation":   true,
	"palette:brightness":   true,
	"palette:r":            true,
	"palette:g":            true,
	"palette:b":            true,
}

var paletteInfixOps = map[string]infixSpec{
	"palette:with-alpha":        {prec: precNormal},
	"palette:with-transparency": {prec: precNormal},
	"palette:with-hue":          {prec: precNormal},
	"palette:with-saturation":   {prec: precNormal},
	"palette:with-brightness":   {prec: precNormal},
	"palette:with-r":            {prec: precNormal},
	"palette:with-g":            {prec: precNormal},
	"palette:with-b":            {prec: precNormal},
}

func (p *parser) installPalette() {
	p.addCommands(paletteCommands)
	p.addReporters(paletteReporters)
	p.addInfix(paletteInfixOps)
}

const gradientSize = 256

func (vm *VM) paletteCommand(name string, args []Value, pos Pos) (error, bool) {
	switch name {
	case "palette:scheme-dialog":
		return fmt.Errorf("palette:scheme-dialog needs a desktop dialog and is not available in this runtime"), true
	case "palette:set-alpha":
		return vm.paletteSetAlpha(args, false, pos), true
	case "palette:set-transparency":
		return vm.paletteSetAlpha(args, true, pos), true
	case "palette:set-hue":
		return vm.paletteSetHSB(args, 0, pos), true
	case "palette:set-saturation":
		return vm.paletteSetHSB(args, 1, pos), true
	case "palette:set-brightness":
		return vm.paletteSetHSB(args, 2, pos), true
	case "palette:set-r":
		return vm.paletteSetChannel(args, 0, pos), true
	case "palette:set-g":
		return vm.paletteSetChannel(args, 1, pos), true
	case "palette:set-b":
		return vm.paletteSetChannel(args, 2, pos), true
	default:
		return nil, false
	}
}

func (vm *VM) paletteReporter(name string, args []Value, pos Pos) (Value, error) {
	switch name {
	case "palette:alpha-of":
		c, err := rgbaOf(args[0])
		if err != nil {
			return Value{}, err
		}
		return Num(float64(c.A)), nil
	case "palette:transparency-of":
		c, err := rgbaOf(args[0])
		if err != nil {
			return Value{}, err
		}
		return Num(transparencyOf(c.A)), nil
	case "palette:hue-of", "palette:saturation-of", "palette:brightness-of":
		return hsbComponent(name, args[0], true)
	case "palette:r-of", "palette:g-of", "palette:b-of":
		return rgbComponent(name, args[0])
	case "palette:with-alpha", "palette:with-transparency", "palette:with-hue",
		"palette:with-saturation", "palette:with-brightness",
		"palette:with-r", "palette:with-g", "palette:with-b":
		return applyPaletteInfix(name, args[0], args[1])
	case "palette:scale-gradient":
		return scaleGradient(args, false)
	case "palette:scale-gradient-hsb":
		return scaleGradient(args, true)
	case "palette:scheme-colors":
		return schemeColors(args)
	case "palette:scale-scheme":
		return scaleScheme(args)
	default:
		return Value{}, fmt.Errorf("primitive %s is not implemented yet", name)
	}
}

func (vm *VM) paletteZero(name string, pos Pos) (Value, error) {
	c, err := vm.agentRGBA()
	if err != nil {
		return Value{}, errorAt(pos, "%s", err.Error())
	}
	switch name {
	case "palette:alpha":
		return Num(float64(c.A)), nil
	case "palette:transparency":
		return Num(transparencyOf(c.A)), nil
	case "palette:hue":
		return hsbComponent("palette:hue-of", rgbValue(c), true)
	case "palette:saturation":
		return hsbComponent("palette:saturation-of", rgbValue(c), true)
	case "palette:brightness":
		return hsbComponent("palette:brightness-of", rgbValue(c), true)
	case "palette:r":
		return Num(float64(c.R)), nil
	case "palette:g":
		return Num(float64(c.G)), nil
	case "palette:b":
		return Num(float64(c.B)), nil
	default:
		return Value{}, errorAt(pos, "primitive %s is not implemented yet", name)
	}
}

func applyPaletteInfix(op string, colorVal, num Value) (Value, error) {
	c, err := rgbaOf(colorVal)
	if err != nil {
		return Value{}, err
	}
	n, err := num.Num()
	if err != nil {
		return Value{}, fmt.Errorf("%s expected a number, got %s", op, num.TypeName())
	}
	switch op {
	case "palette:with-alpha":
		if err := inRange(op, n, 0, 255); err != nil {
			return Value{}, err
		}
		c.A = uint8(math.Round(n))
		return rgbValue(c), nil
	case "palette:with-transparency":
		if err := inRange(op, n, 0, 100); err != nil {
			return Value{}, err
		}
		c.A = alphaFromTransparency(n)
		return rgbValue(c), nil
	case "palette:with-hue", "palette:with-saturation", "palette:with-brightness":
		idx := 0
		lo, hi := 0.0, 360.0
		if op == "palette:with-saturation" {
			idx, lo, hi = 1, 0, 100
		} else if op == "palette:with-brightness" {
			idx, lo, hi = 2, 0, 100
		}
		if err := inRange(op, n, lo, hi); err != nil {
			return Value{}, err
		}
		h, s, b := rgbHSB(int(c.R), int(c.G), int(c.B))
		switch idx {
		case 0:
			h = n
		case 1:
			s = n
		default:
			b = n
		}
		r, g, bl := HSBtoRGB(h, s, b)
		return rgbValue(color.RGBA{r, g, bl, c.A}), nil
	case "palette:with-r", "palette:with-g", "palette:with-b":
		if err := inRange(op, n, 0, 255); err != nil {
			return Value{}, err
		}
		ch := uint8(math.Round(n))
		switch op {
		case "palette:with-r":
			c.R = ch
		case "palette:with-g":
			c.G = ch
		default:
			c.B = ch
		}
		return rgbValue(c), nil
	default:
		return Value{}, fmt.Errorf("unknown operator %s", op)
	}
}

func (vm *VM) paletteSetAlpha(args []Value, transparency bool, pos Pos) error {
	if vm.self == nil {
		return errorAt(pos, "palette can only change the color of a turtle, patch, or link")
	}
	n, err := args[0].Num()
	if err != nil {
		return err
	}
	name := "palette:set-alpha"
	if transparency {
		name = "palette:set-transparency"
	}
	c, err := vm.agentRGBA()
	if err != nil {
		return err
	}
	if transparency {
		if err := inRange(name, n, 0, 100); err != nil {
			return err
		}
		c.A = alphaFromTransparency(n)
	} else {
		if err := inRange(name, n, 0, 255); err != nil {
			return err
		}
		c.A = uint8(math.Round(n))
	}
	return vm.setAgentColor(c)
}

func (vm *VM) paletteSetHSB(args []Value, idx int, pos Pos) error {
	if vm.self == nil {
		return errorAt(pos, "palette can only change the color of a turtle, patch, or link")
	}
	n, err := args[0].Num()
	if err != nil {
		return err
	}
	name := "palette:set-hue"
	lo, hi := 0.0, 360.0
	if idx == 1 {
		name, lo, hi = "palette:set-saturation", 0, 100
	} else if idx == 2 {
		name, lo, hi = "palette:set-brightness", 0, 100
	}
	if err := inRange(name, n, lo, hi); err != nil {
		return err
	}
	c, err := vm.agentRGBA()
	if err != nil {
		return err
	}
	h, s, b := rgbHSB(int(c.R), int(c.G), int(c.B))
	switch idx {
	case 0:
		h = n
	case 1:
		s = n
	default:
		b = n
	}
	r, g, bl := HSBtoRGB(h, s, b)
	return vm.setAgentColor(color.RGBA{r, g, bl, c.A})
}

func (vm *VM) paletteSetChannel(args []Value, idx int, pos Pos) error {
	if vm.self == nil {
		return errorAt(pos, "palette can only change the color of a turtle, patch, or link")
	}
	n, err := args[0].Num()
	if err != nil {
		return err
	}
	name := []string{"palette:set-r", "palette:set-g", "palette:set-b"}[idx]
	if err := inRange(name, n, 0, 255); err != nil {
		return err
	}
	c, err := vm.agentRGBA()
	if err != nil {
		return err
	}
	ch := uint8(math.Round(n))
	switch idx {
	case 0:
		c.R = ch
	case 1:
		c.G = ch
	default:
		c.B = ch
	}
	return vm.setAgentColor(c)
}

func (vm *VM) agentRGBA() (color.RGBA, error) {
	switch a := vm.self.(type) {
	case *Turtle:
		if a == nil || a.dead {
			return color.RGBA{}, fmt.Errorf("that agent is dead")
		}
		if a.HasRGB {
			return a.RGB, nil
		}
		return ColorRGBA(a.Color), nil
	case *Patch:
		if a == nil || a.dead {
			return color.RGBA{}, fmt.Errorf("that agent is dead")
		}
		if a.HasRGB {
			return a.RGB, nil
		}
		return ColorRGBA(a.PColor), nil
	case *Link:
		if a == nil || a.dead {
			return color.RGBA{}, fmt.Errorf("that agent is dead")
		}
		if a.HasRGB {
			return a.RGB, nil
		}
		return ColorRGBA(a.Color), nil
	default:
		return color.RGBA{}, fmt.Errorf("palette can only be used by a turtle, patch, or link")
	}
}

func (vm *VM) setAgentColor(c color.RGBA) error {
	name := "color"
	if _, ok := vm.self.(*Patch); ok {
		name = "pcolor"
	}
	ok, err := vm.setAgentVar(name, rgbValue(c))
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("palette can only change the color of a turtle, patch, or link")
	}
	return nil
}

func rgbaOf(v Value) (color.RGBA, error) {
	c, _, err := ColorFromValue(v)
	return c, err
}

func rgbValue(c color.RGBA) Value {
	if c.A != 255 {
		return List(Num(float64(c.R)), Num(float64(c.G)), Num(float64(c.B)), Num(float64(c.A)))
	}
	return List(Num(float64(c.R)), Num(float64(c.G)), Num(float64(c.B)))
}

func storedColor(has bool, rgba color.RGBA, num float64) Value {
	if !has {
		return Num(num)
	}
	return rgbValue(rgba)
}

func transparencyOf(a uint8) float64 {
	return (255 - float64(a)) / 255 * 100
}

func alphaFromTransparency(t float64) uint8 {
	a := math.Round(255 * (1 - t/100))
	if a < 0 {
		a = 0
	}
	if a > 255 {
		a = 255
	}
	return uint8(a)
}

func inRange(name string, n, lo, hi float64) error {
	if math.IsNaN(n) || math.IsInf(n, 0) || n < lo || n > hi {
		return fmt.Errorf("%s expected a number from %g to %g", name, lo, hi)
	}
	return nil
}

func hsbComponent(name string, v Value, rounded bool) (Value, error) {
	c, err := rgbaOf(v)
	if err != nil {
		return Value{}, err
	}
	var h, s, b float64
	if rounded {
		h, s, b = RGBtoHSB(int(c.R), int(c.G), int(c.B))
	} else {
		h, s, b = rgbHSB(int(c.R), int(c.G), int(c.B))
	}
	switch name {
	case "palette:saturation-of", "palette:saturation":
		return Num(s), nil
	case "palette:brightness-of", "palette:brightness":
		return Num(b), nil
	default:
		return Num(h), nil
	}
}

func rgbComponent(name string, v Value) (Value, error) {
	c, err := rgbaOf(v)
	if err != nil {
		return Value{}, err
	}
	switch name {
	case "palette:g-of":
		return Num(float64(c.G)), nil
	case "palette:b-of":
		return Num(float64(c.B)), nil
	default:
		return Num(float64(c.R)), nil
	}
}

func scaleGradient(args []Value, hsb bool) (Value, error) {
	name := "palette:scale-gradient"
	if hsb {
		name = "palette:scale-gradient-hsb"
	}
	val, err := args[1].Num()
	if err != nil {
		return Value{}, fmt.Errorf("%s expected a number", name)
	}
	min, err := args[2].Num()
	if err != nil {
		return Value{}, fmt.Errorf("%s expected a number", name)
	}
	maxv, err := args[3].Num()
	if err != nil {
		return Value{}, fmt.Errorf("%s expected a number", name)
	}
	var n int
	var sample func(seg, i int) (int, int, int)
	if hsb {
		colors, err := parseHSBList(args[0])
		if err != nil {
			return Value{}, err
		}
		if len(colors) < 2 {
			return Value{}, fmt.Errorf("%s needs at least 2 colors", name)
		}
		n = len(colors)
		sample = func(seg, i int) (int, int, int) {
			row := hsbSegment(colors[seg], colors[seg+1])
			return row[i][0], row[i][1], row[i][2]
		}
	} else {
		colors, err := parseRGBList(args[0])
		if err != nil {
			return Value{}, err
		}
		if len(colors) < 2 {
			return Value{}, fmt.Errorf("%s needs at least 2 colors", name)
		}
		n = len(colors)
		sample = func(seg, i int) (int, int, int) {
			return lerpRGB(colors[seg], colors[seg+1], i)
		}
	}
	perc := scalePerc(val, min, maxv)
	idx := gradientIndex(n, perc)
	seg := idx / gradientSize
	if seg > n-2 {
		seg = n - 2
	}
	if seg < 0 {
		seg = 0
	}
	local := idx - seg*gradientSize
	if local < 0 {
		local = 0
	}
	if local >= gradientSize {
		local = gradientSize - 1
	}
	r, g, b := sample(seg, local)
	return List(Num(float64(r)), Num(float64(g)), Num(float64(b))), nil
}

func scalePerc(val, min, max float64) float64 {
	if math.IsNaN(val) || math.IsNaN(min) || math.IsNaN(max) {
		return 0
	}
	if min > max {
		switch {
		case val < max:
			return 1
		case val > min:
			return 0
		default:
			return (min - val) / (min - max)
		}
	}
	switch {
	case val > max:
		return 1
	case val < min:
		return 0
	case max == min:
		return 0
	default:
		return (val - min) / (max - min)
	}
}

func gradientIndex(n int, perc float64) int {
	if perc < 0 {
		perc = 0
	}
	if perc > 1 {
		perc = 1
	}
	var idx int
	if n < 3 {
		idx = int(math.Round(perc * float64(gradientSize-1)))
	} else {
		span := float64(gradientSize-1) + float64(gradientSize)*float64(n-2)
		idx = int(math.Round(perc * span))
	}
	limit := gradientSize * (n - 1)
	if idx < 0 {
		idx = 0
	}
	if idx >= limit {
		idx = limit - 1
	}
	return idx
}

func lerpRGB(a, b [3]int, i int) (int, int, int) {
	t := float64(i) / float64(gradientSize)
	return lerpByte(a[0], b[0], t), lerpByte(a[1], b[1], t), lerpByte(a[2], b[2], t)
}

func lerpByte(a, b int, t float64) int {
	v := math.Floor(float64(a) + float64(b-a)*t + 0.5)
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return int(v)
}

func hsbSegment(start, end [3]float64) [][3]int {
	div := float64(gradientSize - 1)
	var inc0 float64
	if end[0] > start[0] {
		if start[0]+360-end[0] < end[0]-start[0] {
			inc0 = (end[0] - (start[0] + 360)) / div
		} else {
			inc0 = (end[0] - start[0]) / div
		}
	} else {
		if end[0]+360-start[0] < start[0]-end[0] {
			inc0 = (end[0] + 360 - start[0]) / div
		} else {
			inc0 = (end[0] - start[0]) / div
		}
	}
	inc1 := (end[1] - start[1]) / div
	inc2 := (end[2] - start[2]) / div
	out := make([][3]int, gradientSize)
	h, s, b := start[0], start[1], start[2]
	for i := 0; i < gradientSize; i++ {
		if i > 0 {
			h += inc0
			s += inc1
			b += inc2
			if s < 0 {
				s = 0
			} else if s > 100 {
				s = 100
			}
			if b < 0 {
				b = 0
			} else if b > 100 {
				b = 100
			}
			if h >= 360 {
				h -= 360
			}
			if h < 0 {
				h += 360
			}
		}
		r, g, bl := HSBtoRGB(h, s, b)
		out[i] = [3]int{int(r), int(g), int(bl)}
	}
	return out
}

func parseRGBList(v Value) ([][3]int, error) {
	if v.K != KindList {
		return nil, fmt.Errorf("palette:scale-gradient expected a list of colors")
	}
	out := make([][3]int, len(v.L))
	for i, e := range v.L {
		c, err := rgbaOf(e)
		if err != nil {
			return nil, err
		}
		out[i] = [3]int{int(c.R), int(c.G), int(c.B)}
	}
	return out, nil
}

func parseHSBList(v Value) ([][3]float64, error) {
	if v.K != KindList {
		return nil, fmt.Errorf("palette:scale-gradient-hsb expected a list of HSB colors")
	}
	out := make([][3]float64, len(v.L))
	for i, e := range v.L {
		if e.K != KindList || len(e.L) != 3 {
			return nil, fmt.Errorf("HSB must have three elements")
		}
		var hsb [3]float64
		for k := 0; k < 3; k++ {
			n, err := e.L[k].Num()
			if err != nil {
				return nil, err
			}
			hsb[k] = n
		}
		if hsb[0] < 0 || hsb[0] > 360 {
			return nil, fmt.Errorf("Hue must be in the range from 0 to 360")
		}
		if hsb[1] < 0 || hsb[1] > 100 {
			return nil, fmt.Errorf("Saturation must be in the range from 0 to 100")
		}
		if hsb[2] < 0 || hsb[2] > 100 {
			return nil, fmt.Errorf("Brightness must be in the range from 0 to 100")
		}
		out[i] = hsb
	}
	return out, nil
}

func schemeColors(args []Value) (Value, error) {
	kind, name, err := schemeArgs(args[0], args[1])
	if err != nil {
		return Value{}, err
	}
	n, err := asInt(args[2], "palette:scheme-colors")
	if err != nil {
		return Value{}, err
	}
	leg, err := lookupScheme(kind, name, n)
	if err != nil {
		return Value{}, err
	}
	items := make([]Value, len(leg))
	for i, c := range leg {
		items[i] = List(Num(float64(c[0])), Num(float64(c[1])), Num(float64(c[2])))
	}
	return List(items...), nil
}

func scaleScheme(args []Value) (Value, error) {
	kind, name, err := schemeArgs(args[0], args[1])
	if err != nil {
		return Value{}, err
	}
	n, err := asInt(args[2], "palette:scale-scheme")
	if err != nil {
		return Value{}, err
	}
	val, err := args[3].Num()
	if err != nil {
		return Value{}, fmt.Errorf("palette:scale-scheme expected a number")
	}
	min, err := args[4].Num()
	if err != nil {
		return Value{}, fmt.Errorf("palette:scale-scheme expected a number")
	}
	maxv, err := args[5].Num()
	if err != nil {
		return Value{}, fmt.Errorf("palette:scale-scheme expected a number")
	}
	leg, err := lookupScheme(kind, name, n)
	if err != nil {
		return Value{}, err
	}
	c := leg[schemeIndex(len(leg), val, min, maxv)]
	return List(Num(float64(c[0])), Num(float64(c[1])), Num(float64(c[2]))), nil
}

func schemeArgs(kindV, nameV Value) (string, string, error) {
	if kindV.K != KindString || nameV.K != KindString {
		return "", "", fmt.Errorf("palette scheme type and name must be strings")
	}
	return kindV.S, nameV.S, nil
}

func lookupScheme(kind, name string, n int) ([][3]int, error) {
	if n < 3 {
		return nil, fmt.Errorf("the minimum number of colors in a scheme is 3, not %d", n)
	}
	var table map[string][]schemeLegend
	switch kind {
	case "Sequential":
		table = sequentialSchemes
	case "Divergent":
		table = divergentSchemes
	case "Qualitative":
		table = qualitativeSchemes
	default:
		return nil, fmt.Errorf("scheme type must be Sequential, Divergent, or Qualitative")
	}
	legends, ok := table[name]
	if !ok {
		return nil, fmt.Errorf("there is no %s scheme named %s", kind, name)
	}
	maxN := 0
	for _, leg := range legends {
		if len(leg) > maxN {
			maxN = len(leg)
		}
		if len(leg) == n {
			return leg, nil
		}
	}
	return nil, fmt.Errorf("%s has a maximum of %d colors, not %d", name, maxN, n)
}

// schemeIndex is the ColorBrewer bin used by palette:scale-scheme.
// A reversed range (range2 < range1) walks the legend backward.
func schemeIndex(size int, val, min, max float64) int {
	if size < 1 {
		return 0
	}
	if (max >= min && val <= min) || (min > max && val > min) {
		return 0
	}
	if (min > max && val < max) || (max >= min && val > max) {
		return size - 1
	}
	temp := val - min
	if max < min {
		temp = min - val
	}
	span := math.Abs(max - min)
	if span == 0 {
		return 0
	}
	span /= float64(size)
	idx := int(math.Floor(temp / span))
	if idx < 0 {
		return 0
	}
	if idx > size-1 {
		return size - 1
	}
	return idx
}
