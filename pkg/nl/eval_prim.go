package nl

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
	"unicode"
)

func (vm *VM) turtleFromArg(v Value, what string) (*Turtle, error) {
	switch v.K {
	case KindNum:
		id, err := asInt(v, what)
		if err != nil {
			return nil, err
		}
		return vm.World.Turtle(id), nil
	case KindAgent:
		t, ok := v.A.(*Turtle)
		if !ok || t == nil {
			return nil, fmt.Errorf("%s expected a turtle or integer, got %s", what, v.TypeName())
		}
		if t.dead {
			return nil, fmt.Errorf("that agent is dead")
		}
		return t, nil
	case KindNobody:
		return nil, nil
	default:
		return nil, fmt.Errorf("%s expected a turtle or integer, got %s", what, v.TypeName())
	}
}

func (vm *VM) requireLink(pos Pos) *Link {
	l, ok := vm.self.(*Link)
	if !ok || l == nil || l.dead {
		vm.fail(pos, "this command can only be used by links")
	}
	return l
}

func asInt(v Value, what string) (int, error) {
	n, err := v.Num()
	if err != nil {
		return 0, fmt.Errorf("%s expected a number, got %s", what, v.TypeName())
	}
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, fmt.Errorf("%s expected an integer", what)
	}
	if n > 1e12 || n < -1e12 {
		return 0, fmt.Errorf("%s: number is too large", what)
	}
	return int(math.Trunc(n)), nil
}

// moveTurtle is the shared fd/bk/jump implementation. jumpAll moves the
// whole distance and stays put when that would leave the world. Otherwise
// the turtle walks in unit steps and stops at the last legal position.
func (vm *VM) moveTurtle(t *Turtle, dist float64, jumpAll bool) error {
	if jumpAll {
		// jump stays put when the whole step would leave the world.
		// A tied turtle that cannot follow is still an error.
		return swallowEdge(vm.World.Jump(t, dist))
	}
	return vm.World.Forward(t, dist)
}

func (vm *VM) facePoint(t *Turtle, x, y float64) error {
	h, err := vm.World.HeadingTowards(t.X, t.Y, x, y)
	if err != nil {
		return nil
	}
	return vm.World.SetHeading(t, h)
}

func (vm *VM) moveToAgent(target Value, pos Pos) error {
	t := vm.requireTurtle(pos)
	if target.K == KindNobody || (target.K == KindAgent && (target.A == nil || target.A.IsDead())) {
		return fmt.Errorf("that agent is dead")
	}
	if target.K != KindAgent {
		return fmt.Errorf("move-to expected a turtle or patch")
	}
	if _, ok := target.A.(*Link); ok {
		return fmt.Errorf("you can't move-to a link")
	}
	x, y, err := pointOf(target.A)
	if err != nil {
		return err
	}
	return vm.World.MoveTo(t, x, y)
}

func (vm *VM) primCmd(name string, args []Value, pos Pos) error {
	w := vm.World
	if w.threeD {
		if handled, err := vm.primCmd3D(name, args, pos); handled {
			return err
		}
	}
	switch name {
	case "clear-all", "ca":
		vm.requireObserver(pos, "clear-all")
		w.ClearAll()
		vm.clearOutput()
		vm.clearAllPlots()
		vm.sweepDeadEveryTimers()
	case "clear-turtles", "ct":
		vm.requireObserver(pos, "clear-turtles")
		w.ClearTurtles()
		vm.sweepDeadEveryTimers()
	case "clear-patches", "cp":
		vm.requireObserver(pos, "clear-patches")
		w.ClearPatches()
	case "clear-globals":
		vm.requireObserver(pos, "clear-globals")
		w.ClearGlobals()
	case "clear-links":
		vm.requireObserver(pos, "clear-links")
		w.ClearLinks()
		vm.sweepDeadEveryTimers()
	case "clear-ticks":
		vm.requireObserver(pos, "clear-ticks")
		w.ClearTicks()
	case "error":
		vm.fail(pos, "%s", args[0].Format(false))
	case "hide-link":
		vm.requireLink(pos).Hidden = true
	case "show-link":
		vm.requireLink(pos).Hidden = false
	case "histogram":
		return vm.histogram(args[0])
	case "set-histogram-num-bars":
		return vm.setHistogramNumBars(args[0])
	case "set-plot-pen-color":
		return vm.setPlotPenColor(args[0])
	case "set-plot-pen-mode":
		return vm.setPlotPenMode(args[0])
	case "set-plot-pen-interval":
		return vm.setPlotPenInterval(args[0])
	case "hubnet-reset", "hubnet-fetch-message", "hubnet-kick-all-clients",
		"hubnet-clear-overrides", "hubnet-reset-perspective", "hubnet-kick-client",
		"hubnet-make-plot-narrowcast", "__hubnet-make-plot-narrowcast",
		"hubnet-clear-plot", "__hubnet-clear-plot", "hubnet-create-client", "__hubnet-create-client",
		"hubnet-plot", "__hubnet-plot",
		"hubnet-broadcast", "hubnet-broadcast-message", "hubnet-send", "hubnet-send-follow",
		"hubnet-send-watch", "hubnet-send-override", "hubnet-clear-override",
		"inspect", "stop-inspecting", "stop-inspecting-dead-agents", "export-interface",
		"__set-line-thickness":
		return nil
	case "clear-drawing", "cd":
		w.ClearDrawing()
	case "clear-output":
		vm.clearOutput()
	case "reset-ticks":
		vm.requireObserver(pos, "reset-ticks")
		w.ResetTicks()
		vm.setupPlots()
		vm.updatePlots()
	case "tick":
		vm.requireObserver(pos, "tick")
		if err := w.Tick(); err != nil {
			return err
		}
		vm.updatePlots()
	case "clear-all-plots":
		vm.clearAllPlots()
	case "clear-plot":
		return vm.clearPlot()
	case "setup-plots":
		vm.setupPlots()
	case "update-plots":
		vm.updatePlots()
	case "plot":
		y, err := plotNum(args[0], "plot")
		if err != nil {
			return err
		}
		return vm.plotY(y)
	case "plotxy":
		x, err := plotNum(args[0], "plotxy")
		if err != nil {
			return err
		}
		y, err := plotNum(args[1], "plotxy")
		if err != nil {
			return err
		}
		return vm.plotXY(x, y)
	case "plot-pen-up":
		return vm.plotPenDown(false)
	case "plot-pen-down":
		return vm.plotPenDown(true)
	case "set-plot-x-range":
		return vm.setPlotRange(args[0], args[1], true)
	case "set-plot-y-range":
		return vm.setPlotRange(args[0], args[1], false)
	case "set-current-plot":
		return vm.setCurrentPlot(args[0])
	case "set-current-plot-pen":
		return vm.setCurrentPlotPen(args[0])
	case "auto-plot-on", "autoplot-on":
		return vm.setAutoPlot(true)
	case "auto-plot-off", "autoplot-off":
		return vm.setAutoPlot(false)
	case "plot-pen-reset":
		return vm.plotPenReset()
	case "create-temporary-plot-pen":
		return vm.createTempPen(args[0])
	case "tick-advance":
		vm.requireObserver(pos, "tick-advance")
		n, err := args[0].Num()
		if err != nil {
			return err
		}
		return w.TickAdvance(n)
	case "random-seed":
		d, err := args[0].Num()
		if err != nil {
			return fmt.Errorf("random-seed expected a number, got %s", args[0].TypeName())
		}
		// Command-center and model random-seed keep NetLogo's signed int
		// range (-2147483648 to 2147483647). The World tab instead stores
		// an unsigned 32-bit seed, 0..4294967295. A value >= 2^31 is the
		// same Mersenne Twister state as the negative seed with those low
		// 32 bits (4294967295 and -1, 2147483648 and -2147483648), because
		// setSeed keeps only those bits. This primitive is not a no-op
		// when a session seed override is also set: model code runs after
		// that override and replaces it for the rest of the setup.
		if d <= -2147483649 || d >= 2147483648 {
			return fmt.Errorf("%s is not in the allowable range for random seeds (-2147483648 to 2147483647)", formatNum(d))
		}
		if math.IsNaN(d) {
			d = 0 // Java's (long) NaN
		}
		w.Seed(int64(d))
	case "die":
		if vm.self == nil {
			return fmt.Errorf("only turtles and links can die")
		}
		if _, ok := vm.self.(*Patch); ok {
			return fmt.Errorf("patches cannot die")
		}
		w.Die(vm.self)
		vm.dropEveryTimers(vm.self)
		panic(diePanic{})
	case "fd", "forward":
		n, err := args[0].Num()
		if err != nil {
			return err
		}
		return vm.moveTurtle(vm.requireTurtle(pos), n, false)
	case "bk", "back":
		n, err := args[0].Num()
		if err != nil {
			return err
		}
		return vm.moveTurtle(vm.requireTurtle(pos), -n, false)
	case "jump":
		n, err := args[0].Num()
		if err != nil {
			return err
		}
		return vm.moveTurtle(vm.requireTurtle(pos), n, true)
	case "move-to":
		return vm.moveToAgent(args[0], pos)
	case "rt", "right":
		n, err := args[0].Num()
		if err != nil {
			return err
		}
		return w.Turn(vm.requireTurtle(pos), n)
	case "lt", "left":
		n, err := args[0].Num()
		if err != nil {
			return err
		}
		return w.Turn(vm.requireTurtle(pos), -n)
	case "setxy":
		x, err := args[0].Num()
		if err != nil {
			return err
		}
		y, err := args[1].Num()
		if err != nil {
			return err
		}
		return w.MoveTo(vm.requireTurtle(pos), x, y)
	case "home":
		return w.MoveTo(vm.requireTurtle(pos), 0, 0)
	case "face":
		t := vm.requireTurtle(pos)
		x, y, err := vm.agentPoint(args[0])
		if err != nil {
			return err
		}
		return vm.facePoint(t, x, y)
	case "facexy":
		t := vm.requireTurtle(pos)
		x, err := args[0].Num()
		if err != nil {
			return err
		}
		y, err := args[1].Num()
		if err != nil {
			return err
		}
		return vm.facePoint(t, x, y)
	case "pen-down", "pd", "pendown":
		t := vm.requireTurtle(pos)
		t.PenDown = true
		t.PenErase = false
	case "pen-up", "pu", "penup":
		t := vm.requireTurtle(pos)
		t.PenDown = false
		t.PenErase = false
	case "pen-erase", "pe":
		t := vm.requireTurtle(pos)
		t.PenDown = false
		t.PenErase = true
	case "stamp":
		w.Stamp(vm.requireTurtle(pos), false)
	case "stamp-erase":
		w.Stamp(vm.requireTurtle(pos), true)
	case "hide-turtle", "ht":
		vm.requireTurtle(pos).Hidden = true
	case "show-turtle", "st":
		vm.requireTurtle(pos).Hidden = false
	case "print":
		vm.emit(args[0].Format(false))
	case "output-print":
		vm.emitOutput(args[0].Format(false), true)
	case "show":
		vm.emit(vm.agentTag() + ": " + args[0].Format(false))
	case "output-show":
		vm.emitOutput(vm.agentTag()+": "+args[0].Format(false), true)
	case "type":
		vm.partial += args[0].Format(false)
	case "write":
		vm.partial += args[0].Format(true)
	case "output-type":
		vm.emitOutput(args[0].Format(false), false)
	case "output-write":
		vm.emitOutput(args[0].Format(true), false)
	case "user-message":
		vm.emit(args[0].Format(false))
	case "file-open":
		if args[0].K != KindString {
			return fmt.Errorf("file-open expected a string")
		}
		return vm.fileOpen(args[0].S)
	case "file-close":
		return vm.fileClose()
	case "file-close-all":
		return vm.fileCloseAll()
	case "file-delete":
		if args[0].K != KindString {
			return fmt.Errorf("file-delete expected a string")
		}
		return vm.fileDelete(args[0].S)
	case "file-flush":
		return vm.fileFlush()
	case "file-print":
		return vm.filePrint(args[0])
	case "file-type":
		return vm.fileType(args[0])
	case "file-show":
		return vm.fileShow(args[0])
	case "file-write":
		return vm.fileWrite(args[0])
	case "import-world", "export-world", "import-pcolors", "import-pcolors-rgb":
		return fmt.Errorf("%s is not supported yet", name)
	case "tie":
		l, ok := vm.self.(*Link)
		if !ok || l == nil {
			return fmt.Errorf("tie can only be used by a link")
		}
		l.TieMode = "fixed"
		return nil
	case "untie":
		l, ok := vm.self.(*Link)
		if !ok || l == nil {
			return fmt.Errorf("untie can only be used by a link")
		}
		l.TieMode = "none"
		return nil
	case "layout-spring":
		return vm.layoutSpring(args[0], args[1], args[2], args[3], args[4])
	case "resize-world":
		vm.requireObserver(pos, "resize-world")
		a, err := asInt(args[0], "resize-world")
		if err != nil {
			return err
		}
		b, err := asInt(args[1], "resize-world")
		if err != nil {
			return err
		}
		c, err := asInt(args[2], "resize-world")
		if err != nil {
			return err
		}
		d, err := asInt(args[3], "resize-world")
		if err != nil {
			return err
		}
		if err := ValidWorld(a, b, c, d); err != nil {
			return err
		}
		w.Resize(a, b, c, d, w.WrapX, w.WrapY)
	case "set-default-shape":
		breed := "turtles"
		if args[0].K == KindAgentset && args[0].Set != nil {
			if args[0].Set.Breed != "" {
				breed = args[0].Set.Breed
			}
		} else if args[0].K == KindString {
			breed = strings.ToLower(args[0].S)
		} else {
			return fmt.Errorf("set-default-shape expected a breed")
		}
		if args[1].K != KindString {
			return fmt.Errorf("set-default-shape expected a string shape name")
		}
		w.SetDefaultShape(breed, strings.ToLower(args[1].S))
	case "set-patch-size":
		n, err := args[0].Num()
		if err != nil {
			return err
		}
		if n <= 0 {
			return fmt.Errorf("patch size must be positive")
		}
		w.PatchSize = n
	case "watch":
		if args[0].K != KindAgent || args[0].A == nil || args[0].A.IsDead() {
			return fmt.Errorf("watch expected an agent")
		}
		w.SetPerspective("watch", args[0].A)
	case "follow":
		if args[0].K != KindAgent || args[0].A == nil || args[0].A.IsDead() {
			return fmt.Errorf("follow expected an agent")
		}
		if _, ok := args[0].A.(*Turtle); !ok {
			return fmt.Errorf("follow can only follow turtles")
		}
		w.SetPerspective("follow", args[0].A)
	case "ride":
		if args[0].K != KindAgent || args[0].A == nil || args[0].A.IsDead() {
			return fmt.Errorf("ride expected an agent")
		}
		if _, ok := args[0].A.(*Turtle); !ok {
			return fmt.Errorf("ride can only ride turtles")
		}
		w.SetPerspective("ride", args[0].A)
	case "reset-perspective", "rp":
		w.ResetPerspective()
	case "watch-me":
		if vm.self == nil {
			return fmt.Errorf("watch-me can only be used by an agent")
		}
		w.SetPerspective("watch", vm.self)
	case "follow-me":
		t := vm.requireTurtle(pos)
		w.SetPerspective("follow", t)
	case "ride-me":
		t := vm.requireTurtle(pos)
		w.SetPerspective("ride", t)
	case "beep":
		return nil
	case "reset-timer":
		vm.timer = time.Now()
	case "wait":
		n, err := args[0].Num()
		if err != nil {
			return err
		}
		if n < 0 {
			n = 0
		}
		if n > 5 {
			n = 5
		}
		time.Sleep(time.Duration(n * float64(time.Second)))
	case "display":
		vm.DisplayOn = true
	case "no-display":
		vm.DisplayOn = false
	case "run":
		return vm.doRun(args, pos)
	case "layout-circle":
		return vm.layoutCircle(args[0], args[1])
	default:
		if err, ok := vm.extCommand(name, args, pos); ok {
			return err
		}
		return fmt.Errorf("primitive %s is not implemented yet", name)
	}
	// flush typed text on a following print-less command? NetLogo type
	// waits for print. Leave partial until print. If the line ends, tests
	// can read partial via a final print. Also flush partial into lines
	// when print happens. type alone should still be observable:
	return nil
}

func (vm *VM) emit(s string) {
	if vm.partial != "" {
		s = vm.partial + s
		vm.partial = ""
	}
	vm.lines = append(vm.lines, s)
	if vm.OnPrint != nil {
		vm.OnPrint(s)
	}
}

func (vm *VM) emitOutput(s string, newline bool) {
	if !vm.hasOutput {
		if newline {
			vm.emit(s)
		} else {
			vm.partial += s
		}
		return
	}
	if !newline {
		vm.outputPartial += s
		return
	}
	if vm.outputPartial != "" {
		s = vm.outputPartial + s
		vm.outputPartial = ""
	}
	vm.outputLines = append(vm.outputLines, s)
	vm.outputSeq++
	if len(vm.outputLines) > OutputLineCap {
		vm.outputLines = append([]string(nil), vm.outputLines[len(vm.outputLines)-OutputLineCap:]...)
	}
}

func (vm *VM) clearOutput() {
	if !vm.hasOutput {
		vm.lines = nil
		vm.partial = ""
		if vm.OnClear != nil {
			vm.OnClear()
		}
		return
	}
	vm.outputLines = nil
	vm.outputPartial = ""
	vm.outputSeq = 0
	vm.outputEpoch++
}

func (vm *VM) agentTag() string {
	if vm.self == nil {
		return "observer"
	}
	return strings.Trim(vm.self.AgentString(), "()")
}

func (vm *VM) doRun(args []Value, pos Pos) error {
	if len(args) == 0 {
		return fmt.Errorf("run expected a string or a command task")
	}
	v := args[0]
	inputs := args[1:]
	switch v.K {
	case KindString:
		if len(inputs) != 0 {
			return fmt.Errorf("run of a string does not take extra inputs")
		}
		// Robby's inner loop is `run item state strategy`, and strategy is
		// a list of command names. Parsing that string on every action
		// rebuilds the declaration maps. A bare command procedure is a
		// direct call; anything else is parsed once and cached on the VM.
		if proc := vm.bareCommand(v.S); proc != nil {
			_, err := vm.callCmd(proc, nil)
			return err
		}
		stmts, err := vm.cachedCommands(v.S)
		if err != nil {
			return err
		}
		vm.execStmts(stmts)
		return nil
	case KindTask:
		if v.Task == nil || !v.Task.IsCommand {
			return fmt.Errorf("run expected a command task or a string")
		}
		vm.callTaskCmd(v.Task, inputs, pos)
		return nil
	default:
		return fmt.Errorf("run expected a string or a command task")
	}
}

// bareCommand reports a zero-argument command procedure when src is
// only that procedure's name, ignoring case and surrounding space.
func (vm *VM) bareCommand(src string) *Procedure {
	if vm == nil || vm.Prog == nil {
		return nil
	}
	name := strings.TrimSpace(src)
	if name == "" {
		return nil
	}
	for _, r := range name {
		if r == '-' || r == '_' || r == '?' || r == '.' || unicode.IsLetter(r) || unicode.IsDigit(r) {
			continue
		}
		return nil
	}
	proc := vm.Prog.Proc(strings.ToLower(name))
	if proc == nil || proc.IsReporter || len(proc.Params) != 0 {
		return nil
	}
	return proc
}

func (vm *VM) cachedCommands(src string) ([]Stmt, error) {
	if vm.runCache != nil {
		if stmts, ok := vm.runCache[src]; ok {
			return stmts, nil
		}
	}
	stmts, err := ParseCommands(src, vm.Prog)
	if err != nil {
		return nil, err
	}
	if vm.runCache == nil {
		vm.runCache = map[string][]Stmt{}
	}
	vm.runCache[src] = stmts
	return stmts, nil
}

func (vm *VM) layoutSpring(nodesVal, linksVal, sprConstVal, sprLenVal, repConstVal Value) error {
	var nodes []*Turtle
	switch nodesVal.K {
	case KindAgentset:
		for _, a := range nodesVal.Set.Alive() {
			if t, ok := a.(*Turtle); ok {
				nodes = append(nodes, t)
			}
		}
	case KindList:
		for _, v := range nodesVal.L {
			if v.K == KindAgent {
				if t, ok := v.A.(*Turtle); ok && !t.IsDead() {
					nodes = append(nodes, t)
				}
			}
		}
	default:
		return fmt.Errorf("layout-spring expected a turtle agentset or list")
	}

	var links []*Link
	switch linksVal.K {
	case KindAgentset:
		for _, a := range linksVal.Set.Alive() {
			if l, ok := a.(*Link); ok {
				links = append(links, l)
			}
		}
	case KindList:
		for _, v := range linksVal.L {
			if v.K == KindAgent {
				if l, ok := v.A.(*Link); ok && !l.IsDead() {
					links = append(links, l)
				}
			}
		}
	default:
		return fmt.Errorf("layout-spring expected a link agentset or list")
	}

	k_spring, err := sprConstVal.Num()
	if err != nil {
		return err
	}
	l_spring, err := sprLenVal.Num()
	if err != nil {
		return err
	}
	k_rep, err := repConstVal.Num()
	if err != nil {
		return err
	}

	if len(nodes) == 0 {
		return nil
	}

	nodeSet := make(map[*Turtle]bool, len(nodes))
	fx := make(map[*Turtle]float64, len(nodes))
	fy := make(map[*Turtle]float64, len(nodes))
	for _, t := range nodes {
		nodeSet[t] = true
	}

	for i := 0; i < len(nodes); i++ {
		t1 := nodes[i]
		for j := i + 1; j < len(nodes); j++ {
			t2 := nodes[j]
			dx := t2.X - t1.X
			dy := t2.Y - t1.Y
			d := math.Hypot(dx, dy)
			if d < 0.001 {
				d = 0.001
				dx = 0.001
			}
			repForce := k_rep / (d * d)
			if repForce > 10.0 {
				repForce = 10.0
			}
			fx[t1] -= repForce * (dx / d)
			fy[t1] -= repForce * (dy / d)
			fx[t2] += repForce * (dx / d)
			fy[t2] += repForce * (dy / d)
		}
	}

	for _, l := range links {
		t1, t2 := l.End1, l.End2
		if t1 == nil || t2 == nil || !nodeSet[t1] || !nodeSet[t2] {
			continue
		}
		dx := t2.X - t1.X
		dy := t2.Y - t1.Y
		d := math.Hypot(dx, dy)
		if d < 0.001 {
			d = 0.001
		}
		disp := d - l_spring
		springForce := k_spring * disp
		if springForce > 10.0 {
			springForce = 10.0
		} else if springForce < -10.0 {
			springForce = -10.0
		}
		fx[t1] += springForce * (dx / d)
		fy[t1] += springForce * (dy / d)
		fx[t2] -= springForce * (dx / d)
		fy[t2] -= springForce * (dy / d)
	}

	for _, t := range nodes {
		newX := clampInside(t.X+fx[t]*0.1, vm.World.MinPxcor, vm.World.MaxPxcor, vm.World.WrapX)
		newY := clampInside(t.Y+fy[t]*0.1, vm.World.MinPycor, vm.World.MaxPycor, vm.World.WrapY)
		if err := vm.World.MoveTo(t, newX, newY); err != nil {
			return err
		}
	}
	return nil
}

// clampInside keeps a coordinate inside a non-wrapping axis. The legal
// interval is [minPcor-0.5, maxPcor+0.5). maxPcor+0.5 itself is outside,
// and MoveTo would reject it and leave the turtle unmoved.
func clampInside(x float64, minP, maxP int, wrap bool) float64 {
	if wrap {
		return x
	}
	min := float64(minP) - 0.5
	max := float64(maxP) + 0.5
	if x < min {
		return min
	}
	if x >= max {
		const eps = 1e-9
		if max-eps > min {
			return max - eps
		}
		return min
	}
	return x
}

func (vm *VM) layoutCircle(agents, radius Value) error {
	var live []Agent
	if agents.K == KindAgentset && agents.Set != nil {
		live = agents.Set.Alive()
	} else if agents.K == KindList {
		// sort of an agentset is a list. NetLogo's layout-circle accepts
		// that list, and several Models Library files pass it.
		for _, v := range agents.L {
			if v.K == KindAgent && v.A != nil && !v.A.IsDead() {
				live = append(live, v.A)
			}
		}
	} else {
		return fmt.Errorf("layout-circle expected a turtle agentset or list of turtles")
	}
	r, err := radius.Num()
	if err != nil {
		return err
	}
	n := len(live)
	if n == 0 {
		return nil
	}
	for i, a := range live {
		t, ok := a.(*Turtle)
		if !ok {
			return fmt.Errorf("layout-circle expected turtles")
		}
		h := 360 * float64(i) / float64(n)
		rad := h * math.Pi / 180
		if err := vm.World.MoveTo(t, r*math.Sin(rad), r*math.Cos(rad)); err != nil {
			return err
		}
		if err := vm.World.SetHeading(t, h); err != nil {
			return err
		}
	}
	return nil
}

func linkCmd(name string, w *World) (breed string, directed bool, plural bool, from bool, ok bool) {
	switch name {
	case "create-link-with":
		return "", false, false, false, true
	case "create-links-with":
		return "", false, true, false, true
	case "create-link-to":
		return "", true, false, false, true
	case "create-links-to":
		return "", true, true, false, true
	case "create-link-from":
		return "", true, false, true, true
	case "create-links-from":
		return "", true, true, true, true
	}
	if !strings.HasPrefix(name, "create-") {
		return "", false, false, false, false
	}
	rest := strings.TrimPrefix(name, "create-")
	for pl, b := range w.linkBreeds {
		sg := b.Singular
		switch {
		case rest == sg+"-with" && !b.Directed:
			return pl, false, false, false, true
		case rest == pl+"-with" && !b.Directed:
			return pl, false, true, false, true
		case rest == sg+"-to" && b.Directed:
			return pl, true, false, false, true
		case rest == pl+"-to" && b.Directed:
			return pl, true, true, false, true
		case rest == sg+"-from" && b.Directed:
			return pl, true, false, true, true
		case rest == pl+"-from" && b.Directed:
			return pl, true, true, true, true
		}
	}
	return "", false, false, false, false
}

func (vm *VM) createLinks(name string, args []Value, block []Stmt, pos Pos) {
	breed, directed, plural, from, ok := linkCmd(name, vm.World)
	if !ok {
		vm.fail(pos, "primitive %s is not implemented yet", name)
	}
	self := vm.requireTurtle(pos)
	var others []*Turtle
	if plural {
		if args[0].K != KindAgentset {
			vm.fail(pos, "%s expected a turtle agentset", name)
		}
		for _, a := range args[0].Set.Alive() {
			if _, ok := a.(*Turtle); !ok {
				vm.fail(pos, "%s expected turtles", name)
			}
		}
		// NetLogo creates the links in shufflerator order.
		for _, a := range vm.World.shuffledLive(args[0].Set) {
			others = append(others, a.(*Turtle))
		}
	} else {
		t, ok := args[0].A.(*Turtle)
		if args[0].K != KindAgent || !ok {
			vm.fail(pos, "%s expected a turtle", name)
		}
		others = []*Turtle{t}
	}
	var made []Agent
	for _, other := range others {
		a, b := self, other
		if from {
			a, b = other, self
		}
		l, created, err := vm.World.CreateLink(a, b, directed, breed)
		if err != nil {
			vm.fail(pos, "%s", err.Error())
		}
		// NetLogo leaves an existing link alone and does not run the block.
		if !created {
			continue
		}
		made = append(made, l)
	}
	// NetLogo creates every link first, then asks the new links to run
	// the block in shufflerator order.
	if len(block) > 0 {
		vm.execAskBlock(made, true, block)
	}
}

func (vm *VM) evalZero(name string, pos Pos) (Value, error) {
	w := vm.World
	if w.threeD || zeroArgReporters3D[name] {
		if v, handled, err := vm.evalZero3D(name, pos); handled {
			return v, err
		}
	}
	switch name {
	case "turtles":
		return SetVal(w.Turtles()), nil
	case "patches":
		return SetVal(w.Patches()), nil
	case "links":
		return SetVal(w.Links()), nil
	case "turtles-here":
		p := vm.patchUnderSelf()
		if p == nil {
			return Value{}, errorAt(pos, "turtles-here can only be used by a turtle or patch")
		}
		return SetVal(turtleSet(w.TurtlesHere(p))), nil
	case "patch-here":
		t, ok := vm.self.(*Turtle)
		if !ok {
			return Value{}, errorAt(pos, "patch-here can only be used by a turtle")
		}
		p := w.PatchHere(t)
		if p == nil {
			return Nobody(), nil
		}
		return AgentVal(p), nil
	case "dx":
		t, ok := vm.self.(*Turtle)
		if !ok {
			return Value{}, errorAt(pos, "dx can only be used by a turtle")
		}
		dx, _ := headingDelta(t.Heading, 1)
		return Num(dx), nil
	case "dy":
		t, ok := vm.self.(*Turtle)
		if !ok {
			return Value{}, errorAt(pos, "dy can only be used by a turtle")
		}
		_, dy := headingDelta(t.Heading, 1)
		return Num(dy), nil
	case "neighbors":
		p := vm.patchUnderSelf()
		if p == nil {
			return Value{}, errorAt(pos, "neighbors can only be used by a turtle or patch")
		}
		return SetVal(patchSet(w.Neighbors(p))), nil
	case "neighbors4":
		p := vm.patchUnderSelf()
		if p == nil {
			return Value{}, errorAt(pos, "neighbors4 can only be used by a turtle or patch")
		}
		return SetVal(patchSet(w.Neighbors4(p))), nil
	case "self":
		if vm.self == nil {
			return Value{}, errorAt(pos, "self cannot be used in observer context")
		}
		return AgentVal(vm.self), nil
	case "myself":
		if len(vm.askers) == 0 || vm.askers[len(vm.askers)-1] == nil {
			return Value{}, errorAt(pos, "there is no agent for MYSELF to refer to")
		}
		return AgentVal(vm.askers[len(vm.askers)-1]), nil
	case "min-pxcor":
		return Num(float64(w.MinPxcor)), nil
	case "max-pxcor":
		return Num(float64(w.MaxPxcor)), nil
	case "min-pycor":
		return Num(float64(w.MinPycor)), nil
	case "max-pycor":
		return Num(float64(w.MaxPycor)), nil
	case "world-width":
		return Num(float64(w.width)), nil
	case "world-height":
		return Num(float64(w.height)), nil
	case "patch-size":
		return Num(w.PatchSize), nil
	case "ticks":
		n, err := w.Ticks()
		if err != nil {
			return Value{}, err
		}
		return Num(n), nil
	case "timer":
		return Num(time.Since(vm.timer).Seconds()), nil
	case "new-seed":
		return Num(float64(int32(time.Now().UnixNano()))), nil
	case "random-xcor":
		return Num(w.RandomXcor()), nil
	case "random-ycor":
		return Num(w.RandomYcor()), nil
	case "random-pxcor":
		return Num(float64(w.RandomPxcor())), nil
	case "random-pycor":
		return Num(float64(w.RandomPycor())), nil
	case "netlogo-web?":
		return Bool(false), nil
	case "subject":
		_, a := w.Perspective()
		if a == nil || a.IsDead() {
			return Nobody(), nil
		}
		return AgentVal(a), nil
	case "pi":
		return Num(math.Pi), nil
	case "base-colors":
		return List(
			Num(5), Num(15), Num(25), Num(35), Num(45), Num(55), Num(65),
			Num(75), Num(85), Num(95), Num(105), Num(115), Num(125), Num(135),
		), nil
	case "error-message":
		if len(vm.errMsgs) > 0 {
			return Str(vm.errMsgs[len(vm.errMsgs)-1]), nil
		}
		return Value{}, errorAt(pos, "error-message is only allowed inside carefully")
	case "hubnet-message-waiting?", "hubnet-enter-message?", "hubnet-exit-message?":
		return Bool(false), nil
	case "hubnet-message", "hubnet-message-source", "hubnet-message-tag":
		return Str(""), nil
	case "hubnet-clients-list":
		return List(), nil
	case "mouse-down?", "mouse-inside?":
		return Bool(false), nil
	case "mouse-xcor", "mouse-ycor":
		return Num(0), nil
	case "both-ends":
		l, ok := vm.self.(*Link)
		if !ok {
			return Value{}, errorAt(pos, "both-ends can only be used by a link")
		}
		return SetVal(&AgentSet{Kind: KindTurtle, Agents: []Agent{l.End1, l.End2}}), nil
	case "my-links":
		t, ok := vm.self.(*Turtle)
		if !ok {
			return Value{}, errorAt(pos, "my-links can only be used by a turtle")
		}
		ls := w.LinksOf(t)
		ags := make([]Agent, len(ls))
		for i, l := range ls {
			ags[i] = l
		}
		return SetVal(&AgentSet{Kind: KindLink, Agents: ags}), nil
	case "my-in-links":
		t, ok := vm.self.(*Turtle)
		if !ok {
			return Value{}, errorAt(pos, "my-in-links can only be used by a turtle")
		}
		var ags []Agent
		for _, l := range w.LinksOf(t) {
			if l.Directed && l.End2 == t {
				ags = append(ags, l)
			}
		}
		return SetVal(&AgentSet{Kind: KindLink, Agents: ags}), nil
	case "my-out-links":
		t, ok := vm.self.(*Turtle)
		if !ok {
			return Value{}, errorAt(pos, "my-out-links can only be used by a turtle")
		}
		var ags []Agent
		for _, l := range w.LinksOf(t) {
			if l.Directed && l.End1 == t {
				ags = append(ags, l)
			}
		}
		return SetVal(&AgentSet{Kind: KindLink, Agents: ags}), nil
	case "link-heading":
		l, ok := vm.self.(*Link)
		if !ok {
			return Value{}, errorAt(pos, "link-heading can only be used by a link")
		}
		h, err := w.HeadingTowards(l.End1.X, l.End1.Y, l.End2.X, l.End2.Y)
		if err != nil {
			return Value{}, errorAt(pos, "%s", err.Error())
		}
		return Num(h), nil
	case "shapes":
		lib := w.Shapes
		if lib == nil || lib.Len() == 0 {
			lib = DefaultShapes()
		}
		all := lib.All()
		vals := make([]Value, len(all))
		for i, sh := range all {
			vals[i] = Str(sh.Name)
		}
		return List(vals...), nil
	case "no-turtles":
		return SetVal(&AgentSet{Kind: KindTurtle, Agents: nil}), nil
	case "no-patches":
		return SetVal(&AgentSet{Kind: KindPatch, Agents: nil}), nil
	case "no-links":
		return SetVal(&AgentSet{Kind: KindLink, Agents: nil}), nil
	case "e":
		return Num(math.E), nil
	case "behaviorspace-run-number":
		return Num(0), nil
	case "link-neighbors":
		return vm.linkNeighborSet(0, pos)
	case "out-link-neighbors":
		return vm.linkNeighborSet(1, pos)
	case "in-link-neighbors":
		return vm.linkNeighborSet(-1, pos)
	default:
		if c, ok := colorNames[name]; ok {
			return Num(c), nil
		}
		if v, ok, err := vm.extZero(name, pos); ok {
			return v, err
		}
		return Value{}, errorAt(pos, "nothing named %s has been defined", strings.ToUpper(name))
	}
}

func (vm *VM) linkNeighborSet(dir int, pos Pos) (Value, error) {
	t, ok := vm.self.(*Turtle)
	if !ok {
		return Value{}, errorAt(pos, "link neighbors can only be used by a turtle")
	}
	ns := vm.World.LinkNeighbors(t, dir)
	ags := make([]Agent, len(ns))
	for i, n := range ns {
		ags[i] = n
	}
	return SetVal(&AgentSet{Kind: KindTurtle, Agents: ags}), nil
}

func turtleSet(ts []*Turtle) *AgentSet {
	ags := make([]Agent, len(ts))
	for i, t := range ts {
		ags[i] = t
	}
	return &AgentSet{Kind: KindTurtle, Agents: ags}
}

func patchSet(ps []*Patch) *AgentSet {
	ags := make([]Agent, len(ps))
	for i, p := range ps {
		ags[i] = p
	}
	return &AgentSet{Kind: KindPatch, Agents: ags}
}

func (vm *VM) evalCall(c *callExpr) (Value, error) {
	switch c.Name {
	case "ifelse-value":
		return vm.evalIfelseValue(c)
	case "all?", "max-one-of", "min-one-of", "max-n-of", "min-n-of", "sort-on":
		return vm.evalAgentBlockRep(c)
	case "one-of":
		if len(c.Args) == 1 {
			if w, ok := c.Args[0].(*withExpr); ok {
				return vm.oneOfWith(w)
			}
		}
	}
	if proc := vm.Prog.Proc(c.Name); proc != nil && proc.IsReporter {
		args := vm.evalAll(c.Args, c.Pos)
		return vm.callRep(proc, args)
	}
	args := vm.evalAll(c.Args, c.Pos)
	if vm.World != nil && len(vm.World.breedCalls) > 0 {
		if call, ok := vm.World.breedCalls[c.Name]; ok {
			return vm.evalBreedCallFast(call, c.Name, args, c.Pos)
		}
	}
	v, err := vm.evalReporter(c.Name, args, c.Pos)
	if err != nil {
		return Value{}, err
	}
	return v, nil
}

func (vm *VM) evalIfelseValue(c *callExpr) (Value, error) {
	if len(c.Args) < 2 {
		return Value{}, errorAt(c.Pos, "ifelse-value expected at least 2 inputs")
	}
	// pairs of (cond, value) then a final else. Non-variadic is cond, a, b.
	args := c.Args
	for len(args) > 1 {
		cond, err := vm.eval(args[0])
		if err != nil {
			return Value{}, err
		}
		t, err := cond.Truth()
		if err != nil {
			return Value{}, err
		}
		if t {
			return vm.eval(args[1])
		}
		if len(args) == 3 {
			return vm.eval(args[2])
		}
		args = args[2:]
	}
	if len(args) == 1 {
		return vm.eval(args[0])
	}
	return Value{}, errorAt(c.Pos, "ifelse-value: no conditions matched and no else value provided")
}

func (vm *VM) evalAgentBlockRep(c *callExpr) (Value, error) {
	if c.Name == "min-n-of" || c.Name == "max-n-of" {
		nV, err := vm.eval(c.Args[0])
		if err != nil {
			return Value{}, err
		}
		n, err := asInt(nV, c.Name)
		if err != nil {
			return Value{}, err
		}
		if n < 0 {
			return Value{}, fmt.Errorf("%s expected a non-negative number", c.Name)
		}
		target, err := vm.eval(c.Args[1])
		if err != nil {
			return Value{}, err
		}
		ags, err := vm.asAgentset(target)
		if err != nil {
			return Value{}, err
		}
		live := ags.Alive()
		if n > len(live) {
			return Value{}, fmt.Errorf("%s asked for more agents than the agentset contains", c.Name)
		}
		block := c.Args[2]
		type scoredAgent struct {
			a Agent
			k Value
		}
		// NetLogo scores agents in shufflerator order and keeps that order
		// within a tie, which is how ties break randomly.
		items := make([]scoredAgent, 0, len(live))
		it := vm.World.shuffleSet(ags)
		for {
			a, ok := it.Next()
			if !ok {
				break
			}
			if a.IsDead() {
				continue
			}
			items = append(items, scoredAgent{a: a, k: vm.score(a, block, c.Pos)})
		}
		var sortErr error
		sort.SliceStable(items, func(i, j int) bool {
			cmp, err := compare(items[i].k, items[j].k)
			if err != nil {
				sortErr = err
				return false
			}
			if c.Name == "max-n-of" {
				return cmp > 0
			}
			return cmp < 0
		})
		if sortErr != nil {
			return Value{}, sortErr
		}
		outAgs := make([]Agent, n)
		for i := 0; i < n; i++ {
			outAgs[i] = items[i].a
		}
		return SetVal(&AgentSet{Kind: ags.Kind, Breed: ags.Breed, Agents: outAgs}), nil
	}

	targetExpr := c.Args[0]
	block := c.Args[1]
	// sort-on is [reporter] agentset, the reverse of with and n-of.
	if c.Name == "sort-on" {
		targetExpr = c.Args[1]
		block = c.Args[0]
	}
	target, err := vm.eval(targetExpr)
	if err != nil {
		return Value{}, err
	}
	switch c.Name {
	case "all?":
		ags, err := vm.asAgentset(target)
		if err != nil {
			return Value{}, err
		}
		for _, a := range ags.Alive() {
			v := vm.withAgent(a, func() Value {
				val, err := vm.eval(block)
				if err != nil {
					vm.fail(c.Pos, "%s", err.Error())
				}
				return val
			})
			b, err := v.Truth()
			if err != nil {
				return Value{}, err
			}
			if !b {
				return Bool(false), nil
			}
		}
		return Bool(true), nil
	case "max-one-of", "min-one-of":
		ags, err := vm.asAgentset(target)
		if err != nil {
			return Value{}, err
		}
		live := ags.Alive()
		if len(live) == 0 {
			return Nobody(), nil
		}
		// NetLogo collects every agent tied for the best value, in
		// iteration order, then picks one with nextInt(winners). It draws
		// even when there is a single winner.
		winners := []Agent{live[0]}
		bestV := vm.score(live[0], block, c.Pos)
		for _, a := range live[1:] {
			sv := vm.score(a, block, c.Pos)
			cmp, err := compare(sv, bestV)
			if err != nil {
				return Value{}, err
			}
			if c.Name == "min-one-of" {
				cmp = -cmp
			}
			if cmp > 0 {
				winners, bestV = append(winners[:0], a), sv
			} else if cmp == 0 {
				winners = append(winners, a)
			}
		}
		return AgentVal(winners[vm.World.nextInt(len(winners))]), nil
	case "sort-on":
		type pair struct {
			a Agent
			k Value
		}
		var ps []pair
		if target.K == KindAgentset {
			// NetLogo scores agents in shufflerator order, then stable
			// sorts, so ties break randomly with NetLogo's draws.
			it := vm.World.shuffleSet(target.Set)
			for {
				a, ok := it.Next()
				if !ok {
					break
				}
				if a.IsDead() {
					continue
				}
				ps = append(ps, pair{a, vm.score(a, block, c.Pos)})
			}
		} else if target.K == KindList {
			for _, it := range target.L {
				if it.K != KindAgent {
					return Value{}, fmt.Errorf("sort-on expected agents")
				}
				ps = append(ps, pair{it.A, vm.score(it.A, block, c.Pos)})
			}
		} else {
			return Value{}, fmt.Errorf("sort-on expected an agentset or list")
		}
		var sortErr error
		sort.SliceStable(ps, func(i, j int) bool {
			c, err := compare(ps[i].k, ps[j].k)
			if err != nil {
				sortErr = err
				return false
			}
			return c < 0
		})
		if sortErr != nil {
			return Value{}, sortErr
		}
		out := make([]Value, len(ps))
		for i, p := range ps {
			out[i] = AgentVal(p.a)
		}
		return List(out...), nil
	default:
		return Value{}, fmt.Errorf("primitive %s is not implemented yet", c.Name)
	}
}

func (vm *VM) score(a Agent, block Expr, pos Pos) Value {
	return vm.withAgent(a, func() Value {
		v, err := vm.eval(block)
		if err != nil {
			vm.fail(pos, "%s", err.Error())
		}
		return v
	})
}

func (vm *VM) withAgent(a Agent, fn func() Value) Value {
	vm.pushAgent(a)
	defer vm.popAgent()
	return fn()
}

func (vm *VM) evalBreedCallFast(call breedCallDef, name string, args []Value, pos Pos) (Value, error) {
	switch call.kind {
	case breedCallLinkNeighbor:
		if len(args) == 1 {
			return vm.isLinkNeighbor(name, call.dir, call.breed, args)
		}
	case breedCallIsBreed:
		if len(args) != 1 || args[0].K != KindAgent {
			return Bool(false), nil
		}
		t, ok := args[0].A.(*Turtle)
		if !ok || t.dead {
			return Bool(false), nil
		}
		return Bool(t.Breed == call.breed), nil
	case breedCallOn:
		if len(args) >= 1 {
			ps, err := vm.agentPatches(args[0])
			if err != nil {
				return Value{}, err
			}
			var ags []Agent
			for _, p := range ps {
				for _, t := range vm.World.TurtlesHere(p) {
					if t.Breed == call.breed {
						ags = append(ags, t)
					}
				}
			}
			return SetVal(&AgentSet{Kind: KindTurtle, Breed: call.breed, Agents: ags}), nil
		}
	case breedCallAt:
		if len(args) >= 2 {
			dx, err := args[0].Num()
			if err != nil {
				return Value{}, err
			}
			dy, err := args[1].Num()
			if err != nil {
				return Value{}, err
			}
			dz := 0.0
			if len(args) >= 3 {
				if dz, err = args[2].Num(); err != nil {
					return Value{}, err
				}
			}
			p := vm.patchAtOffset3(dx, dy, dz)
			var ags []Agent
			if p != nil {
				for _, t := range vm.World.TurtlesHere(p) {
					if t.Breed == call.breed {
						ags = append(ags, t)
					}
				}
			}
			return SetVal(&AgentSet{Kind: KindTurtle, Breed: call.breed, Agents: ags}), nil
		}
	case breedCallSingular:
		if len(args) == 1 {
			id, err := asInt(args[0], name)
			if err != nil {
				return Value{}, err
			}
			t := vm.World.Turtle(id)
			if t == nil || t.dead || t.Breed != call.breed {
				return Nobody(), nil
			}
			return AgentVal(t), nil
		}
	case breedCallLinkSingular:
		if len(args) == 2 {
			t1, err := vm.turtleFromArg(args[0], name)
			if err != nil {
				return Value{}, err
			}
			t2, err := vm.turtleFromArg(args[1], name)
			if err != nil {
				return Value{}, err
			}
			if t1 == nil || t2 == nil || t1 == t2 {
				return Nobody(), nil
			}
			l := vm.World.findLinkBreed(t1, t2, call.breed, call.directed)
			if l == nil {
				return Nobody(), nil
			}
			return AgentVal(l), nil
		}
	case breedCallLinkWith:
		if len(args) == 1 {
			self, ok := vm.self.(*Turtle)
			if !ok {
				return Value{}, errorAt(pos, "%s can only be used by a turtle", name)
			}
			other, err := vm.turtleFromArg(args[0], name)
			if err != nil {
				return Value{}, err
			}
			if other == nil || self == other {
				return Nobody(), nil
			}
			l := vm.World.findLinkBreed(self, other, call.breed, false)
			if l == nil {
				return Nobody(), nil
			}
			return AgentVal(l), nil
		}
	case breedCallInLinkFrom:
		if len(args) == 1 {
			self, ok := vm.self.(*Turtle)
			if !ok {
				return Value{}, errorAt(pos, "%s can only be used by a turtle", name)
			}
			other, err := vm.turtleFromArg(args[0], name)
			if err != nil {
				return Value{}, err
			}
			if other == nil || self == other {
				return Nobody(), nil
			}
			l := vm.World.findLinkBreed(other, self, call.breed, true)
			if l == nil {
				return Nobody(), nil
			}
			return AgentVal(l), nil
		}
	case breedCallOutLinkTo:
		if len(args) == 1 {
			self, ok := vm.self.(*Turtle)
			if !ok {
				return Value{}, errorAt(pos, "%s can only be used by a turtle", name)
			}
			other, err := vm.turtleFromArg(args[0], name)
			if err != nil {
				return Value{}, err
			}
			if other == nil || self == other {
				return Nobody(), nil
			}
			l := vm.World.findLinkBreed(self, other, call.breed, true)
			if l == nil {
				return Nobody(), nil
			}
			return AgentVal(l), nil
		}
	case breedCallMyLinks:
		t, ok := vm.self.(*Turtle)
		if !ok {
			return Value{}, errorAt(pos, "%s can only be used by a turtle", name)
		}
		ls := vm.World.LinksOf(t)
		var ags []Agent
		for _, l := range ls {
			if l.Breed != call.breed {
				continue
			}
			if call.dir == 1 && (l.End1 != t || !l.Directed) {
				continue
			}
			if call.dir == -1 && (l.End2 != t || !l.Directed) {
				continue
			}
			ags = append(ags, l)
		}
		return SetVal(&AgentSet{Kind: KindLink, Breed: call.breed, Agents: ags}), nil
	}
	return vm.evalReporter(name, args, pos)
}

func (vm *VM) singularBreed(sg string) (string, bool) {
	for pl, b := range vm.World.breeds {
		if b.Singular == sg {
			return pl, true
		}
	}
	return "", false
}
