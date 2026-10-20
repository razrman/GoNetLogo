package nl

import (
	"fmt"
	"strings"
	"testing"
)

func TestParseOutputWidgetNLogo(t *testing.T) {
	src := `to setup
  clear-all
  output-print "hi"
end
@#$#@#$#@
OUTPUT
10
20
210
120
15
`
	m, err := ParseModel("out.nlogo", src)
	if err != nil {
		t.Fatal(err)
	}
	w := outputWidget(t, m)
	if w.X != 10 || w.Y != 20 || w.W != 200 || w.H != 100 || w.FontSize != 15 {
		t.Fatalf("widget %+v", w)
	}
	if !m.HasOutput() {
		t.Fatal("HasOutput")
	}
	for _, note := range m.Notes {
		if strings.Contains(note, "output") {
			t.Fatalf("output widget was skipped: %q", note)
		}
	}

	// A missing font size uses NetLogo's default of 12.
	bare := `to go end
@#$#@#$#@
OUTPUT
0
0
40
30
`
	m, err = ParseModel("bare.nlogo", bare)
	if err != nil {
		t.Fatal(err)
	}
	w = outputWidget(t, m)
	if w.W != 40 || w.H != 30 || w.FontSize != 12 {
		t.Fatalf("default font %+v", w)
	}
}

func TestParseOutputWidgetNLogoX(t *testing.T) {
	src := `<?xml version="1.0"?>
<model version="NetLogo 7.0.0">
  <code><![CDATA[
to setup
  clear-all
  output-print "xml"
end
]]></code>
  <widgets>
    <view minPxcor="-1" maxPxcor="1" minPycor="-1" maxPycor="1" patchSize="8" wrappingAllowedX="false" wrappingAllowedY="false"/>
    <output x="32" y="48" width="180" height="90" fontSize="11"/>
  </widgets>
</model>`
	m, err := ParseModel("out.nlogox", src)
	if err != nil {
		t.Fatal(err)
	}
	w := outputWidget(t, m)
	if w.X != 32 || w.Y != 48 || w.W != 180 || w.H != 90 || w.FontSize != 11 {
		t.Fatalf("widget %+v", w)
	}
	vm, err := m.NewVM(1)
	if err != nil {
		t.Fatal(err)
	}
	if !vm.HasOutput() {
		t.Fatal("vm did not inherit the output widget")
	}
	if _, err := vm.ExecuteLine("setup"); err != nil {
		t.Fatal(err)
	}
	lines, partial := vm.OutputArea()
	if partial != "" || len(lines) != 1 || lines[0] != "xml" {
		t.Fatalf("area %q %q", lines, partial)
	}
	if got := vm.Output(); len(got) != 0 {
		t.Fatalf("command center %q", got)
	}

	omitted := `<?xml version="1.0"?>
<model version="NetLogo 7.0.0">
  <code>to go end</code>
  <widgets>
    <output></output>
  </widgets>
</model>`
	m, err = ParseModel("default.nlogox", omitted)
	if err != nil {
		t.Fatal(err)
	}
	w = outputWidget(t, m)
	if w.FontSize != 12 || w.W != 0 || w.H != 0 {
		t.Fatalf("defaults %+v", w)
	}
}

func TestOutputModelFile(t *testing.T) {
	m := loadModel(t, "output.nlogo")
	w := outputWidget(t, m)
	if w.X != 16 || w.Y != 110 || w.W != 234 || w.H != 120 || w.FontSize != 13 {
		t.Fatalf("file widget %+v", w)
	}
	vm, err := m.NewVM(1)
	if err != nil {
		t.Fatal(err)
	}
	mustCall(t, vm, "setup")
	lines, partial := vm.OutputArea()
	want := []string{"Output area ready.", `next write is quoted: "widget"`}
	if partial != "" || strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("area %q partial %q", lines, partial)
	}
	if got := vm.Output(); len(got) != 0 {
		t.Fatalf("setup wrote to the command center: %q", got)
	}
	mustCall(t, vm, "go")
	lines, _ = vm.OutputArea()
	if len(lines) != 3 || lines[2] != "0" {
		t.Fatalf("after one go %q", lines)
	}
}

func TestOutputPrimitives(t *testing.T) {
	vm := newTestVM(t, "", -1, 1, true, 1)
	vm.SetHasOutput(true)
	run := func(line string) {
		t.Helper()
		if _, err := vm.ExecuteLine(line); err != nil {
			t.Fatalf("%s: %v", line, err)
		}
	}
	run(`output-print "hi"`)
	run(`output-type "ab"`)
	run(`output-write "c"`)
	run(`output-print "!"`)
	run(`output-show 7`)
	lines, partial := vm.OutputArea()
	want := []string{"hi", `ab"c"!`, "observer: 7"}
	if partial != "" || strings.Join(lines, "|") != strings.Join(want, "|") {
		t.Fatalf("lines %q partial %q", lines, partial)
	}
	if got := vm.Output(); len(got) != 0 {
		t.Fatalf("command center %q", got)
	}

	// print stays on the command center and does not flush the output tail.
	run(`output-type "tail"`)
	run(`print "cmd"`)
	lines, partial = vm.OutputArea()
	if partial != "tail" || len(lines) != 3 {
		t.Fatalf("after print area %q %q", lines, partial)
	}
	if got := vm.Output(); len(got) != 1 || got[0] != "cmd" {
		t.Fatalf("command center %q", got)
	}
	run(`output-print ""`)

	if _, err := vm.ExecuteLine(`crt 1`); err != nil {
		t.Fatal(err)
	}
	if _, err := vm.ExecuteLine(`ask turtles [ output-show "z" ]`); err != nil {
		t.Fatal(err)
	}
	lines, _ = vm.OutputArea()
	if lines[len(lines)-1] != "turtle 0: z" {
		t.Fatalf("turtle show %q", lines)
	}

	cleared := 0
	vm.OnClear = func() { cleared++ }
	run(`clear-output`)
	lines, partial = vm.OutputArea()
	if len(lines) != 0 || partial != "" || cleared != 0 {
		t.Fatalf("clear-output area %q %q cleared %d", lines, partial, cleared)
	}
	if got := vm.Output(); len(got) != 1 || got[0] != "cmd" {
		t.Fatalf("clear-output touched the command center: %q", got)
	}

	run(`output-print "again"`)
	run(`clear-all`)
	lines, partial = vm.OutputArea()
	if len(lines) != 0 || partial != "" {
		t.Fatalf("clear-all left %q %q", lines, partial)
	}
	if got := vm.Output(); len(got) != 1 || got[0] != "cmd" {
		t.Fatalf("clear-all cleared the command center: %q", got)
	}
}

func TestOutputFallsBackWithoutWidget(t *testing.T) {
	vm := newTestVM(t, "", -1, 1, true, 1)
	var cleared int
	vm.OnClear = func() { cleared++ }
	if _, err := vm.ExecuteLine(`output-type "a"`); err != nil {
		t.Fatal(err)
	}
	if _, err := vm.ExecuteLine(`output-write "b"`); err != nil {
		t.Fatal(err)
	}
	if _, err := vm.ExecuteLine(`output-print "c"`); err != nil {
		t.Fatal(err)
	}
	if _, err := vm.ExecuteLine(`output-show 1`); err != nil {
		t.Fatal(err)
	}
	lines, partial := vm.OutputArea()
	if len(lines) != 0 || partial != "" {
		t.Fatalf("output area used without a widget: %q %q", lines, partial)
	}
	got := vm.Output()
	want := []string{`a"b"c`, "observer: 1"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("command center %q", got)
	}
	if _, err := vm.ExecuteLine(`clear-output`); err != nil {
		t.Fatal(err)
	}
	if cleared != 1 || len(vm.Output()) != 0 {
		t.Fatalf("clear-output cleared=%d lines %q", cleared, vm.Output())
	}
	if _, err := vm.ExecuteLine(`print "z"`); err != nil {
		t.Fatal(err)
	}
	if _, err := vm.ExecuteLine(`clear-all`); err != nil {
		t.Fatal(err)
	}
	if cleared != 2 || len(vm.Output()) != 0 {
		t.Fatalf("clear-all cleared=%d lines %q", cleared, vm.Output())
	}
}

func TestOutputDeltaAndCap(t *testing.T) {
	vm := newTestVM(t, "", -1, 1, true, 1)
	vm.SetHasOutput(true)
	reset, epoch, seq, lines, partial := vm.OutputDelta(-1, 0)
	if !reset || epoch != 0 || seq != 0 || len(lines) != 0 || partial != "" {
		t.Fatalf("empty delta reset %v epoch %d seq %d lines %q", reset, epoch, seq, lines)
	}
	for i := 0; i <= OutputLineCap; i++ {
		if _, err := vm.ExecuteLine(fmt.Sprintf("output-print %d", i)); err != nil {
			t.Fatal(err)
		}
	}
	got, partial := vm.OutputArea()
	if partial != "" || len(got) != OutputLineCap || got[0] != "1" || got[len(got)-1] != fmt.Sprint(OutputLineCap) {
		t.Fatalf("cap len %d first %q last %q", len(got), firstOrEmpty(got), lastOrEmpty(got))
	}
	reset, epoch, seq, lines, _ = vm.OutputDelta(-1, 0)
	if !reset || seq != OutputLineCap+1 || len(lines) != OutputLineCap {
		t.Fatalf("full reset seq %d len %d reset %v", seq, len(lines), reset)
	}
	reset, _, seq2, lines, _ := vm.OutputDelta(epoch, seq)
	if reset || seq2 != seq || len(lines) != 0 {
		t.Fatalf("caught up reset %v seq %d lines %d", reset, seq2, len(lines))
	}
	if _, err := vm.ExecuteLine(`output-print "more"`); err != nil {
		t.Fatal(err)
	}
	reset, epoch2, seq3, lines, _ := vm.OutputDelta(epoch, seq)
	if reset || epoch2 != epoch || seq3 != seq+1 || len(lines) != 1 || lines[0] != "more" {
		t.Fatalf("append reset %v epoch %d seq %d lines %q", reset, epoch2, seq3, lines)
	}
	// A cursor behind the retained window receives the whole buffer.
	reset, _, _, lines, _ = vm.OutputDelta(epoch, 0)
	if !reset || len(lines) != OutputLineCap || lines[len(lines)-1] != "more" {
		t.Fatalf("gap reset %v len %d last %q", reset, len(lines), lastOrEmpty(lines))
	}
	if _, err := vm.ExecuteLine(`clear-output`); err != nil {
		t.Fatal(err)
	}
	reset, epoch3, seq4, lines, partial := vm.OutputDelta(epoch2, seq3)
	if !reset || epoch3 != epoch+1 || seq4 != 0 || len(lines) != 0 || partial != "" {
		t.Fatalf("clear delta reset %v epoch %d seq %d lines %q partial %q", reset, epoch3, seq4, lines, partial)
	}
}

func outputWidget(t *testing.T, m *FileModel) Widget {
	t.Helper()
	for _, w := range m.Widgets {
		if w.Kind == "output" {
			return w
		}
	}
	t.Fatal("no output widget")
	return Widget{}
}

func firstOrEmpty(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return lines[0]
}

func lastOrEmpty(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return lines[len(lines)-1]
}
