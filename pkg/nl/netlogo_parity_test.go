package nl

import (
	"math"
	"strconv"
	"strings"
	"testing"
)

// netlogoParityModel is a 33x33 wrapping world, the NetLogo default.
const netlogoParityModel = `CODE@#$#@#$#@
GRAPHICS-WINDOW
200
10
710
521
-1
-1
2.0
1
10
1
1
1
1
1
0
1
-16
16
-16
16
1
1
1
ticks
30.0

@#$#@#$#@
`

// TestNetLogoParitySequence runs a script and compares every reporter with
// the output NetLogo 6.4.0 gave for the same model and script, run headless
// (org.nlogo.headless.HeadlessWorkspace: open, then command or report per
// line). Numbers are compared exactly: after the '#' is the raw IEEE bit
// pattern NetLogo produced. It covers random, random-float, random-normal,
// random-exponential, random-poisson, coordinate reporters, crt color and
// heading, ask and of order, n-of, up-to-n-of, shuffle, one-of, one-of
// with, max/min-one-of, max/min-n-of, sort-on, sort-by, hatch,
// create-links-with, ask over a set with dead agents, and sprout.
//
// One step rounds xcor and ycor to 9 places: fd uses Go's math.Sin and
// math.Cos, which can differ from NetLogo's StrictMath in the last bit.
// That is outside the RNG.
func TestNetLogoParitySequence(t *testing.T) {
	for _, sc := range netlogoParityScenarios {
		t.Run(sc.name, func(t *testing.T) { runParityScenario(t, sc.code, sc.steps) })
	}
}

func runParityScenario(t *testing.T, code string, steps []paritySteps) {
	m, err := ParseModel("parity.nlogo", strings.Replace(netlogoParityModel, "CODE", code, 1))
	if err != nil {
		t.Fatal(err)
	}
	vm, err := m.NewVM(0)
	if err != nil {
		t.Fatal(err)
	}
	for i, s := range steps {
		if s.cmd {
			if _, err := vm.ExecuteLine(s.src); err != nil {
				t.Fatalf("step %d %q: %v", i, s.src, err)
			}
			continue
		}
		e, err := ParseReporter(s.src, vm.Prog)
		if err != nil {
			t.Fatalf("step %d %q: %v", i, s.src, err)
		}
		v, err := vm.Eval(e)
		if err != nil {
			t.Fatalf("step %d %q: %v", i, s.src, err)
		}
		if got := exactString(v); got != s.want {
			t.Fatalf("step %d %s\n got %s\nwant %s (NetLogo 6.4.0)", i, s.src, got, s.want)
		}
	}
}

// exactString prints a value the way the reference outputs were recorded:
// lists in brackets, integers bare, other numbers as Java's
// Double.toString would, and a scalar number followed by its raw bits.
func exactString(v Value) string {
	if v.K == KindNum {
		return javaDouble(v.N) + " #" + strconv.FormatUint(math.Float64bits(v.N), 16)
	}
	return exactInner(v)
}

func exactInner(v Value) string {
	switch v.K {
	case KindNum:
		return javaDouble(v.N)
	case KindList:
		parts := make([]string, len(v.L))
		for i, x := range v.L {
			parts[i] = exactInner(x)
		}
		return "[" + strings.Join(parts, " ") + "]"
	}
	return v.Format(true)
}

func javaDouble(f float64) string {
	if f == float64(int64(f)) && math.Abs(f) <= 9007199254740992 {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'g', -1, 64)
}

type paritySteps struct {
	cmd       bool
	src, want string
}

var netlogoParityScenarios = []struct {
	name  string
	code  string
	steps []paritySteps
}{
	{
		name: `basic`,
		code: `globals [ g ]
turtles-own [ energy ]
to setup
  clear-all
  crt 10 [ set energy random 10 ]
  ask patches [ set pcolor random 5 ]
end
to hatchy
  ask turtles [ hatch 2 [ fd random-float 3 ] ]
end
to linky
  ask turtle 0 [ create-links-with other turtles [ set color random 140 ] ]
end
`,
		steps: []paritySteps{
			{cmd: true, src: `random-seed 1`},
			{src: `(list random 100 random 100 random 100 random 100 random 100 random 100 random 100 random 100)`, want: `[81 88 80 90 48 28 39 46]`},
			{src: `(list random-float 1 random-float 1 random-float 10)`, want: `[0.39676748120097916 0.538816732858284 4.191945092867838]`},
			{src: `(list random 2.5 random -7 random 0 random 1 random 1024 random 1000000007 random 9007199254740992)`, want: `[1 -3 0 0 199 374269995 2883382178852843]`},
			{cmd: true, src: `random-seed 42`},
			{src: `(list random 100 random 100 random 100)`, want: `[1 55 67]`},
			{src: `random-normal 0 1`, want: `0.3189021679975765 #3fd468e4a38b13e3`},
			{src: `random-normal 0 1`, want: `-1.1118801070472497 #bff1ca42cb8d7465`},
			{src: `random-normal 10 2.5`, want: `12.526288216103161 #40290d75a628ddd4`},
			{src: `random-exponential 3`, want: `1.0356260180462349 #3ff091ec96665316`},
			{src: `random-poisson 3.5`, want: `1 #3ff0000000000000`},
			{src: `random-poisson 0.2`, want: `0 #0`},
			{src: `random-poisson 100`, want: `110 #405b800000000000`},
			{src: `(list random-xcor random-ycor random-pxcor random-pycor)`, want: `[-8.97813972273806 -2.4054432210747567 -4 0]`},
			{cmd: true, src: `random-seed 0`},
			{src: `(list random 100 random 100 random 100)`, want: `[83 16 65]`},
			{cmd: true, src: `random-seed -7`},
			{src: `random-float 5`, want: `0.278385479083913 #3fd1d1115416304e`},
			{cmd: true, src: `random-seed 2147483647`},
			{src: `random 50`, want: `37 #4042800000000000`},
			{cmd: true, src: `random-seed 7`},
			{cmd: true, src: `setup`},
			{src: `[ (list who color heading energy) ] of turtles`, want: `[[2 75 313 0] [1 65 163 7] [7 15 245 2] [9 115 219 2] [8 75 356 5] [3 135 299 5] [5 55 315 1] [6 85 268 8] [0 95 146 2] [4 125 260 8]]`},
			{src: `sort [ pcolor ] of patches with [ pxcor = 0 ]`, want: `[0 0 0 0 0 0 0 1 1 1 1 1 1 1 1 1 2 2 2 2 2 2 2 2 3 3 3 3 3 4 4 4 4]`},
			{src: `[who] of n-of 3 turtles`, want: `[1 5 4]`},
			{src: `[who] of n-of 2 turtles`, want: `[0 3]`},
			{src: `[who] of n-of 1 turtles`, want: `[4]`},
			{src: `n-of 4 [1 2 3 4 5 6 7 8]`, want: `[2 3 5 6]`},
			{src: `up-to-n-of 3 [1 2 3 4 5]`, want: `[1 2 5]`},
			{src: `up-to-n-of 9 [1 2 3 4 5]`, want: `[1 2 3 4 5]`},
			{src: `shuffle [1 2 3 4 5 6 7 8 9 10]`, want: `[6 7 5 10 9 8 2 4 3 1]`},
			{src: `one-of [5 6 7]`, want: `5 #4014000000000000`},
			{src: `[who] of one-of turtles`, want: `7 #401c000000000000`},
			{src: `[who] of max-one-of turtles [ energy ]`, want: `6 #4018000000000000`},
			{src: `[who] of min-one-of turtles [ energy ]`, want: `2 #4000000000000000`},
			{src: `[who] of max-n-of 3 turtles [ energy ]`, want: `[4 1 6]`},
			{src: `[who] of min-n-of 4 turtles [ energy ]`, want: `[5 0 7 2]`},
			{src: `map [ t -> [who] of t ] sort-on [ energy ] turtles`, want: `[2 5 7 9 0 3 8 1 4 6]`},
			{src: `map [ t -> [who] of t ] sort-by [ [a b] -> [energy] of a < [energy] of b ] turtles`, want: `[2 5 0 9 7 8 3 1 4 6]`},
			{src: `[who] of one-of turtles with [ energy > 4 ]`, want: `1 #3ff0000000000000`},
			{src: `[ random 10 ] of turtles`, want: `[8 2 1 1 3 7 1 4 2 8]`},
			{cmd: true, src: `ask turtles [ set g random 1000 ]`},
			{src: `g`, want: `444 #407bc00000000000`},
			{src: `random 1000`, want: `491 #407eb00000000000`},
			{cmd: true, src: `hatchy`},
			{src: `count turtles`, want: `30 #403e000000000000`},
			{src: `[ (list who precision xcor 6 precision ycor 6) ] of turtles with [ who > 25 ]`, want: `[[27 -0.592988 0.328698] [29 -1.43637 1.43637] [28 -0.021368 0.021368] [26 -1.537706 0.852365]]`},
			{cmd: true, src: `linky`},
			{src: `sort [ color ] of links`, want: `[8 11 11 14 16 24 25 41 43 62 66 70 74 75 78 79 83 87 102 104 105 106 117 122 124 125 130 131 135]`},
			{src: `random 1000`, want: `323 #4074300000000000`},
			{cmd: true, src: `ask n-of 5 turtles [ die ]`},
			{cmd: true, src: `ask turtles with [ who < 15 ] [ set energy random 7 ]`},
			{src: `[ energy ] of turtles`, want: `[0 8 5 5 1 5 2 4 5 7 8 1 1 6 1 2 2 1 1 7 4 1 3 8 5]`},
			{cmd: true, src: `ask patch 0 0 [ sprout 3 [ set energy random-normal 0 1 ] ]`},
			{src: `[ (list who color heading energy) ] of turtles with [ who >= 30 ]`, want: `[[31 125 153 0.9824945863084619] [32 45 202 0.1454283307916876] [30 115 164 -0.4242456168844571]]`},
			{src: `random 1000`, want: `622 #4083700000000000`},
		},
	},
	{
		name: `dead slots, patches, neighbors, hills`,
		code: `globals [ saved ]
patches-own [ chem ]
to setup
  clear-all
  crt 12 [ setxy random-xcor random-ycor ]
  ask patches [ set chem random-float 10 ]
  set saved turtles with [ who mod 2 = 0 ]
end
`,
		steps: []paritySteps{
			{cmd: true, src: `random-seed 11`},
			{cmd: true, src: `setup`},
			{cmd: true, src: `ask turtle 2 [ die ]`},
			{cmd: true, src: `ask turtle 6 [ die ]`},
			{cmd: true, src: `ask saved [ set heading random 360 ]`},
			{src: `random 1000`, want: `895 #408bf80000000000`},
			{src: `[who] of saved`, want: `[4 0 8 10]`},
			{src: `random 1000`, want: `185 #4067200000000000`},
			{src: `[who] of one-of saved`, want: `10 #4024000000000000`},
			{src: `[who] of n-of 3 saved`, want: `[10 8 4]`},
			{src: `[ (list pxcor pycor) ] of one-of patches`, want: `[-14 0]`},
			{src: `[ (list pxcor pycor) ] of n-of 3 patches`, want: `[[14 -15] [-15 -16] [7 9]]`},
			{src: `random 1000`, want: `863 #408af80000000000`},
			{cmd: true, src: `ask turtles [ move-to one-of neighbors ]`},
			{src: `map [ t -> [ (list who xcor ycor) ] of t ] sort turtles`, want: `[[0 -5 -10] [1 -1 7] [3 0 -3] [4 -2 -16] [5 -12 10] [7 -16 -2] [8 6 15] [9 11 -11] [10 -5 -13] [11 13 -4]]`},
			{cmd: true, src: `ask turtles [ uphill chem ]`},
			{src: `map [ t -> [ (list who xcor ycor heading) ] of t ] sort turtles`, want: `[[0 -6 -11 225] [1 -1 8 0] [3 1 -2 45] [4 -1 -15 45] [5 -11 10 90] [7 -15 -2 90] [8 5 15 270] [9 10 -10 315] [10 -4 -13 90] [11 12 -5 225]]`},
			{cmd: true, src: `ask turtles [ downhill4 chem ]`},
			{src: `map [ t -> [ (list who xcor ycor) ] of t ] sort turtles`, want: `[[0 -6 -12] [1 -2 8] [3 1 -1] [4 -1 -16] [5 -11 11] [7 -15 -3] [8 4 15] [9 10 -11] [10 -4 -14] [11 12 -4]]`},
			{src: `[ count turtles in-radius 5 ] of turtles`, want: `[1 1 3 1 1 1 2 1 1 2]`},
			{cmd: true, src: `ask turtles [ ask one-of turtles [ set color random 140 ] rt random-normal 0 30 fd random-exponential 2 ]`},
			{src: `map [ t -> [ (list who color precision xcor 9 precision ycor 9 heading) ] of t ] sort turtles`, want: `[[0 125 -5.992048581 -12.183527954 177.5191897124762] [1 5 -2.062089027 8.007298889 276.70464601421946] [3 45 0.634142869 -0.465228416 325.6225172611255] [4 107 -0.839336081 -16.235015527 145.64227478498844] [5 46 -11.916192376 14.307275943 344.5160305291789] [7 7 -14.288785793 -3.989251227 144.28605755585127] [8 35 2.482106792 14.610470764 255.60706497465412] [9 20 8.927166412 -15.883546671 192.39010882796862] [10 65 -3.945307413 -14.326777118 170.49849027516757] [11 104 12.059391384 -3.887361714 27.801573431380984]]`},
			{cmd: true, src: `ask turtles [ if random 2 = 0 [ die ] ]`},
			{src: `map [ t -> [ who ] of t ] sort turtles`, want: `[0 1 3 4 5 7 11]`},
			{cmd: true, src: `ask saved [ hatch 1 [ set heading random 360 ] ]`},
			{src: `map [ t -> [ (list who heading) ] of t ] sort turtles`, want: `[[0 177.5191897124762] [1 276.70464601421946] [3 325.6225172611255] [4 145.64227478498844] [5 344.5160305291789] [7 144.28605755585127] [11 27.801573431380984] [12 276] [13 347]]`},
			{src: `random 1000`, want: `263 #4070700000000000`},
			{cmd: true, src: `ask patches [ sprout random 2 ]`},
			{src: `count turtles`, want: `561 #4081880000000000`},
			{src: `random 1000`, want: `547 #4081180000000000`},
			{src: `[ (list pxcor pycor) ] of one-of patches with [ chem > 9.9 ]`, want: `[-13 16]`},
			{src: `map [ p -> [ (list pxcor pycor) ] of p ] sort-on [ round chem ] n-of 6 patches`, want: `[[10 -16] [2 5] [-1 3] [9 14] [-7 10] [-6 -1]]`},
			{src: `random 1000`, want: `897 #408c080000000000`},
		},
	},
}
