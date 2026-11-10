package nl

import (
	"encoding/xml"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// DynamicsError is a problem in the Dynamics tab text. Line is 1-based
// and zero when the message is about the whole definition.
type DynamicsError struct {
	Line int
	Msg  string
}

func (e *DynamicsError) Error() string {
	if e == nil {
		return ""
	}
	if e.Line > 0 {
		return fmt.Sprintf("line %d: %s", e.Line, e.Msg)
	}
	return e.Msg
}

func dynErr(line int, format string, args ...any) error {
	return &DynamicsError{Line: line, Msg: fmt.Sprintf(format, args...)}
}

// Definition is the text the Dynamics tab edits. It lists dt, each
// stock, each flow, and each converter. A value that is one plain line
// sits on the field line. Anything with a newline or surrounding
// space uses a << >> block so the NetLogo expression is kept as written.
// Nil returns an empty string.
func (sd *SystemDynamics) Definition() string {
	if sd == nil {
		return ""
	}
	var b strings.Builder
	dt := sd.DT
	if strings.TrimSpace(dt) == "" {
		dt = "1.0"
	}
	writeDynValue(&b, 0, "dt", dt)
	for _, s := range sd.Stocks {
		b.WriteByte('\n')
		fmt.Fprintf(&b, "stock %s\n", s.Name)
		writeDynValue(&b, 2, "initial", s.Initial)
		writeDynValue(&b, 2, "allow-negative", strconv.FormatBool(s.AllowNegative))
	}
	for _, r := range sd.Rates {
		b.WriteByte('\n')
		fmt.Fprintf(&b, "flow %s\n", r.Name)
		writeDynValue(&b, 2, "expression", r.Expression)
		writeDynValue(&b, 2, "source", r.Source)
		writeDynValue(&b, 2, "sink", r.Sink)
	}
	for _, c := range sd.Converters {
		b.WriteByte('\n')
		fmt.Fprintf(&b, "converter %s\n", c.Name)
		writeDynValue(&b, 2, "expression", c.Expression)
	}
	return b.String()
}

// ParseDynamicsDefinition reads the text Definition writes, and the
// same text after a person edits it. Constant on a converter is
// derived from the expression, the way the file parser does.
func ParseDynamicsDefinition(src string) (*SystemDynamics, error) {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	src = strings.ReplaceAll(src, "\r", "\n")
	if strings.TrimSpace(src) == "" {
		return nil, dynErr(1, "the definition is empty")
	}
	p := &dynParser{lines: strings.Split(src, "\n")}
	sd, err := p.parse()
	if err != nil {
		return nil, err
	}
	return sd, nil
}

type dynParser struct {
	lines []string
	i     int
}

type dynDecl struct {
	kind       string
	name       string
	line       int
	initial    string
	hasInitial bool
	allow      *bool
	expression string
	hasExpr    bool
	source     string
	hasSource  bool
	sink       string
	hasSink    bool
}

func (p *dynParser) parse() (*SystemDynamics, error) {
	sd := &SystemDynamics{}
	seenDT := false
	used := map[string]int{}
	var decls []dynDecl
	for {
		p.skipNoise()
		if p.i >= len(p.lines) {
			break
		}
		line := p.lines[p.i]
		lineNo := p.i + 1
		if leadingIndent(line) != 0 {
			return nil, dynErr(lineNo, "expected dt, stock, flow, or converter")
		}
		p.i++
		key, rest := splitDynToken(strings.TrimSpace(line))
		key = strings.ToLower(key)
		switch key {
		case "dt":
			if seenDT {
				return nil, dynErr(lineNo, "dt is already set")
			}
			value, err := p.parseValue(rest, 0, lineNo)
			if err != nil {
				return nil, err
			}
			if strings.TrimSpace(value) == "" {
				return nil, dynErr(lineNo, "dt needs a value")
			}
			sd.DT = value
			seenDT = true
		case "stock", "flow", "rate", "converter":
			kind := key
			if kind == "rate" {
				kind = "flow"
			}
			name := strings.TrimSpace(rest)
			if name == "" {
				return nil, dynErr(lineNo, "%s needs a name", kind)
			}
			if strings.ContainsAny(name, " \t") {
				return nil, dynErr(lineNo, "%s name must be one word", kind)
			}
			if prev, ok := used[strings.ToLower(name)]; ok {
				return nil, dynErr(lineNo, "%s is already used on line %d", name, prev)
			}
			used[strings.ToLower(name)] = lineNo
			decl := dynDecl{kind: kind, name: name, line: lineNo}
			if err := p.parseFields(&decl); err != nil {
				return nil, err
			}
			decls = append(decls, decl)
		default:
			return nil, dynErr(lineNo, "expected dt, stock, flow, or converter")
		}
	}
	if !seenDT {
		sd.DT = "1.0"
	}
	named := map[string]bool{}
	for _, d := range decls {
		named[d.name] = true
	}
	for _, d := range decls {
		switch d.kind {
		case "stock":
			if !d.hasInitial {
				return nil, dynErr(d.line, "stock %s needs an initial value", d.name)
			}
			// The file parser uses 0 when a stock element is blank.
			if strings.TrimSpace(d.initial) == "" {
				d.initial = "0"
			}
			allow := true
			if d.allow != nil {
				allow = *d.allow
			}
			sd.Stocks = append(sd.Stocks, SDStock{Name: d.name, Initial: d.initial, AllowNegative: allow})
		case "flow":
			if !d.hasExpr || strings.TrimSpace(d.expression) == "" {
				return nil, dynErr(d.line, "flow %s needs an expression", d.name)
			}
			if d.source != "" && !named[d.source] {
				return nil, dynErr(d.line, "flow %s source %q is not a stock, flow, or converter", d.name, d.source)
			}
			if d.sink != "" && !named[d.sink] {
				return nil, dynErr(d.line, "flow %s sink %q is not a stock, flow, or converter", d.name, d.sink)
			}
			sd.Rates = append(sd.Rates, SDRate{Name: d.name, Expression: d.expression, Source: d.source, Sink: d.sink})
		case "converter":
			if !d.hasExpr || strings.TrimSpace(d.expression) == "" {
				return nil, dynErr(d.line, "converter %s needs an expression", d.name)
			}
			sd.Converters = append(sd.Converters, SDConverter{
				Name:       d.name,
				Expression: d.expression,
				Constant:   isSDConstant(d.expression),
			})
		}
	}
	return sd, nil
}

func (p *dynParser) parseFields(decl *dynDecl) error {
	for {
		p.skipNoise()
		if p.i >= len(p.lines) {
			return nil
		}
		line := p.lines[p.i]
		if leadingIndent(line) == 0 {
			return nil
		}
		lineNo := p.i + 1
		ind := leadingIndent(line)
		p.i++
		key, rest := splitDynToken(line[ind:])
		key = strings.ToLower(key)
		value, err := p.parseValue(rest, ind, lineNo)
		if err != nil {
			return err
		}
		switch key {
		case "initial":
			if decl.kind != "stock" {
				return dynErr(lineNo, "%s cannot have an initial value", decl.kind)
			}
			if decl.hasInitial {
				return dynErr(lineNo, "initial is already set")
			}
			decl.initial = value
			decl.hasInitial = true
		case "allow-negative":
			if decl.kind != "stock" {
				return dynErr(lineNo, "%s cannot set allow-negative", decl.kind)
			}
			if decl.allow != nil {
				return dynErr(lineNo, "allow-negative is already set")
			}
			switch strings.ToLower(value) {
			case "true":
				t := true
				decl.allow = &t
			case "false":
				f := false
				decl.allow = &f
			default:
				return dynErr(lineNo, "allow-negative must be true or false")
			}
		case "expression":
			if decl.kind == "stock" {
				return dynErr(lineNo, "stock cannot have an expression")
			}
			if decl.hasExpr {
				return dynErr(lineNo, "expression is already set")
			}
			decl.expression = value
			decl.hasExpr = true
		case "source", "sink":
			if decl.kind != "flow" {
				return dynErr(lineNo, "%s cannot have a %s", decl.kind, key)
			}
			if key == "source" {
				if decl.hasSource {
					return dynErr(lineNo, "source is already set")
				}
				decl.source = value
				decl.hasSource = true
			} else {
				if decl.hasSink {
					return dynErr(lineNo, "sink is already set")
				}
				decl.sink = value
				decl.hasSink = true
			}
		default:
			return dynErr(lineNo, "unknown field %q", key)
		}
	}
}

func (p *dynParser) parseValue(rest string, fieldIndent, lineNo int) (string, error) {
	trimmed := strings.TrimSpace(rest)
	if fence, ok := dynFenceOpener(trimmed); ok {
		return p.readHeredoc(fieldIndent, fence, lineNo)
	}
	return trimmed, nil
}

func (p *dynParser) readHeredoc(fieldIndent int, fence string, openLine int) (string, error) {
	var parts []string
	for p.i < len(p.lines) {
		line := p.lines[p.i]
		if leadingIndent(line) <= fieldIndent && strings.TrimSpace(line) == fence {
			p.i++
			return strings.Join(parts, "\n"), nil
		}
		parts = append(parts, line)
		p.i++
	}
	return "", dynErr(openLine, "missing %s", fence)
}

func (p *dynParser) skipNoise() {
	for p.i < len(p.lines) {
		line := p.lines[p.i]
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, ";") {
			p.i++
			continue
		}
		return
	}
}

func writeDynValue(b *strings.Builder, indent int, key, value string) {
	pad := strings.Repeat(" ", indent)
	if value == "" {
		fmt.Fprintf(b, "%s%s\n", pad, key)
		return
	}
	if simpleDynValue(value) {
		fmt.Fprintf(b, "%s%s %s\n", pad, key, value)
		return
	}
	fence := dynFence(value, indent)
	open := strings.Repeat("<", len(fence))
	fmt.Fprintf(b, "%s%s %s\n", pad, key, open)
	for _, line := range strings.Split(value, "\n") {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	fmt.Fprintf(b, "%s%s\n", pad, fence)
}

func simpleDynValue(value string) bool {
	if strings.Contains(value, "\n") || strings.TrimSpace(value) != value {
		return false
	}
	if _, ok := dynFenceOpener(value); ok {
		return false
	}
	trim := strings.TrimSpace(value)
	return len(trim) < 2 || strings.Trim(trim, ">") != ""
}

func dynFence(value string, fieldIndent int) string {
	for n := 2; n < 20; n++ {
		fence := strings.Repeat(">", n)
		if !heredocWouldClose(value, fence, fieldIndent) {
			return fence
		}
	}
	return strings.Repeat(">", 20)
}

func heredocWouldClose(value, fence string, fieldIndent int) bool {
	for _, line := range strings.Split(value, "\n") {
		if leadingIndent(line) <= fieldIndent && strings.TrimSpace(line) == fence {
			return true
		}
	}
	return false
}

func dynFenceOpener(s string) (string, bool) {
	if len(s) >= 2 && strings.Trim(s, "<") == "" {
		return strings.Repeat(">", len(s)), true
	}
	return "", false
}

func splitDynToken(s string) (string, string) {
	i := 0
	for i < len(s) && s[i] != ' ' && s[i] != '\t' {
		i++
	}
	tok := s[:i]
	rest := s[i:]
	if len(rest) > 0 && (rest[0] == ' ' || rest[0] == '\t') {
		rest = rest[1:]
	}
	return tok, rest
}

func leadingIndent(line string) int {
	i := 0
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	return i
}

// XML is the <systemDynamics> element WriteDynamics puts in a model
// file. Figure indexes point at stocks, flows, and converters in that
// order. An empty flow end is a reservoir and omits its index.
func (sd *SystemDynamics) XML() (string, error) {
	if sd == nil {
		return "", nil
	}
	dt := sd.DT
	if strings.TrimSpace(dt) == "" {
		dt = "1.0"
	}
	index := map[string]int{}
	n := 0
	claim := func(name string) {
		if _, ok := index[name]; !ok {
			index[name] = n
		}
		n++
	}
	for _, s := range sd.Stocks {
		claim(s.Name)
	}
	for _, r := range sd.Rates {
		claim(r.Name)
	}
	for _, c := range sd.Converters {
		claim(c.Name)
	}
	ref := func(name string) (*int, error) {
		if name == "" {
			return nil, nil
		}
		i, ok := index[name]
		if !ok {
			return nil, fmt.Errorf("system dynamics: %q is not in the diagram", name)
		}
		return &i, nil
	}
	var inner strings.Builder
	for _, s := range sd.Stocks {
		allow := "true"
		if !s.AllowNegative {
			allow = "false"
		}
		b, err := xml.Marshal(struct {
			XMLName xml.Name `xml:"stock"`
			Name    string   `xml:"name,attr"`
			Allow   string   `xml:"allowNegative,attr"`
			Text    string   `xml:",chardata"`
		}{Name: s.Name, Allow: allow, Text: s.Initial})
		if err != nil {
			return "", err
		}
		inner.Write(b)
	}
	for _, r := range sd.Rates {
		start, err := ref(r.Source)
		if err != nil {
			return "", err
		}
		end, err := ref(r.Sink)
		if err != nil {
			return "", err
		}
		b, err := xml.Marshal(struct {
			XMLName xml.Name `xml:"rate"`
			Name    string   `xml:"name,attr"`
			Start   *int     `xml:"startFigure,attr,omitempty"`
			End     *int     `xml:"endFigure,attr,omitempty"`
			Text    string   `xml:",chardata"`
		}{Name: r.Name, Start: start, End: end, Text: r.Expression})
		if err != nil {
			return "", err
		}
		inner.Write(b)
	}
	for _, c := range sd.Converters {
		b, err := xml.Marshal(struct {
			XMLName xml.Name `xml:"converter"`
			Name    string   `xml:"name,attr"`
			Text    string   `xml:",chardata"`
		}{Name: c.Name, Text: c.Expression})
		if err != nil {
			return "", err
		}
		inner.Write(b)
	}
	out, err := xml.Marshal(struct {
		XMLName xml.Name `xml:"systemDynamics"`
		DT      string   `xml:"dt,attr"`
		Inner   string   `xml:",innerxml"`
	}{DT: dt, Inner: inner.String()})
	if err != nil {
		return "", err
	}
	return string(out), nil
}

var systemDynamicsElement = regexp.MustCompile(`(?s)<systemDynamics\b[^>]*/>|<systemDynamics\b[^>]*>.*?</systemDynamics>`)

// WriteDynamics returns content with sd as its system dynamics section.
// A .nlogox document keeps every other element. A .nlogo document keeps
// every other @#$#@#$#@ section and writes the diagram in section 6.
// A nil diagram removes one that is already there. This is the write a
// save of either file would use for the diagram.
func WriteDynamics(content string, sd *SystemDynamics) (string, error) {
	xmlText := ""
	if sd != nil {
		var err error
		xmlText, err = sd.XML()
		if err != nil {
			return "", err
		}
	}
	switch dynamicsDocKind(content) {
	case "xml":
		return writeDynamicsXML(content, xmlText)
	case "nlogo":
		return writeDynamicsNLogo(content, xmlText)
	default:
		return "", fmt.Errorf("system dynamics can be written into a .nlogo or .nlogox file")
	}
}

func dynamicsDocKind(content string) string {
	trim := strings.TrimSpace(content)
	if strings.HasPrefix(trim, "<?xml") || strings.HasPrefix(trim, "<model") {
		return "xml"
	}
	if strings.Contains(content, sectionSep) {
		return "nlogo"
	}
	return ""
}

func writeDynamicsXML(content, xmlText string) (string, error) {
	if systemDynamicsElement.MatchString(content) {
		return systemDynamicsElement.ReplaceAllString(content, xmlText), nil
	}
	if xmlText == "" {
		return content, nil
	}
	idx := strings.LastIndex(content, "</model>")
	if idx < 0 {
		return "", fmt.Errorf("system dynamics: .nlogox file has no model element")
	}
	return content[:idx] + xmlText + "\n" + content[idx:], nil
}

func writeDynamicsNLogo(content, xmlText string) (string, error) {
	parts := strings.Split(content, sectionSep)
	if xmlText == "" {
		if len(parts) > 6 {
			parts[6] = "\n"
		}
		return strings.Join(parts, sectionSep), nil
	}
	for len(parts) < 7 {
		parts = append(parts, "\n")
	}
	parts[6] = "\n" + xmlText + "\n"
	return strings.Join(parts, sectionSep), nil
}

var dynamicsLineErr = regexp.MustCompile(`^line (\d+): (.*)$`)

// ExplainDynamicsCompile turns a Compile error into a message about the
// generated procedures, or about the model's own code when the line is
// there. The snippet is the line the compiler pointed at.
func ExplainDynamicsCompile(modelCode, generated string, err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	m := dynamicsLineErr.FindStringSubmatch(msg)
	if m == nil || generated == "" {
		return fmt.Errorf("generated procedures did not compile: %s", msg)
	}
	n, convErr := strconv.Atoi(m[1])
	if convErr != nil {
		return fmt.Errorf("generated procedures did not compile: %s", msg)
	}
	rest := m[2]
	prefix := modelCode
	if !strings.HasSuffix(prefix, "\n") {
		prefix += "\n"
	}
	start := strings.Count(prefix, "\n") + 1
	if n < start {
		return withLineSnippet("the model code did not compile: line %d: %s", n, rest, modelCode)
	}
	genLine := n - start + 1
	return withLineSnippet("generated procedures, line %d: %s", genLine, rest, generated)
}

func withLineSnippet(format string, n int, rest, src string) error {
	snippet := nthDynLine(src, n)
	if snippet == "" {
		return fmt.Errorf(format, n, rest)
	}
	return fmt.Errorf(format+"\n%4d | %s", n, rest, n, snippet)
}

func nthDynLine(src string, n int) string {
	if n <= 0 {
		return ""
	}
	lines := strings.Split(src, "\n")
	if n-1 >= len(lines) {
		return ""
	}
	return lines[n-1]
}
