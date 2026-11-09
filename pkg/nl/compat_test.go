package nl

import (
	"testing"
)

func TestCompatBatchCompileAndRuntime(t *testing.T) {
	tests := []struct {
		name string
		code string
		eval string
		want string
	}{
		{
			name: "perspective commands and subject",
			code: `
to test
  crt 2
  ask turtle 0 [ watch-me ]
end
`,
			eval: `[who] of subject`,
			want: "0",
		},
		{
			name: "reset-perspective",
			code: `
to test
  crt 1 [ watch-me ]
  reset-perspective
end
`,
			eval: `subject = nobody`,
			want: "true",
		},
		{
			name: "follow and ride",
			code: `
to test
  crt 2
  follow turtle 1
end
`,
			eval: `[who] of subject`,
			want: "1",
		},
		{
			name: "die clears subject",
			code: `
to test
  crt 1 [ watch-me die ]
end
`,
			eval: `subject = nobody`,
			want: "true",
		},
		{
			name: "spelling aliases",
			code: `
globals [ g ]
to test
  set g []
  if-else true [ set g lput 1 g ] [ set g lput 2 g ]
  crt 1 [
    penup
    pendown
  ]
end
`,
			eval: `(word butfirst [1 2 3] butlast [4 5 6] (run-result [ -> 99 ]) g)`,
			want: "[2 3][4 5]99[1]",
		},
		{
			name: "at-points turtle and patch",
			code: `
globals [ res ]
to test
  clear-all
  crt 1 [
    setxy 0 0
    let ps patches at-points [[1 0] [0 1]]
    set res count ps
  ]
end
`,
			eval: `res`,
			want: "2",
		},
		{
			name: "breed singular reporter",
			code: `
breed [ particles particle ]
globals [ pwho ]
to test
  clear-all
  create-particles 2
  set pwho [who] of particle 1
end
`,
			eval: `pwho`,
			want: "1",
		},
		{
			name: "breed singular nobody on mismatch",
			code: `
breed [ particles particle ]
globals [ res ]
to test
  clear-all
  crt 1 ; default turtles breed, who 0
  set res (particle 0 = nobody)
end
`,
			eval: `res`,
			want: "true",
		},
		{
			name: "hsb and rgb color reporters",
			code: `
globals [ ok-r ok-h ok-ah ok-ar ok-eh ok-er ]
to test
  let r rgb 255 0 0
  let h hsb 0 100 100
  let ah approximate-hsb 0 0 0
  let ar approximate-rgb 0 0 0
  let eh extract-hsb red
  let er extract-rgb red
  set ok-r (r = [255 0 0])
  set ok-h (h = [255 0 0])
  set ok-ah (ah = 0)
  set ok-ar (ar = 0)
  set ok-eh (length eh = 3)
  set ok-er (length er = 3)
end
`,
			eval: `(word ok-r ok-h ok-ah ok-ar ok-eh ok-er)`,
			want: "truetruetruetruetruetrue",
		},
		{
			name: "approximate-rgb red-channel mean",
			code: `
to test
end
`,
			// rmean is (r1 + r2) / 2. NetLogo writes r1 + r2 / 2, which
			// divides r2 first and reports 32.9 for this color.
			eval: `approximate-rgb 83 72 54`,
			want: "33",
		},
		{
			name: "user dialog stubs in headless",
			code: `
globals [ ui uyn uo ]
to test
  set ui user-input "name?"
  set uyn user-yes-or-no? "ready?"
  set uo user-one-of "choice" ["a" "b"]
  user-message "hi"
end
`,
			eval: `(word ui uyn uo)`,
			want: "falsea",
		},
		{
			name: "link-neighbor? undirected and directed",
			code: `
globals [ n1 n2 neg-out neg-in neg-breed neg-dead ]
directed-link-breed [ arrows arrow ]
undirected-link-breed [ roads road ]
to test
  clear-all
  crt 4
  ask turtle 0 [
    create-link-with turtle 1
    create-arrow-to turtle 2
    set n1 link-neighbor? turtle 1
    set n2 arrow-neighbor? turtle 2
  ]
  set neg-out [out-link-neighbor? turtle 0] of turtle 2 ; 0 -> 2, so 2's out-link-neighbor? 0 should be false
  set neg-in [in-link-neighbor? turtle 2] of turtle 0   ; 0 -> 2, so 0's in-link-neighbor? 2 should be false
  ask turtle 3 [
    create-road-with turtle 0
  ]
  set neg-breed [arrow-neighbor? turtle 3] of turtle 0  ; turtle 0 and 3 only share a road, not an arrow
  ask links [ die ]
  set neg-dead [link-neighbor? turtle 1] of turtle 0    ; false after links die
end
`,
			eval: `(word n1 n2 " " neg-out neg-in neg-breed neg-dead)`,
			want: "truetrue falsefalsefalsefalse",
		},
		{
			name: "remove-duplicates",
			code: `
to test
end
`,
			eval: `remove-duplicates [2 7 4 7 2 5]`,
			want: "[2 7 4 5]",
		},
		{
			name: "parenthesized ifelse with no else",
			code: `
globals [ x ]
to test
  set x 0
  let c 2
  (ifelse
    c = 1 [ set x 10 ]
    c = 2 [ set x 20 ]
    c = 3 [ set x 30 ])
  let d 99
  (ifelse
    d = 1 [ set x 1 ]
    d = 2 [ set x 2 ])
end
`,
			eval: `x`,
			want: "20",
		},
		{
			name: "single argument turtle-set patch-set link-set",
			code: `
globals [ c1 c2 c3 ]
to test
  clear-all
  crt 2
  ask turtle 0 [ create-link-with turtle 1 ]
  set c1 count turtle-set turtle 0
  set c2 count patch-set patch 0 0
  set c3 count link-set one-of links
end
`,
			eval: `(word c1 c2 c3)`,
			want: "111",
		},
		{
			name: "subtract-headings",
			code: `
to test
end
`,
			eval: `(word (subtract-headings 80 60) (subtract-headings 60 80) (subtract-headings 5 355) (subtract-headings 0 180))`,
			want: "20-2010180",
		},
		{
			name: "shuffle list",
			code: `
globals [ len same ]
to test
  let orig [1 2 3 4 5 6 7 8 9 10]
  let shuf shuffle orig
  set len length shuf
  set same (orig = shuf)
end
`,
			eval: `(word len same)`,
			want: "10false",
		},
		{
			name: "min-n-of and max-n-of",
			code: `
globals [ mn mx ]
to test
  clear-all
  crt 10 [ set size who ]
  set mn sort [who] of min-n-of 3 turtles [size]
  set mx sort [who] of max-n-of 3 turtles [size]
end
`,
			eval: `(word mn mx)`,
			want: "[0 1 2][7 8 9]",
		},
		{
			name: "dx and dy reporters",
			code: `
globals [ ok-dx ok-dy ]
to test
  clear-all
  crt 1 [
    set heading 90
    set ok-dx (precision dx 4 = 1 and precision dy 4 = 0)
    set heading 0
    set ok-dy (precision dx 4 = 0 and precision dy 4 = 1)
  ]
end
`,
			eval: `(word ok-dx ok-dy)`,
			want: "truetrue",
		},
		{
			name: "mouse reporters in headless",
			code: `
to test
end
`,
			eval: `(word mouse-down? mouse-inside? mouse-xcor mouse-ycor)`,
			want: "falsefalse00",
		},
		{
			name: "who in closure compiler",
			code: `
globals [ sum-who ]
to test
  clear-all
  set sum-who 0
  crt 5 [
    if who >= 0 [
      set sum-who sum-who + who
    ]
  ]
end
`,
			eval: `sum-who`,
			want: "10",
		},
		{
			name: "pi reporter",
			code: `
to test
end
`,
			eval: `precision pi 4`,
			want: "3.1416",
		},
		{
			name: "base-colors reporter",
			code: `
to test
end
`,
			eval: `base-colors`,
			want: "[5 15 25 35 45 55 65 75 85 95 105 115 125 135]",
		},
		{
			name: "carefully error and error-message",
			code: `
globals [ caught ]
to test
  carefully [
    error "custom error occurred"
  ] [
    set caught error-message
  ]
end
`,
			eval: `caught`,
			want: "custom error occurred",
		},
		{
			name: "word variadic and concise task",
			code: `
to test
end
`,
			eval: `(word (word "a" "b") "-" (word) "-" (word "one") "-" (word 1 2 3 4) "-" (reduce word ["x" "y" "z"]))`,
			want: "ab--one-1234-xyz",
		},
		{
			name: "hide-link and show-link",
			code: `
globals [ h1 h2 ]
to test
  crt 2
  ask turtle 0 [ create-link-to turtle 1 ]
  ask link 0 1 [
    hide-link
    set h1 [hidden?] of self
    show-link
    set h2 [hidden?] of self
  ]
end
`,
			eval: `(word h1 "-" h2)`,
			want: "true-false",
		},
		{
			name: "hubnet and inspect stubs",
			code: `
to test
  hubnet-reset
  hubnet-fetch-message
  hubnet-broadcast "tag" 10
  hubnet-send "client" "tag" 20
  inspect turtle 0
  stop-inspecting turtle 0
  stop-inspecting-dead-agents
  export-interface "test.png"
  __set-line-thickness 2
end
`,
			eval: `(word hubnet-message-waiting? "-" hubnet-message "-" hubnet-message-source "-" (length hubnet-clients-list))`,
			want: "false---0",
		},
		{
			name: "statistical distribution and list reporters",
			code: `
to test
end
`,
			eval: `(word (median [3 1 2]) "-" (median [1 2 3 4]) "-" (modes [2 1 2 3 3]) "-" (is-number? (random-normal 10 2)) "-" (is-number? (random-exponential 5)))`,
			want: "2-2.5-[2 3]-true-true",
		},
		{
			name: "in-cone wrapping and self-inclusion",
			code: `
globals [ res ]
to test
  clear-all
  crt 1 [
    setxy max-pxcor 0
    set heading 90
  ]
  crt 1 [
    setxy min-pxcor 0
  ]
  ;; In a wrapping world, caller at max-pxcor facing 90 sees min-pxcor across the seam.
  ;; in-cone includes both the other turtle across the seam and the caller itself.
  ask turtle 0 [
    set res count (turtles in-cone 3 90)
  ]
end
`,
			eval: `res`,
			want: "2",
		},
		{
			name: "link-breed reporters on directed and undirected breeds",
			code: `
undirected-link-breed [ edges edge ]
directed-link-breed [ arrows arrow ]
globals [ res ]
to test
  clear-all
  crt 3
  ask turtle 0 [
    create-edge-with turtle 1
    create-arrow-to turtle 2
  ]
  set res (word
    (edge 0 1 != nobody) "-" (edge 1 0 != nobody) "-"
    (arrow 0 2 != nobody) "-" (arrow 2 0 = nobody) "-"
    ([count my-edges] of turtle 0) "-"
    ([count my-out-arrows] of turtle 0) "-"
    ([count my-in-arrows] of turtle 2))
end
`,
			eval: `res`,
			want: "true-true-true-true-1-1-1",
		},
		{
			name: "carefully nested and around ask",
			code: `
globals [ out ]
to test
  set out []
  carefully [
    ask patches [
      error "patch error"
    ]
  ] [
    set out lput error-message out
  ]
  carefully [
    carefully [
      error "inner"
    ] [
      set out lput error-message out
      error "outer"
    ]
  ] [
    set out lput error-message out
  ]
end
`,
			eval: `out`,
			want: "[\"patch error\" \"inner\" \"outer\"]",
		},
		{
			name: "median and modes with mixed types",
			code: `
to test
end
`,
			eval: `(word (median ["skip" 10 "me" 20 30]) "-" (modes ["a" 1 "a" 2 2]))`,
			want: "20-[\"a\" 2]",
		},
		{
			name: "run-result with string and anonymous reporter",
			code: `
to test
end
`,
			eval: `(word (run-result "5 + 7") "-" (run-result [ -> 42 ]) "-" (run-result [ x -> x * 2 ] 11))`,
			want: "12-42-22",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := newTestVM(t, tt.code, -5, 5, true, 1)
			p := vm.Prog.Proc("test")
			if p != nil {
				mustCall(t, vm, "test")
			}
			got := mustEval(t, vm, tt.eval)
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPlotPenReset(t *testing.T) {
	code := `
to setup
  set-current-plot "test"
  set-current-plot-pen "p1"
  plot 10
  plot 20
  plot-pen-reset
end
`
	vm := plotProgram(t, code, onePlot("test", true, 0, 10, 0, 10, PlotPenSpec{Name: "p1", Interval: 2, Mode: 0}))
	mustCall(t, vm, "setup")
	_, pen, err := vm.plotTarget()
	if err != nil {
		t.Fatalf("plot target error: %v", err)
	}
	if pen.x != 0 || !pen.down || len(pen.points) != 0 {
		t.Fatalf("pen reset state invalid: x=%f, down=%v, points=%d", pen.x, pen.down, len(pen.points))
	}
}

func TestRandomNormalAndExponentialReproducibility(t *testing.T) {
	code := `
globals [ val1 val2 ]
to test
  set val1 random-normal 50 10
  set val2 random-exponential 25
end
`
	prog, err := Parse(code)
	if err != nil {
		t.Fatal(err)
	}

	vm1 := NewVM(prog, NewWorld(-5, 5, -5, 5, true, true))
	vm1.World.Seed(12345)
	mustCall(t, vm1, "test")
	v1_normal := vm1.World.globals["val1"].N
	v1_exp := vm1.World.globals["val2"].N

	vm2 := NewVM(prog, NewWorld(-5, 5, -5, 5, true, true))
	vm2.World.Seed(12345)
	mustCall(t, vm2, "test")
	v2_normal := vm2.World.globals["val1"].N
	v2_exp := vm2.World.globals["val2"].N

	if v1_normal != v2_normal || v1_exp != v2_exp {
		t.Fatalf("RNG not reproducible: normal %v vs %v, exp %v vs %v", v1_normal, v2_normal, v1_exp, v2_exp)
	}
}
