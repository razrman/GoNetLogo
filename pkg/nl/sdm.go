package nl

import (
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// SystemDynamics is the stock-and-flow diagram stored in a model file.
// NetLogo does not put the diagram's procedures in the code tab. It
// compiles the diagram into globals and procedures, and the code tab
// calls those. Source reproduces that compilation: Euler integration,
// with each flow reported as its expression times dt.
type SystemDynamics struct {
	DT         string
	Stocks     []SDStock
	Rates      []SDRate
	Converters []SDConverter
}

// SDStock is one stock. Initial is a NetLogo expression, which may
// contain comments. AllowNegative false clamps the stock at zero.
type SDStock struct {
	Name          string
	Initial       string
	AllowNegative bool
}

// SDRate is one flow. Source and Sink are the names of the figures the
// flow leaves and enters. A reservoir has no name, so that end is empty.
type SDRate struct {
	Name       string
	Expression string
	Source     string
	Sink       string
}

// SDConverter is one auxiliary variable. A constant expression becomes a
// global set during system-dynamics-setup. Anything else is a reporter,
// re-evaluated each step.
type SDConverter struct {
	Name       string
	Expression string
	Constant   bool
}

type sdFigure struct {
	kind          string
	name          string
	text          string
	allowNegative bool
	start, end    int
	hasStart      bool
	hasEnd        bool
}

// parseSDSection reads the classic .nlogo system-dynamics section.
// NetLogo 6 stores the same <systemDynamics> element the .nlogox file
// uses. A section without that element is not a diagram.
func parseSDSection(section string) (*SystemDynamics, error) {
	i := strings.Index(section, "<systemDynamics")
	if i < 0 {
		return nil, nil
	}
	dec := xml.NewDecoder(strings.NewReader(section[i:]))
	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("system dynamics: %w", err)
	}
	se, ok := tok.(xml.StartElement)
	if !ok || se.Name.Local != "systemDynamics" {
		return nil, fmt.Errorf("system dynamics: expected a systemDynamics element")
	}
	return parseSystemDynamics(dec, se)
}

// parseSystemDynamics reads a <systemDynamics> element. The decoder has
// already consumed the start tag. Every child element is one diagram
// figure, including bindings, because rate endpoints are figure indexes
// in document order.
func parseSystemDynamics(dec *xml.Decoder, start xml.StartElement) (*SystemDynamics, error) {
	dt := attr(start, "dt")
	if strings.TrimSpace(dt) == "" {
		dt = "1.0"
	}
	var figures []sdFigure
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil, fmt.Errorf("system dynamics: unclosed systemDynamics element")
		}
		if err != nil {
			return nil, fmt.Errorf("system dynamics: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			fig, err := readSDFigure(dec, t)
			if err != nil {
				return nil, err
			}
			figures = append(figures, fig)
		case xml.EndElement:
			if t.Name.Local == "systemDynamics" {
				return systemDynamicsFrom(dt, figures), nil
			}
		}
	}
}

func readSDFigure(dec *xml.Decoder, se xml.StartElement) (sdFigure, error) {
	fig := sdFigure{kind: se.Name.Local, name: attr(se, "name")}
	switch se.Name.Local {
	case "stock":
		text, err := readElementText(dec, se.Name.Local)
		if err != nil {
			return sdFigure{}, fmt.Errorf("system dynamics: %w", err)
		}
		fig.text = text
		fig.allowNegative = true
		if v, ok := attrPresent(se, "allowNegative"); ok && strings.EqualFold(strings.TrimSpace(v), "false") {
			fig.allowNegative = false
		}
	case "rate":
		text, err := readElementText(dec, se.Name.Local)
		if err != nil {
			return sdFigure{}, fmt.Errorf("system dynamics: %w", err)
		}
		fig.text = text
		if start, ok, err := attrIndex(se, "startFigure"); err != nil {
			return sdFigure{}, err
		} else if ok {
			fig.start = start
			fig.hasStart = true
		}
		if end, ok, err := attrIndex(se, "endFigure"); err != nil {
			return sdFigure{}, err
		} else if ok {
			fig.end = end
			fig.hasEnd = true
		}
	case "converter":
		text, err := readElementText(dec, se.Name.Local)
		if err != nil {
			return sdFigure{}, fmt.Errorf("system dynamics: %w", err)
		}
		fig.text = text
	default:
		// Reservoirs, bindings, and any later figure kind still occupy an
		// index. Skip their contents.
		if err := dec.Skip(); err != nil {
			return sdFigure{}, fmt.Errorf("system dynamics: %w", err)
		}
	}
	return fig, nil
}

func attrPresent(se xml.StartElement, name string) (string, bool) {
	for _, a := range se.Attr {
		if a.Name.Local == name {
			return a.Value, true
		}
	}
	return "", false
}

func attrIndex(se xml.StartElement, name string) (int, bool, error) {
	v, ok := attrPresent(se, name)
	if !ok || strings.TrimSpace(v) == "" {
		return 0, false, nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return 0, false, fmt.Errorf("system dynamics: %s %q is not an integer", name, v)
	}
	return n, true, nil
}

func figureName(figures []sdFigure, index int) string {
	if index < 0 || index >= len(figures) {
		return ""
	}
	return figures[index].name
}

func systemDynamicsFrom(dt string, figures []sdFigure) *SystemDynamics {
	sd := &SystemDynamics{DT: dt}
	for _, fig := range figures {
		if fig.name == "" {
			continue
		}
		switch fig.kind {
		case "stock":
			initial := fig.text
			if strings.TrimSpace(initial) == "" {
				initial = "0"
			}
			sd.Stocks = append(sd.Stocks, SDStock{
				Name:          fig.name,
				Initial:       initial,
				AllowNegative: fig.allowNegative,
			})
		case "rate":
			if fig.text == "" {
				continue
			}
			src, sink := "", ""
			if fig.hasStart {
				src = figureName(figures, fig.start)
			}
			if fig.hasEnd {
				sink = figureName(figures, fig.end)
			}
			sd.Rates = append(sd.Rates, SDRate{
				Name:       fig.name,
				Expression: fig.text,
				Source:     src,
				Sink:       sink,
			})
		case "converter":
			if fig.text == "" {
				continue
			}
			sd.Converters = append(sd.Converters, SDConverter{
				Name:       fig.name,
				Expression: fig.text,
				Constant:   isSDConstant(fig.text),
			})
		}
	}
	return sd
}

// Source is the NetLogo fragment NetLogo's System Dynamics compiler
// would add to the model. Empty when sd is nil.
func (sd *SystemDynamics) Source() string {
	if sd == nil {
		return ""
	}
	dt := sd.DT
	if strings.TrimSpace(dt) == "" {
		dt = "1.0"
	}
	stocks := append([]SDStock(nil), sd.Stocks...)
	sort.SliceStable(stocks, func(i, j int) bool {
		return sdStockLess(stocks[i], stocks[j])
	})
	var constants, variables []SDConverter
	for _, c := range sd.Converters {
		if c.Constant {
			constants = append(constants, c)
		} else {
			variables = append(variables, c)
		}
	}

	var globals, procs, plots strings.Builder
	globals.WriteString(";; System dynamics model globals\nglobals [\n")
	procs.WriteString(";; Initializes the system dynamics model.\n")
	procs.WriteString(";; Call this in your model's SETUP procedure.\n")
	procs.WriteString("to system-dynamics-setup\n reset-ticks\n set dt ")
	procs.WriteString(dt)
	procs.WriteString("\n")
	if len(constants) > 0 {
		globals.WriteString(" ;; constants\n")
		procs.WriteString(" ;; initialize constant values\n")
		for _, c := range constants {
			globals.WriteString(" ")
			globals.WriteString(c.Name)
			globals.WriteString("\n")
			procs.WriteString(" set ")
			procs.WriteString(c.Name)
			procs.WriteString(" ")
			procs.WriteString(sdValue(c.Expression))
			procs.WriteString("\n")
		}
	}
	if len(stocks) > 0 {
		globals.WriteString(" ;; stock values\n")
		procs.WriteString(" ;; initialize stock values\n")
		for _, s := range stocks {
			globals.WriteString(" ")
			globals.WriteString(s.Name)
			globals.WriteString("\n")
			procs.WriteString(" set ")
			procs.WriteString(s.Name)
			procs.WriteString(" ")
			procs.WriteString(sdValue(s.Initial))
			procs.WriteString("\n")
		}
	}
	globals.WriteString(" ;; size of each step, see SYSTEM-DYNAMICS-GO\n dt\n]\n\n")
	procs.WriteString("end\n\n")
	procs.WriteString(";; Step through the system dynamics model by performing next iteration of Euler's method.\n")
	procs.WriteString(";; Call this in your model's GO procedure.\n")
	procs.WriteString("to system-dynamics-go\n")
	plots.WriteString(";; Plot the current state of the system dynamics model's stocks\n")
	plots.WriteString(";; Call this procedure in your plot's update commands.\n")
	plots.WriteString("to system-dynamics-do-plot\n")
	if len(variables) > 0 || len(sd.Rates) > 0 {
		procs.WriteString("\n ;; compute variable and flow values once per step\n")
		for _, c := range variables {
			writeLocal(&procs, c.Name)
		}
		for _, r := range sd.Rates {
			writeLocal(&procs, r.Name)
		}
	}
	if len(stocks) > 0 {
		procs.WriteString("\n ;; update stock values\n")
		procs.WriteString(" ;; use temporary variables so order of computation doesn't affect result.\n")
		for _, s := range stocks {
			procs.WriteString(stockUpdate(s, sd.Rates))
			plots.WriteString(" if plot-pen-exists? \"")
			plots.WriteString(s.Name)
			plots.WriteString("\" [\n")
			plots.WriteString(" set-current-plot-pen \"")
			plots.WriteString(s.Name)
			plots.WriteString("\"\n")
			plots.WriteString(" plotxy ticks ")
			plots.WriteString(s.Name)
			plots.WriteString("\n ]\n")
		}
		for _, s := range stocks {
			procs.WriteString(" set ")
			procs.WriteString(s.Name)
			procs.WriteString(" new-")
			procs.WriteString(s.Name)
			procs.WriteString("\n")
		}
	}
	procs.WriteString("\n tick-advance dt\nend\n\n")
	plots.WriteString("end\n\n")

	var reps strings.Builder
	for _, r := range sd.Rates {
		reps.WriteString(";; Report value of flow\nto-report ")
		reps.WriteString(r.Name)
		reps.WriteString("\n report ( ")
		reps.WriteString(sdValue(r.Expression))
		reps.WriteString("\n ) * dt\nend\n\n")
	}
	for _, c := range variables {
		reps.WriteString(";; Report value of variable\nto-report ")
		reps.WriteString(c.Name)
		reps.WriteString("\n report ")
		reps.WriteString(sdValue(c.Expression))
		reps.WriteString("\nend\n\n")
	}
	return globals.String() + procs.String() + plots.String() + reps.String()
}

func writeLocal(b *strings.Builder, name string) {
	b.WriteString(" let local-")
	b.WriteString(name)
	b.WriteString(" ")
	b.WriteString(name)
	b.WriteString("\n")
}

func sdValue(expr string) string {
	if strings.TrimSpace(expr) == "" {
		return "0"
	}
	return expr
}

// stockUpdate is one new-stock binding. Flows already include dt, so the
// stock changes by the flow amount itself. A stock that cannot go
// negative is wrapped in max(list 0 ...).
func stockUpdate(s SDStock, rates []SDRate) string {
	var b strings.Builder
	b.WriteString(" let new-")
	b.WriteString(s.Name)
	if s.AllowNegative {
		b.WriteString(" ( ")
	} else {
		b.WriteString(" max( list 0 ( ")
	}
	b.WriteString(s.Name)
	b.WriteString(" ")
	for _, r := range rates {
		if s.Name != "" && r.Source == s.Name {
			b.WriteString("- local-")
			b.WriteString(r.Name)
			b.WriteString(" ")
		}
		if s.Name != "" && r.Sink == s.Name {
			b.WriteString("+ local-")
			b.WriteString(r.Name)
			b.WriteString(" ")
		}
	}
	if !s.AllowNegative {
		b.WriteString(") ")
	}
	b.WriteString(")\n")
	return b.String()
}

// sdStockLess matches NetLogo's stock ordering: literal initial values
// first, then by name. A stock whose initial value reads another stock
// then sees the literal stocks already set.
func sdStockLess(a, b SDStock) bool {
	ac := isSDConstant(a.Initial)
	bc := isSDConstant(b.Initial)
	if ac != bc {
		return ac
	}
	return strings.ToUpper(a.Name) < strings.ToUpper(b.Name)
}

// isSDConstant reports whether expr is one NetLogo literal, the test
// read-from-string uses to decide that a converter is a constant.
// Comments are ignored, as they are by the compiler tokenizer.
func isSDConstant(expr string) bool {
	toks := Lex(strings.ToUpper(expr))
	for _, t := range toks {
		if t.Kind == TokError {
			return false
		}
	}
	ok, n := parseSDLiteral(toks, 0)
	if !ok || n >= len(toks) {
		return false
	}
	return toks[n].Kind == TokEOF
}

func parseSDLiteral(toks []Token, i int) (bool, int) {
	if i >= len(toks) {
		return false, i
	}
	switch toks[i].Kind {
	case TokNumber, TokString:
		return true, i + 1
	case TokIdent:
		switch toks[i].Lit {
		case "true", "false", "nobody":
			return true, i + 1
		default:
			return false, i
		}
	case TokLBrack:
		i++
		if i < len(toks) && toks[i].Kind == TokRBrack {
			return true, i + 1
		}
		for i < len(toks) && toks[i].Kind != TokEOF {
			if toks[i].Kind == TokRBrack {
				return true, i + 1
			}
			ok, j := parseSDLiteral(toks, i)
			if !ok {
				return false, i
			}
			i = j
		}
		return false, i
	default:
		return false, i
	}
}
