package nl

import (
	"fmt"
	"image/color"
	"math"
)

// NetLogo colors live in [0, 140). Multiples of 10 plus 5 are the base hues.
// Shades below the base are darker; shades above are lighter. Black is 0 and
// white is 9.9, overriding the gray ramp at those two points.
//
// The mapping follows the NetLogo color model (Uri Wilensky / CCL): fourteen
// base RGB triples, then a brightness/saturation step of (n - 5) / 50.48.

const maxColor = 140.0

var colorNames = map[string]float64{
	"gray": 5, "grey": 5,
	"red":       15,
	"orange":    25,
	"brown":     35,
	"yellow":    45,
	"green":     55,
	"lime":      65,
	"turquoise": 75,
	"cyan":      85,
	"sky":       95,
	"blue":      105,
	"violet":    115,
	"magenta":   125,
	"pink":      135,
	"black":     0,
	"white":     9.9,
}

var baseRGB = [14][3]int{
	{140, 140, 140},
	{215, 48, 39},
	{241, 105, 19},
	{156, 109, 70},
	{237, 237, 47},
	{87, 176, 58},
	{42, 209, 57},
	{27, 158, 119},
	{82, 196, 196},
	{43, 140, 190},
	{50, 92, 168},
	{123, 78, 163},
	{166, 25, 105},
	{224, 126, 149},
}

// argbCache[i] is the color for number i/10.
var argbCache [1400]color.RGBA

func init() {
	for i := 0; i < 1400; i++ {
		argbCache[i] = computeARGB(i)
	}
	argbCache[0] = color.RGBA{0, 0, 0, 255}
	argbCache[99] = color.RGBA{255, 255, 255, 255}
}

func computeARGB(colorTimesTen int) color.RGBA {
	base := colorTimesTen / 100
	if base > 13 {
		base = 13
	}
	r := baseRGB[base][0]
	g := baseRGB[base][1]
	b := baseRGB[base][2]
	step := float64(colorTimesTen%100-50)/50.48 + 0.012
	if step < 0 {
		r += int(float64(r) * step)
		g += int(float64(g) * step)
		b += int(float64(b) * step)
	} else if step > 0 {
		r += int(float64(0xff-r) * step)
		g += int(float64(0xff-g) * step)
		b += int(float64(0xff-b) * step)
	}
	r = clampByte(r)
	g = clampByte(g)
	b = clampByte(b)
	return color.RGBA{uint8(r), uint8(g), uint8(b), 255}
}

func clampByte(v int) int {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}

// ModulateColor wraps a color number into [0, 140).
func ModulateColor(c float64) float64 {
	if c >= 0 && c < maxColor {
		return c
	}
	c = math.Mod(c, maxColor)
	if c < 0 {
		c += maxColor
	}
	if c >= maxColor {
		c = 139.9999999999999
	}
	return c
}

// ColorRGBA maps a NetLogo color number to sRGB.
func ColorRGBA(c float64) color.RGBA {
	c = ModulateColor(c)
	idx := int(c * 10)
	if idx < 0 {
		idx = 0
	}
	if idx > 1399 {
		idx = 1399
	}
	return argbCache[idx]
}

// ColorFromValue accepts a number or an RGB / RGBA list.
func ColorFromValue(v Value) (color.RGBA, float64, error) {
	switch v.K {
	case KindNum:
		return ColorRGBA(v.N), v.N, nil
	case KindList:
		if len(v.L) != 3 && len(v.L) != 4 {
			return color.RGBA{}, 0, errf("a color must be a number or an RGB list")
		}
		ns := make([]int, len(v.L))
		for i, e := range v.L {
			n, err := e.Num()
			if err != nil {
				return color.RGBA{}, 0, err
			}
			ns[i] = int(n)
			if ns[i] < 0 || ns[i] > 255 {
				return color.RGBA{}, 0, errf("RGB values must be in 0-255")
			}
		}
		a := 255
		if len(ns) == 4 {
			a = ns[3]
		}
		return color.RGBA{uint8(ns[0]), uint8(ns[1]), uint8(ns[2]), uint8(a)}, 0, nil
	default:
		return color.RGBA{}, 0, errf("expected a color, got %s", v.TypeName())
	}
}

// CentralColor is the base hue (5, 15, ..., 135) for a shade.
func CentralColor(c float64) float64 {
	c = ModulateColor(c)
	return (float64(int(c/10)) + 0.5) * 10
}

// ScaleColor implements scale-color.
func ScaleColor(base, n, r1, r2 float64) float64 {
	colorBase := CentralColor(base) - 5
	var perc float64
	if r1 > r2 {
		switch {
		case n < r2:
			perc = 1
		case n > r1:
			perc = 0
		default:
			perc = (r1 - n) / (r1 - r2)
		}
	} else if r1 == r2 {
		if n > r1 {
			perc = 1
		} else {
			perc = 0
		}
	} else {
		switch {
		case n > r2:
			perc = 1
		case n <= r1:
			perc = 0
		default:
			perc = (n - r1) / (r2 - r1)
		}
	}
	perc *= 10
	if perc >= 9.9999 {
		perc = 9.9999
	} else if perc < 0 {
		perc = 0
	}
	return colorBase + perc
}

func HSBtoRGB(h, s, b float64) (uint8, uint8, uint8) {
	if h < 0 {
		h = 0
	} else if h > 360 {
		h = 360
	}
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
	hf := float32(h / 360.0)
	sf := float32(s / 100.0)
	bf := float32(b / 100.0)
	if sf == 0 {
		v := uint8(int(bf*255.0 + 0.5))
		return v, v, v
	}
	hPos := (hf - float32(math.Floor(float64(hf)))) * 6.0
	f := hPos - float32(math.Floor(float64(hPos)))
	p := bf * (1.0 - sf)
	q := bf * (1.0 - sf*f)
	t := bf * (1.0 - (sf * (1.0 - f)))
	var r, g, bb int
	switch int(hPos) {
	case 0:
		r = int(bf*255.0 + 0.5)
		g = int(t*255.0 + 0.5)
		bb = int(p*255.0 + 0.5)
	case 1:
		r = int(q*255.0 + 0.5)
		g = int(bf*255.0 + 0.5)
		bb = int(p*255.0 + 0.5)
	case 2:
		r = int(p*255.0 + 0.5)
		g = int(bf*255.0 + 0.5)
		bb = int(t*255.0 + 0.5)
	case 3:
		r = int(p*255.0 + 0.5)
		g = int(q*255.0 + 0.5)
		bb = int(bf*255.0 + 0.5)
	case 4:
		r = int(t*255.0 + 0.5)
		g = int(p*255.0 + 0.5)
		bb = int(bf*255.0 + 0.5)
	default:
		r = int(bf*255.0 + 0.5)
		g = int(p*255.0 + 0.5)
		bb = int(q*255.0 + 0.5)
	}
	return uint8(clampByte(r)), uint8(clampByte(g)), uint8(clampByte(bb))
}

func RGBtoHSB(r, g, b int) (float64, float64, float64) {
	h, s, br := rgbHSB(r, g, b)
	return roundPlaces(h, 3), roundPlaces(s, 3), roundPlaces(br, 3)
}

// rgbHSB is Java Color.RGBtoHSB scaled to hue 0–360 and saturation and
// brightness 0–100, without rounding. palette:with-* uses the unrounded
// components so a round trip matches the extension.
func rgbHSB(r, g, b int) (float64, float64, float64) {
	rf := float32(clampByte(r))
	gf := float32(clampByte(g))
	bf := float32(clampByte(b))

	cmax := rf
	if gf > cmax {
		cmax = gf
	}
	if bf > cmax {
		cmax = bf
	}
	cmin := rf
	if gf < cmin {
		cmin = gf
	}
	if bf < cmin {
		cmin = bf
	}

	brightness := cmax / 255.0
	var saturation float32
	if cmax != 0 {
		saturation = (cmax - cmin) / cmax
	}
	var hue float32
	if saturation != 0 {
		redc := (cmax - rf) / (cmax - cmin)
		greenc := (cmax - gf) / (cmax - cmin)
		bluec := (cmax - bf) / (cmax - cmin)
		if rf == cmax {
			hue = bluec - greenc
		} else if gf == cmax {
			hue = 2.0 + redc - bluec
		} else {
			hue = 4.0 + greenc - redc
		}
		hue = hue / 6.0
		if hue < 0 {
			hue = hue + 1.0
		}
	}
	return float64(hue * 360.0), float64(saturation * 100.0), float64(brightness * 100.0)
}

func roundPlaces(n float64, places int) float64 {
	p := math.Pow(10, float64(places))
	return math.Round(n*p) / p
}

func colorDistance(r1, g1, b1, r2, g2, b2 int) int64 {
	// Compuphase perceptual distance (https://www.compuphase.com/cmetric.htm):
	//   rmean = (r1 + r2) / 2
	// NetLogo's Color.scala writes `r1 + r2 / 2`. Division binds tighter than
	// addition, so that expression is r1 + (r2 / 2), not the mean of the two
	// red channels. approximate-rgb uses the mean.
	rmean := int64(r1+r2) / 2
	rd := int64(r1 - r2)
	gd := int64(g1 - g2)
	bd := int64(b1 - b2)
	return (((512 + rmean) * rd * rd) >> 8) + 4*gd*gd + (((767 - rmean) * bd * bd) >> 8)
}

func ApproximateRGB(r, g, b int) float64 {
	r = clampByte(r)
	g = clampByte(g)
	b = clampByte(b)
	for i := 0; i < 1400; i++ {
		c := argbCache[i]
		if int(c.R) == r && int(c.G) == g && int(c.B) == b {
			return float64(i) / 10.0
		}
	}
	var smallestDistance int64 = 100000000
	var closest float64
	for i := 0; i < 1400; i++ {
		c := argbCache[i]
		dist := colorDistance(r, g, b, int(c.R), int(c.G), int(c.B))
		if dist < smallestDistance {
			smallestDistance = dist
			closest = float64(i) / 10.0
		}
	}
	return closest
}

func errf(f string, args ...any) error {
	return &rtError{msg: fmt.Sprintf(f, args...)}
}
