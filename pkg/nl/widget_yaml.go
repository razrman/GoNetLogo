package nl

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// FormatWidgetYAML is one widget's editable properties.
// kind is shown and cannot be changed. index, the live disabled flag,
// and a monitor reading are left out.
func FormatWidgetYAML(w Widget) (string, error) {
	doc, err := widgetDocument(w)
	if err != nil {
		return "", err
	}
	return marshalPlain(doc)
}

// FormatWidgetsYAML is the Widgets tab document. The view is the first
// entry. The model's widgets follow, in model order.
func FormatWidgetsYAML(view ViewSpec, widgets []Widget) (string, error) {
	docs := make([]any, 0, len(widgets)+1)
	docs = append(docs, formatView(view))
	for _, w := range widgets {
		doc, err := widgetDocument(w)
		if err != nil {
			return "", err
		}
		docs = append(docs, doc)
	}
	return marshalPlain(docs)
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

// ApplyWidgetsYAML reads the Widgets tab list. The list has exactly one
// kind: view entry; it cannot be removed, duplicated, or given another
// kind. The other entries are matched to existing widgets by kind and
// variable (or display, when the widget has no variable). A match is
// edited and keeps fields the document does not show, such as plot pens.
// Unmatched entries are added. Existing widgets with no entry are
// removed. The result order is the document order, with the view entry
// skipped. Changing kind on a widget that still has the same variable or
// display is rejected. Renaming variable or display and reordering that
// same widget in one save is treated as removing it and adding a new one.
func ApplyWidgetsYAML(view ViewSpec, existing []Widget, text string) (ViewSpec, []Widget, []int, error) {
	if strings.TrimSpace(text) == "" {
		return view, nil, nil, fmt.Errorf("the widget document is empty")
	}
	dec := yaml.NewDecoder(strings.NewReader(text))
	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil {
		return view, nil, nil, friendlyYAMLError(err)
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); err != io.EOF {
		return view, nil, nil, fmt.Errorf("the widget document must be a single list")
	}
	root := &doc
	if root.Kind == yaml.DocumentNode {
		if len(root.Content) == 0 {
			return view, nil, nil, fmt.Errorf("the view cannot be removed")
		}
		root = root.Content[0]
	}
	if root.Kind != yaml.SequenceNode {
		return view, nil, nil, fmt.Errorf("the widget document must be a list of widgets")
	}
	type item struct {
		index                   int
		kind, display, variable string
		node                    *yaml.Node
	}
	var items []item
	var viewNode *yaml.Node
	viewAt := -1
	seen := map[string]int{}
	for i, node := range root.Content {
		if node.Kind != yaml.MappingNode {
			return view, nil, nil, fmt.Errorf("widget %d: each entry must be a map", i)
		}
		kind, display, variable, err := peekWidget(node)
		if err != nil {
			return view, nil, nil, fmt.Errorf("widget %d: %w", i, err)
		}
		if kind == "view" {
			if viewAt >= 0 {
				return view, nil, nil, fmt.Errorf("widget %d (view): duplicates the view", i)
			}
			viewAt = i
			viewNode = node
			continue
		}
		id := stableID(kind, variable, display)
		if id != "" {
			key := kind + "\x00" + id
			if prev, ok := seen[key]; ok {
				return view, nil, nil, fmt.Errorf("widget %d (%s): duplicates widget %d", i, widgetLabel(display, variable, kind), prev)
			}
			seen[key] = i
		}
		for _, old := range existing {
			if id == "" || stableID(old.Kind, old.Variable, old.Display) != id || idClass(old.Kind) != idClass(kind) {
				continue
			}
			if old.Kind != kind {
				return view, nil, nil, fmt.Errorf("widget %d (%s): kind cannot be changed from %s to %s", i, widgetLabel(display, variable, kind), old.Kind, kind)
			}
		}
		items = append(items, item{index: i, kind: kind, display: display, variable: variable, node: node})
	}
	if viewAt < 0 {
		return view, nil, nil, fmt.Errorf("the view cannot be removed, and its kind cannot be changed")
	}
	nextView, err := decodeView(view, viewNode)
	if err != nil {
		return view, nil, nil, fmt.Errorf("widget %d (view): %w", viewAt, err)
	}
	used := make([]bool, len(existing))
	from := make([]int, len(items))
	assigned := make([]bool, len(items))
	for i, item := range items {
		id := stableID(item.kind, item.variable, item.display)
		if id == "" {
			continue
		}
		match := -1
		for j, old := range existing {
			if used[j] || old.Kind != item.kind {
				continue
			}
			if stableID(old.Kind, old.Variable, old.Display) == id {
				if match >= 0 {
					return view, nil, nil, fmt.Errorf("widget %d (%s): matches more than one existing widget", item.index, widgetLabel(item.display, item.variable, item.kind))
				}
				match = j
			}
		}
		if match >= 0 {
			used[match] = true
			from[i] = match
			assigned[i] = true
		}
	}
	for i, item := range items {
		if assigned[i] {
			continue
		}
		if i < len(existing) && !used[i] && existing[i].Kind == item.kind {
			used[i] = true
			from[i] = i
			assigned[i] = true
			continue
		}
		from[i] = -1
	}
	out := make([]Widget, len(items))
	for i, item := range items {
		base := Widget{Kind: item.kind}
		if assigned[i] {
			base = existing[from[i]]
		}
		next, err := decodeWidget(base, item.node)
		if err != nil {
			return view, nil, nil, fmt.Errorf("widget %d (%s): %w", item.index, widgetLabel(item.display, item.variable, item.kind), err)
		}
		out[i] = next
	}
	return nextView, out, from, nil
}

func widgetLabel(display, variable, kind string) string {
	name := strings.TrimSpace(display)
	if name == "" {
		name = strings.TrimSpace(variable)
	}
	if name == "" {
		return kind
	}
	return name + ", " + kind
}

func idClass(kind string) string {
	switch kind {
	case "slider", "switch", "chooser", "input":
		return "var"
	default:
		return "label"
	}
}

func stableID(kind, variable, display string) string {
	switch kind {
	case "slider", "switch", "chooser", "input":
		return strings.ToLower(strings.TrimSpace(variable))
	default:
		return strings.TrimSpace(display)
	}
}

func peekWidget(node *yaml.Node) (kind, display, variable string, err error) {
	kind, ok := mappingString(node, "kind")
	if !ok || strings.TrimSpace(kind) == "" {
		return "", "", "", fmt.Errorf("kind is required")
	}
	if kind != "view" {
		if _, err := newWidgetDocument(kind); err != nil {
			return "", "", "", fmt.Errorf("unknown kind %q", kind)
		}
	}
	display, _ = mappingString(node, "display")
	variable, _ = mappingString(node, "variable")
	return kind, display, variable, nil
}

func mappingString(node *yaml.Node, key string) (string, bool) {
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			val := node.Content[i+1]
			if val.Kind != yaml.ScalarNode {
				return "", true
			}
			return val.Value, true
		}
	}
	return "", false
}

func decodeWidget(base Widget, node *yaml.Node) (Widget, error) {
	allowed := widgetKeys(base.Kind)
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i].Value
		if !allowed[key] {
			return base, fmt.Errorf("unknown key %q", key)
		}
		one := yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{node.Content[i], node.Content[i+1]}}
		doc, err := newWidgetDocument(base.Kind)
		if err != nil {
			return base, err
		}
		if err := one.Decode(doc); err != nil {
			return base, fmt.Errorf("%s: %s", key, yamlTypeHint(err))
		}
	}
	doc, err := newWidgetDocument(base.Kind)
	if err != nil {
		return base, err
	}
	if err := node.Decode(doc); err != nil {
		return base, friendlyYAMLError(err)
	}
	return doc.apply(base)
}

func widgetKeys(kind string) map[string]bool {
	common := []string{"kind", "display", "x", "y", "width", "height", "description"}
	var extra []string
	switch kind {
	case "button":
		extra = []string{"source", "forever", "agent", "disableUntilTicks"}
	case "slider":
		extra = []string{"variable", "min", "max", "value", "step", "units"}
	case "switch":
		extra = []string{"variable", "on"}
	case "monitor":
		extra = []string{"source", "precision"}
	case "chooser":
		extra = []string{"variable", "choices", "current"}
	case "input":
		extra = []string{"variable", "text", "inputType"}
	case "plot":
		extra = []string{"xLabel", "yLabel", "xMin", "xMax", "yMin", "yMax", "autoPlot", "legend"}
	case "output":
		extra = []string{"fontSize"}
	}
	out := map[string]bool{}
	for _, k := range append(common, extra...) {
		out[k] = true
	}
	return out
}

func yamlTypeHint(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "into float"):
		return "value must be a number"
	case strings.Contains(msg, "into int"):
		return "value must be an integer"
	case strings.Contains(msg, "into bool"):
		return "value must be true or false"
	case strings.Contains(msg, "into []string"), strings.Contains(msg, "into string"):
		return "value must be text"
	default:
		return "value has the wrong type"
	}
}

// marshalPlain encodes doc with plain keys. yaml.v3 quotes YAML 1.1
// boolean words such as y, n, on, and off. Those keys are still strings,
// so the Widgets tab writes them unquoted.
func marshalPlain(doc any) (string, error) {
	var node yaml.Node
	if err := node.Encode(doc); err != nil {
		return "", err
	}
	clearKeyStyle(&node)
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&node); err != nil {
		return "", err
	}
	if err := enc.Close(); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func clearKeyStyle(n *yaml.Node) {
	switch n.Kind {
	case yaml.DocumentNode, yaml.SequenceNode:
		for _, c := range n.Content {
			clearKeyStyle(c)
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			key := n.Content[i]
			key.Style = 0
			key.Tag = ""
			clearKeyStyle(n.Content[i+1])
		}
	}
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

// RewriteViewGeometry writes the live world size back into the view
// widget. Position, font, and the tick label are left as they are.
func RewriteViewGeometry(source string, view ViewSpec) (string, error) {
	section := ExtractWidgetSection(source)
	var next string
	var err error
	if section.XML {
		next, err = rewriteXMLView(section.Text, view)
	} else {
		next, err = rewriteClassicViewSection(section.Text, view)
	}
	if err != nil {
		return "", err
	}
	return ReplaceWidgetSection(source, next)
}

func rewriteClassicViewSection(section string, view ViewSpec) (string, error) {
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
	found := false
	for i, block := range blocks {
		if strings.EqualFold(strings.TrimSpace(block[0]), "GRAPHICS-WINDOW") {
			blocks[i] = rewriteGraphicsWindow(block, view)
			found = true
			break
		}
	}
	if !found {
		return "", fmt.Errorf("model has no graphics window")
	}
	parts := make([]string, len(blocks))
	for i, block := range blocks {
		parts[i] = strings.Join(block, "\n")
	}
	return strings.Join(parts, "\n\n"), nil
}

// WriteWidgets rewrites the view and every widget in source. from[i] is
// the previous index of widgets[i], or -1 when it is new. The graphics
// window keeps the lines this editor does not show.
func WriteWidgets(source string, view ViewSpec, widgets []Widget, from []int) (string, error) {
	if len(widgets) != len(from) {
		return "", fmt.Errorf("widget edit length mismatch")
	}
	section := ExtractWidgetSection(source)
	var next string
	var err error
	if section.XML {
		next, err = writeXMLWidgets(section.Text, view, widgets, from)
	} else {
		next, err = writeClassicWidgets(section.Text, view, widgets, from)
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

func formatView(v ViewSpec) viewDoc {
	doc := viewDoc{
		Kind: "view", PatchSize: v.PatchSize, FontSize: v.FontSize,
		MinPxcor: v.MinPxcor, MaxPxcor: v.MaxPxcor, MinPycor: v.MinPycor, MaxPycor: v.MaxPycor,
		WrapX: v.WrapX, WrapY: v.WrapY,
		box: box{X: v.X, Y: v.Y, Width: v.W, Height: v.H, Description: v.Description},
	}
	if v.Is3D {
		minZ, maxZ := v.MinPzcor, v.MaxPzcor
		wrapZ := v.WrapZ
		doc.MinPzcor, doc.MaxPzcor, doc.WrapZ = &minZ, &maxZ, &wrapZ
	}
	if v.UpdateModeStored {
		mode := UpdateModeName(v.UpdateMode)
		doc.UpdateMode = &mode
	}
	if v.TickLabel != "" {
		label := v.TickLabel
		doc.TicksLabel = &label
	}
	return doc
}

type viewDoc struct {
	Kind       string  `yaml:"kind"`
	PatchSize  float64 `yaml:"patchSize"`
	FontSize   int     `yaml:"fontSize"`
	MinPxcor   int     `yaml:"minPxcor"`
	MaxPxcor   int     `yaml:"maxPxcor"`
	MinPycor   int     `yaml:"minPycor"`
	MaxPycor   int     `yaml:"maxPycor"`
	MinPzcor   *int    `yaml:"minPzcor,omitempty"`
	MaxPzcor   *int    `yaml:"maxPzcor,omitempty"`
	WrapX      bool    `yaml:"wrapX"`
	WrapY      bool    `yaml:"wrapY"`
	WrapZ      *bool   `yaml:"wrapZ,omitempty"`
	UpdateMode *string `yaml:"updateMode,omitempty"`
	TicksLabel *string `yaml:"ticksLabel,omitempty"`
	box        `yaml:",inline"`
}

func (d *viewDoc) apply(v ViewSpec) (ViewSpec, error) {
	if strings.TrimSpace(d.Kind) != "view" {
		got := strings.TrimSpace(d.Kind)
		if got == "" {
			return v, fmt.Errorf("kind is required")
		}
		return v, fmt.Errorf("kind cannot be changed from view to %s", got)
	}
	if d.PatchSize <= 0 || d.PatchSize > 100 {
		return v, fmt.Errorf("patchSize must be positive and at most 100")
	}
	if d.FontSize < 0 {
		return v, fmt.Errorf("fontSize must be zero or positive")
	}
	if d.Width < 0 || d.Height < 0 {
		return v, fmt.Errorf("width and height must be zero or positive")
	}
	desc, err := cleanDescription(d.Description)
	if err != nil {
		return v, err
	}
	if v.Is3D {
		if (d.MinPzcor == nil) != (d.MaxPzcor == nil) {
			return v, fmt.Errorf("minPzcor and maxPzcor must both be set")
		}
	}
	next := v
	next.X, next.Y, next.W, next.H = d.X, d.Y, d.Width, d.Height
	next.Description = desc
	next.PatchSize = d.PatchSize
	next.FontSize = d.FontSize
	next.MinPxcor, next.MaxPxcor = d.MinPxcor, d.MaxPxcor
	next.MinPycor, next.MaxPycor = d.MinPycor, d.MaxPycor
	next.WrapX, next.WrapY = d.WrapX, d.WrapY
	if v.Is3D {
		if d.MinPzcor != nil {
			next.MinPzcor, next.MaxPzcor = *d.MinPzcor, *d.MaxPzcor
		}
		if d.WrapZ != nil {
			next.WrapZ = *d.WrapZ
		}
		if err := ValidWorld3D(next.MinPxcor, next.MaxPxcor, next.MinPycor, next.MaxPycor, next.MinPzcor, next.MaxPzcor); err != nil {
			return v, fmt.Errorf("minPxcor: %w", err)
		}
	} else if err := ValidWorld(next.MinPxcor, next.MaxPxcor, next.MinPycor, next.MaxPycor); err != nil {
		return v, fmt.Errorf("minPxcor: %w", err)
	}
	if d.UpdateMode != nil {
		if !v.UpdateModeStored {
			return v, fmt.Errorf("updateMode is not stored for this view")
		}
		mode, err := ParseUpdateMode(*d.UpdateMode)
		if err != nil {
			return v, fmt.Errorf("updateMode: %w", err)
		}
		next.UpdateMode = mode
	}
	if d.TicksLabel != nil {
		label := strings.TrimSpace(*d.TicksLabel)
		if strings.ContainsAny(label, "\r\n") {
			return v, fmt.Errorf("ticksLabel must be a single line")
		}
		next.TickLabel = label
	}
	return next, nil
}

func decodeView(base ViewSpec, node *yaml.Node) (ViewSpec, error) {
	allowed := viewKeys(base)
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i].Value
		if !allowed[key] {
			return base, fmt.Errorf("unknown key %q", key)
		}
	}
	var doc viewDoc
	if err := node.Decode(&doc); err != nil {
		return base, friendlyYAMLError(err)
	}
	return doc.apply(base)
}

func viewKeys(v ViewSpec) map[string]bool {
	keys := []string{
		"kind", "x", "y", "width", "height", "description",
		"patchSize", "fontSize",
		"minPxcor", "maxPxcor", "minPycor", "maxPycor",
		"wrapX", "wrapY", "ticksLabel",
	}
	if v.Is3D {
		keys = append(keys, "minPzcor", "maxPzcor", "wrapZ")
	}
	if v.UpdateModeStored {
		keys = append(keys, "updateMode")
	}
	out := map[string]bool{}
	for _, key := range keys {
		out[key] = true
	}
	return out
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

func writeClassicWidgets(section string, view ViewSpec, widgets []Widget, from []int) (string, error) {
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
	var others, old [][]string
	sawView := false
	for _, block := range blocks {
		kind := strings.ToUpper(strings.TrimSpace(block[0]))
		if kind == "GRAPHICS-WINDOW" {
			others = append(others, rewriteGraphicsWindow(block, view))
			sawView = true
			continue
		}
		if classicWidgetKind(kind) {
			old = append(old, block)
			continue
		}
		others = append(others, block)
	}
	if !sawView {
		return "", fmt.Errorf("model has no graphics window")
	}
	out := append([][]string{}, others...)
	for i, w := range widgets {
		base := []string{classicKindLine(w.Kind)}
		if from[i] >= 0 {
			if from[i] >= len(old) {
				return "", fmt.Errorf("widget %d is not in the interface", from[i])
			}
			base = old[from[i]]
		}
		out = append(out, rewriteClassicBlock(base, w))
	}
	parts := make([]string, len(out))
	for i, block := range out {
		parts[i] = strings.Join(block, "\n")
	}
	if len(parts) == 0 {
		return "", nil
	}
	return strings.Join(parts, "\n\n"), nil
}

func rewriteGraphicsWindow(lines []string, v ViewSpec) []string {
	out := append([]string(nil), lines...)
	set := func(i int, val string) {
		for len(out) <= i {
			out = append(out, "")
		}
		out[i] = val
	}
	if len(out) == 0 || !strings.EqualFold(strings.TrimSpace(out[0]), "GRAPHICS-WINDOW") {
		out = []string{"GRAPHICS-WINDOW"}
	}
	set(1, strconv.Itoa(v.X))
	set(2, strconv.Itoa(v.Y))
	set(3, strconv.Itoa(v.X+v.W))
	set(4, strconv.Itoa(v.Y+v.H))
	set(7, formatWidgetNum(v.PatchSize))
	set(9, strconv.Itoa(v.FontSize))
	set(14, bit(v.WrapX))
	set(15, bit(v.WrapY))
	set(17, strconv.Itoa(v.MinPxcor))
	set(18, strconv.Itoa(v.MaxPxcor))
	set(19, strconv.Itoa(v.MinPycor))
	set(20, strconv.Itoa(v.MaxPycor))
	if v.Is3D {
		set(21, strconv.Itoa(v.MinPzcor))
		set(22, strconv.Itoa(v.MaxPzcor))
	} else if v.UpdateModeStored {
		set(21, strconv.Itoa(v.UpdateMode))
	}
	if v.TickLabel != "" || lineAt(out, 24) != "" {
		label := v.TickLabel
		if label == "" {
			label = "NIL"
		}
		set(24, label)
	}
	kept := out[:0]
	for _, line := range out {
		if strings.HasPrefix(strings.TrimSpace(line), widgetDescriptionPrefix) {
			continue
		}
		kept = append(kept, line)
	}
	if v.Description != "" {
		kept = append(kept, widgetDescriptionPrefix+" "+v.Description)
	}
	return kept
}

func bit(on bool) string {
	if on {
		return "1"
	}
	return "0"
}

func rewriteXMLView(section string, v ViewSpec) (string, error) {
	loc := viewOpenTag.FindStringIndex(section)
	if loc == nil {
		return "", fmt.Errorf("model has no view")
	}
	tag := section[loc[0]:loc[1]]
	tag = upsertAttr(tag, "x", strconv.Itoa(v.X))
	tag = upsertAttr(tag, "y", strconv.Itoa(v.Y))
	tag = upsertAttr(tag, "width", strconv.Itoa(v.W))
	tag = upsertAttr(tag, "height", strconv.Itoa(v.H))
	tag = upsertAttr(tag, "patchSize", formatWidgetNum(v.PatchSize))
	tag = upsertAttr(tag, "fontSize", strconv.Itoa(v.FontSize))
	tag = upsertAttr(tag, "minPxcor", strconv.Itoa(v.MinPxcor))
	tag = upsertAttr(tag, "maxPxcor", strconv.Itoa(v.MaxPxcor))
	tag = upsertAttr(tag, "minPycor", strconv.Itoa(v.MinPycor))
	tag = upsertAttr(tag, "maxPycor", strconv.Itoa(v.MaxPycor))
	tag = upsertAttr(tag, "wrappingAllowedX", strconv.FormatBool(v.WrapX))
	tag = upsertAttr(tag, "wrappingAllowedY", strconv.FormatBool(v.WrapY))
	tag = upsertAttr(tag, "description", v.Description)
	if v.Is3D {
		tag = upsertAttr(tag, "minPzcor", strconv.Itoa(v.MinPzcor))
		tag = upsertAttr(tag, "maxPzcor", strconv.Itoa(v.MaxPzcor))
		tag = upsertAttr(tag, "wrappingAllowedZ", strconv.FormatBool(v.WrapZ))
	}
	if v.UpdateModeStored {
		tag = upsertAttr(tag, "updateMode", strconv.Itoa(v.UpdateMode))
	}
	if v.TickLabel == "" {
		tag = removeAttr(tag, "tickCounterLabel")
	} else {
		tag = upsertAttr(tag, "tickCounterLabel", v.TickLabel)
	}
	return section[:loc[0]] + tag + section[loc[1]:], nil
}

func removeAttr(tag, name string) string {
	re := regexp.MustCompile(`(?i)\s` + regexp.QuoteMeta(name) + `\s*=\s*("[^"]*"|'[^']*')`)
	return re.ReplaceAllString(tag, "")
}

func classicKindLine(kind string) string {
	switch kind {
	case "button":
		return "BUTTON"
	case "slider":
		return "SLIDER"
	case "switch":
		return "SWITCH"
	case "monitor":
		return "MONITOR"
	case "chooser":
		return "CHOOSER"
	case "input":
		return "INPUTBOX"
	case "note":
		return "TEXTBOX"
	case "plot":
		return "PLOT"
	case "output":
		return "OUTPUT"
	default:
		return strings.ToUpper(kind)
	}
}

func writeXMLWidgets(section string, view ViewSpec, widgets []Widget, from []int) (string, error) {
	section, err := rewriteXMLView(section, view)
	if err != nil {
		return "", err
	}
	matches := xmlWidgetTag.FindAllStringIndex(section, -1)
	spans := make([][2]int, len(matches))
	for i, match := range matches {
		end := xmlElementEnd(section, match[0])
		if end < 0 {
			return "", fmt.Errorf("widget %d has a broken tag", i)
		}
		spans[i] = [2]int{match[0], end}
	}
	var b strings.Builder
	if len(spans) == 0 {
		idx := strings.LastIndex(strings.ToLower(section), "</widgets>")
		if idx < 0 {
			return "", fmt.Errorf("the interface must be a <widgets> element")
		}
		b.WriteString(section[:idx])
		for i, w := range widgets {
			if i > 0 {
				b.WriteString("\n    ")
			}
			b.WriteString(freshXMLWidget(w))
		}
		if len(widgets) > 0 {
			b.WriteString("\n  ")
		}
		b.WriteString(section[idx:])
		return b.String(), nil
	}
	b.WriteString(section[:spans[0][0]])
	for i, w := range widgets {
		if i > 0 {
			b.WriteByte('\n')
		}
		if from[i] >= 0 {
			if from[i] >= len(spans) {
				return "", fmt.Errorf("widget %d is not in the interface", from[i])
			}
			el := section[spans[from[i]][0]:spans[from[i]][1]]
			b.WriteString(rewriteXMLElement(el, w))
			continue
		}
		b.WriteString(freshXMLWidget(w))
	}
	b.WriteString(section[spans[len(spans)-1][1]:])
	return b.String(), nil
}

func xmlElementEnd(section string, tagStart int) int {
	tagEnd := xmlTagEnd(section, tagStart)
	if tagEnd < 0 {
		return -1
	}
	if strings.HasSuffix(section[tagStart:tagEnd], "/>") {
		return tagEnd
	}
	name := ""
	if m := xmlWidgetTag.FindStringSubmatch(section[tagStart:]); len(m) > 1 {
		name = m[1]
	}
	rel := strings.Index(strings.ToLower(section[tagEnd:]), "</"+strings.ToLower(name)+">")
	if rel < 0 {
		return -1
	}
	return tagEnd + rel + len("</") + len(name) + len(">")
}

func freshXMLWidget(w Widget) string {
	switch w.Kind {
	case "button", "monitor":
		return rewriteXMLElement("<"+w.Kind+">"+escapeXMLText(w.Source)+"</"+w.Kind+">", w)
	case "note":
		return rewriteXMLElement("<note>"+escapeXMLText(w.Display)+"</note>", w)
	default:
		return rewriteXMLElement("<"+w.Kind+"/>", w)
	}
}

func rewriteXMLElement(element string, w Widget) string {
	tagEnd := xmlTagEnd(element, 0)
	if tagEnd < 0 {
		return element
	}
	tag := upsertXMLAttrs(element[:tagEnd], w)
	body := element[tagEnd:]
	if w.Kind == "button" || w.Kind == "monitor" {
		closeAt := strings.Index(strings.ToLower(body), "</"+w.Kind+">")
		if closeAt >= 0 {
			body = ">" + escapeXMLText(codeText(w)) + body[closeAt:]
			tag = strings.TrimSuffix(strings.TrimSuffix(tag, "/>"), ">")
		}
	}
	return tag + body
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
