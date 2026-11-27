package nl

import (
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// FormatWidgetYAML is the Widgets tab document for one widget.
// kind is shown and cannot be changed. index, the live disabled flag,
// and a monitor reading are left out.
func FormatWidgetYAML(w Widget) (string, error) {
	doc, err := widgetDocument(w)
	if err != nil {
		return "", err
	}
	b, err := yaml.Marshal(doc)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ApplyWidgetYAML checks a Widgets tab document and returns the widget
// with those values. The caller's widget is not modified. A bad document
// returns the original widget and an error that names the problem.
func ApplyWidgetYAML(w Widget, text string) (Widget, error) {
	if strings.TrimSpace(text) == "" {
		return w, fmt.Errorf("the widget document is empty")
	}
	dec := yaml.NewDecoder(strings.NewReader(text))
	dec.KnownFields(true)
	doc, err := newWidgetDocument(w.Kind)
	if err != nil {
		return w, err
	}
	if err := dec.Decode(doc); err != nil {
		return w, friendlyYAMLError(err)
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); err != io.EOF {
		return w, fmt.Errorf("the widget document must be a single map")
	}
	next, err := doc.apply(w)
	if err != nil {
		return w, err
	}
	return next, nil
}

// WriteWidget edits one widget inside a model file. Other sections stay
// as they are. index is the widget's position among parsed widgets.
func WriteWidget(source string, index int, w Widget) (string, error) {
	section := ExtractWidgetSection(source)
	var next string
	var err error
	if section.XML {
		next, err = patchXMLWidget(section.Text, index, w)
	} else {
		next, err = patchClassicWidget(section.Text, index, w)
	}
	if err != nil {
		return "", err
	}
	return ReplaceWidgetSection(source, next)
}

type widgetDoc interface {
	apply(Widget) (Widget, error)
}

func newWidgetDocument(kind string) (widgetDoc, error) {
	switch kind {
	case "button":
		return &buttonDoc{}, nil
	case "slider":
		return &sliderDoc{}, nil
	case "switch":
		return &switchDoc{}, nil
	case "monitor":
		return &monitorDoc{}, nil
	case "chooser":
		return &chooserDoc{}, nil
	case "input":
		return &inputDoc{}, nil
	case "note":
		return &noteDoc{}, nil
	case "plot":
		return &plotDoc{}, nil
	case "output":
		return &outputDoc{}, nil
	default:
		return nil, fmt.Errorf("this widget cannot be edited as YAML")
	}
}

func widgetDocument(w Widget) (any, error) {
	switch w.Kind {
	case "button":
		return buttonDoc{
			Kind: w.Kind, Display: w.Display, Source: w.Source, Forever: w.Forever,
			Agent: w.Agent, DisableUntilTicks: w.DisableUntilTicks,
			box: boxOf(w),
		}, nil
	case "slider":
		return sliderDoc{
			Kind: w.Kind, Display: w.Display, Variable: w.Variable,
			Min: w.Min, Max: w.Max, Value: w.Value, Step: w.Step, Units: w.Units,
			box: boxOf(w),
		}, nil
	case "switch":
		return switchDoc{
			Kind: w.Kind, Display: w.Display, Variable: w.Variable, On: w.On,
			box: boxOf(w),
		}, nil
	case "monitor":
		return monitorDoc{
			Kind: w.Kind, Display: w.Display, Source: w.Source, Precision: w.Precision,
			box: boxOf(w),
		}, nil
	case "chooser":
		choices := make([]string, len(w.Choices))
		for i, c := range w.Choices {
			choices[i] = c.Format(false)
		}
		return chooserDoc{
			Kind: w.Kind, Display: w.Display, Variable: w.Variable,
			Choices: choices, Current: w.Current,
			box: boxOf(w),
		}, nil
	case "input":
		return inputDoc{
			Kind: w.Kind, Display: w.Display, Variable: w.Variable, Text: w.Text, InputType: w.InputType,
			box: boxOf(w),
		}, nil
	case "note":
		return noteDoc{
			Kind: w.Kind, Display: w.Display,
			box: boxOf(w),
		}, nil
	case "plot":
		return plotDoc{
			Kind: w.Kind, Display: w.Display, XLabel: w.XLabel, YLabel: w.YLabel,
			XMin: w.XMin, XMax: w.XMax, YMin: w.YMin, YMax: w.YMax,
			AutoPlot: w.AutoPlot, Legend: w.Legend,
			box: boxOf(w),
		}, nil
	case "output":
		return outputDoc{
			Kind: w.Kind, Display: w.Display, FontSize: w.FontSize,
			box: boxOf(w),
		}, nil
	default:
		return nil, fmt.Errorf("this widget cannot be edited as YAML")
	}
}

type box struct {
	X           int    `yaml:"x"`
	Y           int    `yaml:"y"`
	Width       int    `yaml:"width"`
	Height      int    `yaml:"height"`
	Description string `yaml:"description"`
}

func boxOf(w Widget) box {
	return box{X: w.X, Y: w.Y, Width: w.W, Height: w.H, Description: w.Description}
}

func (b box) place(w Widget) (Widget, error) {
	if b.Width < 0 || b.Height < 0 {
		return w, fmt.Errorf("width and height must be zero or positive")
	}
	desc, err := cleanDescription(b.Description)
	if err != nil {
		return w, err
	}
	w.X, w.Y, w.W, w.H = b.X, b.Y, b.Width, b.Height
	w.Description = desc
	return w, nil
}

func cleanDescription(s string) (string, error) {
	if strings.ContainsAny(s, "\r\n") {
		return "", fmt.Errorf("description must be a single line")
	}
	s = strings.TrimSpace(s)
	if len(s) > 200 {
		return "", fmt.Errorf("description must be 200 characters or shorter")
	}
	return s, nil
}

func sameKind(got, want string) error {
	if strings.TrimSpace(got) == "" {
		return fmt.Errorf("kind is required")
	}
	if got != want {
		return fmt.Errorf("kind cannot be changed (this widget is a %s)", want)
	}
	return nil
}

type buttonDoc struct {
	Kind              string `yaml:"kind"`
	Display           string `yaml:"display"`
	Source            string `yaml:"source"`
	Forever           bool   `yaml:"forever"`
	Agent             string `yaml:"agent"`
	DisableUntilTicks bool   `yaml:"disableUntilTicks"`
	box               `yaml:",inline"`
}

func (d *buttonDoc) apply(w Widget) (Widget, error) {
	if err := sameKind(d.Kind, w.Kind); err != nil {
		return w, err
	}
	if strings.TrimSpace(d.Display) == "" {
		return w, fmt.Errorf("display is required")
	}
	agent := strings.ToLower(strings.TrimSpace(d.Agent))
	if agent == "" {
		agent = "observer"
	}
	switch agent {
	case "observer", "turtle", "turtles", "patch", "patches", "link", "links":
	default:
		return w, fmt.Errorf("agent must be observer, turtle, patch, or link")
	}
	next, err := d.place(w)
	if err != nil {
		return w, err
	}
	next.Display = d.Display
	next.Source = d.Source
	next.Forever = d.Forever
	next.Agent = agent
	next.DisableUntilTicks = d.DisableUntilTicks
	return next, nil
}

type sliderDoc struct {
	Kind     string  `yaml:"kind"`
	Display  string  `yaml:"display"`
	Variable string  `yaml:"variable"`
	Min      float64 `yaml:"min"`
	Max      float64 `yaml:"max"`
	Value    float64 `yaml:"value"`
	Step     float64 `yaml:"step"`
	Units    string  `yaml:"units"`
	box      `yaml:",inline"`
}

func (d *sliderDoc) apply(w Widget) (Widget, error) {
	if err := sameKind(d.Kind, w.Kind); err != nil {
		return w, err
	}
	variable := strings.ToLower(strings.TrimSpace(d.Variable))
	if variable == "" {
		return w, fmt.Errorf("variable is required")
	}
	if d.Min > d.Max {
		return w, fmt.Errorf("min must not be greater than max")
	}
	if d.Step <= 0 {
		return w, fmt.Errorf("step must be greater than 0")
	}
	next, err := d.place(w)
	if err != nil {
		return w, err
	}
	next.Display = d.Display
	if strings.TrimSpace(next.Display) == "" {
		next.Display = variable
	}
	next.Variable = variable
	next.Min, next.Max, next.Value, next.Step = d.Min, d.Max, d.Value, d.Step
	next.Units = d.Units
	return next, nil
}

type switchDoc struct {
	Kind     string `yaml:"kind"`
	Display  string `yaml:"display"`
	Variable string `yaml:"variable"`
	On       bool   `yaml:"on"`
	box      `yaml:",inline"`
}

func (d *switchDoc) apply(w Widget) (Widget, error) {
	if err := sameKind(d.Kind, w.Kind); err != nil {
		return w, err
	}
	variable := strings.ToLower(strings.TrimSpace(d.Variable))
	if variable == "" {
		return w, fmt.Errorf("variable is required")
	}
	next, err := d.place(w)
	if err != nil {
		return w, err
	}
	next.Display = d.Display
	if strings.TrimSpace(next.Display) == "" {
		next.Display = variable
	}
	next.Variable = variable
	next.On = d.On
	return next, nil
}

type monitorDoc struct {
	Kind      string `yaml:"kind"`
	Display   string `yaml:"display"`
	Source    string `yaml:"source"`
	Precision int    `yaml:"precision"`
	box       `yaml:",inline"`
}

func (d *monitorDoc) apply(w Widget) (Widget, error) {
	if err := sameKind(d.Kind, w.Kind); err != nil {
		return w, err
	}
	if strings.TrimSpace(d.Source) == "" {
		return w, fmt.Errorf("source is required")
	}
	next, err := d.place(w)
	if err != nil {
		return w, err
	}
	next.Display = d.Display
	if strings.TrimSpace(next.Display) == "" {
		next.Display = d.Source
	}
	next.Source = d.Source
	next.Precision = d.Precision
	return next, nil
}

type chooserDoc struct {
	Kind     string   `yaml:"kind"`
	Display  string   `yaml:"display"`
	Variable string   `yaml:"variable"`
	Choices  []string `yaml:"choices"`
	Current  int      `yaml:"current"`
	box      `yaml:",inline"`
}

func (d *chooserDoc) apply(w Widget) (Widget, error) {
	if err := sameKind(d.Kind, w.Kind); err != nil {
		return w, err
	}
	variable := strings.ToLower(strings.TrimSpace(d.Variable))
	if variable == "" {
		return w, fmt.Errorf("variable is required")
	}
	if len(d.Choices) == 0 {
		return w, fmt.Errorf("choices must list at least one value")
	}
	if d.Current < 0 || d.Current >= len(d.Choices) {
		return w, fmt.Errorf("current must be an index in choices")
	}
	next, err := d.place(w)
	if err != nil {
		return w, err
	}
	next.Display = d.Display
	if strings.TrimSpace(next.Display) == "" {
		next.Display = variable
	}
	next.Variable = variable
	next.Choices = make([]Value, len(d.Choices))
	for i, c := range d.Choices {
		next.Choices[i] = parseChoiceItem(c)
	}
	next.Current = d.Current
	return next, nil
}

type inputDoc struct {
	Kind      string `yaml:"kind"`
	Display   string `yaml:"display"`
	Variable  string `yaml:"variable"`
	Text      string `yaml:"text"`
	InputType string `yaml:"inputType"`
	box       `yaml:",inline"`
}

func (d *inputDoc) apply(w Widget) (Widget, error) {
	if err := sameKind(d.Kind, w.Kind); err != nil {
		return w, err
	}
	variable := strings.ToLower(strings.TrimSpace(d.Variable))
	if variable == "" {
		return w, fmt.Errorf("variable is required")
	}
	next, err := d.place(w)
	if err != nil {
		return w, err
	}
	next.Display = d.Display
	if strings.TrimSpace(next.Display) == "" {
		next.Display = variable
	}
	next.Variable = variable
	next.Text = d.Text
	next.InputType = strings.ToLower(strings.TrimSpace(d.InputType))
	if next.InputType == "number" || next.InputType == "color" {
		next.Value = atof(d.Text)
	}
	return next, nil
}

type noteDoc struct {
	Kind    string `yaml:"kind"`
	Display string `yaml:"display"`
	box     `yaml:",inline"`
}

func (d *noteDoc) apply(w Widget) (Widget, error) {
	if err := sameKind(d.Kind, w.Kind); err != nil {
		return w, err
	}
	next, err := d.place(w)
	if err != nil {
		return w, err
	}
	next.Display = d.Display
	return next, nil
}

type plotDoc struct {
	Kind     string  `yaml:"kind"`
	Display  string  `yaml:"display"`
	XLabel   string  `yaml:"xLabel"`
	YLabel   string  `yaml:"yLabel"`
	XMin     float64 `yaml:"xMin"`
	XMax     float64 `yaml:"xMax"`
	YMin     float64 `yaml:"yMin"`
	YMax     float64 `yaml:"yMax"`
	AutoPlot bool    `yaml:"autoPlot"`
	Legend   bool    `yaml:"legend"`
	box      `yaml:",inline"`
}

func (d *plotDoc) apply(w Widget) (Widget, error) {
	if err := sameKind(d.Kind, w.Kind); err != nil {
		return w, err
	}
	if strings.TrimSpace(d.Display) == "" {
		return w, fmt.Errorf("display is required")
	}
	next, err := d.place(w)
	if err != nil {
		return w, err
	}
	next.Display = d.Display
	next.XLabel, next.YLabel = d.XLabel, d.YLabel
	next.XMin, next.XMax, next.YMin, next.YMax = d.XMin, d.XMax, d.YMin, d.YMax
	next.AutoPlot, next.Legend = d.AutoPlot, d.Legend
	return next, nil
}

type outputDoc struct {
	Kind     string `yaml:"kind"`
	Display  string `yaml:"display"`
	FontSize int    `yaml:"fontSize"`
	box      `yaml:",inline"`
}

func (d *outputDoc) apply(w Widget) (Widget, error) {
	if err := sameKind(d.Kind, w.Kind); err != nil {
		return w, err
	}
	if d.FontSize < 0 {
		return w, fmt.Errorf("fontSize must be zero or positive")
	}
	next, err := d.place(w)
	if err != nil {
		return w, err
	}
	next.Display = d.Display
	if strings.TrimSpace(next.Display) == "" {
		next.Display = "Output"
	}
	next.FontSize = d.FontSize
	return next, nil
}

func friendlyYAMLError(err error) error {
	msg := err.Error()
	if strings.Contains(msg, "field ") && strings.Contains(msg, "not found") {
		// yaml.v3: "line 2: field color not found in type nl.sliderDoc"
		field := msg
		if i := strings.Index(msg, "field "); i >= 0 {
			field = msg[i+len("field "):]
			if j := strings.Index(field, " "); j >= 0 {
				field = field[:j]
			}
		}
		return fmt.Errorf("unknown key %q", field)
	}
	if strings.Contains(msg, "cannot unmarshal") {
		return fmt.Errorf("a value has the wrong type (%s)", strings.TrimSpace(msg))
	}
	return fmt.Errorf("could not read the widget YAML: %s", strings.TrimSpace(msg))
}

func patchClassicWidget(section string, index int, w Widget) (string, error) {
	lines := strings.Split(strings.ReplaceAll(section, "\r\n", "\n"), "\n")
	var blocks [][]string
	var cur []string
	flush := func() {
		if len(cur) == 0 {
			return
		}
		blocks = append(blocks, cur)
		cur = nil
	}
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		cur = append(cur, line)
	}
	flush()
	n := 0
	found := false
	for i, block := range blocks {
		kind := strings.ToUpper(strings.TrimSpace(block[0]))
		if !classicWidgetKind(kind) {
			continue
		}
		if n == index {
			blocks[i] = rewriteClassicBlock(block, w)
			found = true
			break
		}
		n++
	}
	if !found {
		return "", fmt.Errorf("widget %d is not in the interface", index)
	}
	parts := make([]string, len(blocks))
	for i, block := range blocks {
		parts[i] = strings.Join(block, "\n")
	}
	return strings.Join(parts, "\n\n"), nil
}

func classicWidgetKind(kind string) bool {
	switch kind {
	case "BUTTON", "SLIDER", "SWITCH", "MONITOR", "CHOOSER", "INPUTBOX", "TEXTBOX", "PLOT", "OUTPUT":
		return true
	default:
		return false
	}
}

func rewriteClassicBlock(lines []string, w Widget) []string {
	out := append([]string(nil), lines...)
	set := func(i int, v string) {
		for len(out) <= i {
			out = append(out, "")
		}
		out[i] = v
	}
	set(1, strconv.Itoa(w.X))
	set(2, strconv.Itoa(w.Y))
	set(3, strconv.Itoa(w.X+w.W))
	set(4, strconv.Itoa(w.Y+w.H))
	switch w.Kind {
	case "button":
		set(5, nilOr(w.Display))
		set(6, escapeLogo(w.Source))
		if w.Forever {
			set(7, "T")
		} else if lineAt(out, 7) == "T" {
			set(7, "NIL")
		}
		set(10, strings.ToUpper(w.Agent))
		if w.DisableUntilTicks {
			set(15, "0")
		} else if lineAt(out, 15) == "0" {
			set(15, "1")
		}
	case "slider":
		set(5, nilOr(w.Display))
		set(6, w.Variable)
		set(7, formatWidgetNum(w.Min))
		set(8, formatWidgetNum(w.Max))
		set(9, formatWidgetNum(w.Value))
		set(10, formatWidgetNum(w.Step))
		set(12, nilOr(w.Units))
	case "switch":
		set(5, nilOr(w.Display))
		set(6, w.Variable)
		if w.On {
			set(7, "0")
		} else {
			set(7, "1")
		}
	case "monitor":
		set(5, nilOr(w.Display))
		set(6, escapeLogo(w.Source))
		set(7, strconv.Itoa(w.Precision))
	case "chooser":
		set(5, escapeLogo(w.Display))
		set(6, w.Variable)
		set(7, formatChoices(w.Choices))
		set(8, strconv.Itoa(w.Current))
	case "input":
		set(5, w.Variable)
		if w.Text == "" {
			set(6, "NIL")
		} else {
			set(6, escapeLogo(w.Text))
		}
	case "note":
		set(5, escapeLogo(w.Display))
	case "output":
		if w.FontSize > 0 {
			set(5, strconv.Itoa(w.FontSize))
		}
	case "plot":
		pens := len(out)
		for i, line := range out {
			if strings.EqualFold(strings.TrimSpace(line), "PENS") {
				pens = i
				break
			}
		}
		headerAt := 5
		put := func(i int, v string) {
			if i >= pens {
				return
			}
			set(i, v)
		}
		put(headerAt, nilOr(w.Display))
		put(headerAt+1, nilOr(w.XLabel))
		put(headerAt+2, nilOr(w.YLabel))
		put(headerAt+3, formatWidgetNum(w.XMin))
		put(headerAt+4, formatWidgetNum(w.XMax))
		put(headerAt+5, formatWidgetNum(w.YMin))
		put(headerAt+6, formatWidgetNum(w.YMax))
		auto := "false"
		if w.AutoPlot {
			auto = "true"
		}
		put(headerAt+7, auto)
		legend := "false"
		if w.Legend {
			legend = "true"
		}
		put(headerAt+8, legend)
	}
	kept := out[:0]
	for _, line := range out {
		if strings.HasPrefix(strings.TrimSpace(line), widgetDescriptionPrefix) {
			continue
		}
		kept = append(kept, line)
	}
	if w.Description != "" {
		kept = append(kept, widgetDescriptionPrefix+" "+w.Description)
	}
	return kept
}

func nilOr(s string) string {
	if strings.TrimSpace(s) == "" {
		return "NIL"
	}
	return s
}

func formatWidgetNum(n float64) string {
	return strconv.FormatFloat(n, 'f', -1, 64)
}

func formatChoices(choices []Value) string {
	parts := make([]string, len(choices))
	for i, c := range choices {
		parts[i] = c.Format(false)
	}
	return strings.Join(parts, " ")
}

func escapeLogo(s string) string {
	if s == "" {
		return "NIL"
	}
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '"':
			b.WriteString(`\"`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

var xmlWidgetTag = regexp.MustCompile(`(?i)<(button|slider|switch|monitor|chooser|input|note|plot|output)\b`)

func patchXMLWidget(section string, index int, w Widget) (string, error) {
	matches := xmlWidgetTag.FindAllStringIndex(section, -1)
	if index < 0 || index >= len(matches) {
		return "", fmt.Errorf("widget %d is not in the interface", index)
	}
	start := matches[index][0]
	tagEnd := xmlTagEnd(section, start)
	if tagEnd < 0 {
		return "", fmt.Errorf("widget %d has a broken tag", index)
	}
	tag := upsertXMLAttrs(section[start:tagEnd], w)
	body := section[tagEnd:]
	if w.Kind == "button" || w.Kind == "monitor" {
		closeAt := strings.Index(strings.ToLower(body), "</"+w.Kind+">")
		if closeAt >= 0 {
			inner := escapeXMLText(codeText(w))
			body = ">" + inner + body[closeAt:]
			if strings.HasSuffix(tag, "/>") {
				tag = strings.TrimSuffix(tag, "/>") + ">"
			} else if strings.HasSuffix(tag, ">") {
				tag = strings.TrimSuffix(tag, ">")
			}
		}
	}
	return section[:start] + tag + body, nil
}

func codeText(w Widget) string {
	if w.Kind == "button" || w.Kind == "monitor" {
		return w.Source
	}
	return ""
}

func xmlTagEnd(s string, start int) int {
	inQuote := byte(0)
	for i := start; i < len(s); i++ {
		c := s[i]
		if inQuote != 0 {
			if c == inQuote {
				inQuote = 0
			}
			continue
		}
		if c == '"' || c == '\'' {
			inQuote = c
			continue
		}
		if c == '>' {
			return i + 1
		}
	}
	return -1
}

func upsertXMLAttrs(tag string, w Widget) string {
	attrs := map[string]string{"description": w.Description}
	switch w.Kind {
	case "button":
		attrs["display"] = w.Display
		attrs["forever"] = strconv.FormatBool(w.Forever)
		attrs["kind"] = xmlAgent(w.Agent)
		attrs["disableUntilTicks"] = strconv.FormatBool(w.DisableUntilTicks)
	case "slider":
		attrs["display"] = w.Display
		attrs["variable"] = w.Variable
		attrs["min"] = formatWidgetNum(w.Min)
		attrs["max"] = formatWidgetNum(w.Max)
		attrs["default"] = formatWidgetNum(w.Value)
		attrs["step"] = formatWidgetNum(w.Step)
		attrs["units"] = w.Units
	case "switch":
		attrs["display"] = w.Display
		attrs["variable"] = w.Variable
		attrs["on"] = strconv.FormatBool(w.On)
	case "monitor":
		attrs["display"] = w.Display
		attrs["precision"] = strconv.Itoa(w.Precision)
	case "chooser":
		attrs["display"] = w.Display
		attrs["variable"] = w.Variable
		attrs["current"] = strconv.Itoa(w.Current)
	case "input":
		attrs["variable"] = w.Variable
		if w.InputType != "" {
			attrs["type"] = w.InputType
		}
	case "output":
		if w.FontSize > 0 {
			attrs["fontSize"] = strconv.Itoa(w.FontSize)
		}
	case "plot":
		attrs["display"] = w.Display
		attrs["xLabel"] = w.XLabel
		attrs["yLabel"] = w.YLabel
	}
	for name, value := range attrs {
		tag = upsertAttr(tag, name, value)
	}
	return tag
}

func upsertAttr(tag, name, value string) string {
	re := regexp.MustCompile(`(?i)\s` + regexp.QuoteMeta(name) + `\s*=\s*("[^"]*"|'[^']*')`)
	if value == "" && name == "description" {
		return re.ReplaceAllString(tag, "")
	}
	encoded := " " + name + `="` + escapeXMLAttr(value) + `"`
	if re.MatchString(tag) {
		return re.ReplaceAllString(tag, encoded)
	}
	if value == "" && name != "display" && name != "variable" {
		return tag
	}
	if strings.HasSuffix(tag, "/>") {
		return strings.TrimSuffix(tag, "/>") + encoded + "/>"
	}
	if strings.HasSuffix(tag, ">") {
		return strings.TrimSuffix(tag, ">") + encoded + ">"
	}
	return tag
}

func xmlAgent(agent string) string {
	agent = strings.TrimSpace(agent)
	if agent == "" {
		return "Observer"
	}
	return strings.ToUpper(agent[:1]) + agent[1:]
}

func escapeXMLAttr(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	return s
}

func escapeXMLText(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	return s
}

func parseChoiceItem(s string) Value {
	if n, err := strconv.ParseFloat(s, 64); err == nil && !strings.ContainsAny(s, "eE") {
		return Num(n)
	}
	return Str(s)
}
