package nl

import (
	"encoding/xml"
	"errors"
	"fmt"
	"strings"
)

// WidgetSection is the interface text the Widgets tab edits.
// XML is true when Text is the .nlogox widgets element.
type WidgetSection struct {
	Text string
	XML  bool
}

// ExtractWidgetSection returns the editable interface of a model file.
// A classic file yields the section between the first two separators.
// A .nlogox file yields the widgets element, tags included.
func ExtractWidgetSection(source string) WidgetSection {
	if ModelTextIsXML(source) {
		start, end, ok := findWidgetsElement(source)
		if !ok {
			return WidgetSection{XML: true}
		}
		return WidgetSection{Text: source[start:end], XML: true}
	}
	parts := strings.Split(source, sectionSep)
	if len(parts) < 2 {
		return WidgetSection{}
	}
	return WidgetSection{Text: parts[1]}
}

// ReplaceWidgetSection writes the edited interface back into a model file.
// Code, info, shapes, and every other section stay byte for byte.
func ReplaceWidgetSection(source, section string) (string, error) {
	if ModelTextIsXML(source) {
		return replaceXMLWidgets(source, section)
	}
	return replaceClassicWidgets(source, section)
}

// ParseWidgetSection reads an interface section into a view and widgets.
// is3D selects the classic 3D graphics-window layout. Errors include the
// 1-based line in the section when the parser can name one.
func ParseWidgetSection(section string, xml, is3D bool) (ViewSpec, []Widget, []string, error) {
	if xml {
		return parseWidgetXML(section, is3D)
	}
	m := &FileModel{Is3D: is3D, View: DefaultView()}
	if err := parseWidgetSection(m, section); err != nil {
		return ViewSpec{}, nil, nil, err
	}
	if !m.View.Set {
		return ViewSpec{}, nil, nil, fmt.Errorf("line 1: the interface needs a graphics window")
	}
	if m.Widgets == nil {
		m.Widgets = []Widget{}
	}
	return m.View, m.Widgets, append([]string(nil), m.Notes...), nil
}

func replaceClassicWidgets(source, section string) (string, error) {
	parts := strings.Split(source, sectionSep)
	if len(parts) < 2 {
		return "", fmt.Errorf("model has no widget section")
	}
	parts[1] = section
	return strings.Join(parts, sectionSep), nil
}

func replaceXMLWidgets(source, section string) (string, error) {
	if !strings.Contains(strings.ToLower(section), "<widgets") || !strings.Contains(strings.ToLower(section), "</widgets>") {
		return "", fmt.Errorf("line 1: the interface must be a <widgets> element")
	}
	start, end, ok := findWidgetsElement(source)
	if !ok {
		return "", fmt.Errorf("model has no <widgets> element")
	}
	return source[:start] + section + source[end:], nil
}

func findWidgetsElement(source string) (start, end int, ok bool) {
	lower := strings.ToLower(source)
	start = strings.Index(lower, "<widgets")
	if start < 0 {
		return 0, 0, false
	}
	rel := strings.Index(lower[start:], "</widgets>")
	if rel < 0 {
		return 0, 0, false
	}
	return start, start + rel + len("</widgets>"), true
}

// widgetXMLPrefix is wrapped around a widgets element so the existing
// .nlogox reader can parse it. Its line count is subtracted from errors.
const widgetXMLPrefix = "<model><code><![CDATA[to __widget_edit end\n]]></code>\n"

func parseWidgetXML(section string, is3D bool) (ViewSpec, []Widget, []string, error) {
	full := widgetXMLPrefix + section + "\n</model>"
	m, err := parseNLogoX("widgets.nlogox", full)
	if err != nil {
		return ViewSpec{}, nil, nil, shiftXMLLine(err, strings.Count(widgetXMLPrefix, "\n"))
	}
	if is3D {
		m.Is3D = true
		m.View.Is3D = true
	}
	if !m.View.Set {
		return ViewSpec{}, nil, nil, fmt.Errorf("line 1: the interface needs a view")
	}
	if m.Widgets == nil {
		m.Widgets = []Widget{}
	}
	return m.View, m.Widgets, append([]string(nil), m.Notes...), nil
}

func shiftXMLLine(err error, prefixLines int) error {
	if err == nil || prefixLines <= 0 {
		return err
	}
	var syn *xml.SyntaxError
	if errors.As(err, &syn) {
		line := syn.Line - prefixLines
		if line < 1 {
			line = 1
		}
		return fmt.Errorf("line %d: %s", line, syn.Msg)
	}
	return err
}
