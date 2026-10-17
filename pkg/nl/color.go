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

func errf(f string, args ...any) error {
	return &rtError{msg: fmt.Sprintf(f, args...)}
}
