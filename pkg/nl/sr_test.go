package nl

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSRPrimitives(t *testing.T) {
	if _, err := exec.LookPath("R"); err != nil {
		t.Skip("R is not installed")
	}
	code := `
extensions [ sr ]
globals [ four coef slope xs rhome ]
to test
  sr:setup
  set four sr:runresult "2 + 2"
  sr:set "y" [1 2 3 4]
  sr:run "x <- 1:4"
  sr:run "fm <- lm(y ~ x)"
  set coef sr:runresult "fm$coefficients[1]"
  set slope sr:runresult "fm$coefficients[2]"
  set rhome sr:r-home
  crt 2
  ask turtle 0 [ setxy 1 2 ]
  ask turtle 1 [ setxy 3 4 ]
  (sr:set-agent-data-frame "ag" turtles "who" "xcor" "ycor")
  set xs sr:runresult "ag$xcor"
  sr:run "ct <- cor(ag$xcor, ag$ycor)"
end
`
	prog, err := Parse(code)
	if err != nil {
		t.Fatal(err)
	}
	vm := NewVM(prog, NewWorld(-5, 5, -5, 5, false, false))
	defer vm.Close()
	mustCall(t, vm, "test")
	if got := mustEval(t, vm, "four"); got != "4" {
		t.Fatalf("2+2 = %s", got)
	}
	if got := mustEval(t, vm, "item 1 item 0 coef"); got != "0" {
		t.Fatalf("intercept %s full %s", got, mustEval(t, vm, "coef"))
	}
	if got := mustEval(t, vm, "item 1 item 0 slope"); got != "1" {
		t.Fatalf("slope %s full %s", got, mustEval(t, vm, "slope"))
	}
	if got := mustEval(t, vm, "xs"); got != "[1 3]" {
		t.Fatalf("xcor column %s", got)
	}
	if got := mustEval(t, vm, "sr:runresult \"ct\""); got != "1" {
		t.Fatalf("correlation %s", got)
	}
	if mustEval(t, vm, "rhome") == "" {
		t.Fatal("sr:r-home was empty")
	}
}

func TestSRRequiresSetup(t *testing.T) {
	if _, err := exec.LookPath("R"); err != nil {
		t.Skip("R is not installed")
	}
	prog, err := Parse("extensions [ sr ]\nto test\n  let x sr:runresult \"1\"\nend\n")
	if err != nil {
		t.Fatal(err)
	}
	vm := NewVM(prog, NewWorld(-1, 1, -1, 1, false, false))
	defer vm.Close()
	_, err = vm.Call("test")
	if err == nil || !strings.Contains(err.Error(), "sr:setup") {
		t.Fatalf("err %v", err)
	}
}

func TestSRLibraryModel(t *testing.T) {
	if _, err := exec.LookPath("R"); err != nil {
		t.Skip("R is not installed")
	}
	m := loadModel(t, "library/Code Examples/Extensions Examples/sr/Linear Regression Example.nlogox")
	vm, err := m.NewVM(1)
	if err != nil {
		t.Fatal(err)
	}
	defer vm.Close()
	mustCall(t, vm, "setup")
	stopped, err := vm.Call("go")
	if err != nil {
		t.Fatal(err)
	}
	if stopped {
		t.Fatal("go stopped on the first tick")
	}
}

func TestSRModelsCompile(t *testing.T) {
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
		exts := declaredExts(m.Code)
		if len(exts) == 0 {
			return nil
		}
		for _, e := range exts {
			if e != "sr" && e != "palette" {
				return nil
			}
		}
		hasSR := false
		for _, e := range exts {
			if e == "sr" {
				hasSR = true
			}
		}
		if !hasSR {
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
	if n < 15 {
		t.Fatalf("found %d sr models, want at least 15", n)
	}
}
