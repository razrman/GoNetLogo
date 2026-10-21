package nl

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

func asInt(v Value, what string) (int, error) {
	n, err := v.Num()
	if err != nil {
		return 0, fmt.Errorf("%s expected a number, got %s", what, v.TypeName())
	}
	if math.IsNaN(n) || math.IsInf(n, 0) || n != math.Trunc(n) {
		return 0, fmt.Errorf("%s expected an integer", what)
	}
	if n > 1e12 || n < -1e12 {
		return 0, fmt.Errorf("%s: number is too large", what)
	}
	return int(n), nil
}

// moveTurtle is the shared fd/bk/jump implementation. jumpAll moves the
// whole distance and stays put when that would leave the world. Otherwise
// the turtle walks in unit steps and stops at the last legal position.
func (vm *VM) moveTurtle(t *Turtle, dist float64, jumpAll bool) {
	if jumpAll {
		_ = vm.World.Jump(t, dist)
		return
	}
	_ = vm.World.Forward(t, dist)
}

func (vm *VM) facePoint(t *Turtle, x, y float64) {
	h, err := vm.World.HeadingTowards(t.X, t.Y, x, y)
	if err != nil {
		return
	}
	vm.World.SetHeading(t, h)
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
	switch name {
	case "clear-all", "ca":
		vm.requireObserver(pos, "clear-all")
		w.ClearAll()
		vm.clearOutput()
		vm.clearAllPlots()
	case "clear-turtles", "ct":
		vm.requireObserver(pos, "clear-turtles")
		w.ClearTurtles()
	case "clear-patches", "cp":
		vm.requireObserver(pos, "clear-patches")
		w.ClearPatches()
	case "clear-globals":
		vm.requireObserver(pos, "clear-globals")
		w.ClearGlobals()
	case "clear-links":
		vm.requireObserver(pos, "clear-links")
		w.ClearLinks()
	case "clear-ticks":
		vm.requireObserver(pos, "clear-ticks")
		w.ClearTicks()
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
	case "auto-plot-on":
		return vm.setAutoPlot(true)
	case "auto-plot-off":
		return vm.setAutoPlot(false)
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
		n, err := asInt(args[0], "random-seed")
		if err != nil {
			return err
		}
		w.Seed(int64(n))
	case "die":
		if vm.self == nil {
			return fmt.Errorf("only turtles and links can die")
		}
		if _, ok := vm.self.(*Patch); ok {
			return fmt.Errorf("patches cannot die")
		}
		w.Die(vm.self)
		panic(diePanic{})
	case "fd", "forward":
		n, err := args[0].Num()
		if err != nil {
			return err
		}
		vm.moveTurtle(vm.requireTurtle(pos), n, false)
	case "bk", "back":
		n, err := args[0].Num()
		if err != nil {
			return err
		}
		vm.moveTurtle(vm.requireTurtle(pos), -n, false)
	case "jump":
		n, err := args[0].Num()
		if err != nil {
			return err
		}
		vm.moveTurtle(vm.requireTurtle(pos), n, true)
	case "move-to":
		return vm.moveToAgent(args[0], pos)
	case "rt", "right":
		n, err := args[0].Num()
		if err != nil {
			return err
		}
		w.Turn(vm.requireTurtle(pos), n)
	case "lt", "left":
		n, err := args[0].Num()
		if err != nil {
			return err
		}
		w.Turn(vm.requireTurtle(pos), -n)
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
		vm.facePoint(t, x, y)
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
		vm.facePoint(t, x, y)
	case "pen-down", "pd":
		vm.requireTurtle(pos).PenDown = true
	case "pen-up", "pu":
		vm.requireTurtle(pos).PenDown = false
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
		stmts, err := ParseCommands(v.S, vm.Prog)
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

func (vm *VM) layoutCircle(agents, radius Value) error {
	if agents.K != KindAgentset {
		return fmt.Errorf("layout-circle expected a turtle agentset")
	}
	r, err := radius.Num()
	if err != nil {
		return err
	}
	live := agents.Set.Alive()
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
		vm.World.SetHeading(t, h)
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
			t, ok := a.(*Turtle)
			if !ok {
				vm.fail(pos, "%s expected turtles", name)
			}
			others = append(others, t)
		}
	} else {
		t, ok := args[0].A.(*Turtle)
		if args[0].K != KindAgent || !ok {
			vm.fail(pos, "%s expected a turtle", name)
		}
		others = []*Turtle{t}
	}
	for _, other := range others {
		a, b := self, other
		if from {
			a, b = other, self
		}
		l, created, err := vm.World.CreateLink(a, b, directed, breed)
		if err != nil {
			vm.fail(pos, "%s", err.Error())
		}
		if !created {
			vm.fail(pos, "there is already a link between those turtles")
		}
		if block != nil {
			vm.execAgentBlock(l, block)
		}
	}
}

func (vm *VM) evalZero(name string, pos Pos) (Value, error) {
	w := vm.World
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
	case "all?", "max-one-of", "min-one-of", "sort-on":
		return vm.evalAgentBlockRep(c)
	}
	if proc := vm.Prog.Proc(c.Name); proc != nil && proc.IsReporter {
		args := vm.evalAll(c.Args, c.Pos)
		return vm.callRep(proc, args)
	}
	if strings.HasSuffix(c.Name, "-on") || strings.HasSuffix(c.Name, "-at") || strings.HasPrefix(c.Name, "is-") {
		args := vm.evalAll(c.Args, c.Pos)
		if v, ok, err := vm.evalBreedCall(c.Name, args, c.Pos); ok || err != nil {
			return v, err
		}
	}
	args := vm.evalAll(c.Args, c.Pos)
	v, err := vm.evalReporter(c.Name, args, c.Pos)
	if err != nil {
		return Value{}, err
	}
	return v, nil
}

func (vm *VM) evalIfelseValue(c *callExpr) (Value, error) {
	if len(c.Args) < 3 {
		return Value{}, errorAt(c.Pos, "ifelse-value expected at least 3 inputs")
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
	return Value{}, errorAt(c.Pos, "ifelse-value expected an else value")
}

func (vm *VM) evalAgentBlockRep(c *callExpr) (Value, error) {
	target, err := vm.eval(c.Args[0])
	if err != nil {
		return Value{}, err
	}
	block := c.Args[1]
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
		best := live[0]
		bestV := vm.score(best, block, c.Pos)
		for _, a := range live[1:] {
			sv := vm.score(a, block, c.Pos)
			cmp, err := compare(sv, bestV)
			if err != nil {
				return Value{}, err
			}
			if c.Name == "max-one-of" && cmp > 0 || c.Name == "min-one-of" && cmp < 0 {
				best, bestV = a, sv
			}
		}
		return AgentVal(best), nil
	case "sort-on":
		var agents []Agent
		if target.K == KindAgentset {
			agents = target.Set.Alive()
		} else if target.K == KindList {
			for _, it := range target.L {
				if it.K != KindAgent {
					return Value{}, fmt.Errorf("sort-on expected agents")
				}
				agents = append(agents, it.A)
			}
		} else {
			return Value{}, fmt.Errorf("sort-on expected an agentset or list")
		}
		type pair struct {
			a Agent
			k Value
		}
		ps := make([]pair, len(agents))
		for i, a := range agents {
			ps[i] = pair{a, vm.score(a, block, c.Pos)}
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

func (vm *VM) evalBreedCall(name string, args []Value, pos Pos) (Value, bool, error) {
	if strings.HasPrefix(name, "is-") && strings.HasSuffix(name, "?") {
		sg := strings.TrimSuffix(strings.TrimPrefix(name, "is-"), "?")
		if pl, ok := vm.singularBreed(sg); ok {
			if len(args) != 1 || args[0].K != KindAgent {
				return Bool(false), true, nil
			}
			t, ok := args[0].A.(*Turtle)
			if !ok || t.dead {
				return Bool(false), true, nil
			}
			return Bool(t.Breed == pl), true, nil
		}
	}
	if strings.HasSuffix(name, "-on") {
		br := strings.TrimSuffix(name, "-on")
		if _, ok := vm.World.breeds[br]; ok {
			p, err := vm.agentPatch(args[0])
			if err != nil {
				return Value{}, true, err
			}
			var ags []Agent
			if p != nil {
				for _, t := range vm.World.TurtlesHere(p) {
					if t.Breed == br {
						ags = append(ags, t)
					}
				}
			}
			return SetVal(&AgentSet{Kind: KindTurtle, Breed: br, Agents: ags}), true, nil
		}
	}
	if strings.HasSuffix(name, "-at") {
		br := strings.TrimSuffix(name, "-at")
		if _, ok := vm.World.breeds[br]; ok {
			dx, err := args[0].Num()
			if err != nil {
				return Value{}, true, err
			}
			dy, err := args[1].Num()
			if err != nil {
				return Value{}, true, err
			}
			p := vm.patchAtOffset(dx, dy)
			var ags []Agent
			if p != nil {
				for _, t := range vm.World.TurtlesHere(p) {
					if t.Breed == br {
						ags = append(ags, t)
					}
				}
			}
			return SetVal(&AgentSet{Kind: KindTurtle, Breed: br, Agents: ags}), true, nil
		}
	}
	return Value{}, false, nil
}

func (vm *VM) singularBreed(sg string) (string, bool) {
	for pl, b := range vm.World.breeds {
		if b.Singular == sg {
			return pl, true
		}
	}
	return "", false
}
