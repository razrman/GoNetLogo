package nl

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// Section separator used by the classic .nlogo text format.
const sectionSep = "@#$#@#$#@"

// ViewSpec is the world geometry stored in a model's view widget.
type ViewSpec struct {
	MinPxcor, MaxPxcor int
	MinPycor, MaxPycor int
	WrapX, WrapY       bool
	PatchSize          float64
	Set                bool
}

// DefaultView is NetLogo's default world: -16..16, wrapping, patch size 13.
func DefaultView() ViewSpec {
	return ViewSpec{MinPxcor: -16, MaxPxcor: 16, MinPycor: -16, MaxPycor: 16, WrapX: true, WrapY: true, PatchSize: 13, Set: true}
}

// Widget is one interface widget. Plots are recorded as Kind "plot" and are not run.
type Widget struct {
	Kind     string // button, slider, switch, monitor, chooser, input, note, plot, output
	X, Y     int
	W, H     int
	Display  string
	Variable string
	Source   string
	Forever  bool
	// Agent is observer, turtle, patch, or link.
	Agent             string
	Min, Max          float64
	Value             float64
	Step              float64
	On                bool
	Choices           []Value
	Current           int
	Units             string
	Precision         int
	InputType         string
	Text              string
	DisableUntilTicks bool
}

// FileModel is a program plus the widgets parsed from a .nlogo or .nlogox file.
type FileModel struct {
	Name    string
	Code    string
	View    ViewSpec
	Widgets []Widget
	// Notes lists widgets this milestone does not run (plots, and anything unrecognized).
	Notes []string
}

// LoadFile reads a .nlogo or .nlogox model.
func LoadFile(path string) (*FileModel, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	m, err := ParseModel(path, string(b))
	if err != nil {
		return nil, err
	}
	return m, nil
}

// ParseModel accepts a .nlogo document, a .nlogox document, or bare NetLogo code.
// The text decides when it is clearly one format. The filename extension is
// the tie-breaker, and it also selects a parser when the text has neither
// the classic section separator nor an XML model tag.
func ParseModel(name, content string) (*FileModel, error) {
	trim := strings.TrimSpace(content)
	ext := modelExt(name)
	hasSep := strings.Contains(content, sectionSep)
	looksXML := strings.HasPrefix(trim, "<?xml") || strings.HasPrefix(trim, "<model") || strings.Contains(trim, "<model ")
	// A .nlogox file whose code happens to contain the section separator
	// stays XML. Any other file with the separator is the classic text format.
	if hasSep && !(looksXML && ext == ".nlogox") {
		return parseNLogo(name, content)
	}
	if looksXML || ext == ".nlogox" {
		return parseNLogoX(name, content)
	}
	if ext == ".nlogo" {
		return parseNLogo(name, content)
	}
	return &FileModel{Name: name, Code: content, View: DefaultView()}, nil
}

func modelExt(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	if i := strings.LastIndex(name, "."); i >= 0 {
		return strings.ToLower(name[i:])
	}
	return ""
}

func parseNLogo(name, content string) (*FileModel, error) {
	parts := strings.Split(content, sectionSep)
	code := ""
	if len(parts) > 0 {
		code = parts[0]
	}
	widgets := ""
	if len(parts) > 1 {
		widgets = parts[1]
	}
	m := &FileModel{Name: name, Code: code, View: DefaultView()}
	if strings.TrimSpace(widgets) != "" {
		if err := parseWidgetSection(m, widgets); err != nil {
			return nil, err
		}
	}
	if !m.View.Set {
		m.View = DefaultView()
	}
	return m, nil
}

func parseWidgetSection(m *FileModel, section string) error {
	lines := strings.Split(strings.ReplaceAll(section, "\r\n", "\n"), "\n")
	var block []string
	flush := func() error {
		if len(block) == 0 {
			return nil
		}
		// Drop a trailing empty line that split may have kept inside the block.
		for len(block) > 0 && strings.TrimSpace(block[len(block)-1]) == "" {
			block = block[:len(block)-1]
		}
		if len(block) == 0 {
			return nil
		}
		err := parseWidgetBlock(m, block)
		block = nil
		return err
	}
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			if err := flush(); err != nil {
				return err
			}
			continue
		}
		block = append(block, line)
	}
	return flush()
}

func parseWidgetBlock(m *FileModel, lines []string) error {
	kind := strings.ToUpper(strings.TrimSpace(lines[0]))
	switch kind {
	case "GRAPHICS-WINDOW":
		v, err := parseViewLines(lines)
		if err != nil {
			return err
		}
		m.View = v
	case "BUTTON":
		w, err := parseButtonLines(lines)
		if err != nil {
			return err
		}
		m.Widgets = append(m.Widgets, w)
	case "SLIDER":
		w, err := parseSliderLines(lines)
		if err != nil {
			return err
		}
		m.Widgets = append(m.Widgets, w)
	case "SWITCH":
		w, err := parseSwitchLines(lines)
		if err != nil {
			return err
		}
		m.Widgets = append(m.Widgets, w)
	case "MONITOR":
		w, err := parseMonitorLines(lines)
		if err != nil {
			return err
		}
		m.Widgets = append(m.Widgets, w)
	case "CHOOSER":
		w, err := parseChooserLines(lines)
		if err != nil {
			return err
		}
		m.Widgets = append(m.Widgets, w)
	case "INPUTBOX":
		w, err := parseInputLines(lines)
		if err != nil {
			return err
		}
		m.Widgets = append(m.Widgets, w)
	case "TEXTBOX":
		m.Widgets = append(m.Widgets, Widget{
			Kind: "note", Display: optUnescape(lineAt(lines, 5)),
			X: atoi(lineAt(lines, 1)), Y: atoi(lineAt(lines, 2)),
		})
	case "PLOT":
		name := optString(lineAt(lines, 5))
		if name == "" {
			name = "plot"
		}
		m.Notes = append(m.Notes, "plot not implemented: "+name)
		m.Widgets = append(m.Widgets, Widget{Kind: "plot", Display: name})
	case "OUTPUT":
		m.Notes = append(m.Notes, "output widget is not separate from the command center")
	default:
		m.Notes = append(m.Notes, "skipped widget "+kind)
	}
	return nil
}

func parseViewLines(lines []string) (ViewSpec, error) {
	// Layout follows NetLogo's ViewReader: patch size, wrapping, then pcor bounds.
	ps := atof(lineAt(lines, 7))
	if ps <= 0 {
		ps = 13
	}
	v := ViewSpec{
		WrapX:     lineAt(lines, 14) == "1",
		WrapY:     lineAt(lines, 15) == "1",
		MinPxcor:  atoi(lineAt(lines, 17)),
		MaxPxcor:  atoi(lineAt(lines, 18)),
		MinPycor:  atoi(lineAt(lines, 19)),
		MaxPycor:  atoi(lineAt(lines, 20)),
		PatchSize: ps,
		Set:       true,
	}
	if v.MaxPxcor < v.MinPxcor || v.MaxPycor < v.MinPycor {
		return ViewSpec{}, fmt.Errorf("view has an empty world")
	}
	return v, nil
}

func parseButtonLines(lines []string) (Widget, error) {
	display := optString(lineAt(lines, 5))
	source := unescapeLogo(optString(lineAt(lines, 6)))
	if display == "" {
		display = firstLine(source)
	}
	agent := strings.ToLower(lineAt(lines, 10))
	if agent == "" {
		agent = "observer"
	}
	return Widget{
		Kind:              "button",
		X:                 atoi(lineAt(lines, 1)),
		Y:                 atoi(lineAt(lines, 2)),
		W:                 atoi(lineAt(lines, 3)) - atoi(lineAt(lines, 1)),
		H:                 atoi(lineAt(lines, 4)) - atoi(lineAt(lines, 2)),
		Display:           display,
		Source:            source,
		Forever:           lineAt(lines, 7) == "T",
		Agent:             agent,
		DisableUntilTicks: lineAt(lines, 15) == "0",
	}, nil
}

func parseSliderLines(lines []string) (Widget, error) {
	variable := optString(lineAt(lines, 6))
	display := optString(lineAt(lines, 5))
	if display == "" {
		display = variable
	}
	return Widget{
		Kind:     "slider",
		X:        atoi(lineAt(lines, 1)),
		Y:        atoi(lineAt(lines, 2)),
		Display:  display,
		Variable: strings.ToLower(variable),
		Min:      atof(lineAt(lines, 7)),
		Max:      atof(lineAt(lines, 8)),
		Value:    atof(lineAt(lines, 9)),
		Step:     atof(lineAt(lines, 10)),
		Units:    optString(lineAt(lines, 12)),
	}, nil
}

func parseSwitchLines(lines []string) (Widget, error) {
	// The on/off line is inverted in the file: "0" means on.
	variable := optString(lineAt(lines, 6))
	display := optString(lineAt(lines, 5))
	if display == "" {
		display = variable
	}
	return Widget{
		Kind:     "switch",
		X:        atoi(lineAt(lines, 1)),
		Y:        atoi(lineAt(lines, 2)),
		Display:  display,
		Variable: strings.ToLower(variable),
		On:       lineAt(lines, 7) == "0",
	}, nil
}

func parseMonitorLines(lines []string) (Widget, error) {
	display := optString(lineAt(lines, 5))
	source := unescapeLogo(optString(lineAt(lines, 6)))
	if display == "" {
		display = source
	}
	prec := atoi(lineAt(lines, 7))
	if lineAt(lines, 7) == "" {
		prec = 17
	}
	return Widget{
		Kind:      "monitor",
		X:         atoi(lineAt(lines, 1)),
		Y:         atoi(lineAt(lines, 2)),
		Display:   display,
		Source:    source,
		Precision: prec,
	}, nil
}

func parseChooserLines(lines []string) (Widget, error) {
	variable := optString(lineAt(lines, 6))
	display := unescapeLogo(optString(lineAt(lines, 5)))
	if display == "" {
		display = variable
	}
	choices := parseChoiceList(lineAt(lines, 7))
	cur := atoi(lineAt(lines, 8))
	if cur < 0 || cur >= len(choices) {
		cur = 0
	}
	return Widget{
		Kind:     "chooser",
		Display:  display,
		Variable: strings.ToLower(variable),
		Choices:  choices,
		Current:  cur,
	}, nil
}

func parseInputLines(lines []string) (Widget, error) {
	variable := optString(lineAt(lines, 5))
	raw := lineAt(lines, 6)
	text := ""
	if raw != "NIL" {
		text = unescapeLogo(raw)
	}
	typ := lineAt(lines, 9)
	w := Widget{
		Kind:      "input",
		Display:   variable,
		Variable:  strings.ToLower(variable),
		Text:      text,
		InputType: typ,
	}
	if typ == "Number" || typ == "Color" {
		w.Value = atof(text)
	}
	return w, nil
}

func parseChoiceList(s string) []Value {
	var out []Value
	i := 0
	for i < len(s) {
		for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
			i++
		}
		if i >= len(s) {
			break
		}
		if s[i] == '"' {
			j := i + 1
			var b strings.Builder
			for j < len(s) {
				if s[j] == '\\' && j+1 < len(s) {
					b.WriteByte(s[j+1])
					j += 2
					continue
				}
				if s[j] == '"' {
					j++
					break
				}
				b.WriteByte(s[j])
				j++
			}
			out = append(out, Str(b.String()))
			i = j
			continue
		}
		j := i
		for j < len(s) && s[j] != ' ' && s[j] != '\t' {
			j++
		}
		tok := s[i:j]
		i = j
		switch strings.ToLower(tok) {
		case "true":
			out = append(out, Bool(true))
		case "false":
			out = append(out, Bool(false))
		case "nobody":
			out = append(out, Nobody())
		default:
			if n, err := strconv.ParseFloat(tok, 64); err == nil {
				out = append(out, Num(n))
			} else if c, ok := colorNames[strings.ToLower(tok)]; ok {
				out = append(out, Num(c))
			} else {
				out = append(out, Str(tok))
			}
		}
	}
	return out
}

// --- .nlogox (NetLogo 7 XML) ---

func parseNLogoX(name, content string) (*FileModel, error) {
	dec := xml.NewDecoder(strings.NewReader(content))
	m := &FileModel{Name: name, View: DefaultView()}
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch se.Name.Local {
		case "code":
			var text string
			if err := dec.DecodeElement(&text, &se); err != nil {
				return nil, err
			}
			m.Code = text
		case "view", "view3d":
			m.View = viewFromXML(se)
			if err := dec.Skip(); err != nil {
				return nil, err
			}
		case "button":
			var text string
			if err := dec.DecodeElement(&text, &se); err != nil {
				return nil, err
			}
			m.Widgets = append(m.Widgets, buttonFromXML(se, text))
		case "slider":
			m.Widgets = append(m.Widgets, sliderFromXML(se))
			if err := dec.Skip(); err != nil {
				return nil, err
			}
		case "switch":
			m.Widgets = append(m.Widgets, switchFromXML(se))
			if err := dec.Skip(); err != nil {
				return nil, err
			}
		case "monitor":
			var text string
			if err := dec.DecodeElement(&text, &se); err != nil {
				return nil, err
			}
			m.Widgets = append(m.Widgets, monitorFromXML(se, text))
		case "chooser":
			w, err := chooserFromXML(dec, se)
			if err != nil {
				return nil, err
			}
			m.Widgets = append(m.Widgets, w)
		case "input":
			var text string
			if err := dec.DecodeElement(&text, &se); err != nil {
				return nil, err
			}
			m.Widgets = append(m.Widgets, inputFromXML(se, text))
		case "note":
			var text string
			if err := dec.DecodeElement(&text, &se); err != nil {
				return nil, err
			}
			m.Widgets = append(m.Widgets, Widget{Kind: "note", Display: text})
		case "plot":
			disp := attr(se, "display")
			if disp == "" {
				disp = "plot"
			}
			m.Notes = append(m.Notes, "plot not implemented: "+disp)
			m.Widgets = append(m.Widgets, Widget{Kind: "plot", Display: disp})
			if err := dec.Skip(); err != nil {
				return nil, err
			}
		}
	}
	if strings.TrimSpace(m.Code) == "" {
		return nil, fmt.Errorf("model has no code")
	}
	if !m.View.Set {
		m.View = DefaultView()
	}
	return m, nil
}

func viewFromXML(se xml.StartElement) ViewSpec {
	ps := atof(attr(se, "patchSize"))
	if ps <= 0 {
		ps = 13
	}
	return ViewSpec{
		MinPxcor:  atoi(attr(se, "minPxcor")),
		MaxPxcor:  atoi(attr(se, "maxPxcor")),
		MinPycor:  atoi(attr(se, "minPycor")),
		MaxPycor:  atoi(attr(se, "maxPycor")),
		WrapX:     attr(se, "wrappingAllowedX") == "true",
		WrapY:     attr(se, "wrappingAllowedY") == "true",
		PatchSize: ps,
		Set:       true,
	}
}

func buttonFromXML(se xml.StartElement, source string) Widget {
	display := attr(se, "display")
	if display == "" {
		display = firstLine(strings.TrimSpace(source))
	}
	kind := strings.ToLower(attr(se, "kind"))
	if kind == "" {
		kind = "observer"
	}
	return Widget{
		Kind:              "button",
		Display:           display,
		Source:            strings.TrimSpace(source),
		Forever:           attr(se, "forever") == "true",
		Agent:             kind,
		DisableUntilTicks: attr(se, "disableUntilTicks") == "true",
	}
}

func sliderFromXML(se xml.StartElement) Widget {
	variable := attr(se, "variable")
	display := attr(se, "display")
	if display == "" {
		display = variable
	}
	step := atof(attr(se, "step"))
	if step == 0 {
		step = 1
	}
	return Widget{
		Kind:     "slider",
		Display:  display,
		Variable: strings.ToLower(variable),
		Min:      atof(attr(se, "min")),
		Max:      atof(attr(se, "max")),
		Value:    atof(attr(se, "default")),
		Step:     step,
		Units:    attr(se, "units"),
	}
}

func switchFromXML(se xml.StartElement) Widget {
	variable := attr(se, "variable")
	display := attr(se, "display")
	if display == "" {
		display = variable
	}
	return Widget{
		Kind:     "switch",
		Display:  display,
		Variable: strings.ToLower(variable),
		On:       attr(se, "on") == "true",
	}
}

func monitorFromXML(se xml.StartElement, source string) Widget {
	display := attr(se, "display")
	source = strings.TrimSpace(source)
	if display == "" {
		display = source
	}
	prec := 17
	if a := attr(se, "precision"); a != "" {
		prec = atoi(a)
	}
	return Widget{Kind: "monitor", Display: display, Source: source, Precision: prec}
}

func chooserFromXML(dec *xml.Decoder, se xml.StartElement) (Widget, error) {
	variable := attr(se, "variable")
	display := attr(se, "display")
	if display == "" {
		display = variable
	}
	cur := atoi(attr(se, "current"))
	var choices []Value
	for {
		tok, err := dec.Token()
		if err != nil {
			return Widget{}, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "choice" {
				var text string
				if err := dec.DecodeElement(&text, &t); err != nil {
					return Widget{}, err
				}
				choices = append(choices, choiceValue(attr(t, "type"), attr(t, "value"), text))
			} else if err := dec.Skip(); err != nil {
				return Widget{}, err
			}
		case xml.EndElement:
			if t.Name.Local == "chooser" {
				if cur < 0 || cur >= len(choices) {
					cur = 0
				}
				return Widget{
					Kind: "chooser", Display: display, Variable: strings.ToLower(variable),
					Choices: choices, Current: cur,
				}, nil
			}
		}
	}
}

func choiceValue(typ, value, text string) Value {
	switch typ {
	case "double", "number":
		return Num(atof(value))
	case "boolean":
		return Bool(value == "true")
	case "list":
		return Str(strings.TrimSpace(text))
	default:
		if value == "" {
			return Str(text)
		}
		return Str(value)
	}
}

func inputFromXML(se xml.StartElement, text string) Widget {
	variable := attr(se, "variable")
	typ := attr(se, "type")
	w := Widget{Kind: "input", Display: variable, Variable: strings.ToLower(variable), Text: text, InputType: typ}
	if typ == "number" || typ == "color" {
		w.Value = atof(text)
	}
	return w
}

func attr(se xml.StartElement, name string) string {
	for _, a := range se.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

// --- compile and apply ---

// Compile parses the code and marks widget variables as interface globals,
// so clear-all leaves sliders and switches alone.
func (m *FileModel) Compile() (*Program, error) {
	prog, err := Parse(m.Code)
	if err != nil {
		return nil, err
	}
	if prog.InterfaceVars == nil {
		prog.InterfaceVars = map[string]bool{}
	}
	for _, w := range m.Widgets {
		if w.Variable != "" {
			prog.InterfaceVars[w.Variable] = true
		}
	}
	return prog, nil
}

// NewVM builds a world from the view and installs widget values.
// seed 0 keeps the clock-seeded RNG from NewWorld.
func (m *FileModel) NewVM(seed int64) (*VM, error) {
	prog, err := m.Compile()
	if err != nil {
		return nil, err
	}
	v := m.View
	if !v.Set {
		v = DefaultView()
	}
	if v.MaxPxcor < v.MinPxcor || v.MaxPycor < v.MinPycor {
		return nil, fmt.Errorf("the world is empty")
	}
	w := NewWorld(v.MinPxcor, v.MaxPxcor, v.MinPycor, v.MaxPycor, v.WrapX, v.WrapY)
	if v.PatchSize > 0 {
		w.PatchSize = v.PatchSize
	}
	if seed != 0 {
		w.Seed(seed)
	}
	vm := NewVM(prog, w)
	m.Apply(vm)
	return vm, nil
}

// Apply copies slider, switch, chooser, and input values into globals.
func (m *FileModel) Apply(vm *VM) {
	for i := range m.Widgets {
		w := &m.Widgets[i]
		if w.Variable == "" {
			continue
		}
		switch w.Kind {
		case "slider":
			vm.World.SetGlobal(w.Variable, Num(w.Value))
		case "switch":
			vm.World.SetGlobal(w.Variable, Bool(w.On))
		case "chooser":
			if w.Current >= 0 && w.Current < len(w.Choices) {
				vm.World.SetGlobal(w.Variable, w.Choices[w.Current])
			}
		case "input":
			if strings.EqualFold(w.InputType, "number") || strings.EqualFold(w.InputType, "color") || w.InputType == "Number" || w.InputType == "Color" {
				vm.World.SetGlobal(w.Variable, Num(w.Value))
			} else if w.Text != "" {
				vm.World.SetGlobal(w.Variable, Str(w.Text))
			}
		}
	}
}

// ButtonByName finds a button whose display or source equals name.
func (m *FileModel) ButtonByName(name string) *Widget {
	name = strings.ToLower(name)
	for i := range m.Widgets {
		w := &m.Widgets[i]
		if w.Kind != "button" {
			continue
		}
		if strings.ToLower(w.Display) == name || strings.ToLower(strings.TrimSpace(w.Source)) == name {
			return w
		}
	}
	return nil
}

func lineAt(lines []string, i int) string {
	if i < 0 || i >= len(lines) {
		return ""
	}
	return strings.TrimSpace(lines[i])
}

func optString(s string) string {
	if s == "NIL" {
		return ""
	}
	return s
}

func optUnescape(s string) string {
	s = optString(s)
	if s == "" {
		return ""
	}
	return unescapeLogo(s)
}

func unescapeLogo(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		switch s[i+1] {
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'r':
			b.WriteByte('\r')
		case '\\', '"':
			b.WriteByte(s[i+1])
		default:
			b.WriteByte(s[i+1])
		}
		i++
	}
	return b.String()
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

func atoi(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		f, err2 := strconv.ParseFloat(s, 64)
		if err2 != nil {
			return 0
		}
		return int(f)
	}
	return n
}

func atof(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return n
}
