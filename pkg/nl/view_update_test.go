package nl

import (
	"os"
	"strings"
	"testing"
)

func TestParseViewUpdateMode(t *testing.T) {
	cases := []struct {
		name string
		path string
		want int
	}{
		{"robby nlogo", "../../models/Robby the Robot.nlogo", UpdateOnTicks},
		{"robby nlogox", "../../models/library/Sample Models/Computer Science/Robby the Robot.nlogox", UpdateOnTicks},
		{"wolf sheep nlogo", "../../models/wolfsheep.nlogo", UpdateOnTicks},
		{"termites nlogox", "../../models/library/Sample Models/Biology/Termites.nlogox", UpdateContinuous},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			raw, err := os.ReadFile(c.path)
			if err != nil {
				t.Fatal(err)
			}
			m, err := ParseModel(c.path, string(raw))
			if err != nil {
				t.Fatal(err)
			}
			if m.View.UpdateMode != c.want {
				t.Fatalf("update mode %d, want %d", m.View.UpdateMode, c.want)
			}
			vm, err := m.NewVM(1)
			if err != nil {
				t.Fatal(err)
			}
			if vm.UpdatesOnTicks != (c.want == UpdateOnTicks) {
				t.Fatalf("UpdatesOnTicks %v, want mode %d", vm.UpdatesOnTicks, c.want)
			}
		})
	}
}

func TestRewriteViewUpdateMode(t *testing.T) {
	classic := strings.Join([]string{
		"to go",
		"  tick",
		"end",
		sectionSep,
		"GRAPHICS-WINDOW",
		"0", "0", "0", "0", "-1", "-1", "12.0", "1", "10", "1", "1", "1", "0", "0", "0", "1",
		"-1", "1", "-1", "1",
		"1",
		"1", "1", "ticks", "30.0",
		sectionSep,
		sectionSep,
		sectionSep,
		"NetLogo 6.4.0",
		sectionSep,
	}, "\n")
	on, err := ParseModel("ticks.nlogo", classic)
	if err != nil {
		t.Fatal(err)
	}
	if on.View.UpdateMode != UpdateOnTicks {
		t.Fatalf("parsed %d", on.View.UpdateMode)
	}
	off, err := RewriteUpdateMode(classic, UpdateContinuous)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(off, "\n1\n1\n1\nticks") {
		t.Fatalf("update line stayed on:\n%s", off)
	}
	parsed, err := ParseModel("ticks.nlogo", off)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.View.UpdateMode != UpdateContinuous {
		t.Fatalf("rewritten mode %d", parsed.View.UpdateMode)
	}
	back, err := RewriteUpdateMode(off, UpdateOnTicks)
	if err != nil {
		t.Fatal(err)
	}
	again, err := ParseModel("ticks.nlogo", back)
	if err != nil {
		t.Fatal(err)
	}
	if again.View.UpdateMode != UpdateOnTicks {
		t.Fatalf("restored mode %d", again.View.UpdateMode)
	}

	xml := `<model version="NetLogo 6.4.0"><code><![CDATA[to go
  tick
end
]]></code><widgets><view minPxcor="-1" maxPxcor="1" minPycor="-1" maxPycor="1" patchSize="12" updateMode="0"/></widgets></model>`
	next, err := RewriteUpdateMode(xml, UpdateOnTicks)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(next, `updateMode="1"`) {
		t.Fatalf("xml rewrite: %s", next)
	}
	xm, err := ParseModel("m.nlogox", next)
	if err != nil {
		t.Fatal(err)
	}
	if xm.View.UpdateMode != UpdateOnTicks {
		t.Fatalf("xml mode %d", xm.View.UpdateMode)
	}
	missing := `<model><code><![CDATA[to go
end
]]></code><widgets><view minPxcor="0" maxPxcor="1" minPycor="0" maxPycor="1" patchSize="12"/></widgets></model>`
	added, err := RewriteUpdateMode(missing, UpdateOnTicks)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(added, `updateMode="1"`) {
		t.Fatalf("attribute was not added: %s", added)
	}

	classic3d := strings.Replace(classic, "NetLogo 6.4.0", "NetLogo 3D 6.4.0", 1)
	if _, err := RewriteUpdateMode(classic3d, UpdateContinuous); err != ErrUpdateModeNotStored {
		t.Fatalf("3D classic rewrite err %v", err)
	}
}

func TestParseUpdateModeNames(t *testing.T) {
	n, err := ParseUpdateMode("on ticks")
	if err != nil || n != UpdateOnTicks {
		t.Fatalf("on ticks -> %d %v", n, err)
	}
	n, err = ParseUpdateMode("continuous")
	if err != nil || n != UpdateContinuous {
		t.Fatalf("continuous -> %d %v", n, err)
	}
	if _, err := ParseUpdateMode("sometimes"); err == nil {
		t.Fatal("accepted an unknown mode")
	}
}
