package nl

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

// scope is a lexical environment. Let bindings live in the innermost map
// and are visible to parent blocks in the same procedure, not to called
// procedures.
type scope struct {
	vars   map[string]Value
	parent *scope
	// born is VM.tasks at allocation. A task that closes over this scope
	// bumps tasks, and the scope is then not reused.
	born int
}

func (s *scope) get(name string) (Value, bool) {
	for s != nil {
		if v, ok := s.vars[name]; ok {
			return v, true
		}
		s = s.parent
	}
	return Value{}, false
}

func (s *scope) set(name string, v Value) bool {
	for s != nil {
		if _, ok := s.vars[name]; ok {
			s.vars[name] = v
			return true
		}
		s = s.parent
	}
	return false
}

func (s *scope) let(name string, v Value) error {
	if s == nil {
		return fmt.Errorf("let is not allowed here")
	}
	if _, ok := s.vars[name]; ok {
		return fmt.Errorf("there is already a local variable here called %s", strings.ToUpper(name))
	}
	s.vars[name] = v
	return nil
}

func (vm *VM) allocScope(parent *scope) *scope {
	var s *scope
	if n := len(vm.scopePool); n > 0 {
		s = vm.scopePool[n-1]
		vm.scopePool = vm.scopePool[:n-1]
		for k := range s.vars {
			delete(s.vars, k)
		}
		if s.vars == nil {
			s.vars = map[string]Value{}
		}
	} else {
		s = &scope{vars: map[string]Value{}}
	}
	s.parent = parent
	s.born = vm.tasks
	return s
}

func (vm *VM) freeScope(s *scope) {
	if s == nil || s.born != vm.tasks {
		return
	}
	s.parent = nil
	vm.scopePool = append(vm.scopePool, s)
}

type stopPanic struct{}
type diePanic struct{}
type reportPanic struct{ v Value }

// interruptPanic aborts the button that is running. It is not NetLogo
// stop: ask and called procedures do not catch it. The top-level run
// turns it into a stopped button so a UI stop can cut off a long go.
type interruptPanic struct{}

// VM executes a compiled program against a world.
type VM struct {
	World   *World
	Prog    *Program
	scope   *scope
	self    Agent
	askers  []Agent
	lines   []string
	partial string
	OnPrint func(string)
	// OnClear runs when clear-output or clear-all clears the command center
	// because the model has no output widget.
	OnClear func()
	// hasOutput routes output-* to the output area. Without a widget those
	// commands share the command center, matching NetLogo.
	hasOutput     bool
	outputLines   []string
	outputPartial string
	outputEpoch   int
	outputSeq     int
	// DisplayOn mirrors no-display / display. The GUI may skip frames when false.
	DisplayOn bool
	timer     time.Time
	scopePool []*scope
	tasks     int
	plots     *plotMgr

	// halt is set by the UI. The interpreter polls it between commands.
	halt atomic.Bool
	// yield, when set, runs on the frame cadence with the world lock released.
	yield      func()
	yieldEvery time.Duration
	lastYield  time.Time
	stepN      uint32
	// external counts DoLocked callers. Checkpoint does not yield or halt
	// while a snapshot or a monitor is reading the world.
	external int
	// askBuf collects one breed for ask without allocating a fresh agentset.
	askBuf []Agent
	// runCache remembers run strings that are not a bare command name.
	runCache map[string][]Stmt
}

// NewVM builds a VM for a program. The world is resized only when spec is non-nil.
func NewVM(prog *Program, w *World) *VM {
	if w == nil {
		w = NewWorld(-16, 16, -16, 16, true, true)
	}
	if prog != nil {
		w.SetProgramVars(prog)
	}
	return &VM{World: w, Prog: prog, DisplayOn: true, timer: time.Now()}
}

// OutputLineCap is how many completed output-area lines are retained.
const OutputLineCap = 2000

// Output returns lines produced by print, show, and user-message.
func (vm *VM) Output() []string { return append([]string(nil), vm.lines...) }

// SetHasOutput selects the output area for output-* commands.
// Clear-all and clear-output then clear that area instead of the command center.
func (vm *VM) SetHasOutput(on bool) { vm.hasOutput = on }

// HasOutput reports whether output-* writes to the output area.
func (vm *VM) HasOutput() bool { return vm.hasOutput }

// OutputArea returns completed output-area lines and the unfinished tail
// from output-type / output-write. The line slice is a copy capped at
// OutputLineCap.
func (vm *VM) OutputArea() (lines []string, partial string) {
	lines = append([]string{}, vm.outputLines...)
	if lines == nil {
		lines = []string{}
	}
	return lines, vm.outputPartial
}

// OutputText is the output area as one string, including an unfinished line.
func (vm *VM) OutputText() string {
	if len(vm.outputLines) == 0 {
		return vm.outputPartial
	}
	body := strings.Join(vm.outputLines, "\n")
	if vm.outputPartial != "" {
		body += "\n" + vm.outputPartial
	}
	return body
}

// OutputDelta returns output-area lines the caller has not seen.
// clientEpoch and clientSeq are the epoch and completed-line count from
// the caller's previous update. A mismatched epoch, a cursor past the
// current count, or a cursor older than the retained window sets reset
// and returns every retained line.
func (vm *VM) OutputDelta(clientEpoch, clientSeq int) (reset bool, epoch, seq int, lines []string, partial string) {
	epoch = vm.outputEpoch
	seq = vm.outputSeq
	partial = vm.outputPartial
	full := func() []string {
		lines := append([]string{}, vm.outputLines...)
		if lines == nil {
			return []string{}
		}
		return lines
	}
	if clientEpoch != epoch || clientSeq > seq {
		return true, epoch, seq, full(), partial
	}
	if clientSeq == seq {
		return false, epoch, seq, []string{}, partial
	}
	n := len(vm.outputLines)
	if n == 0 {
		return true, epoch, seq, []string{}, partial
	}
	first := seq - n + 1
	if clientSeq+1 < first {
		return true, epoch, seq, full(), partial
	}
	idx := clientSeq - first + 1
	if idx < 0 {
		idx = 0
	}
	if idx > n {
		idx = n
	}
	lines = append([]string{}, vm.outputLines[idx:]...)
	if lines == nil {
		lines = []string{}
	}
	return false, epoch, seq, lines, partial
}

// Call runs a command procedure. stopped is true when that procedure
// executed stop, which is how a forever button knows to halt.
func (vm *VM) Call(name string, args ...Value) (stopped bool, err error) {
	vm.World.mu.Lock()
	defer vm.World.unlockPublish()
	defer vm.finishInterrupt(&stopped)
	stopped, err = vm.call(name, args...)
	return stopped, err
}

func (vm *VM) call(name string, args ...Value) (bool, error) {
	proc := vm.Prog.Proc(name)
	if proc == nil {
		return false, fmt.Errorf("nothing named %s has been defined", strings.ToUpper(name))
	}
	if proc.IsReporter {
		return false, fmt.Errorf("%s is a reporter, not a command", name)
	}
	return vm.callCmd(proc, args)
}

// CallReporter runs a to-report procedure.
func (vm *VM) CallReporter(name string, args ...Value) (v Value, err error) {
	vm.World.mu.Lock()
	defer vm.World.unlockPublish()
	defer vm.finishInterruptErr(&err)
	proc := vm.Prog.Proc(name)
	if proc == nil || !proc.IsReporter {
		return Value{}, fmt.Errorf("nothing named %s has been defined as a reporter", strings.ToUpper(name))
	}
	return vm.callRep(proc, args)
}

// RunStatements executes commands in the observer context.
func (vm *VM) RunStatements(stmts []Stmt) (err error) {
	vm.World.mu.Lock()
	defer vm.World.unlockPublish()
	defer vm.finishInterruptErr(&err)
	return vm.runStatements(stmts)
}

func (vm *VM) runStatements(stmts []Stmt) (err error) {
	defer vm.recover(&err, nil)
	vm.execStmts(stmts)
	return nil
}

// Eval evaluates one reporter in the current context.
func (vm *VM) Eval(e Expr) (v Value, err error) {
	vm.World.mu.Lock()
	defer vm.World.unlockPublish()
	defer vm.finishInterruptErr(&err)
	return vm.evalTop(e)
}

func (vm *VM) evalTop(e Expr) (v Value, err error) {
	defer vm.recover(&err, nil)
	return vm.eval(e)
}

// ExecuteLine runs one command-center line. A reporter line returns its printed form.
func (vm *VM) ExecuteLine(src string) (out string, err error) {
	vm.World.mu.Lock()
	defer vm.World.unlockPublish()
	defer vm.finishInterruptErr(&err)
	return vm.executeLine(src)
}

func (vm *VM) executeLine(src string) (string, error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return "", nil
	}
	if isCommandLine(src, vm.Prog) {
		stmts, err := ParseCommands(src, vm.Prog)
		if err != nil {
			return "", err
		}
		if err := vm.runStatements(stmts); err != nil {
			return "", err
		}
		return "", nil
	}
	e, err := ParseReporter(src, vm.Prog)
	if err != nil {
		return "", err
	}
	v, err := vm.evalTop(e)
	if err != nil {
		return "", err
	}
	return v.Format(false), nil
}

func isCommandLine(src string, prog *Program) bool {
	toks, err := cleanTokens(src)
	if err != nil || len(toks) == 0 {
		return false
	}
	t := toks[0]
	if t.Kind == TokLParen && len(toks) > 1 {
		t = toks[1]
	}
	if t.Kind != TokIdent {
		return false
	}
	name := t.Lit
	switch name {
	case "let", "set", "if", "ifelse", "while", "repeat", "loop", "ask", "foreach",
		"report", "stop", "create-turtles", "crt", "create-ordered-turtles", "cro",
		"sprout", "hatch":
		return true
	}
	if _, ok := commands[name]; ok {
		return true
	}
	if prog != nil {
		if pr := prog.Proc(name); pr != nil && !pr.IsReporter {
			return true
		}
		for _, b := range prog.Breeds {
			if name == "create-"+b.Plural || name == "create-ordered-"+b.Plural ||
				name == "sprout-"+b.Plural || name == "hatch-"+b.Plural {
				return true
			}
		}
	}
	return false
}

func (vm *VM) recover(errp *error, stopped *bool) {
	if r := recover(); r != nil {
		switch e := r.(type) {
		case *rtError:
			*errp = e
		case rtError:
			*errp = &e
		case stopPanic:
			if stopped != nil {
				*stopped = true
			}
		case diePanic:
			// died outside an ask; the agent is already gone
		case reportPanic:
			*errp = fmt.Errorf("REPORT can only be used inside TO-REPORT")
		default:
			panic(r)
		}
	}
}

func (vm *VM) fail(pos Pos, format string, args ...any) {
	panic(&rtError{msg: fmt.Sprintf(format, args...), pos: pos})
}

func (vm *VM) callCmd(proc *Procedure, args []Value) (stopped bool, err error) {
	if len(args) != len(proc.Params) {
		return false, fmt.Errorf("%s expected %d inputs", proc.Name, len(proc.Params))
	}
	vm.ensureCompiled()
	defer vm.recover(&err, &stopped)
	saved := vm.scope
	var sc *scope
	if proc.cmdFn != nil && !proc.needsScope {
		vm.scope = nil
	} else {
		sc = vm.allocScope(nil)
		vm.scope = sc
		for i, name := range proc.Params {
			vm.scope.vars[name] = args[i]
		}
	}
	defer func() {
		vm.scope = saved
		vm.freeScope(sc)
	}()
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(stopPanic); ok {
				stopped = true
				return
			}
			panic(r)
		}
	}()
	if proc.cmdFn != nil {
		proc.cmdFn(vm)
	} else {
		vm.execStmts(proc.Body)
	}
	return false, nil
}

func (vm *VM) callRep(proc *Procedure, args []Value) (v Value, err error) {
	if len(args) != len(proc.Params) {
		return Value{}, fmt.Errorf("%s expected %d inputs", proc.Name, len(proc.Params))
	}
	vm.ensureCompiled()
	defer vm.recover(&err, nil)
	saved := vm.scope
	var sc *scope
	if proc.cmdFn != nil && !proc.needsScope {
		vm.scope = nil
	} else {
		sc = vm.allocScope(nil)
		vm.scope = sc
		for i, name := range proc.Params {
			vm.scope.vars[name] = args[i]
		}
	}
	defer func() {
		vm.scope = saved
		vm.freeScope(sc)
	}()
	defer func() {
		if r := recover(); r != nil {
			if rp, ok := r.(reportPanic); ok {
				v = rp.v
				return
			}
			panic(r)
		}
	}()
	if proc.cmdFn != nil {
		proc.cmdFn(vm)
	} else {
		vm.execStmts(proc.Body)
	}
	return Value{}, errorAt(proc.Pos, "%s is a reporter and didn't report a value", proc.Name)
}

func (vm *VM) execStmts(stmts []Stmt) {
	for _, s := range stmts {
		vm.execStmt(s)
	}
}

func (vm *VM) execStmt(s Stmt) {
	switch s := s.(type) {
	case *cmdStmt:
		vm.execCmd(s)
	case *letStmt:
		vm.execLet(s)
	case *setStmt:
		vm.execSet(s)
	case *ifStmt:
		b, err := vm.eval(s.Cond)
		if err != nil {
			vm.fail(s.Pos, "%s", err.Error())
		}
		t, err := b.Truth()
		if err != nil {
			vm.fail(s.Pos, "%s", err.Error())
		}
		if t {
			vm.block(s.Body)
		}
	case *ifelseStmt:
		b, err := vm.eval(s.Cond)
		if err != nil {
			vm.fail(s.Pos, "%s", err.Error())
		}
		t, err := b.Truth()
		if err != nil {
			vm.fail(s.Pos, "%s", err.Error())
		}
		if t {
			vm.block(s.Then)
		} else {
			vm.block(s.Else)
		}
	case *whileStmt:
		for {
			b, err := vm.eval(s.Cond)
			if err != nil {
				vm.fail(s.Pos, "%s", err.Error())
			}
			t, err := b.Truth()
			if err != nil {
				vm.fail(s.Pos, "%s", err.Error())
			}
			if !t {
				return
			}
			vm.checkpoint()
			vm.block(s.Body)
		}
	case *repeatStmt:
		n, err := vm.eval(s.Times)
		if err != nil {
			vm.fail(s.Pos, "%s", err.Error())
		}
		k, err := asInt(n, "repeat")
		if err != nil {
			vm.fail(s.Pos, "%s", err.Error())
		}
		if k < 0 {
			vm.fail(s.Pos, "repeat expected a non-negative number")
		}
		for i := 0; i < k; i++ {
			vm.checkpoint()
			vm.block(s.Body)
		}
	case *loopStmt:
		for {
			vm.checkpoint()
			vm.block(s.Body)
		}
	case *askStmt:
		vm.execAsk(s)
	case *createStmt:
		vm.execCreate(s)
	case *sproutStmt:
		vm.execSprout(s)
	case *hatchStmt:
		vm.execHatch(s)
	case *foreachStmt:
		vm.execForeach(s)
	case *reportStmt:
		v, err := vm.eval(s.Value)
		if err != nil {
			vm.fail(s.Pos, "%s", err.Error())
		}
		panic(reportPanic{v})
	case *stopStmt:
		panic(stopPanic{})
	default:
		vm.fail(s.at(), "unknown statement")
	}
}

// block runs commands in a fresh let-scope that can see the enclosing lets.
func (vm *VM) block(stmts []Stmt) {
	saved := vm.scope
	sc := vm.allocScope(saved)
	vm.scope = sc
	defer func() {
		vm.scope = saved
		vm.freeScope(sc)
	}()
	vm.execStmts(stmts)
}

func (vm *VM) execLet(s *letStmt) {
	v, err := vm.eval(s.Value)
	if err != nil {
		vm.fail(s.Pos, "%s", err.Error())
	}
	if len(s.Names) == 1 {
		if err := vm.scope.let(s.Names[0], v); err != nil {
			vm.fail(s.Pos, "%s", err.Error())
		}
		return
	}
	if v.K != KindList || len(v.L) != len(s.Names) {
		vm.fail(s.Pos, "let expected a list of %d values", len(s.Names))
	}
	for i, name := range s.Names {
		if err := vm.scope.let(name, v.L[i]); err != nil {
			vm.fail(s.Pos, "%s", err.Error())
		}
	}
}

func (vm *VM) execSet(s *setStmt) {
	v, err := vm.eval(s.Value)
	if err != nil {
		vm.fail(s.Pos, "%s", err.Error())
	}
	vals := []Value{v}
	if len(s.Names) > 1 {
		if v.K != KindList || len(v.L) != len(s.Names) {
			vm.fail(s.Pos, "set expected a list of %d values", len(s.Names))
		}
		vals = v.L
	}
	for i, name := range s.Names {
		if err := vm.assign(name, vals[i], s.Pos); err != nil {
			vm.fail(s.Pos, "%s", err.Error())
		}
	}
}

func (vm *VM) assign(name string, v Value, pos Pos) error {
	if vm.scope != nil && vm.scope.set(name, v) {
		return nil
	}
	if vm.self != nil {
		if ok, err := vm.setAgentVar(name, v); ok || err != nil {
			return err
		}
	}
	if vm.World.globals != nil {
		if _, ok := vm.World.globals[name]; ok {
			vm.World.globals[name] = v
			return nil
		}
	}
	return errorAt(pos, "this isn't something you can use SET on: %s", strings.ToUpper(name))
}

func (vm *VM) pushAgent(a Agent) {
	vm.askers = append(vm.askers, vm.self)
	vm.self = a
}

func (vm *VM) popAgent() {
	vm.self = vm.askers[len(vm.askers)-1]
	vm.askers = vm.askers[:len(vm.askers)-1]
}

// execAgentBlock runs commands as agent. stop and die end only this agent.
func (vm *VM) execAgentBlock(a Agent, body []Stmt) {
	if a == nil || a.IsDead() {
		return
	}
	vm.pushAgent(a)
	saved := vm.scope
	sc := vm.allocScope(saved)
	vm.scope = sc
	defer func() {
		vm.scope = saved
		vm.popAgent()
		vm.freeScope(sc)
	}()
	defer func() {
		if r := recover(); r != nil {
			switch r.(type) {
			case stopPanic, diePanic:
				return
			default:
				panic(r)
			}
		}
	}()
	vm.execStmts(body)
}

func (vm *VM) execAsk(s *askStmt) {
	v, err := vm.eval(s.Agents)
	if err != nil {
		vm.fail(s.Pos, "%s", err.Error())
	}
	var agents []Agent
	switch v.K {
	case KindAgent:
		if v.A == nil || v.A.IsDead() {
			vm.fail(s.Pos, "that agent is dead")
		}
		agents = []Agent{v.A}
	case KindAgentset:
		if vm.self != nil && v.Set != nil && v.Set.All && (v.Set.Kind == KindTurtle || v.Set.Kind == KindPatch) {
			vm.fail(s.Pos, "only the observer can ASK the set of all %ss", v.Set.Kind)
		}
		agents = vm.World.Shuffle(v.Set.Alive())
	default:
		vm.fail(s.Pos, "ask expected an agent or agentset, got %s", v.TypeName())
	}
	for _, a := range agents {
		if a.IsDead() {
			continue
		}
		vm.checkpoint()
		vm.execAgentBlock(a, s.Body)
	}
}

func (vm *VM) execCreate(s *createStmt) {
	if vm.self != nil {
		vm.fail(s.Pos, "only the observer can create turtles")
	}
	n, err := vm.eval(s.Count)
	if err != nil {
		vm.fail(s.Pos, "%s", err.Error())
	}
	k, err := asInt(n, "create-turtles")
	if err != nil {
		vm.fail(s.Pos, "%s", err.Error())
	}
	if k < 0 {
		vm.fail(s.Pos, "create-turtles expected a non-negative number")
	}
	var made []Agent
	for i := 0; i < k; i++ {
		var t *Turtle
		if s.Ordered {
			t = vm.World.CreateTurtle(s.Breed, 0, 0, 0, 0)
			t.Color = float64(10*i + 5)
			vm.World.SetHeading(t, 360*float64(i)/float64(k))
		} else {
			cn := vm.World.rng.Intn(14)
			h := vm.World.rng.Intn(360)
			t = vm.World.CreateTurtle(s.Breed, 0, 0, cn, h)
		}
		made = append(made, t)
	}
	for _, a := range made {
		vm.execAgentBlock(a, s.Body)
	}
}

func (vm *VM) execSprout(s *sproutStmt) {
	p, ok := vm.self.(*Patch)
	if !ok || p == nil {
		vm.fail(s.Pos, "sprout can only be used by a patch")
	}
	n, err := vm.eval(s.Count)
	if err != nil {
		vm.fail(s.Pos, "%s", err.Error())
	}
	k, err := asInt(n, "sprout")
	if err != nil {
		vm.fail(s.Pos, "%s", err.Error())
	}
	var made []Agent
	for i := 0; i < k; i++ {
		cn := vm.World.rng.Intn(14)
		h := vm.World.rng.Intn(360)
		t := vm.World.CreateTurtle(s.Breed, float64(p.PX), float64(p.PY), cn, h)
		made = append(made, t)
	}
	for _, a := range made {
		vm.execAgentBlock(a, s.Body)
	}
}

func (vm *VM) execHatch(s *hatchStmt) {
	parent, ok := vm.self.(*Turtle)
	if !ok || parent == nil || parent.dead {
		vm.fail(s.Pos, "hatch can only be used by a turtle")
	}
	n, err := vm.eval(s.Count)
	if err != nil {
		vm.fail(s.Pos, "%s", err.Error())
	}
	k, err := asInt(n, "hatch")
	if err != nil {
		vm.fail(s.Pos, "%s", err.Error())
	}
	var made []Agent
	for i := 0; i < k; i++ {
		made = append(made, vm.World.Hatch(parent, s.Breed))
	}
	for _, a := range made {
		vm.execAgentBlock(a, s.Body)
	}
}

func (vm *VM) execForeach(s *foreachStmt) {
	lists := make([]Value, len(s.Lists))
	n := -1
	for i, e := range s.Lists {
		v, err := vm.eval(e)
		if err != nil {
			vm.fail(s.Pos, "%s", err.Error())
		}
		if v.K != KindList {
			vm.fail(s.Pos, "foreach expected a list, got %s", v.TypeName())
		}
		if n < 0 {
			n = len(v.L)
		} else if len(v.L) != n {
			vm.fail(s.Pos, "foreach expected lists of the same length")
		}
		lists[i] = v
	}
	taskV, err := vm.eval(s.Task)
	if err != nil {
		vm.fail(s.Pos, "%s", err.Error())
	}
	if taskV.K != KindTask || taskV.Task == nil || !taskV.Task.IsCommand {
		vm.fail(s.Pos, "foreach expected a command task")
	}
	for i := 0; i < n; i++ {
		vm.checkpoint()
		args := make([]Value, len(lists))
		for j := range lists {
			args[j] = lists[j].L[i]
		}
		vm.callTaskCmd(taskV.Task, args, s.Pos)
	}
}

// fitTaskArgs matches inputs to a task. The old bracket form has a single
// "?" parameter: it accepts any number of inputs and binds "?" to the first,
// and it also accepts none so `run [ print 1 ]` can run.
func (vm *VM) fitTaskArgs(t *Task, args []Value, pos Pos) []Value {
	if t != nil && len(t.Params) == 1 && t.Params[0] == "?" {
		if len(args) == 0 {
			return nil
		}
		if len(args) == 1 {
			return args
		}
		return []Value{args[0]}
	}
	n := 0
	if t != nil {
		n = len(t.Params)
	}
	if n != len(args) {
		vm.fail(pos, "task expected %d inputs", n)
	}
	return args
}

func (vm *VM) callTaskCmd(t *Task, args []Value, pos Pos) {
	args = vm.fitTaskArgs(t, args, pos)
	saved := vm.scope
	sc := vm.allocScope(t.Scope)
	vm.scope = sc
	for i, name := range t.Params {
		if i < len(args) {
			vm.scope.vars[name] = args[i]
		}
	}
	defer func() {
		vm.scope = saved
		vm.freeScope(sc)
	}()
	if t.cmdFn != nil && (vm.Prog == nil || !vm.Prog.noClosure) {
		t.cmdFn(vm)
		return
	}
	vm.execStmts(t.CmdBody)
}

func (vm *VM) callTaskRep(t *Task, args []Value, pos Pos) Value {
	args = vm.fitTaskArgs(t, args, pos)
	saved := vm.scope
	sc := vm.allocScope(t.Scope)
	vm.scope = sc
	for i, name := range t.Params {
		if i < len(args) {
			vm.scope.vars[name] = args[i]
		}
	}
	defer func() {
		vm.scope = saved
		vm.freeScope(sc)
	}()
	if t.repFn != nil && (vm.Prog == nil || !vm.Prog.noClosure) {
		return t.repFn(vm)
	}
	if t.RepBody == nil {
		vm.fail(pos, "reporter task has no body")
	}
	v, err := vm.eval(t.RepBody)
	if err != nil {
		vm.fail(pos, "%s", err.Error())
	}
	return v
}

func (vm *VM) execCmd(s *cmdStmt) {
	if proc := vm.Prog.Proc(s.Name); proc != nil && !proc.IsReporter {
		args := vm.evalAll(s.Args, s.Pos)
		stopped, err := vm.callCmd(proc, args)
		if err != nil {
			vm.fail(s.Pos, "%s", err.Error())
		}
		_ = stopped
		// A nested procedure's stop does not stop the caller.
		return
	}
	if s.Name == "diffuse" || s.Name == "diffuse4" {
		if len(s.Args) != 2 {
			vm.fail(s.Pos, "%s expected 2 inputs", s.Name)
		}
		id, ok := s.Args[0].(*identExpr)
		if !ok {
			vm.fail(s.Pos, "%s expected a patch variable", s.Name)
		}
		amtV, err := vm.eval(s.Args[1])
		if err != nil {
			vm.fail(s.Pos, "%s", err.Error())
		}
		amt, err := amtV.Num()
		if err != nil {
			vm.fail(s.Pos, "%s", err.Error())
		}
		if err := vm.World.Diffuse(id.Name, amt, s.Name == "diffuse4"); err != nil {
			vm.fail(s.Pos, "%s", err.Error())
		}
		return
	}
	spec, _ := commands[s.Name]
	if spec.optBlock || strings.Contains(s.Name, "-with") || strings.Contains(s.Name, "-to") || strings.Contains(s.Name, "-from") {
		vm.execMaybeLink(s)
		return
	}
	args := vm.evalAll(s.Args, s.Pos)
	if err := vm.primCmd(s.Name, args, s.Pos); err != nil {
		vm.fail(s.Pos, "%s", err.Error())
	}
}

func (vm *VM) execMaybeLink(s *cmdStmt) {
	// Optional trailing command block is not evaluated as a value.
	var block []Stmt
	argsE := s.Args
	if len(argsE) > 0 {
		if cb, ok := argsE[len(argsE)-1].(*cmdBlockExpr); ok {
			block = cb.Body
			argsE = argsE[:len(argsE)-1]
		}
	}
	if _, _, _, _, isLink := linkCmd(s.Name, vm.World); isLink {
		args := vm.evalAll(argsE, s.Pos)
		vm.createLinks(s.Name, args, block, s.Pos)
		return
	}
	args := vm.evalAll(s.Args, s.Pos)
	if err := vm.primCmd(s.Name, args, s.Pos); err != nil {
		vm.fail(s.Pos, "%s", err.Error())
	}
}

func (vm *VM) evalAll(es []Expr, pos Pos) []Value {
	out := make([]Value, len(es))
	for i, e := range es {
		v, err := vm.eval(e)
		if err != nil {
			vm.fail(pos, "%s", err.Error())
		}
		out[i] = v
	}
	return out
}

func (vm *VM) eval(e Expr) (Value, error) {
	switch e := e.(type) {
	case *numExpr:
		return Num(e.N), nil
	case *strExpr:
		return Str(e.S), nil
	case *boolExpr:
		return Bool(e.B), nil
	case *nobodyExpr:
		return Nobody(), nil
	case *identExpr:
		return vm.evalIdent(e.Name, e.Pos)
	case *listExpr:
		elems := make([]Value, len(e.Elems))
		for i, el := range e.Elems {
			v, err := vm.eval(el)
			if err != nil {
				return Value{}, err
			}
			elems[i] = v
		}
		return List(elems...), nil
	case *callExpr:
		return vm.evalCall(e)
	case *infixExpr:
		return vm.evalInfix(e)
	case *prefixExpr:
		r, err := vm.eval(e.Right)
		if err != nil {
			return Value{}, err
		}
		if e.Op == "-" {
			n, err := r.Num()
			if err != nil {
				return Value{}, err
			}
			return Num(-n), nil
		}
		return Value{}, errorAt(e.Pos, "unknown operator %s", e.Op)
	case *lambdaExpr:
		vm.tasks++
		task := &Task{
			Params:    e.Params,
			CmdBody:   e.Body,
			RepBody:   e.Rep,
			IsCommand: e.IsCommand,
			Scope:     vm.scope,
		}
		if vm.Prog == nil || !vm.Prog.noClosure {
			task.cmdFn = e.cmdFn
			task.repFn = e.repFn
		}
		return Value{K: KindTask, Task: task}, nil
	case *ofExpr:
		return vm.evalOf(e)
	case *withExpr:
		return vm.evalWith(e)
	case *repBlockExpr:
		return vm.eval(e.Body)
	case *cmdBlockExpr:
		return Value{}, errorAt(e.Pos, "expected a reporter, got a command block")
	case *pendingExpr:
		return Value{}, errorAt(e.Pos, "unresolved block")
	default:
		return Value{}, fmt.Errorf("unknown expression")
	}
}

func (vm *VM) evalIdent(name string, pos Pos) (Value, error) {
	if vm.scope != nil {
		if v, ok := vm.scope.get(name); ok {
			return v, nil
		}
	}
	if vm.self != nil {
		if v, ok, err := vm.getAgentVar(name); ok || err != nil {
			return v, err
		}
	}
	if v, ok := vm.World.Global(name); ok {
		return v, nil
	}
	if _, ok := vm.World.breeds[name]; ok {
		return SetVal(vm.World.TurtlesOfBreed(name)), nil
	}
	if b, ok := vm.World.linkBreeds[name]; ok {
		return SetVal(vm.linksOfBreed(b.Plural)), nil
	}
	if name == "turtles" {
		return SetVal(vm.World.Turtles()), nil
	}
	if name == "patches" {
		return SetVal(vm.World.Patches()), nil
	}
	if name == "links" {
		return SetVal(vm.World.Links()), nil
	}
	if c, ok := colorNames[name]; ok {
		return Num(c), nil
	}
	if name == "true" {
		return Bool(true), nil
	}
	if name == "false" {
		return Bool(false), nil
	}
	if name == "nobody" {
		return Nobody(), nil
	}
	if zeroArgReporters[name] {
		return vm.evalZero(name, pos)
	}
	if strings.HasSuffix(name, "-here") {
		br := strings.TrimSuffix(name, "-here")
		if _, ok := vm.World.breeds[br]; ok {
			return SetVal(vm.breedHere(br)), nil
		}
	}
	if proc := vm.Prog.Proc(name); proc != nil && proc.IsReporter && len(proc.Params) == 0 {
		return vm.callRep(proc, nil)
	}
	return Value{}, errorAt(pos, "nothing named %s has been defined", strings.ToUpper(name))
}

func (vm *VM) linksOfBreed(plural string) *AgentSet {
	all := vm.World.Links()
	var ags []Agent
	for _, a := range all.Agents {
		if l, ok := a.(*Link); ok && l.Breed == plural {
			ags = append(ags, l)
		}
	}
	return &AgentSet{Kind: KindLink, Breed: plural, Agents: ags}
}

func (vm *VM) breedHere(plural string) *AgentSet {
	p := vm.patchUnderSelf()
	var ags []Agent
	if p != nil {
		for _, t := range vm.World.TurtlesHere(p) {
			if t.Breed == plural {
				ags = append(ags, t)
			}
		}
	}
	return &AgentSet{Kind: KindTurtle, Breed: plural, Agents: ags}
}

func (vm *VM) patchUnderSelf() *Patch {
	switch a := vm.self.(type) {
	case *Patch:
		return a
	case *Turtle:
		return vm.World.PatchHere(a)
	default:
		return nil
	}
}

func (vm *VM) requireTurtle(pos Pos) *Turtle {
	t, ok := vm.self.(*Turtle)
	if !ok || t == nil || t.dead {
		vm.fail(pos, "this command can only be used by turtles")
	}
	return t
}

// CompileButton parses a button's source. Turtle, patch, and link buttons
// run once for every agent of that kind, which is ask <set> [ source ].
func CompileButton(prog *Program, source, agent string) ([]Stmt, error) {
	src := strings.TrimSpace(source)
	if src == "" {
		return nil, nil
	}
	switch strings.ToLower(strings.TrimSpace(agent)) {
	case "turtle":
		src = "ask turtles [ " + src + " ]"
	case "patch":
		src = "ask patches [ " + src + " ]"
	case "link":
		src = "ask links [ " + src + " ]"
	}
	return ParseCommands(src, prog)
}

// DoLocked runs fn while holding the world lock. It does not publish a
// frame. fn must not call a method that locks the world again.
func (vm *VM) DoLocked(fn func()) {
	if fn == nil {
		return
	}
	if vm == nil || vm.World == nil {
		fn()
		return
	}
	vm.World.mu.Lock()
	vm.external++
	defer func() {
		vm.external--
		vm.World.mu.Unlock()
	}()
	fn()
}

// LineUnlocked runs one command-center line. The caller holds the world
// lock. An interrupt while a monitor is reading does not abort the button
// that owns the lock on another goroutine; that button still sees halt.
func (vm *VM) LineUnlocked(src string) (out string, err error) {
	defer vm.finishInterruptErr(&err)
	return vm.executeLine(src)
}

func (vm *VM) finishInterrupt(stopped *bool) {
	if r := recover(); r != nil {
		if _, ok := r.(interruptPanic); ok {
			if stopped != nil {
				*stopped = true
			}
			return
		}
		panic(r)
	}
}

func (vm *VM) finishInterruptErr(errp *error) {
	if r := recover(); r != nil {
		if _, ok := r.(interruptPanic); ok {
			if errp != nil && *errp == nil {
				*errp = fmt.Errorf("stopped")
			}
			return
		}
		panic(r)
	}
}

// RunCompiled runs button or command-center commands. stop in a procedure
// called directly from this list stops the caller (a forever button). stop
// nested inside another procedure does not. A UI halt unwinds here too.
func (vm *VM) RunCompiled(stmts []Stmt) (stopped bool, err error) {
	vm.World.mu.Lock()
	defer vm.World.unlockPublish()
	defer vm.finishInterrupt(&stopped)
	stopped, err = vm.runStmts(stmts)
	return stopped, err
}

// runStmts runs commands. The caller holds World.mu. Plot setup and update
// use this so a pen can call plot without locking the world a second time.
// Interrupt is not caught here: a pen that is mid-update lets it reach
// RunCompiled, so stop still aborts the button.
func (vm *VM) runStmts(stmts []Stmt) (stopped bool, err error) {
	defer vm.recover(&err, &stopped)
	for _, s := range stmts {
		if c, ok := s.(*cmdStmt); ok {
			if proc := vm.Prog.Proc(c.Name); proc != nil && !proc.IsReporter {
				args := vm.evalAll(c.Args, c.Pos)
				st, e := vm.callCmd(proc, args)
				if e != nil {
					return false, e
				}
				if st {
					return true, nil
				}
				continue
			}
		}
		vm.execStmt(s)
	}
	return false, nil
}

func (vm *VM) requireObserver(pos Pos, what string) {
	if vm.self != nil {
		vm.fail(pos, "only the observer can use %s", what)
	}
}
