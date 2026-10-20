package nl

import "image/color"

// ViewLabel is one non-empty turtle, patch, or link label, ready to draw.
// Turtle labels set Turtle. Patch labels set PX and PY. Link labels set
// Link and the midpoint X, Y.
type ViewLabel struct {
	Text    string
	R, G, B uint8
	Turtle  bool
	Who     int
	PX, PY  int
	Link    bool
	X, Y    float64
}

// DisplayLabel formats a stored label for the view.
// The agent keeps the original value. Numbers use NetLogo's number format.
// Every other value uses the same text as print. An empty string is not drawn.
func DisplayLabel(v Value) string {
	return LabelString(v)
}

// ViewLabels lists turtle, patch, and link labels that should be drawn.
// Empty labels and hidden turtles and links are left out.
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
	for _, l := range w.links {
		text, col, ok := l.viewLabel()
		if !ok {
			continue
		}
		out = append(out, ViewLabel{
			Text: text, R: col.R, G: col.G, B: col.B,
			Link: true,
			X:    (l.End1.X + l.End2.X) / 2,
			Y:    (l.End1.Y + l.End2.Y) / 2,
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

func (l *Link) viewLabel() (string, color.RGBA, bool) {
	if l == nil || l.dead || l.Hidden || l.End1 == nil || l.End2 == nil || l.End1.dead || l.End2.dead {
		return "", color.RGBA{}, false
	}
	text := DisplayLabel(l.Label)
	if text == "" {
		return "", color.RGBA{}, false
	}
	return text, labelInk(l.Vars, "label-color"), true
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
