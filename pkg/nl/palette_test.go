package nl

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPalettePrimitives(t *testing.T) {
	code := `
extensions [ palette ]
globals [ hue sat bri r g b scheme grad withh alpha trans ]
to test
  set hue palette:hue-of red
  set sat palette:saturation-of red
  set bri palette:brightness-of red
  set r palette:r-of red
  set g palette:g-of red
  set b palette:b-of red
  set scheme palette:scheme-colors "Divergent" "Spectral" 3
  set grad palette:scale-gradient [[255 0 0] [0 0 255]] 0 0 10
  set withh red palette:with-hue 100
  set alpha palette:alpha-of [3 14 159 26]
  set trans palette:transparency-of [100 100 100 100]
  ask patch 0 0 [
    set pcolor palette:scale-scheme "Qualitative" "Set3" 7 0 0 10
    palette:set-hue 120
  ]
end
`
	prog, err := Parse(code)
	if err != nil {
		t.Fatal(err)
	}
	vm := NewVM(prog, NewWorld(-2, 2, -2, 2, false, false))
	mustCall(t, vm, "test")
	if got := mustEval(t, vm, "hue"); got != "3.103" {
		t.Fatalf("hue %s", got)
	}
	if got := mustEval(t, vm, "sat"); got != "80.93" {
		t.Fatalf("saturation %s", got)
	}
	if got := mustEval(t, vm, "bri"); got != "84.314" {
		t.Fatalf("brightness %s", got)
	}
	if got := mustEval(t, vm, "(word r \" \" g \" \" b)"); got != "215 50 41" {
		t.Fatalf("rgb %s", got)
	}
	if got := mustEval(t, vm, "scheme"); got != "[[252 141 89] [255 255 191] [153 213 148]]" {
		t.Fatalf("spectral %s", got)
	}
	if got := mustEval(t, vm, "grad"); got != "[255 0 0]" {
		t.Fatalf("gradient start %s", got)
	}
	if got := mustEval(t, vm, "withh"); got != "[99 215 41]" {
		t.Fatalf("with-hue %s", got)
	}
	if got := mustEval(t, vm, "alpha"); got != "26" {
		t.Fatalf("alpha %s", got)
	}
	wantTrans := (255.0 - 100.0) / 255.0 * 100.0
	if math.Abs(vm.World.globals["trans"].N-wantTrans) > 1e-9 {
		t.Fatalf("transparency %v, want %v", vm.World.globals["trans"].N, wantTrans)
	}
	p := vm.World.GetPatch(0, 0)
	if p == nil || !p.HasRGB {
		t.Fatal("patch color was not stored as RGB")
	}
	h, _, _ := RGBtoHSB(int(p.RGB.R), int(p.RGB.G), int(p.RGB.B))
	if math.Abs(h-120) > 1 {
		t.Fatalf("set-hue left hue %v rgb %v", h, p.RGB)
	}
}

func TestPaletteRequiresExtension(t *testing.T) {
	_, err := Parse("to go\n  show palette:scale-gradient [[0 0 0] [1 1 1]] 0 0 1\nend\n")
	if err == nil {
		t.Fatal("palette primitive parsed without the extension")
	}
}

func TestPaletteLibraryModel(t *testing.T) {
	m := loadModel(t, "library/Code Examples/Extensions Examples/palette/Palette Example.nlogox")
	vm, err := m.NewVM(1)
	if err != nil {
		t.Fatal(err)
	}
	mustCall(t, vm, "setup")
	if n, err := vm.World.Ticks(); err != nil || n < 1 {
		t.Fatalf("ticks after setup: %v %v", n, err)
	}
	colored := 0
	for _, a := range vm.World.Patches().Agents {
		if a.(*Patch).HasRGB {
			colored++
		}
	}
	if colored == 0 {
		t.Fatal("Palette Example setup did not color patches")
	}
	mustCall(t, vm, "go")
}

func TestPaletteModelsCompile(t *testing.T) {
	root := filepath.Join("..", "..", "models", "library")
	n := 0
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".nlogo", ".nlogox", ".nlogo3d", ".nlogox3d":
		default:
			return nil
		}
		m, err := LoadFile(path)
		if err != nil {
			t.Errorf("load %s: %v", path, err)
			return nil
		}
		var only bool
		var saw bool
		for _, e := range declaredExts(m.Code) {
			saw = true
			if e != "palette" {
				only = false
				return nil
			}
			only = true
		}
		if !saw || !only {
			return nil
		}
		n++
		if _, err := m.Compile(); err != nil {
			t.Errorf("compile %s: %v", path, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n < 7 {
		t.Fatalf("found %d palette-only models, want at least 7", n)
	}
}

func declaredExts(code string) []string {
	low := strings.ToLower(code)
	i := strings.Index(low, "extensions")
	if i < 0 {
		return nil
	}
	rest := code[i+len("extensions"):]
	lb := strings.Index(rest, "[")
	rb := strings.Index(rest, "]")
	if lb < 0 || rb < 0 || rb < lb {
		return nil
	}
	return strings.Fields(strings.ToLower(rest[lb+1 : rb]))
}
