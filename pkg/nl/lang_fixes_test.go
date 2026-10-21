package nl

import (
	"bytes"
	"testing"
)

// Procedures below are the shapes that failed on Robby the Robot:
// concise task names, parenthesized multi-branch ifelse-value, and
// numeric labels.
const langFixSrc = `
globals [ chromosome acc hits x ]
patches-own [ cans ]

to-report action-symbol [ gene ]
  report (ifelse-value
    gene = 0 [ "m" ]
    gene = 1 [ "l" ]
    gene = 2 [ "r" ]
    [ "?" ])
end

to-report twice [ n ]
  report n * 2
end

to-report answer
  report 42
end

to shout [ n ]
  print n
end

to quiet
  print 0
end

to add [ a b ]
  set acc acc + a + b
end

to touch
  set hits hits + 1
end

to-report cans-on? [ p ]
  report ifelse-value is-patch? p [ [cans] of p > 0 ] [ false ]
end

to-report sensor
  report (word
    (ifelse-value is-patch? patch-ahead 1
      [ ifelse-value cans-on? patch-ahead 1 [ "c" ] [ "e" ] ]
      [ "w" ])
    (ifelse-value is-patch? patch-right-and-ahead 90 1
      [ ifelse-value cans-on? patch-right-and-ahead 90 1 [ "c" ] [ "e" ] ]
      [ "w" ]))
end

to-report probe-abs
  report map abs [ -1 2 -3 ]
end

to-report probe-round
  report map round [ 1.2 2.2 2.7 ]
end

to-report probe-plus
  report (map + [ 1 2 3 ] [ 4 5 6 ])
end

to-report probe-reduce
  report reduce + [ 1 2 3 4 ]
end

to-report probe-filter
  report filter is-number? [ 1 "x" 3 true ]
end

to-report probe-sort
  report sort-by > [ 1 3 2 ]
end

to-report probe-n
  report n-values 4 abs
end

to-report probe-symbol
  report map action-symbol chromosome
end

to-report probe-var
  let t [ n -> n + 1 ]
  report map t [ 1 2 ]
end

to-report probe-if
  report (ifelse-value true [ 1 ] [ 2 ])
end

to-report probe-if-plain
  report ifelse-value true [ 1 ] [ 2 ]
end

to-report probe-nested
  report (ifelse-value true [ ifelse-value false [ 1 ] [ 2 ] ] [ 3 ])
end

to-report probe-multi
  report (ifelse-value false [ 1 / 0 ] true [ 3 ] [ 1 / 0 ])
end

to-report probe-runresult
  report (word runresult answer (runresult twice 4) (runresult abs -5) runresult "2 + 2")
end

to load-chrom
  set chromosome [ 0 2 1 9 ]
end

to show-each
  foreach [ 1 2 3 ] print
end

to sum-pairs
  set acc 0
  (foreach [ 1 2 ] [ 10 20 ] add)
end

to do-run
  (run shout 5)
  run quiet
end

to branch
  set hits 0
  set x 0
  (ifelse
    false [ touch set x 1 ]
    true [ set x 2 ]
    [ touch set x 3 ])
end

to label-num
  clear-all
  crt 1
  ask turtles [
    set label 0
    set label label - 5
  ]
end

to place
  clear-all
  ask patches [ set cans 0 ]
  ask patch 0 1 [ set cans 3 ]
  crt 1
  ask turtles [ setxy 0 0 set heading 0 ]
end

to-report one-label
  report [label] of turtle 0
end

to-report label-sum
  report sum [label] of turtles
end

to-report label-num?
  report is-number? [label] of turtle 0
end

to-report label-str?
  report is-string? [label] of turtle 0
end
`

func forceTreeWalker(prog *Program) {
	if prog == nil {
		return
	}
	prog.compiled = true
	prog.noClosure = true
	for _, p := range prog.Procedures {
		p.cmdFn = nil
	}
}

func TestConciseTasksAndIfelseValue(t *testing.T) {
	for _, walk := range []bool{false, true} {
		mode := "closure compiler"
		if walk {
			mode = "tree walker"
		}
		vm := newTestVM(t, langFixSrc, -2, 2, false, 1)
		if walk {
			forceTreeWalker(vm.Prog)
		}
		check := func(expr, want string) {
			t.Helper()
			if got := mustEval(t, vm, expr); got != want {
				t.Fatalf("%s: %s = %s, want %s", mode, expr, got, want)
			}
		}
		check("probe-abs", "[1 2 3]")
		check("probe-round", "[1 2 3]")
		check("probe-plus", "[5 7 9]")
		check("probe-reduce", "10")
		check("probe-filter", "[1 3]")
		check("probe-sort", "[3 2 1]")
		check("probe-n", "[0 1 2 3]")
		check("probe-var", "[2 3]")
		check("probe-if", "1")
		check("probe-if-plain", "1")
		check("probe-nested", "2")
		check("probe-multi", "3")
		check("probe-runresult", "42854")
		if !walk && vm.Prog.Proc("probe-abs").cmdFn == nil {
			t.Fatal("closure compiler did not compile probe-abs")
		}
		if walk && vm.Prog.Proc("probe-abs").cmdFn != nil {
			t.Fatal("tree walker test compiled probe-abs")
		}

		mustCall(t, vm, "load-chrom")
		check("probe-symbol", `["m" "r" "l" "?"]`)
		// The command center parses the same concise call directly.
		check("map action-symbol chromosome", `["m" "r" "l" "?"]`)
		check("map abs [ -1 2 -3 ]", "[1 2 3]")
		check("(ifelse-value true [ 1 ] [ 2 ])", "1")
		check("(ifelse-value true [ ifelse-value false [ 1 ] [ 2 ] ] [ 3 ])", "2")

		mustCall(t, vm, "show-each")
		out := vm.Output()
		if len(out) != 3 || out[0] != "1" || out[1] != "2" || out[2] != "3" {
			t.Fatalf("%s foreach print: %q", mode, out)
		}
		mustCall(t, vm, "sum-pairs")
		check("acc", "33")
		mustCall(t, vm, "do-run")
		out = vm.Output()
		if len(out) != 5 || out[3] != "5" || out[4] != "0" {
			t.Fatalf("%s run: %q", mode, out)
		}
		mustCall(t, vm, "branch")
		check("x", "2")
		check("hits", "0")
	}
}

func TestNumericLabels(t *testing.T) {
	for _, walk := range []bool{false, true} {
		mode := "closure compiler"
		if walk {
			mode = "tree walker"
		}
		vm := newTestVM(t, langFixSrc, -2, 2, false, 1)
		if walk {
			forceTreeWalker(vm.Prog)
		}
		mustCall(t, vm, "label-num")
		if got := mustEval(t, vm, "label-sum"); got != "-5" {
			t.Fatalf("%s sum: %s", mode, got)
		}
		if got := mustEval(t, vm, "label-num?"); got != "true" {
			t.Fatalf("%s is-number?: %s", mode, got)
		}
		if got := mustEval(t, vm, "one-label"); got != "-5" {
			t.Fatalf("%s label: %s", mode, got)
		}
		live := vm.World.Turtles().Alive()
		if len(live) != 1 {
			t.Fatalf("%s turtles: %d", mode, len(live))
		}
		tu := live[0].(*Turtle)
		if tu.Label.K != KindNum || tu.Label.N != -5 {
			t.Fatalf("%s stored label %#v", mode, tu.Label)
		}
		if LabelString(tu.Label) != "-5" {
			t.Fatalf("%s render text %q", mode, LabelString(tu.Label))
		}

		mustEval(t, vm, "ask turtles [ set label \"hi\" ]")
		if got := mustEval(t, vm, "one-label"); got != "hi" {
			t.Fatalf("%s string label: %s", mode, got)
		}
		if got := mustEval(t, vm, "label-str?"); got != "true" {
			t.Fatalf("%s is-string?: %s", mode, got)
		}

		mustEval(t, vm, "ask patch 0 0 [ set plabel 10 set plabel plabel + 1 ]")
		if got := mustEval(t, vm, "[plabel] of patch 0 0"); got != "11" {
			t.Fatalf("%s plabel: %s", mode, got)
		}
		if got := mustEval(t, vm, "is-number? [plabel] of patch 0 0"); got != "true" {
			t.Fatalf("%s plabel type: %s", mode, got)
		}
		p := vm.World.GetPatch(0, 0)
		if p.Label.K != KindNum || p.Label.N != 11 || LabelString(p.Label) != "11" {
			t.Fatalf("%s stored plabel %#v", mode, p.Label)
		}

		mustEval(t, vm, "crt 1")
		mustEval(t, vm, "ask turtle 0 [ create-link-with turtle 1 ]")
		mustEval(t, vm, "ask link 0 1 [ set label 4 set label label + 1 ]")
		if got := mustEval(t, vm, "[label] of link 0 1"); got != "5" {
			t.Fatalf("%s link label: %s", mode, got)
		}
	}
}

func TestSensorIfelseValue(t *testing.T) {
	for _, walk := range []bool{false, true} {
		vm := newTestVM(t, langFixSrc, -2, 2, false, 1)
		if walk {
			forceTreeWalker(vm.Prog)
		}
		mustCall(t, vm, "place")
		if got := mustEval(t, vm, "[sensor] of turtle 0"); got != "ce" {
			t.Fatalf("walk=%v sensor on a can: %s", walk, got)
		}
		mustEval(t, vm, "ask turtles [ setxy 2 0 set heading 90 ]")
		if got := mustEval(t, vm, "[sensor] of turtle 0"); got != "we" {
			t.Fatalf("walk=%v sensor off the edge: %s", walk, got)
		}
	}
}

func TestLabelRenderedAsText(t *testing.T) {
	blank := NewWorld(0, 2, 0, 2, false, false)
	blank.CreateTurtle("", 1, 1, 5, 0)
	plain := RenderWorld(blank, 16)

	labeled := NewWorld(0, 2, 0, 2, false, false)
	tu := labeled.CreateTurtle("", 1, 1, 5, 0)
	tu.Label = Num(42)
	if labeled.GetPatch(1, 1) == nil {
		t.Fatal("missing patch")
	}
	labeled.GetPatch(0, 0).Label = Num(7)
	painted := RenderWorld(labeled, 16)
	if bytes.Equal(plain.Pix, painted.Pix) {
		t.Fatal("numeric labels did not change the rendered frame")
	}
	if LabelString(tu.Label) != "42" || tu.Label.K != KindNum {
		t.Fatalf("turtle label stayed %s %v", tu.Label.TypeName(), LabelString(tu.Label))
	}
}
