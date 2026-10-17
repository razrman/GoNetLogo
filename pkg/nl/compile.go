package nl

import (
	"fmt"
	"math"
	"strings"
)

// compile turns each procedure body into a Go closure once. Constants,
// global and agent-variable slots, and breed names are resolved while
// compiling. The closures call the same world operations as the tree
// walker, so a fixed seed still produces the same agent order and the
// same random numbers. Forms that are not specialized fall back to the
// tree walker.

type cmdFn func(*VM)
type repFn func(*VM) Value

type ownSlot struct {
	name string
	all  bool // declared with turtles-own / patches-own, not breed-own
}

type compiler struct {
	prog       *Program
	globals    map[string]int
	globalName []string
	turtle     map[string]ownSlot
	patch      map[string]ownSlot
	breeds     map[string]bool
	linkBreeds map[string]bool
}

func (vm *VM) ensureCompiled() {
	if vm.Prog == nil || vm.Prog.compiled {
		return
	}
	compileProgram(vm.Prog)
}

func compileProgram(prog *Program) {
	if prog == nil || prog.compiled {
		return
	}
	c := newCompiler(prog)
	for _, p := range prog.Procedures {
		locals := map[string]bool{}
		for _, name := range p.Params {
			locals[name] = true
		}
		p.needsScope = len(p.Params) > 0 || hasLexical(p.Body)
		p.cmdFn = c.compileStmts(p.Body, locals)
	}
	prog.compiled = true
}

func newCompiler(prog *Program) *compiler {
	c := &compiler{
		prog:       prog,
		globals:    map[string]int{},
		turtle:     map[string]ownSlot{},
		patch:      map[string]ownSlot{},
		breeds:     map[string]bool{},
		linkBreeds: map[string]bool{},
	}
	addGlobal := func(name string) {
		if _, ok := c.globals[name]; ok {
			return
		}
		c.globals[name] = len(c.globalName)
		c.globalName = append(c.globalName, name)
	}
	for _, name := range prog.Globals {
		addGlobal(name)
	}
	for name := range prog.InterfaceVars {
		addGlobal(name)
	}
	for _, name := range prog.TurtlesOwn {
		c.turtle[name] = ownSlot{name: name, all: true}
	}
	for _, names := range prog.BreedOwn {
		for _, name := range names {
			if _, ok := c.turtle[name]; ok {
				continue
			}
			c.turtle[name] = ownSlot{name: name, all: false}
		}
	}
	for _, name := range prog.PatchesOwn {
		c.patch[name] = ownSlot{name: name, all: true}
	}
	for _, b := range prog.Breeds {
		c.breeds[b.Plural] = true
	}
	for _, b := range prog.LinkBreeds {
		c.linkBreeds[b.Plural] = true
	}
	return c
}

func hasLexical(stmts []Stmt) bool {
	for _, s := range stmts {
		if stmtLexical(s) {
			return true
		}
	}
	return false
}

func stmtLexical(s Stmt) bool {
	switch s := s.(type) {
	case *letStmt:
		return true
	case *cmdStmt:
		return exprsLexical(s.Args)
	case *setStmt:
		return exprLexical(s.Value)
	case *ifStmt:
		return exprLexical(s.Cond) || hasLexical(s.Body)
	case *ifelseStmt:
		return exprLexical(s.Cond) || hasLexical(s.Then) || hasLexical(s.Else)
	case *whileStmt:
		return exprLexical(s.Cond) || hasLexical(s.Body)
	case *repeatStmt:
		return exprLexical(s.Times) || hasLexical(s.Body)
	case *loopStmt:
		return hasLexical(s.Body)
	case *askStmt:
		return exprLexical(s.Agents) || hasLexical(s.Body)
	case *createStmt:
		return exprLexical(s.Count) || hasLexical(s.Body)
	case *sproutStmt:
		return exprLexical(s.Count) || hasLexical(s.Body)
	case *hatchStmt:
		return exprLexical(s.Count) || hasLexical(s.Body)
	case *foreachStmt:
		return exprsLexical(s.Lists) || exprLexical(s.Task)
	case *reportStmt:
		return exprLexical(s.Value)
	default:
		return false
	}
}

func exprsLexical(es []Expr) bool {
	for _, e := range es {
		if exprLexical(e) {
			return true
		}
	}
	return false
}

func exprLexical(e Expr) bool {
	switch e := e.(type) {
	case *lambdaExpr:
		return true
	case *listExpr:
		return exprsLexical(e.Elems)
	case *callExpr:
		return exprsLexical(e.Args)
	case *infixExpr:
		return exprLexical(e.Left) || exprLexical(e.Right)
	case *prefixExpr:
		return exprLexical(e.Right)
	case *cmdBlockExpr:
		return hasLexical(e.Body)
	case *repBlockExpr:
		return exprLexical(e.Body)
	case *ofExpr:
		return exprLexical(e.Block) || exprLexical(e.Agents)
	case *withExpr:
		return exprLexical(e.Agents) || exprLexical(e.Block)
	default:
		return false
	}
}

// directLets is true when this statement list itself binds a name.
// Nested blocks get their own scopes, matching block().
func directLets(stmts []Stmt) bool {
	for _, s := range stmts {
		if _, ok := s.(*letStmt); ok {
			return true
		}
	}
	return false
}

func (c *compiler) compileStmts(stmts []Stmt, locals map[string]bool) cmdFn {
	fns := make([]cmdFn, len(stmts))
	loc := locals
	for i, s := range stmts {
		if ls, ok := s.(*letStmt); ok {
			fns[i] = c.compileLet(ls, loc)
			loc = copyLocals(loc)
			for _, name := range ls.Names {
				loc[name] = true
			}
			continue
		}
		fns[i] = c.compileStmt(s, loc)
	}
	return func(vm *VM) {
		for _, fn := range fns {
			fn(vm)
		}
	}
}

func copyLocals(in map[string]bool) map[string]bool {
	out := make(map[string]bool, len(in)+1)
	for k, v := range in {
		out[k] = v
	}
	return out
}

func (c *compiler) compileBlock(stmts []Stmt, locals map[string]bool) cmdFn {
	body := c.compileStmts(stmts, locals)
	if !directLets(stmts) {
		return body
	}
	return func(vm *VM) {
		saved := vm.scope
		vm.scope = &scope{vars: map[string]Value{}, parent: saved}
		defer func() { vm.scope = saved }()
		body(vm)
	}
}

func (c *compiler) compileStmt(s Stmt, locals map[string]bool) cmdFn {
	switch s := s.(type) {
	case *cmdStmt:
		return c.compileCmd(s, locals)
	case *letStmt:
		return c.compileLet(s, locals)
	case *setStmt:
		return c.compileSet(s, locals)
	case *ifStmt:
		cond := c.compileExpr(s.Cond, locals)
		body := c.compileBlock(s.Body, locals)
		pos := s.Pos
		return func(vm *VM) {
			if truthAt(vm, cond(vm), pos) {
				body(vm)
			}
		}
	case *ifelseStmt:
		cond := c.compileExpr(s.Cond, locals)
		thn := c.compileBlock(s.Then, locals)
		els := c.compileBlock(s.Else, locals)
		pos := s.Pos
		return func(vm *VM) {
			if truthAt(vm, cond(vm), pos) {
				thn(vm)
			} else {
				els(vm)
			}
		}
	case *whileStmt:
		cond := c.compileExpr(s.Cond, locals)
		body := c.compileBlock(s.Body, locals)
		pos := s.Pos
		return func(vm *VM) {
			for {
				if !truthAt(vm, cond(vm), pos) {
					return
				}
				body(vm)
			}
		}
	case *repeatStmt:
		times := c.compileExpr(s.Times, locals)
		body := c.compileBlock(s.Body, locals)
		pos := s.Pos
		return func(vm *VM) {
			k, err := asInt(times(vm), "repeat")
			if err != nil {
				vm.fail(pos, "%s", err.Error())
			}
			if k < 0 {
				vm.fail(pos, "repeat expected a non-negative number")
			}
			for i := 0; i < k; i++ {
				body(vm)
			}
		}
	case *loopStmt:
		body := c.compileBlock(s.Body, locals)
		return func(vm *VM) {
			for {
				body(vm)
			}
		}
	case *askStmt:
		return c.compileAsk(s, locals)
	case *createStmt:
		return func(vm *VM) { vm.execCreate(s) }
	case *sproutStmt:
		return func(vm *VM) { vm.execSprout(s) }
	case *hatchStmt:
		return c.compileHatch(s, locals)
	case *foreachStmt:
		return func(vm *VM) { vm.execForeach(s) }
	case *reportStmt:
		val := c.compileExpr(s.Value, locals)
		return func(vm *VM) {
			panic(reportPanic{val(vm)})
		}
	case *stopStmt:
		return func(vm *VM) { panic(stopPanic{}) }
	default:
		return func(vm *VM) { vm.execStmt(s) }
	}
}

func (c *compiler) compileLet(s *letStmt, locals map[string]bool) cmdFn {
	val := c.compileExpr(s.Value, locals)
	names := append([]string(nil), s.Names...)
	pos := s.Pos
	return func(vm *VM) {
		v := val(vm)
		if len(names) == 1 {
			if err := vm.scope.let(names[0], v); err != nil {
				vm.fail(pos, "%s", err.Error())
			}
			return
		}
		if v.K != KindList || len(v.L) != len(names) {
			vm.fail(pos, "let expected a list of %d values", len(names))
		}
		for i, name := range names {
			if err := vm.scope.let(name, v.L[i]); err != nil {
				vm.fail(pos, "%s", err.Error())
			}
		}
	}
}

func (c *compiler) compileSet(s *setStmt, locals map[string]bool) cmdFn {
	if len(s.Names) != 1 {
		return func(vm *VM) { vm.execSet(s) }
	}
	name := s.Names[0]
	val := c.compileExpr(s.Value, locals)
	pos := s.Pos
	if locals[name] {
		return func(vm *VM) {
			v := val(vm)
			if vm.scope != nil && vm.scope.set(name, v) {
				return
			}
			if err := vm.assign(name, v, pos); err != nil {
				vm.fail(pos, "%s", err.Error())
			}
		}
	}
	if slot, ok := c.turtle[name]; ok && slot.all {
		key := slot.name
		return func(vm *VM) {
			v := val(vm)
			if t, ok := vm.self.(*Turtle); ok {
				t.Vars[key] = v
				return
			}
			if err := vm.assign(key, v, pos); err != nil {
				vm.fail(pos, "%s", err.Error())
			}
		}
	}
	if slot, ok := c.patch[name]; ok && slot.all {
		key := slot.name
		return func(vm *VM) {
			v := val(vm)
			switch a := vm.self.(type) {
			case *Turtle:
				p := vm.World.PatchHere(a)
				if p == nil {
					vm.fail(pos, "turtle is not on a patch")
				}
				p.Vars[key] = v
			case *Patch:
				a.Vars[key] = v
			default:
				if err := vm.assign(key, v, pos); err != nil {
					vm.fail(pos, "%s", err.Error())
				}
			}
		}
	}
	if name == "pcolor" {
		return func(vm *VM) {
			v := val(vm)
			switch a := vm.self.(type) {
			case *Turtle:
				p := vm.World.PatchHere(a)
				if p == nil {
					vm.fail(pos, "turtle is not on a patch")
				}
				if err := setPatchColor(p, v); err != nil {
					vm.fail(pos, "%s", err.Error())
				}
			case *Patch:
				if err := setPatchColor(a, v); err != nil {
					vm.fail(pos, "%s", err.Error())
				}
			default:
				if err := vm.assign(name, v, pos); err != nil {
					vm.fail(pos, "%s", err.Error())
				}
			}
		}
	}
	if _, ok := c.globals[name]; ok && !agentBuiltin(name) {
		key := name
		return func(vm *VM) {
			v := val(vm)
			if _, ok := vm.World.globals[key]; ok {
				vm.World.globals[key] = v
				return
			}
			if err := vm.assign(key, v, pos); err != nil {
				vm.fail(pos, "%s", err.Error())
			}
		}
	}
	return func(vm *VM) {
		v := val(vm)
		if err := vm.assign(name, v, pos); err != nil {
			vm.fail(pos, "%s", err.Error())
		}
	}
}

func (c *compiler) compileAsk(s *askStmt, locals map[string]bool) cmdFn {
	agents := c.compileExpr(s.Agents, locals)
	body := c.compileStmts(s.Body, locals)
	child := directLets(s.Body)
	pos := s.Pos
	return func(vm *VM) {
		v := agents(vm)
		var list []Agent
		switch v.K {
		case KindAgent:
			if v.A == nil || v.A.IsDead() {
				vm.fail(pos, "that agent is dead")
			}
			list = []Agent{v.A}
		case KindAgentset:
			if vm.self != nil && v.Set != nil && v.Set.All && (v.Set.Kind == KindTurtle || v.Set.Kind == KindPatch) {
				vm.fail(pos, "only the observer can ASK the set of all %ss", v.Set.Kind)
			}
			list = vm.World.Shuffle(v.Set.Alive())
		default:
			vm.fail(pos, "ask expected an agent or agentset, got %s", v.TypeName())
		}
		for _, a := range list {
			if a.IsDead() {
				continue
			}
			vm.runAgent(a, body, child)
		}
	}
}

func (c *compiler) compileHatch(s *hatchStmt, locals map[string]bool) cmdFn {
	count := c.compileExpr(s.Count, locals)
	body := c.compileStmts(s.Body, locals)
	child := directLets(s.Body)
	breed := s.Breed
	pos := s.Pos
	return func(vm *VM) {
		parent, ok := vm.self.(*Turtle)
		if !ok || parent == nil || parent.dead {
			vm.fail(pos, "hatch can only be used by a turtle")
		}
		k, err := asInt(count(vm), "hatch")
		if err != nil {
			vm.fail(pos, "%s", err.Error())
		}
		var made []Agent
		for i := 0; i < k; i++ {
			made = append(made, vm.World.Hatch(parent, breed))
		}
		for _, a := range made {
			vm.runAgent(a, body, child)
		}
	}
}

func (vm *VM) runAgent(a Agent, body cmdFn, child bool) {
	if a == nil || a.IsDead() {
		return
	}
	vm.pushAgent(a)
	saved := vm.scope
	if child {
		vm.scope = &scope{vars: map[string]Value{}, parent: saved}
	}
	defer func() {
		vm.scope = saved
		vm.popAgent()
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
	body(vm)
}

func (c *compiler) compileCmd(s *cmdStmt, locals map[string]bool) cmdFn {
	if proc := c.prog.Proc(s.Name); proc != nil && !proc.IsReporter {
		pos := s.Pos
		if len(s.Args) == 0 {
			return func(vm *VM) {
				if _, err := vm.callCmd(proc, nil); err != nil {
					vm.fail(pos, "%s", err.Error())
				}
			}
		}
		args := c.compileExprs(s.Args, locals)
		return func(vm *VM) {
			if _, err := vm.callCmd(proc, evalReps(vm, args)); err != nil {
				vm.fail(pos, "%s", err.Error())
			}
		}
	}
	if s.Name == "diffuse" || s.Name == "diffuse4" {
		return func(vm *VM) { vm.execCmd(s) }
	}
	spec, known := commands[s.Name]
	if !known || spec.optBlock || strings.Contains(s.Name, "-with") || strings.Contains(s.Name, "-to") || strings.Contains(s.Name, "-from") {
		return func(vm *VM) { vm.execCmd(s) }
	}
	pos := s.Pos
	switch s.Name {
	case "fd", "forward", "bk", "back", "jump":
		arg := c.compileExpr(s.Args[0], locals)
		name := s.Name
		return func(vm *VM) {
			n, err := arg(vm).Num()
			if err != nil {
				vm.fail(pos, "%s", err.Error())
			}
			if name == "bk" || name == "back" {
				n = -n
			}
			if err := vm.World.Forward(vm.requireTurtle(pos), n); err != nil {
				vm.fail(pos, "%s", err.Error())
			}
		}
	case "rt", "right", "lt", "left":
		arg := c.compileExpr(s.Args[0], locals)
		sign := 1.0
		if s.Name == "lt" || s.Name == "left" {
			sign = -1
		}
		return func(vm *VM) {
			n, err := arg(vm).Num()
			if err != nil {
				vm.fail(pos, "%s", err.Error())
			}
			vm.World.Turn(vm.requireTurtle(pos), sign*n)
		}
	case "die":
		return func(vm *VM) {
			if err := vm.primCmd("die", nil, pos); err != nil {
				vm.fail(pos, "%s", err.Error())
			}
		}
	}
	args := c.compileExprs(s.Args, locals)
	name := s.Name
	return func(vm *VM) {
		if err := vm.primCmd(name, evalReps(vm, args), pos); err != nil {
			vm.fail(pos, "%s", err.Error())
		}
	}
}

func (c *compiler) compileExprs(es []Expr, locals map[string]bool) []repFn {
	out := make([]repFn, len(es))
	for i, e := range es {
		out[i] = c.compileExpr(e, locals)
	}
	return out
}

func evalReps(vm *VM, fns []repFn) []Value {
	if len(fns) == 0 {
		return nil
	}
	out := make([]Value, len(fns))
	for i, fn := range fns {
		out[i] = fn(vm)
	}
	return out
}

func (c *compiler) compileExpr(e Expr, locals map[string]bool) repFn {
	switch e := e.(type) {
	case *numExpr:
		v := Num(e.N)
		return func(*VM) Value { return v }
	case *strExpr:
		v := Str(e.S)
		return func(*VM) Value { return v }
	case *boolExpr:
		v := Bool(e.B)
		return func(*VM) Value { return v }
	case *nobodyExpr:
		v := Nobody()
		return func(*VM) Value { return v }
	case *identExpr:
		return c.compileIdent(e.Name, e.Pos, locals)
	case *listExpr:
		elems := c.compileExprs(e.Elems, locals)
		return func(vm *VM) Value { return List(evalReps(vm, elems)...) }
	case *callExpr:
		return c.compileCall(e, locals)
	case *infixExpr:
		return c.compileInfix(e, locals)
	case *prefixExpr:
		right := c.compileExpr(e.Right, locals)
		pos := e.Pos
		if e.Op == "-" {
			return func(vm *VM) Value {
				n, err := right(vm).Num()
				if err != nil {
					vm.fail(pos, "%s", err.Error())
				}
				return Num(-n)
			}
		}
		return func(vm *VM) Value {
			v, err := vm.eval(e)
			if err != nil {
				vm.fail(pos, "%s", err.Error())
			}
			return v
		}
	case *repBlockExpr:
		return c.compileExpr(e.Body, locals)
	default:
		pos := e.at()
		return func(vm *VM) Value {
			v, err := vm.eval(e)
			if err != nil {
				vm.fail(pos, "%s", err.Error())
			}
			return v
		}
	}
}

func (c *compiler) compileCall(e *callExpr, locals map[string]bool) repFn {
	pos := e.Pos
	switch e.Name {
	case "ifelse-value", "all?", "max-one-of", "min-one-of", "sort-on":
		return func(vm *VM) Value {
			v, err := vm.eval(e)
			if err != nil {
				vm.fail(pos, "%s", err.Error())
			}
			return v
		}
	case "random":
		if len(e.Args) == 1 {
			arg := c.compileExpr(e.Args[0], locals)
			return func(vm *VM) Value {
				n, err := asInt(arg(vm), "random")
				if err != nil {
					vm.fail(pos, "%s", err.Error())
				}
				k, err := vm.World.RandomInt(n)
				if err != nil {
					vm.fail(pos, "%s", err.Error())
				}
				return Num(float64(k))
			}
		}
	case "any?":
		if len(e.Args) == 1 {
			arg := c.compileExpr(e.Args[0], locals)
			return func(vm *VM) Value {
				ags, err := vm.asAgentset(arg(vm))
				if err != nil {
					vm.fail(pos, "%s", err.Error())
				}
				return Bool(ags.Count() > 0)
			}
		}
	case "one-of":
		if len(e.Args) == 1 {
			arg := c.compileExpr(e.Args[0], locals)
			return func(vm *VM) Value {
				v, err := vm.oneOf(arg(vm))
				if err != nil {
					vm.fail(pos, "%s", err.Error())
				}
				return v
			}
		}
	case "not":
		if len(e.Args) == 1 {
			arg := c.compileExpr(e.Args[0], locals)
			return func(vm *VM) Value {
				b, err := arg(vm).Truth()
				if err != nil {
					vm.fail(pos, "%s", err.Error())
				}
				return Bool(!b)
			}
		}
	}
	if proc := c.prog.Proc(e.Name); proc != nil && proc.IsReporter {
		args := c.compileExprs(e.Args, locals)
		return func(vm *VM) Value {
			v, err := vm.callRep(proc, evalReps(vm, args))
			if err != nil {
				vm.fail(pos, "%s", err.Error())
			}
			return v
		}
	}
	if strings.HasSuffix(e.Name, "-on") || strings.HasSuffix(e.Name, "-at") || strings.HasPrefix(e.Name, "is-") {
		return func(vm *VM) Value {
			v, err := vm.eval(e)
			if err != nil {
				vm.fail(pos, "%s", err.Error())
			}
			return v
		}
	}
	args := c.compileExprs(e.Args, locals)
	name := e.Name
	return func(vm *VM) Value {
		v, err := vm.evalReporter(name, evalReps(vm, args), pos)
		if err != nil {
			vm.fail(pos, "%s", err.Error())
		}
		return v
	}
}

func (c *compiler) compileInfix(e *infixExpr, locals map[string]bool) repFn {
	if e.Op == "with-min" || e.Op == "with-max" || e.Op == "in-radius" {
		pos := e.Pos
		return func(vm *VM) Value {
			v, err := vm.eval(e)
			if err != nil {
				vm.fail(pos, "%s", err.Error())
			}
			return v
		}
	}
	left := c.compileExpr(e.Left, locals)
	right := c.compileExpr(e.Right, locals)
	op := e.Op
	pos := e.Pos
	return func(vm *VM) Value {
		l := left(vm)
		if op == "and" || op == "or" {
			lb, err := l.Truth()
			if err != nil {
				vm.fail(pos, "%s", err.Error())
			}
			if op == "and" && !lb {
				return Bool(false)
			}
			if op == "or" && lb {
				return Bool(true)
			}
			r := right(vm)
			rb, err := r.Truth()
			if err != nil {
				vm.fail(pos, "%s", err.Error())
			}
			if op == "or" {
				return Bool(lb || rb)
			}
			return Bool(lb && rb)
		}
		r := right(vm)
		v, err := applyInfix(op, l, r)
		if err != nil {
			vm.fail(pos, "%s", err.Error())
		}
		return v
	}
}

func applyInfix(op string, l, r Value) (Value, error) {
	switch op {
	case "+":
		if l.K == KindString || r.K == KindString {
			return Str(l.Format(false) + r.Format(false)), nil
		}
		return binNum(l, r, func(a, b float64) float64 { return a + b })
	case "-":
		return binNum(l, r, func(a, b float64) float64 { return a - b })
	case "*":
		return binNum(l, r, func(a, b float64) float64 { return a * b })
	case "/":
		a, b, err := twoNum(l, r)
		if err != nil {
			return Value{}, err
		}
		if b == 0 {
			return Value{}, fmt.Errorf("division by zero")
		}
		return Num(a / b), nil
	case "^":
		a, b, err := twoNum(l, r)
		if err != nil {
			return Value{}, err
		}
		return Num(math.Pow(a, b)), nil
	case "mod":
		a, b, err := twoNum(l, r)
		if err != nil {
			return Value{}, err
		}
		if b == 0 {
			return Value{}, fmt.Errorf("division by zero")
		}
		m := math.Mod(a, b)
		if m < 0 && b > 0 || m > 0 && b < 0 {
			m += b
		}
		return Num(m), nil
	case "=":
		return Bool(l.Equals(r)), nil
	case "!=":
		return Bool(!l.Equals(r)), nil
	case "<", ">", "<=", ">=":
		cmp, err := compare(l, r)
		if err != nil {
			return Value{}, err
		}
		switch op {
		case "<":
			return Bool(cmp < 0), nil
		case ">":
			return Bool(cmp > 0), nil
		case "<=":
			return Bool(cmp <= 0), nil
		default:
			return Bool(cmp >= 0), nil
		}
	case "xor":
		lb, err := l.Truth()
		if err != nil {
			return Value{}, err
		}
		rb, err := r.Truth()
		if err != nil {
			return Value{}, err
		}
		return Bool(lb != rb), nil
	default:
		return Value{}, fmt.Errorf("unknown operator %s", op)
	}
}

func (c *compiler) compileIdent(name string, pos Pos, locals map[string]bool) repFn {
	resolved := c.resolveIdent(name, pos)
	if !locals[name] {
		return resolved
	}
	return func(vm *VM) Value {
		if vm.scope != nil {
			if v, ok := vm.scope.get(name); ok {
				return v
			}
		}
		return resolved(vm)
	}
}

func (c *compiler) resolveIdent(name string, pos Pos) repFn {
	if slot, ok := c.turtle[name]; ok && slot.all {
		key := slot.name
		return func(vm *VM) Value {
			if t, ok := vm.self.(*Turtle); ok {
				if v, ok := t.Vars[key]; ok {
					return v
				}
			}
			return mustIdent(vm, name, pos)
		}
	}
	if slot, ok := c.patch[name]; ok && slot.all {
		key := slot.name
		return func(vm *VM) Value {
			switch a := vm.self.(type) {
			case *Patch:
				if v, ok := a.Vars[key]; ok {
					return v
				}
				return Num(0)
			case *Turtle:
				p := vm.World.PatchHere(a)
				if p == nil {
					return Num(0)
				}
				if v, ok := p.Vars[key]; ok {
					return v
				}
				return Num(0)
			default:
				return mustIdent(vm, name, pos)
			}
		}
	}
	if name == "pcolor" {
		return func(vm *VM) Value {
			switch a := vm.self.(type) {
			case *Patch:
				return patchColorValue(a)
			case *Turtle:
				p := vm.World.PatchHere(a)
				if p == nil {
					return Num(0)
				}
				return patchColorValue(p)
			default:
				return mustIdent(vm, name, pos)
			}
		}
	}
	if _, ok := c.globals[name]; ok && !agentBuiltin(name) {
		key := name
		return func(vm *VM) Value {
			if v, ok := vm.World.globals[key]; ok {
				return v
			}
			return mustIdent(vm, name, pos)
		}
	}
	if c.breeds[name] {
		breed := name
		return func(vm *VM) Value {
			return SetVal(vm.World.TurtlesOfBreed(breed))
		}
	}
	if c.linkBreeds[name] {
		breed := name
		return func(vm *VM) Value {
			return SetVal(vm.linksOfBreed(breed))
		}
	}
	switch name {
	case "turtles":
		return func(vm *VM) Value { return SetVal(vm.World.Turtles()) }
	case "patches":
		return func(vm *VM) Value { return SetVal(vm.World.Patches()) }
	case "links":
		return func(vm *VM) Value { return SetVal(vm.World.Links()) }
	case "true":
		v := Bool(true)
		return func(*VM) Value { return v }
	case "false":
		v := Bool(false)
		return func(*VM) Value { return v }
	case "nobody":
		v := Nobody()
		return func(*VM) Value { return v }
	}
	if n, ok := colorNames[name]; ok {
		v := Num(n)
		return func(*VM) Value { return v }
	}
	if strings.HasSuffix(name, "-here") {
		br := strings.TrimSuffix(name, "-here")
		if c.breeds[br] {
			return func(vm *VM) Value { return SetVal(vm.breedHere(br)) }
		}
	}
	if zeroArgReporters[name] {
		return func(vm *VM) Value {
			v, err := vm.evalZero(name, pos)
			if err != nil {
				vm.fail(pos, "%s", err.Error())
			}
			return v
		}
	}
	if proc := c.prog.Proc(name); proc != nil && proc.IsReporter && len(proc.Params) == 0 {
		return func(vm *VM) Value {
			v, err := vm.callRep(proc, nil)
			if err != nil {
				vm.fail(pos, "%s", err.Error())
			}
			return v
		}
	}
	return func(vm *VM) Value { return mustIdent(vm, name, pos) }
}

func mustIdent(vm *VM, name string, pos Pos) Value {
	v, err := vm.evalIdent(name, pos)
	if err != nil {
		vm.fail(pos, "%s", err.Error())
	}
	return v
}

func truthAt(vm *VM, v Value, pos Pos) bool {
	t, err := v.Truth()
	if err != nil {
		vm.fail(pos, "%s", err.Error())
	}
	return t
}

func agentBuiltin(name string) bool {
	switch name {
	case "who", "color", "heading", "xcor", "ycor", "shape", "label", "label-color",
		"breed", "hidden?", "size", "pen-size", "pen-mode", "pxcor", "pycor", "pcolor",
		"plabel", "plabel-color", "end1", "end2", "thickness", "tie-mode":
		return true
	default:
		return false
	}
}
