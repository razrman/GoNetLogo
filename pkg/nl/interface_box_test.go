package nl

import (
	"os"
	"strings"
	"testing"
)

func TestInterfaceBoxes(t *testing.T) {
	m := loadModel(t, "fire.nlogo")
	if m.View.X != 280 || m.View.Y != 10 || m.View.W != 312 || m.View.H != 312 {
		t.Fatalf("view box %+v", m.View)
	}
	setup := widgetBy(m, "button", "setup")
	if setup.X != 16 || setup.Y != 20 || setup.W != 94 || setup.H != 33 {
		t.Fatalf("setup box %+v", setup)
	}
	density := widgetBy(m, "slider", "density")
	if density.X != 16 || density.Y != 70 || density.W != 234 || density.H != 33 {
		t.Fatalf("slider box %+v", density)
	}

	lib := loadModel(t, "library/Sample Models/Earth Science/Fire.nlogox")
	if lib.View.X != 270 || lib.View.Y != 15 || lib.View.W != 506 || lib.View.H != 506 {
		t.Fatalf("library view %+v", lib.View)
	}
	if got := widgetBy(lib, "button", "setup"); got.X != 50 || got.Y != 75 || got.W != 80 || got.H != 45 {
		t.Fatalf("library setup %+v", got)
	}

	xml := `<?xml version="1.0"?>
<model version="NetLogo 6.4.0">
  <code><![CDATA[to setup
  clear-all
end
]]></code>
  <widgets>
    <view x="270" y="15" width="506" height="400" minPxcor="-2" maxPxcor="2" minPycor="-2" maxPycor="2" patchSize="12"/>
    <button x="14" y="10" width="80" height="48" display="setup">setup</button>
    <slider x="16" y="70" width="180" height="50" variable="density" min="0" max="100" default="40" step="1"/>
  </widgets>
</model>`
	xm, err := ParseModel("t.nlogox", xml)
	if err != nil {
		t.Fatal(err)
	}
	if xm.View.X != 270 || xm.View.Y != 15 || xm.View.W != 506 || xm.View.H != 400 {
		t.Fatalf("xml view %+v", xm.View)
	}
	button := widgetBy(xm, "button", "setup")
	if button.X != 14 || button.Y != 10 || button.W != 80 || button.H != 48 {
		t.Fatalf("xml button %+v", button)
	}
	slider := widgetBy(xm, "slider", "density")
	if slider.X != 16 || slider.Y != 70 || slider.W != 180 || slider.H != 50 {
		t.Fatalf("xml slider %+v", slider)
	}
}

func TestMissingInterfaceBoxStaysZero(t *testing.T) {
	src := strings.Join([]string{
		"to setup\nend\n",
		"\nGRAPHICS-WINDOW\n0\n0\n0\n0\n-1\n-1\n12\n0\n0\n0\n0\n0\n0\n1\n1\n0\n-1\n1\n-1\n1\n",
		"\n",
		"\n",
		"\nNetLogo 6.4.0\n",
		"\n",
	}, sectionSep)
	m, err := ParseModel("empty.nlogo", src)
	if err != nil {
		t.Fatal(err)
	}
	if m.View.W != 0 || m.View.H != 0 {
		t.Fatalf("expected no view box, got %+v", m.View)
	}
}

func TestFlocking3DViewBox(t *testing.T) {
	m := loadModel(t, "library/3D/Sample Models/Flocking 3D Alternate.nlogox3d")
	if !m.View.Is3D || m.View.X != 0 || m.View.Y != 0 || m.View.W != 256 || m.View.H != 276 {
		t.Fatalf("flocking view %+v", m.View)
	}
	if m.View.MinPxcor != -12 || m.View.MaxPxcor != 12 || m.View.MinPzcor != -12 || m.View.MaxPzcor != 12 {
		t.Fatalf("flocking world %+v", m.View)
	}
}

func TestRobbyNotesKeepTheirFont(t *testing.T) {
	m := loadModel(t, "library/Sample Models/Computer Science/Robby the Robot.nlogox")
	note := widgetBy(m, "note", "More similar\nto best strategy")
	if note.FontSize != 11 || note.W != 150 || note.H != 56 {
		t.Fatalf("note %+v", note)
	}
	classic, err := ParseModel("robby.nlogo", mustRead(t, "../../models/Robby the Robot.nlogo"))
	if err != nil {
		t.Fatal(err)
	}
	legend := widgetBy(classic, "note", "\"↑\" =   move-north  \n \"→\"  = move-east \n \"↓\"  = move-south  \n \"←\"  = move-west\n \"+\"   = move-random\n \"x\"    =  stay-put   \n\"●\"    =  pick-up-can")
	if legend.FontSize != 13 || legend.W != 158 || legend.H != 143 {
		t.Fatalf("classic note %+v", legend)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func widgetBy(m *FileModel, kind, display string) Widget {
	for _, w := range m.Widgets {
		if w.Kind == kind && w.Display == display {
			return w
		}
	}
	return Widget{}
}
