package nl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDynamicsDefinitionRoundTrip(t *testing.T) {
	models := []string{
		"library/Sample Models/System Dynamics/Exponential Growth.nlogox",
		"library/Sample Models/System Dynamics/Logistic Growth.nlogox",
		"library/Sample Models/System Dynamics/Wolf Sheep Predation (System Dynamics).nlogox",
		"library/Sample Models/System Dynamics/Wolf Sheep Predation (Docked Hybrid).nlogox",
		"library/Sample Models/System Dynamics/Unverified/Tabonuco Yagrumo.nlogox",
		"library/Sample Models/System Dynamics/Unverified/Tabonuco Yagrumo Hybrid.nlogox",
	}
	for _, name := range models {
		t.Run(name, func(t *testing.T) {
			m := loadModel(t, name)
			if m.Dynamics == nil {
				t.Fatal("no system dynamics diagram")
			}
			text := m.Dynamics.Definition()
			again, err := ParseDynamicsDefinition(text)
			if err != nil {
				t.Fatalf("parse definition: %v\n%s", err, text)
			}
			if again.Definition() != text {
				t.Fatalf("definition did not settle\n---\n%s", again.Definition())
			}
			if again.Source() != m.Dynamics.Source() {
				t.Fatalf("generated source changed\n--- want\n%s\n--- got\n%s", m.Dynamics.Source(), again.Source())
			}
			assertSameDynamics(t, m.Dynamics, again)

			raw, err := os.ReadFile(filepath.Join("..", "..", "models", name))
			if err != nil {
				t.Fatal(err)
			}
			wrote, err := WriteDynamics(string(raw), again)
			if err != nil {
				t.Fatal(err)
			}
			m2, err := ParseModel(name, wrote)
			if err != nil {
				t.Fatal(err)
			}
			if m2.Code != m.Code {
				t.Fatal("rewriting the diagram changed the code")
			}
			if m2.Info != m.Info {
				t.Fatal("rewriting the diagram changed the info tab")
			}
			if len(m2.Widgets) != len(m.Widgets) {
				t.Fatalf("widgets %d, want %d", len(m2.Widgets), len(m.Widgets))
			}
			if m2.Dynamics == nil {
				t.Fatal("rewritten file lost the diagram")
			}
			assertSameDynamics(t, again, m2.Dynamics)
			if m2.Dynamics.Source() != m.Dynamics.Source() {
				t.Fatal("file round trip changed the generated source")
			}
		})
	}
}

func TestDynamicsEditSurvivesBothFileFormats(t *testing.T) {
	m := loadModel(t, "library/Sample Models/System Dynamics/Exponential Growth.nlogox")
	editedText := strings.Replace(m.Dynamics.Definition(), "initial 1\n", "initial 6\n", 1)
	if editedText == m.Dynamics.Definition() {
		t.Fatal("initial value was not in the definition")
	}
	sd, err := ParseDynamicsDefinition(editedText)
	if err != nil {
		t.Fatal(err)
	}
	if sd.Stocks[0].Initial != "6" {
		t.Fatalf("initial %q", sd.Stocks[0].Initial)
	}
	raw, err := os.ReadFile(filepath.Join("..", "..", "models", "library/Sample Models/System Dynamics/Exponential Growth.nlogox"))
	if err != nil {
		t.Fatal(err)
	}
	nlogo := strings.Join([]string{m.Code, "\n", "\n", "\n", "\nNetLogo 6.4.0\n", "\n", "\n"}, sectionSep)
	for _, doc := range []struct {
		name, body string
	}{
		{"edited.nlogox", string(raw)},
		{"edited.nlogo", nlogo},
	} {
		t.Run(doc.name, func(t *testing.T) {
			wrote, err := WriteDynamics(doc.body, sd)
			if err != nil {
				t.Fatal(err)
			}
			got, err := ParseModel(doc.name, wrote)
			if err != nil {
				t.Fatal(err)
			}
			if got.Code != m.Code {
				t.Fatal("code changed")
			}
			assertSameDynamics(t, sd, got.Dynamics)
			vm, err := got.NewVM(1)
			if err != nil {
				t.Fatal(err)
			}
			mustCall(t, vm, "setup")
			if n := numGlobal(t, vm, "stock"); n != 6 {
				t.Fatalf("stock = %v, want 6", n)
			}
			cleared, err := WriteDynamics(wrote, nil)
			if err != nil {
				t.Fatal(err)
			}
			plain, err := ParseModel(doc.name, cleared)
			if err != nil {
				t.Fatal(err)
			}
			if plain.Dynamics != nil {
				t.Fatal("clearing the diagram left one behind")
			}
			if plain.Code != m.Code {
				t.Fatal("clearing the diagram changed the code")
			}
		})
	}
}

func TestDynamicsDefinitionPreservesMultiline(t *testing.T) {
	sd, err := ParseDynamicsDefinition(`dt 1

stock gaps
  initial <<
;; since we don't know what order
0.6
  >>
  allow-negative false

flow grows
  expression <<
gaps * 2
  [ more ]
  >>
  source
  sink gaps

converter amp
  expression <<
a < b & c
  >>
`)
	if err != nil {
		t.Fatal(err)
	}
	if sd.Stocks[0].Initial != ";; since we don't know what order\n0.6" {
		t.Fatalf("initial %q", sd.Stocks[0].Initial)
	}
	if sd.Stocks[0].AllowNegative {
		t.Fatal("allow-negative was lost")
	}
	if sd.Rates[0].Expression != "gaps * 2\n  [ more ]" || sd.Rates[0].Source != "" || sd.Rates[0].Sink != "gaps" {
		t.Fatalf("flow %+v", sd.Rates[0])
	}
	if sd.Converters[0].Expression != "a < b & c" || sd.Converters[0].Constant {
		t.Fatalf("converter %+v", sd.Converters[0])
	}
	again, err := ParseDynamicsDefinition(sd.Definition())
	if err != nil {
		t.Fatal(err)
	}
	assertSameDynamics(t, sd, again)

	xmlText, err := sd.XML()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(xmlText, "a &lt; b &amp; c") && !strings.Contains(xmlText, "a < b & c") {
		t.Fatalf("xml lost the expression: %s", xmlText)
	}
	doc := `<?xml version="1.0"?>
<model>
  <code><![CDATA[to setup
  system-dynamics-setup
end
to go
  system-dynamics-go
end
]]></code>
</model>`
	wrote, err := WriteDynamics(doc, sd)
	if err != nil {
		t.Fatal(err)
	}
	m, err := ParseModel("multi.nlogox", wrote)
	if err != nil {
		t.Fatalf("parse xml: %v\n%s", err, wrote)
	}
	assertSameDynamics(t, sd, m.Dynamics)
}

func TestDynamicsParseErrors(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{"empty", "  \n", "line 1: the definition is empty"},
		{"no name", "stock\n", "line 1: stock needs a name"},
		{"bad flag", "stock a\n  initial 1\n  allow-negative yes\n", "line 3: allow-negative must be true or false"},
		{"unknown end", "stock a\n  initial 1\nflow f\n  expression 1\n  sink missing\n", `sink "missing"`},
		{"duplicate", "stock a\n  initial 1\nconverter A\n  expression 2\n", "already used"},
		{"open block", "stock a\n  initial <<\n1\n", "missing >>"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseDynamicsDefinition(tc.src)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %v, want %q", err, tc.want)
			}
		})
	}
}

func TestExplainDynamicsCompile(t *testing.T) {
	sd, err := ParseDynamicsDefinition("dt 1\n\nstock s\n  initial 1\n  allow-negative true\n\nflow f\n  expression (\n  sink s\n")
	if err != nil {
		t.Fatal(err)
	}
	m := &FileModel{
		Code:     "to setup\n  system-dynamics-setup\nend\nto go\n  system-dynamics-go\nend\n",
		Dynamics: sd,
		View:     DefaultView(),
	}
	_, err = m.Compile()
	if err == nil {
		t.Fatal("bad flow expression compiled")
	}
	msg := ExplainDynamicsCompile(m.Code, sd.Source(), err)
	if msg == nil || !strings.Contains(msg.Error(), "generated procedures, line") || !strings.Contains(msg.Error(), " | ") {
		t.Fatalf("explain: %v", msg)
	}
}

func assertSameDynamics(t *testing.T, want, got *SystemDynamics) {
	t.Helper()
	if want == nil || got == nil {
		t.Fatalf("nil diagram want %v got %v", want != nil, got != nil)
	}
	if want.DT != got.DT {
		t.Fatalf("dt %q, want %q", got.DT, want.DT)
	}
	if len(want.Stocks) != len(got.Stocks) || len(want.Rates) != len(got.Rates) || len(want.Converters) != len(got.Converters) {
		t.Fatalf("sizes stocks %d/%d rates %d/%d converters %d/%d",
			len(got.Stocks), len(want.Stocks), len(got.Rates), len(want.Rates), len(got.Converters), len(want.Converters))
	}
	for i := range want.Stocks {
		if got.Stocks[i] != want.Stocks[i] {
			t.Fatalf("stock %d %+v, want %+v", i, got.Stocks[i], want.Stocks[i])
		}
	}
	for i := range want.Rates {
		if got.Rates[i] != want.Rates[i] {
			t.Fatalf("flow %d %+v, want %+v", i, got.Rates[i], want.Rates[i])
		}
	}
	for i := range want.Converters {
		if got.Converters[i] != want.Converters[i] {
			t.Fatalf("converter %d %+v, want %+v", i, got.Converters[i], want.Converters[i])
		}
	}
}
