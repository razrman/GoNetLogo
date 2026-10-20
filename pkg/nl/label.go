package nl

import (
	"image/color"
	"math"
	"strconv"
)

// ViewLabel is one non-empty turtle or patch label, ready to draw.
// Turtle labels set Turtle; patch labels set PX and PY.
type ViewLabel struct {
	Text    string
	R, G, B uint8
	Turtle  bool
	Who     int
	PX, PY  int
}

// DisplayLabel formats a stored label for the view.
// Turtle.Label and Patch.Label stay strings. A plain number, including a
// numeric string such as "3.0", uses NetLogo's number format. Any other
// text is unchanged. An empty string is not drawn.
func DisplayLabel(s string) string {
	if s == "" || !plainNumber(s) {
		return s
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		return s
	}
	return formatNum(n)
}

func plainNumber(s string) bool {
	i := 0
	if s[0] == '+' || s[0] == '-' {
		i++
		if i >= len(s) {
			return false
		}
	}
	digits := 0
	dot := false
	for ; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
			digits++
		case c == '.' && !dot:
			dot = true
		default:
			return false
		}
	}
	return digits > 0
}

// ViewLabels lists turtle and patch labels that should be drawn.
// Empty labels and hidden turtles are left out.
func (w *World) ViewLabels() []ViewLabel {
	if w == nil {
		return nil
	}
	var out []ViewLabel
	for _, p := range w.patchOrder {
		text, col, ok := p.viewLabel()
		if !ok {
			continue
		}
		out = append(out, ViewLabel{
			Text: text, R: col.R, G: col.G, B: col.B,
			PX: p.PX, PY: p.PY,
		})
	}
	if len(w.whoSeq) != len(w.turtles) {
		w.rebuildWhoSeq()
	}
	for _, id := range w.whoSeq {
		t := w.turtles[id]
		if t == nil || t.dead {
			continue
		}
		text, col, ok := t.viewLabel()
		if !ok {
			continue
		}
		out = append(out, ViewLabel{
			Text: text, R: col.R, G: col.G, B: col.B,
			Turtle: true, Who: t.Who,
		})
	}
	return out
}

func (t *Turtle) viewLabel() (string, color.RGBA, bool) {
	if t == nil || t.dead || t.Hidden {
		return "", color.RGBA{}, false
	}
	text := DisplayLabel(t.Label)
	if text == "" {
		return "", color.RGBA{}, false
	}
	return text, labelInk(t.Vars, "label-color"), true
}

func (p *Patch) viewLabel() (string, color.RGBA, bool) {
	if p == nil {
		return "", color.RGBA{}, false
	}
	text := DisplayLabel(p.Label)
	if text == "" {
		return "", color.RGBA{}, false
	}
	return text, labelInk(p.Vars, "plabel-color"), true
}

func labelInk(vars map[string]Value, key string) color.RGBA {
	if vars != nil {
		if v, ok := vars[key]; ok {
			if c, _, err := ColorFromValue(v); err == nil {
				if c.A == 0 {
					c.A = 255
				}
				return c
			}
		}
	}
	return ColorRGBA(9.9)
}
