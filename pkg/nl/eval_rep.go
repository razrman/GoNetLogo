package nl

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode"
)

func (vm *VM) evalReporter(name string, args []Value, pos Pos) (Value, error) {
	switch name {
	case "turtle":
		id, err := asInt(args[0], "turtle")
		if err != nil {
			return Value{}, err
		}
		t := vm.World.Turtle(id)
		if t == nil {
			return Nobody(), nil
		}
		return AgentVal(t), nil
	case "patch":
		x, err := asInt(args[0], "patch")
		if err != nil {
			return Value{}, err
		}
		y, err := asInt(args[1], "patch")
		if err != nil {
			return Value{}, err
		}
		p := vm.World.PatchAtPcor(x, y)
		if p == nil {
			return Nobody(), nil
		}
		return AgentVal(p), nil
	case "link":
		a, err := asInt(args[0], "link")
		if err != nil {
			return Value{}, err
		}
		b, err := asInt(args[1], "link")
		if err != nil {
			return Value{}, err
		}
		t1 := vm.World.Turtle(a)
		t2 := vm.World.Turtle(b)
		if t1 == nil || t2 == nil {
			return Nobody(), nil
		}
		if l := vm.World.findLink(t1, t2, false); l != nil {
			return AgentVal(l), nil
		}
		if l := vm.World.findLink(t1, t2, true); l != nil {
			return AgentVal(l), nil
		}
		return Nobody(), nil
	case "turtles-on":
		p, err := vm.agentPatch(args[0])
		if err != nil {
			return Value{}, err
		}
		if p == nil {
			return SetVal(turtleSet(nil)), nil
		}
		return SetVal(turtleSet(vm.World.TurtlesHere(p))), nil
	case "turtles-at":
		dx, err := args[0].Num()
		if err != nil {
			return Value{}, err
		}
		dy, err := args[1].Num()
		if err != nil {
			return Value{}, err
		}
		p := vm.patchAtOffset(dx, dy)
		if p == nil {
			return SetVal(turtleSet(nil)), nil
		}
		return SetVal(turtleSet(vm.World.TurtlesHere(p))), nil
	case "patch-ahead":
		d, err := args[0].Num()
		if err != nil {
			return Value{}, err
		}
		t, ok := vm.self.(*Turtle)
		if !ok {
			return Value{}, fmt.Errorf("patch-ahead can only be used by a turtle")
		}
		rad := t.Heading * math.Pi / 180
		p := vm.World.PatchAt(t.X+d*math.Sin(rad), t.Y+d*math.Cos(rad))
		if p == nil {
			return Nobody(), nil
		}
		return AgentVal(p), nil
	case "patch-at":
		dx, err := args[0].Num()
		if err != nil {
			return Value{}, err
		}
		dy, err := args[1].Num()
		if err != nil {
			return Value{}, err
		}
		p := vm.patchAtOffset(dx, dy)
		if p == nil {
			return Nobody(), nil
		}
		return AgentVal(p), nil
	case "patch-left-and-ahead", "patch-right-and-ahead":
		ang, err := args[0].Num()
		if err != nil {
			return Value{}, err
		}
		dist, err := args[1].Num()
		if err != nil {
			return Value{}, err
		}
		t, ok := vm.self.(*Turtle)
		if !ok {
			return Value{}, fmt.Errorf("%s can only be used by a turtle", name)
		}
		if name == "patch-left-and-ahead" {
			ang = -ang
		}
		h := (t.Heading + ang) * math.Pi / 180
		p := vm.World.PatchAt(t.X+dist*math.Sin(h), t.Y+dist*math.Cos(h))
		if p == nil {
			return Nobody(), nil
		}
		return AgentVal(p), nil
	case "one-of":
		return vm.oneOf(args[0])
	case "n-of":
		return vm.nOf(args[0], args[1])
	case "any?":
		ags, err := vm.asAgentset(args[0])
		if err != nil {
			return Value{}, err
		}
		return Bool(ags.Count() > 0), nil
	case "count":
		switch args[0].K {
		case KindAgentset:
			return Num(float64(args[0].Set.Count())), nil
		case KindList:
			return Num(float64(len(args[0].L))), nil
		default:
			return Value{}, fmt.Errorf("count expected an agentset or list")
		}
	case "sum":
		return vm.foldNum(args[0], 0, func(a, b float64) float64 { return a + b })
	case "mean":
		xs, err := numsOf(args[0])
		if err != nil {
			return Value{}, err
		}
		if len(xs) == 0 {
			return Value{}, fmt.Errorf("mean of an empty list")
		}
		s := 0.0
		for _, n := range xs {
			s += n
		}
		return Num(s / float64(len(xs))), nil
	case "max", "min":
		return vm.extreme(args[0], name == "max")
	case "variance", "standard-deviation":
		xs, err := numsOf(args[0])
		if err != nil {
			return Value{}, err
		}
		if len(xs) < 2 {
			return Value{}, fmt.Errorf("%s needs at least two numbers", name)
		}
		mean := 0.0
		for _, n := range xs {
			mean += n
		}
		mean /= float64(len(xs))
		acc := 0.0
		for _, n := range xs {
			d := n - mean
			acc += d * d
		}
		// NetLogo variance is the sample variance.
		vari := acc / float64(len(xs)-1)
		if name == "variance" {
			return Num(vari), nil
		}
		return Num(math.Sqrt(vari)), nil
	case "map":
		return vm.doMap(args, pos)
	case "filter":
		return vm.doFilter(args, pos)
	case "reduce":
		return vm.doReduce(args, pos)
	case "n-values":
		return vm.doNValues(args, pos)
	case "range":
		return vm.doRange(args)
	case "random":
		n, err := asInt(args[0], "random")
		if err != nil {
			return Value{}, err
		}
		k, err := vm.World.RandomInt(n)
		if err != nil {
			return Value{}, err
		}
		return Num(float64(k)), nil
	case "random-float":
		n, err := args[0].Num()
		if err != nil {
			return Value{}, err
		}
		f, err := vm.World.RandomFloat(n)
		if err != nil {
			return Value{}, err
		}
		return Num(f), nil
	case "scale-color":
		c, err := args[0].Num()
		if err != nil {
			return Value{}, err
		}
		n, err := args[1].Num()
		if err != nil {
			return Value{}, err
		}
		a, err := args[2].Num()
		if err != nil {
			return Value{}, err
		}
		b, err := args[3].Num()
		if err != nil {
			return Value{}, err
		}
		return Num(ScaleColor(c, n, a, b)), nil
	case "list":
		return List(append([]Value(nil), args...)...), nil
	case "sentence", "se":
		var out []Value
		for _, a := range args {
			if a.K == KindList {
				out = append(out, a.L...)
			} else {
				out = append(out, a)
			}
		}
		return List(out...), nil
	case "fput":
		if args[1].K != KindList {
			return Value{}, fmt.Errorf("fput expected a list")
		}
		return List(append([]Value{args[0]}, args[1].L...)...), nil
	case "lput":
		if args[1].K != KindList {
			return Value{}, fmt.Errorf("lput expected a list")
		}
		return List(append(append([]Value{}, args[1].L...), args[0])...), nil
	case "but-first", "bf":
		return butEnd(args[0], true)
	case "but-last", "bl":
		return butEnd(args[0], false)
	case "first":
		return endItem(args[0], true)
	case "last":
		return endItem(args[0], false)
	case "item":
		return vm.item(args[0], args[1])
	case "length":
		switch args[0].K {
		case KindList:
			return Num(float64(len(args[0].L))), nil
		case KindString:
			return Num(float64(len([]rune(args[0].S)))), nil
		default:
			return Value{}, fmt.Errorf("length expected a list or string")
		}
	case "empty?":
		switch args[0].K {
		case KindList:
			return Bool(len(args[0].L) == 0), nil
		case KindString:
			return Bool(args[0].S == ""), nil
		default:
			return Value{}, fmt.Errorf("empty? expected a list or string")
		}
	case "member?":
		return Bool(member(args[0], args[1])), nil
	case "position":
		i := position(args[0], args[1])
		if i < 0 {
			return Bool(false), nil
		}
		return Num(float64(i)), nil
	case "reverse":
		if args[0].K == KindString {
			r := []rune(args[0].S)
			for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
				r[i], r[j] = r[j], r[i]
			}
			return Str(string(r)), nil
		}
		if args[0].K != KindList {
			return Value{}, fmt.Errorf("reverse expected a list or string")
		}
		out := append([]Value(nil), args[0].L...)
		for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
			out[i], out[j] = out[j], out[i]
		}
		return List(out...), nil
	case "sort":
		return vm.doSort(args[0])
	case "sort-by":
		return vm.doSortBy(args[0], args[1], pos)
	case "remove":
		return removeVal(args[0], args[1])
	case "remove-item":
		return removeItem(args[0], args[1])
	case "replace-item":
		return replaceItem(args[0], args[1], args[2])
	case "sublist":
		return sublist(args[0], args[1], args[2])
	case "substring":
		return substring(args[0], args[1], args[2])
	case "word":
		var b strings.Builder
		for _, a := range args {
			b.WriteString(a.Format(false))
		}
		return Str(b.String()), nil
	case "read-from-string":
		if args[0].K != KindString {
			return Value{}, fmt.Errorf("read-from-string expected a string")
		}
		return readLiteral(args[0].S)
	case "not":
		b, err := args[0].Truth()
		if err != nil {
			return Value{}, err
		}
		return Bool(!b), nil
	case "abs":
		n, err := args[0].Num()
		if err != nil {
			return Value{}, err
		}
		return Num(math.Abs(n)), nil
	case "round":
		n, err := args[0].Num()
		if err != nil {
			return Value{}, err
		}
		return Num(math.Floor(n + 0.5)), nil
	case "floor":
		n, err := args[0].Num()
		if err != nil {
			return Value{}, err
		}
		return Num(math.Floor(n)), nil
	case "ceiling":
		n, err := args[0].Num()
		if err != nil {
			return Value{}, err
		}
		return Num(math.Ceil(n)), nil
	case "int":
		n, err := args[0].Num()
		if err != nil {
			return Value{}, err
		}
		return Num(math.Trunc(n)), nil
	case "precision", "approximate":
		n, err := args[0].Num()
		if err != nil {
			return Value{}, err
		}
		p, err := asInt(args[1], name)
		if err != nil {
			return Value{}, err
		}
		pow := math.Pow(10, float64(p))
		return Num(math.Floor(n*pow+0.5) / pow), nil
	case "sqrt":
		n, err := args[0].Num()
		if err != nil {
			return Value{}, err
		}
		if n < 0 {
			return Value{}, fmt.Errorf("sqrt of a negative number")
		}
		return Num(math.Sqrt(n)), nil
	case "sin":
		n, err := args[0].Num()
		if err != nil {
			return Value{}, err
		}
		return Num(math.Sin(n * math.Pi / 180)), nil
	case "cos":
		n, err := args[0].Num()
		if err != nil {
			return Value{}, err
		}
		return Num(math.Cos(n * math.Pi / 180)), nil
	case "tan":
		n, err := args[0].Num()
		if err != nil {
			return Value{}, err
		}
		return Num(math.Tan(n * math.Pi / 180)), nil
	case "asin":
		n, err := args[0].Num()
		if err != nil {
			return Value{}, err
		}
		return Num(math.Asin(n) * 180 / math.Pi), nil
	case "acos":
		n, err := args[0].Num()
		if err != nil {
			return Value{}, err
		}
		return Num(math.Acos(n) * 180 / math.Pi), nil
	case "atan":
		x, err := args[0].Num()
		if err != nil {
			return Value{}, err
		}
		y, err := args[1].Num()
		if err != nil {
			return Value{}, err
		}
		h := math.Atan2(x, y) * 180 / math.Pi
		if h < 0 {
			h += 360
		}
		return Num(h), nil
	case "ln":
		n, err := args[0].Num()
		if err != nil {
			return Value{}, err
		}
		if n <= 0 {
			return Value{}, fmt.Errorf("ln of a non-positive number")
		}
		return Num(math.Log(n)), nil
	case "log":
		n, err := args[0].Num()
		if err != nil {
			return Value{}, err
		}
		b, err := args[1].Num()
		if err != nil {
			return Value{}, err
		}
		if n <= 0 || b <= 0 || b == 1 {
			return Value{}, fmt.Errorf("log of an invalid number")
		}
		return Num(math.Log(n) / math.Log(b)), nil
	case "exp":
		n, err := args[0].Num()
		if err != nil {
			return Value{}, err
		}
		return Num(math.Exp(n)), nil
	case "remainder":
		a, err := args[0].Num()
		if err != nil {
			return Value{}, err
		}
		b, err := args[1].Num()
		if err != nil {
			return Value{}, err
		}
		if b == 0 {
			return Value{}, fmt.Errorf("division by zero")
		}
		return Num(math.Mod(a, b)), nil
	case "other":
		ags, err := vm.asAgentset(args[0])
		if err != nil {
			return Value{}, err
		}
		var keep []Agent
		for _, a := range ags.Alive() {
			if a != vm.self {
				keep = append(keep, a)
			}
		}
		cp := *ags
		cp.All = false
		cp.Agents = keep
		return SetVal(&cp), nil
	case "distance":
		x, y, err := vm.agentPoint(args[0])
		if err != nil {
			return Value{}, err
		}
		sx, sy, err := vm.selfPoint()
		if err != nil {
			return Value{}, err
		}
		return Num(vm.World.Distance(sx, sy, x, y)), nil
	case "distancexy":
		x, err := args[0].Num()
		if err != nil {
			return Value{}, err
		}
		y, err := args[1].Num()
		if err != nil {
			return Value{}, err
		}
		sx, sy, err := vm.selfPoint()
		if err != nil {
			return Value{}, err
		}
		return Num(vm.World.Distance(sx, sy, x, y)), nil
	case "towards":
		x, y, err := vm.agentPoint(args[0])
		if err != nil {
			return Value{}, err
		}
		sx, sy, err := vm.selfPoint()
		if err != nil {
			return Value{}, err
		}
		return Num(vm.World.HeadingTowards(sx, sy, x, y)), nil
	case "towardsxy":
		x, err := args[0].Num()
		if err != nil {
			return Value{}, err
		}
		y, err := args[1].Num()
		if err != nil {
			return Value{}, err
		}
		sx, sy, err := vm.selfPoint()
		if err != nil {
			return Value{}, err
		}
		return Num(vm.World.HeadingTowards(sx, sy, x, y)), nil
	case "can-move?":
		d, err := args[0].Num()
		if err != nil {
			return Value{}, err
		}
		t, ok := vm.self.(*Turtle)
		if !ok {
			return Value{}, fmt.Errorf("can-move? can only be used by a turtle")
		}
		return Bool(vm.World.CanMove(t, d)), nil
	case "shade-of?":
		a, err := args[0].Num()
		if err != nil {
			return Value{}, err
		}
		b, err := args[1].Num()
		if err != nil {
			return Value{}, err
		}
		return Bool(CentralColor(a) == CentralColor(b)), nil
	case "runresult":
		return vm.runResult(args[0], pos)
	case "turtle-set", "patch-set", "link-set":
		return vm.buildSet(name, args)
	case "in-link-from", "out-link-to", "link-with":
		return vm.findMyLink(name, args)
	default:
		if strings.HasPrefix(name, "is-") && strings.HasSuffix(name, "?") {
			return vm.isType(name, args[0]), nil
		}
		return Value{}, fmt.Errorf("primitive %s is not implemented yet", name)
	}
}

func (vm *VM) isType(name string, v Value) Value {
	ok := false
	switch name {
	case "is-number?":
		ok = v.K == KindNum
	case "is-string?":
		ok = v.K == KindString
	case "is-list?":
		ok = v.K == KindList
	case "is-boolean?":
		ok = v.K == KindBool
	case "is-agent?":
		ok = v.K == KindAgent
	case "is-turtle?":
		_, ok = v.A.(*Turtle)
		ok = ok && v.K == KindAgent
	case "is-patch?":
		_, ok = v.A.(*Patch)
		ok = ok && v.K == KindAgent
	case "is-link?":
		_, ok = v.A.(*Link)
		ok = ok && v.K == KindAgent
	case "is-nobody?":
		ok = v.K == KindNobody
	case "is-agentset?":
		ok = v.K == KindAgentset
	case "is-turtle-set?":
		ok = v.K == KindAgentset && v.Set != nil && v.Set.Kind == KindTurtle
	case "is-patch-set?":
		ok = v.K == KindAgentset && v.Set != nil && v.Set.Kind == KindPatch
	case "is-link-set?":
		ok = v.K == KindAgentset && v.Set != nil && v.Set.Kind == KindLink
	}
	return Bool(ok)
}

func (vm *VM) evalInfix(e *infixExpr) (Value, error) {
	if e.Op == "and" || e.Op == "or" {
		l, err := vm.eval(e.Left)
		if err != nil {
			return Value{}, err
		}
		lb, err := l.Truth()
		if err != nil {
			return Value{}, err
		}
		if e.Op == "and" && !lb {
			return Bool(false), nil
		}
		if e.Op == "or" && lb {
			return Bool(true), nil
		}
		r, err := vm.eval(e.Right)
		if err != nil {
			return Value{}, err
		}
		rb, err := r.Truth()
		if err != nil {
			return Value{}, err
		}
		if e.Op == "or" {
			return Bool(lb || rb), nil
		}
		return Bool(lb && rb), nil
	}
	if e.Op == "with-min" || e.Op == "with-max" {
		return vm.withExtreme(e)
	}
	if e.Op == "in-radius" {
		return vm.inRadius(e)
	}
	l, err := vm.eval(e.Left)
	if err != nil {
		return Value{}, err
	}
	r, err := vm.eval(e.Right)
	if err != nil {
		return Value{}, err
	}
	switch e.Op {
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
		r := math.Mod(a, b)
		if r < 0 && b > 0 || r > 0 && b < 0 {
			r += b
		}
		return Num(r), nil
	case "=":
		return Bool(l.Equals(r)), nil
	case "!=":
		return Bool(!l.Equals(r)), nil
	case "<", ">", "<=", ">=":
		c, err := compare(l, r)
		if err != nil {
			return Value{}, err
		}
		switch e.Op {
		case "<":
			return Bool(c < 0), nil
		case ">":
			return Bool(c > 0), nil
		case "<=":
			return Bool(c <= 0), nil
		default:
			return Bool(c >= 0), nil
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
		return Value{}, fmt.Errorf("unknown operator %s", e.Op)
	}
}

func binNum(l, r Value, fn func(a, b float64) float64) (Value, error) {
	a, b, err := twoNum(l, r)
	if err != nil {
		return Value{}, err
	}
	return Num(fn(a, b)), nil
}

func twoNum(l, r Value) (float64, float64, error) {
	a, err := l.Num()
	if err != nil {
		return 0, 0, err
	}
	b, err := r.Num()
	if err != nil {
		return 0, 0, err
	}
	return a, b, nil
}

func compare(l, r Value) (int, error) {
	if l.K == KindNum && r.K == KindNum {
		switch {
		case l.N < r.N:
			return -1, nil
		case l.N > r.N:
			return 1, nil
		default:
			return 0, nil
		}
	}
	if l.K == KindString && r.K == KindString {
		a := strings.ToLower(l.S)
		b := strings.ToLower(r.S)
		switch {
		case a < b:
			return -1, nil
		case a > b:
			return 1, nil
		default:
			return 0, nil
		}
	}
	if l.K == KindAgent && r.K == KindAgent {
		return compareAgents(l.A, r.A)
	}
	return 0, fmt.Errorf("can't compare %s and %s", l.TypeName(), r.TypeName())
}

func compareAgents(a, b Agent) (int, error) {
	if a.AgentKind() != b.AgentKind() {
		return 0, fmt.Errorf("can't compare a %s and a %s", a.AgentKind(), b.AgentKind())
	}
	switch a.AgentKind() {
	case KindTurtle:
		d := a.(*Turtle).Who - b.(*Turtle).Who
		return sgn(d), nil
	case KindPatch:
		pa, pb := a.(*Patch), b.(*Patch)
		if pa.PY != pb.PY {
			return sgn(pa.PY - pb.PY), nil
		}
		return sgn(pa.PX - pb.PX), nil
	default:
		return sgn(a.(*Link).ID - b.(*Link).ID), nil
	}
}

func sgn(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	default:
		return 0
	}
}

func (vm *VM) evalOf(e *ofExpr) (Value, error) {
	target, err := vm.eval(e.Agents)
	if err != nil {
		return Value{}, err
	}
	switch target.K {
	case KindAgent:
		if target.A == nil || target.A.IsDead() {
			return Value{}, fmt.Errorf("that agent is dead")
		}
		return vm.withAgent(target.A, func() Value {
			v, err := vm.eval(e.Block)
			if err != nil {
				vm.fail(e.Pos, "%s", err.Error())
			}
			return v
		}), nil
	case KindAgentset:
		live := target.Set.Alive()
		out := make([]Value, len(live))
		for i, a := range live {
			out[i] = vm.withAgent(a, func() Value {
				v, err := vm.eval(e.Block)
				if err != nil {
					vm.fail(e.Pos, "%s", err.Error())
				}
				return v
			})
		}
		return List(out...), nil
	default:
		return Value{}, fmt.Errorf("of expected an agent or agentset")
	}
}

func (vm *VM) evalWith(e *withExpr) (Value, error) {
	target, err := vm.eval(e.Agents)
	if err != nil {
		return Value{}, err
	}
	ags, err := vm.asAgentset(target)
	if err != nil {
		return Value{}, err
	}
	var keep []Agent
	for _, a := range ags.Alive() {
		v := vm.withAgent(a, func() Value {
			val, err := vm.eval(e.Block)
			if err != nil {
				vm.fail(e.Pos, "%s", err.Error())
			}
			return val
		})
		b, err := v.Truth()
		if err != nil {
			return Value{}, err
		}
		if b {
			keep = append(keep, a)
		}
	}
	return SetVal(&AgentSet{Kind: ags.Kind, Breed: ags.Breed, Agents: keep}), nil
}

func (vm *VM) withExtreme(e *infixExpr) (Value, error) {
	target, err := vm.eval(e.Left)
	if err != nil {
		return Value{}, err
	}
	ags, err := vm.asAgentset(target)
	if err != nil {
		return Value{}, err
	}
	live := ags.Alive()
	if len(live) == 0 {
		return SetVal(&AgentSet{Kind: ags.Kind, Breed: ags.Breed}), nil
	}
	best := vm.score(live[0], e.Right, e.Pos)
	var keep []Agent
	keep = append(keep, live[0])
	for _, a := range live[1:] {
		sv := vm.score(a, e.Right, e.Pos)
		c, err := compare(sv, best)
		if err != nil {
			return Value{}, err
		}
		better := e.Op == "with-max" && c > 0 || e.Op == "with-min" && c < 0
		same := c == 0
		if better {
			best = sv
			keep = []Agent{a}
		} else if same {
			keep = append(keep, a)
		}
	}
	return SetVal(&AgentSet{Kind: ags.Kind, Breed: ags.Breed, Agents: keep}), nil
}

func (vm *VM) inRadius(e *infixExpr) (Value, error) {
	target, err := vm.eval(e.Left)
	if err != nil {
		return Value{}, err
	}
	ags, err := vm.asAgentset(target)
	if err != nil {
		return Value{}, err
	}
	r, err := vm.eval(e.Right)
	if err != nil {
		return Value{}, err
	}
	rad, err := r.Num()
	if err != nil {
		return Value{}, err
	}
	sx, sy, err := vm.selfPoint()
	if err != nil {
		return Value{}, err
	}
	var keep []Agent
	for _, a := range ags.Alive() {
		x, y, err := pointOf(a)
		if err != nil {
			return Value{}, err
		}
		if vm.World.Distance(sx, sy, x, y) <= rad {
			keep = append(keep, a)
		}
	}
	return SetVal(&AgentSet{Kind: ags.Kind, Breed: ags.Breed, Agents: keep}), nil
}

func (vm *VM) asAgentset(v Value) (*AgentSet, error) {
	if v.K != KindAgentset || v.Set == nil {
		return nil, fmt.Errorf("expected an agentset, got %s", v.TypeName())
	}
	return v.Set, nil
}

func (vm *VM) oneOf(v Value) (Value, error) {
	switch v.K {
	case KindAgentset:
		live := v.Set.Alive()
		if len(live) == 0 {
			return Nobody(), nil
		}
		return AgentVal(live[vm.World.rng.Intn(len(live))]), nil
	case KindList:
		if len(v.L) == 0 {
			return Value{}, fmt.Errorf("one-of an empty list")
		}
		return v.L[vm.World.rng.Intn(len(v.L))], nil
	default:
		return Value{}, fmt.Errorf("one-of expected a list or agentset")
	}
}

func (vm *VM) nOf(nVal, src Value) (Value, error) {
	n, err := asInt(nVal, "n-of")
	if err != nil {
		return Value{}, err
	}
	if n < 0 {
		return Value{}, fmt.Errorf("n-of expected a non-negative number")
	}
	switch src.K {
	case KindList:
		if n > len(src.L) {
			return Value{}, fmt.Errorf("n-of asked for more items than the list contains")
		}
		idx := vm.World.rng.Perm(len(src.L))[:n]
		sort.Ints(idx)
		out := make([]Value, n)
		for i, j := range idx {
			out[i] = src.L[j]
		}
		return List(out...), nil
	case KindAgentset:
		live := src.Set.Alive()
		if n > len(live) {
			return Value{}, fmt.Errorf("n-of asked for more agents than the agentset contains")
		}
		idx := vm.World.rng.Perm(len(live))[:n]
		sort.Ints(idx)
		ags := make([]Agent, n)
		for i, j := range idx {
			ags[i] = live[j]
		}
		return SetVal(&AgentSet{Kind: src.Set.Kind, Breed: src.Set.Breed, Agents: ags}), nil
	default:
		return Value{}, fmt.Errorf("n-of expected a list or agentset")
	}
}

func (vm *VM) foldNum(v Value, zero float64, fn func(a, b float64) float64) (Value, error) {
	xs, err := numsOf(v)
	if err != nil {
		return Value{}, err
	}
	acc := zero
	for _, n := range xs {
		acc = fn(acc, n)
	}
	return Num(acc), nil
}

func numsOf(v Value) ([]float64, error) {
	if v.K != KindList {
		return nil, fmt.Errorf("expected a list, got %s", v.TypeName())
	}
	out := make([]float64, len(v.L))
	for i, e := range v.L {
		n, err := e.Num()
		if err != nil {
			return nil, fmt.Errorf("expected a list of numbers")
		}
		out[i] = n
	}
	return out, nil
}

func (vm *VM) extreme(v Value, max bool) (Value, error) {
	if v.K != KindList || len(v.L) == 0 {
		return Value{}, fmt.Errorf("expected a non-empty list")
	}
	best := v.L[0]
	for _, e := range v.L[1:] {
		c, err := compare(e, best)
		if err != nil {
			return Value{}, err
		}
		if max && c > 0 || !max && c < 0 {
			best = e
		}
	}
	return best, nil
}

func (vm *VM) doMap(args []Value, pos Pos) (Value, error) {
	if args[0].K != KindTask || args[0].Task == nil || args[0].Task.IsCommand {
		return Value{}, fmt.Errorf("map expected a reporter task")
	}
	lists := args[1:]
	n := -1
	for _, l := range lists {
		if l.K != KindList {
			return Value{}, fmt.Errorf("map expected a list")
		}
		if n < 0 {
			n = len(l.L)
		} else if len(l.L) != n {
			return Value{}, fmt.Errorf("map expected lists of the same length")
		}
	}
	out := make([]Value, n)
	for i := 0; i < n; i++ {
		in := make([]Value, len(lists))
		for j := range lists {
			in[j] = lists[j].L[i]
		}
		out[i] = vm.callTaskRep(args[0].Task, in, pos)
	}
	return List(out...), nil
}

func (vm *VM) doFilter(args []Value, pos Pos) (Value, error) {
	if args[0].K != KindTask || args[0].Task == nil || args[0].Task.IsCommand {
		return Value{}, fmt.Errorf("filter expected a reporter task")
	}
	if args[1].K != KindList {
		return Value{}, fmt.Errorf("filter expected a list")
	}
	var out []Value
	for _, item := range args[1].L {
		v := vm.callTaskRep(args[0].Task, []Value{item}, pos)
		b, err := v.Truth()
		if err != nil {
			return Value{}, err
		}
		if b {
			out = append(out, item)
		}
	}
	if out == nil {
		out = []Value{}
	}
	return List(out...), nil
}

func (vm *VM) doReduce(args []Value, pos Pos) (Value, error) {
	if args[0].K != KindTask || args[0].Task == nil || args[0].Task.IsCommand {
		return Value{}, fmt.Errorf("reduce expected a reporter task")
	}
	if args[1].K != KindList || len(args[1].L) == 0 {
		return Value{}, fmt.Errorf("reduce expected a non-empty list")
	}
	acc := args[1].L[0]
	for _, item := range args[1].L[1:] {
		acc = vm.callTaskRep(args[0].Task, []Value{acc, item}, pos)
	}
	return acc, nil
}

func (vm *VM) doNValues(args []Value, pos Pos) (Value, error) {
	n, err := asInt(args[0], "n-values")
	if err != nil {
		return Value{}, err
	}
	if n < 0 {
		return Value{}, fmt.Errorf("n-values expected a non-negative number")
	}
	if args[1].K != KindTask || args[1].Task == nil || args[1].Task.IsCommand {
		return Value{}, fmt.Errorf("n-values expected a reporter task")
	}
	out := make([]Value, n)
	for i := 0; i < n; i++ {
		if len(args[1].Task.Params) == 0 {
			out[i] = vm.callTaskRep(args[1].Task, nil, pos)
		} else {
			out[i] = vm.callTaskRep(args[1].Task, []Value{Num(float64(i))}, pos)
		}
	}
	return List(out...), nil
}

func (vm *VM) doRange(args []Value) (Value, error) {
	nums := make([]int, len(args))
	for i, a := range args {
		n, err := asInt(a, "range")
		if err != nil {
			return Value{}, err
		}
		nums[i] = n
	}
	var start, end, step int
	switch len(nums) {
	case 1:
		start, end, step = 0, nums[0], 1
	case 2:
		start, end, step = nums[0], nums[1], 1
	case 3:
		start, end, step = nums[0], nums[1], nums[2]
	default:
		return Value{}, fmt.Errorf("range expected 1, 2, or 3 inputs")
	}
	if step == 0 {
		return Value{}, fmt.Errorf("range step cannot be zero")
	}
	var out []Value
	if step > 0 {
		for i := start; i < end; i += step {
			out = append(out, Num(float64(i)))
			if len(out) > 1000000 {
				return Value{}, fmt.Errorf("range is too large")
			}
		}
	} else {
		for i := start; i > end; i += step {
			out = append(out, Num(float64(i)))
			if len(out) > 1000000 {
				return Value{}, fmt.Errorf("range is too large")
			}
		}
	}
	if out == nil {
		out = []Value{}
	}
	return List(out...), nil
}

func (vm *VM) doSort(v Value) (Value, error) {
	var items []Value
	switch v.K {
	case KindList:
		items = append([]Value(nil), v.L...)
	case KindAgentset:
		for _, a := range v.Set.Alive() {
			items = append(items, AgentVal(a))
		}
	default:
		return Value{}, fmt.Errorf("sort expected a list or agentset")
	}
	var sortErr error
	sort.SliceStable(items, func(i, j int) bool {
		c, err := compare(items[i], items[j])
		if err != nil {
			sortErr = err
			return false
		}
		return c < 0
	})
	if sortErr != nil {
		return Value{}, sortErr
	}
	return List(items...), nil
}

func (vm *VM) doSortBy(task, list Value, pos Pos) (Value, error) {
	if task.K != KindTask || task.Task == nil || task.Task.IsCommand {
		return Value{}, fmt.Errorf("sort-by expected a reporter task")
	}
	if list.K != KindList {
		return Value{}, fmt.Errorf("sort-by expected a list")
	}
	items := append([]Value(nil), list.L...)
	var sortErr error
	sort.SliceStable(items, func(i, j int) bool {
		v := vm.callTaskRep(task.Task, []Value{items[i], items[j]}, pos)
		b, err := v.Truth()
		if err != nil {
			sortErr = err
			return false
		}
		return b
	})
	if sortErr != nil {
		return Value{}, sortErr
	}
	return List(items...), nil
}

func butEnd(v Value, first bool) (Value, error) {
	if v.K == KindString {
		r := []rune(v.S)
		if len(r) == 0 {
			return Value{}, fmt.Errorf("list or string is empty")
		}
		if first {
			return Str(string(r[1:])), nil
		}
		return Str(string(r[:len(r)-1])), nil
	}
	if v.K != KindList || len(v.L) == 0 {
		return Value{}, fmt.Errorf("list is empty")
	}
	if first {
		return List(append([]Value(nil), v.L[1:]...)...), nil
	}
	return List(append([]Value(nil), v.L[:len(v.L)-1]...)...), nil
}

func endItem(v Value, first bool) (Value, error) {
	if v.K == KindString {
		r := []rune(v.S)
		if len(r) == 0 {
			return Value{}, fmt.Errorf("string is empty")
		}
		if first {
			return Str(string(r[0])), nil
		}
		return Str(string(r[len(r)-1])), nil
	}
	if v.K != KindList || len(v.L) == 0 {
		return Value{}, fmt.Errorf("list is empty")
	}
	if first {
		return v.L[0], nil
	}
	return v.L[len(v.L)-1], nil
}

func (vm *VM) item(index, src Value) (Value, error) {
	i, err := asInt(index, "item")
	if err != nil {
		return Value{}, err
	}
	if i < 0 {
		return Value{}, fmt.Errorf("item index is negative")
	}
	if src.K == KindString {
		r := []rune(src.S)
		if i >= len(r) {
			return Value{}, fmt.Errorf("item index is too large")
		}
		return Str(string(r[i])), nil
	}
	if src.K != KindList {
		return Value{}, fmt.Errorf("item expected a list or string")
	}
	if i >= len(src.L) {
		return Value{}, fmt.Errorf("item index is too large")
	}
	return src.L[i], nil
}

func member(item, src Value) bool {
	if src.K == KindString {
		if item.K != KindString {
			return false
		}
		return strings.Contains(strings.ToLower(src.S), strings.ToLower(item.S))
	}
	if src.K != KindList {
		return false
	}
	for _, e := range src.L {
		if e.Equals(item) {
			return true
		}
	}
	return false
}

func position(item, src Value) int {
	if src.K == KindString && item.K == KindString {
		return strings.Index(strings.ToLower(src.S), strings.ToLower(item.S))
	}
	if src.K != KindList {
		return -1
	}
	for i, e := range src.L {
		if e.Equals(item) {
			return i
		}
	}
	return -1
}

func removeVal(item, src Value) (Value, error) {
	if src.K != KindList {
		return Value{}, fmt.Errorf("remove expected a list")
	}
	var out []Value
	for _, e := range src.L {
		if !e.Equals(item) {
			out = append(out, e)
		}
	}
	if out == nil {
		out = []Value{}
	}
	return List(out...), nil
}

func removeItem(index, src Value) (Value, error) {
	i, err := asInt(index, "remove-item")
	if err != nil {
		return Value{}, err
	}
	if src.K != KindList || i < 0 || i >= len(src.L) {
		return Value{}, fmt.Errorf("remove-item index out of range")
	}
	out := append(append([]Value{}, src.L[:i]...), src.L[i+1:]...)
	return List(out...), nil
}

func replaceItem(index, src, val Value) (Value, error) {
	i, err := asInt(index, "replace-item")
	if err != nil {
		return Value{}, err
	}
	if src.K != KindList || i < 0 || i >= len(src.L) {
		return Value{}, fmt.Errorf("replace-item index out of range")
	}
	out := append([]Value(nil), src.L...)
	out[i] = val
	return List(out...), nil
}

func sublist(src, a, b Value) (Value, error) {
	i, err := asInt(a, "sublist")
	if err != nil {
		return Value{}, err
	}
	j, err := asInt(b, "sublist")
	if err != nil {
		return Value{}, err
	}
	if src.K != KindList || i < 0 || j < i || j > len(src.L) {
		return Value{}, fmt.Errorf("sublist range is invalid")
	}
	return List(append([]Value(nil), src.L[i:j]...)...), nil
}

func substring(src, a, b Value) (Value, error) {
	if src.K != KindString {
		return Value{}, fmt.Errorf("substring expected a string")
	}
	i, err := asInt(a, "substring")
	if err != nil {
		return Value{}, err
	}
	j, err := asInt(b, "substring")
	if err != nil {
		return Value{}, err
	}
	r := []rune(src.S)
	if i < 0 || j < i || j > len(r) {
		return Value{}, fmt.Errorf("substring range is invalid")
	}
	return Str(string(r[i:j])), nil
}

func readLiteral(s string) (Value, error) {
	s = strings.TrimSpace(s)
	e, err := ParseReporter(s, &Program{})
	if err != nil {
		return Value{}, fmt.Errorf("read-from-string could not read %q", s)
	}
	// Only constants are legal. Reuse a throwaway VM so color names resolve.
	vm := NewVM(&Program{}, NewWorld(-1, 1, -1, 1, true, true))
	v, err := vm.eval(e)
	if err != nil {
		return Value{}, err
	}
	return v, nil
}

func (vm *VM) runResult(v Value, pos Pos) (Value, error) {
	switch v.K {
	case KindString:
		e, err := ParseReporter(v.S, vm.Prog)
		if err != nil {
			return Value{}, err
		}
		return vm.eval(e)
	case KindTask:
		if v.Task == nil || v.Task.IsCommand {
			return Value{}, fmt.Errorf("runresult expected a reporter task")
		}
		return vm.callTaskRep(v.Task, nil, pos), nil
	default:
		return Value{}, fmt.Errorf("runresult expected a string or reporter task")
	}
}

func (vm *VM) buildSet(name string, args []Value) (Value, error) {
	var kind AgentKind
	switch name {
	case "turtle-set":
		kind = KindTurtle
	case "patch-set":
		kind = KindPatch
	default:
		kind = KindLink
	}
	var ags []Agent
	seen := map[Agent]bool{}
	add := func(a Agent) error {
		if a == nil || a.IsDead() {
			return nil
		}
		if a.AgentKind() != kind {
			return fmt.Errorf("%s expected %ss", name, kind)
		}
		if !seen[a] {
			seen[a] = true
			ags = append(ags, a)
		}
		return nil
	}
	for _, arg := range args {
		switch arg.K {
		case KindAgent:
			if err := add(arg.A); err != nil {
				return Value{}, err
			}
		case KindAgentset:
			for _, a := range arg.Set.Alive() {
				if err := add(a); err != nil {
					return Value{}, err
				}
			}
		case KindNobody:
			continue
		default:
			return Value{}, fmt.Errorf("%s expected agents", name)
		}
	}
	return SetVal(&AgentSet{Kind: kind, Agents: ags}), nil
}

func (vm *VM) findMyLink(name string, args []Value) (Value, error) {
	self, ok := vm.self.(*Turtle)
	if !ok {
		return Value{}, fmt.Errorf("%s can only be used by a turtle", name)
	}
	other, ok := args[0].A.(*Turtle)
	if args[0].K != KindAgent || !ok {
		return Value{}, fmt.Errorf("%s expected a turtle", name)
	}
	var l *Link
	switch name {
	case "link-with":
		l = vm.World.findLink(self, other, false)
	case "out-link-to":
		l = vm.World.findLink(self, other, true)
	case "in-link-from":
		l = vm.World.findLink(other, self, true)
	}
	if l == nil {
		return Nobody(), nil
	}
	return AgentVal(l), nil
}

func (vm *VM) agentPoint(v Value) (float64, float64, error) {
	if v.K != KindAgent {
		return 0, 0, fmt.Errorf("expected an agent")
	}
	return pointOf(v.A)
}

func pointOf(a Agent) (float64, float64, error) {
	switch a := a.(type) {
	case *Turtle:
		return a.X, a.Y, nil
	case *Patch:
		return float64(a.PX), float64(a.PY), nil
	default:
		return 0, 0, fmt.Errorf("expected a turtle or patch")
	}
}

func (vm *VM) selfPoint() (float64, float64, error) {
	if vm.self == nil {
		return 0, 0, fmt.Errorf("this reporter can only be used by a turtle or patch")
	}
	return pointOf(vm.self)
}

func (vm *VM) agentPatch(v Value) (*Patch, error) {
	switch v.K {
	case KindAgent:
		switch a := v.A.(type) {
		case *Patch:
			return a, nil
		case *Turtle:
			return vm.World.PatchHere(a), nil
		default:
			return nil, fmt.Errorf("expected a turtle or patch")
		}
	case KindNobody:
		return nil, nil
	default:
		return nil, fmt.Errorf("expected an agent")
	}
}

func (vm *VM) patchAtOffset(dx, dy float64) *Patch {
	switch a := vm.self.(type) {
	case *Turtle:
		return vm.World.PatchAt(a.X+dx, a.Y+dy)
	case *Patch:
		return vm.World.PatchAt(float64(a.PX)+dx, float64(a.PY)+dy)
	default:
		return vm.World.PatchAt(dx, dy)
	}
}

func (vm *VM) getAgentVar(name string) (Value, bool, error) {
	switch a := vm.self.(type) {
	case *Turtle:
		return vm.getTurtleVar(a, name)
	case *Patch:
		return vm.getPatchVar(a, name)
	case *Link:
		return vm.getLinkVar(a, name)
	default:
		return Value{}, false, nil
	}
}

func (vm *VM) getTurtleVar(t *Turtle, name string) (Value, bool, error) {
	switch name {
	case "who":
		return Num(float64(t.Who)), true, nil
	case "color":
		return Num(t.Color), true, nil
	case "heading":
		return Num(t.Heading), true, nil
	case "xcor":
		return Num(t.X), true, nil
	case "ycor":
		return Num(t.Y), true, nil
	case "shape":
		return Str(t.Shape), true, nil
	case "label":
		return Str(t.Label), true, nil
	case "label-color":
		if v, ok := t.Vars["label-color"]; ok {
			return v, true, nil
		}
		return Num(9.9), true, nil
	case "breed":
		if t.Breed == "" {
			return SetVal(vm.World.Turtles()), true, nil
		}
		return SetVal(vm.World.TurtlesOfBreed(t.Breed)), true, nil
	case "hidden?":
		return Bool(t.Hidden), true, nil
	case "size":
		return Num(t.Size), true, nil
	case "pen-size":
		return Num(t.PenSize), true, nil
	case "pen-mode":
		if t.PenDown {
			return Str("down"), true, nil
		}
		return Str("up"), true, nil
	case "pxcor":
		p := vm.World.PatchHere(t)
		if p == nil {
			return Nobody(), true, nil
		}
		return Num(float64(p.PX)), true, nil
	case "pycor":
		p := vm.World.PatchHere(t)
		if p == nil {
			return Nobody(), true, nil
		}
		return Num(float64(p.PY)), true, nil
	case "pcolor":
		p := vm.World.PatchHere(t)
		if p == nil {
			return Num(0), true, nil
		}
		return patchColorValue(p), true, nil
	case "plabel":
		p := vm.World.PatchHere(t)
		if p == nil {
			return Str(""), true, nil
		}
		return Str(p.Label), true, nil
	}
	if v, ok := t.Vars[name]; ok && vm.World.turtleOwns(t, name) {
		return v, true, nil
	}
	if vm.World.patchOwns(name) {
		p := vm.World.PatchHere(t)
		if p == nil {
			return Num(0), true, nil
		}
		if v, ok := p.Vars[name]; ok {
			return v, true, nil
		}
		return Num(0), true, nil
	}
	return Value{}, false, nil
}

func (vm *VM) getPatchVar(p *Patch, name string) (Value, bool, error) {
	switch name {
	case "pxcor":
		return Num(float64(p.PX)), true, nil
	case "pycor":
		return Num(float64(p.PY)), true, nil
	case "pcolor":
		return patchColorValue(p), true, nil
	case "plabel":
		return Str(p.Label), true, nil
	case "plabel-color":
		if v, ok := p.Vars["plabel-color"]; ok {
			return v, true, nil
		}
		return Num(9.9), true, nil
	}
	if vm.World.patchOwns(name) {
		if v, ok := p.Vars[name]; ok {
			return v, true, nil
		}
		return Num(0), true, nil
	}
	return Value{}, false, nil
}

func (vm *VM) getLinkVar(l *Link, name string) (Value, bool, error) {
	switch name {
	case "end1":
		return AgentVal(l.End1), true, nil
	case "end2":
		return AgentVal(l.End2), true, nil
	case "color":
		return Num(l.Color), true, nil
	case "label":
		return Str(l.Label), true, nil
	case "label-color":
		if v, ok := l.Vars["label-color"]; ok {
			return v, true, nil
		}
		return Num(9.9), true, nil
	case "hidden?":
		return Bool(l.Hidden), true, nil
	case "breed":
		if l.Breed == "" {
			return SetVal(vm.World.Links()), true, nil
		}
		return SetVal(vm.linksOfBreed(l.Breed)), true, nil
	case "thickness":
		return Num(l.Thickness), true, nil
	case "shape":
		return Str(l.Shape), true, nil
	case "tie-mode":
		return Str(l.TieMode), true, nil
	}
	if v, ok := l.Vars[name]; ok {
		return v, true, nil
	}
	return Value{}, false, nil
}

func patchColorValue(p *Patch) Value {
	if p.HasRGB {
		return List(Num(float64(p.RGB.R)), Num(float64(p.RGB.G)), Num(float64(p.RGB.B)))
	}
	return Num(p.PColor)
}

func (vm *VM) setAgentVar(name string, v Value) (bool, error) {
	switch a := vm.self.(type) {
	case *Turtle:
		return vm.setTurtleVar(a, name, v)
	case *Patch:
		return vm.setPatchVar(a, name, v)
	case *Link:
		return vm.setLinkVar(a, name, v)
	default:
		return false, nil
	}
}

func (vm *VM) setTurtleVar(t *Turtle, name string, v Value) (bool, error) {
	switch name {
	case "who", "pxcor", "pycor":
		return true, fmt.Errorf("you can't change %s", name)
	case "color":
		n, _, err := ColorFromValue(v)
		if err != nil {
			return true, err
		}
		if v.K == KindNum {
			t.Color = v.N
		} else {
			// Keep a numeric stand-in; the RGB is not stored on turtles yet.
			t.Color = float64(n.R)
			_ = n
		}
		return true, nil
	case "heading":
		n, err := v.Num()
		if err != nil {
			return true, err
		}
		vm.World.SetHeading(t, n)
		return true, nil
	case "xcor":
		n, err := v.Num()
		if err != nil {
			return true, err
		}
		return true, vm.World.MoveTo(t, n, t.Y)
	case "ycor":
		n, err := v.Num()
		if err != nil {
			return true, err
		}
		return true, vm.World.MoveTo(t, t.X, n)
	case "shape":
		if v.K != KindString {
			return true, fmt.Errorf("shape must be a string")
		}
		t.Shape = strings.ToLower(v.S)
		return true, nil
	case "label":
		t.Label = v.Format(false)
		return true, nil
	case "label-color":
		t.Vars["label-color"] = v
		return true, nil
	case "breed":
		if v.K != KindAgentset || v.Set == nil || v.Set.Kind != KindTurtle {
			return true, fmt.Errorf("breed must be a turtle breed")
		}
		return true, vm.World.SetBreed(t, v.Set.Breed)
	case "hidden?":
		b, err := v.Truth()
		if err != nil {
			return true, err
		}
		t.Hidden = b
		return true, nil
	case "size":
		n, err := v.Num()
		if err != nil {
			return true, err
		}
		t.Size = n
		return true, nil
	case "pen-size":
		n, err := v.Num()
		if err != nil {
			return true, err
		}
		t.PenSize = n
		return true, nil
	case "pen-mode":
		if v.K != KindString {
			return true, fmt.Errorf("pen-mode must be a string")
		}
		switch strings.ToLower(v.S) {
		case "up":
			t.PenDown = false
		case "down":
			t.PenDown = true
		default:
			return true, fmt.Errorf("pen-mode must be \"up\" or \"down\"")
		}
		return true, nil
	case "pcolor":
		p := vm.World.PatchHere(t)
		if p == nil {
			return true, fmt.Errorf("turtle is not on a patch")
		}
		return true, setPatchColor(p, v)
	case "plabel":
		p := vm.World.PatchHere(t)
		if p == nil {
			return true, fmt.Errorf("turtle is not on a patch")
		}
		p.Label = v.Format(false)
		return true, nil
	}
	if vm.World.turtleOwns(t, name) {
		t.Vars[name] = v
		return true, nil
	}
	if vm.World.patchOwns(name) {
		p := vm.World.PatchHere(t)
		if p == nil {
			return true, fmt.Errorf("turtle is not on a patch")
		}
		p.Vars[name] = v
		return true, nil
	}
	return false, nil
}

func (vm *VM) setPatchVar(p *Patch, name string, v Value) (bool, error) {
	switch name {
	case "pxcor", "pycor":
		return true, fmt.Errorf("you can't change %s", name)
	case "pcolor":
		return true, setPatchColor(p, v)
	case "plabel":
		p.Label = v.Format(false)
		return true, nil
	case "plabel-color":
		p.Vars["plabel-color"] = v
		return true, nil
	}
	if vm.World.patchOwns(name) {
		p.Vars[name] = v
		return true, nil
	}
	return false, nil
}

func (vm *VM) setLinkVar(l *Link, name string, v Value) (bool, error) {
	switch name {
	case "end1", "end2":
		return true, fmt.Errorf("you can't change %s", name)
	case "color":
		if v.K != KindNum {
			return true, fmt.Errorf("color must be a number")
		}
		l.Color = v.N
		return true, nil
	case "label":
		l.Label = v.Format(false)
		return true, nil
	case "hidden?":
		b, err := v.Truth()
		if err != nil {
			return true, err
		}
		l.Hidden = b
		return true, nil
	case "thickness":
		n, err := v.Num()
		if err != nil {
			return true, err
		}
		l.Thickness = n
		return true, nil
	case "shape":
		if v.K != KindString {
			return true, fmt.Errorf("shape must be a string")
		}
		l.Shape = strings.ToLower(v.S)
		return true, nil
	case "tie-mode":
		if v.K != KindString {
			return true, fmt.Errorf("tie-mode must be a string")
		}
		l.TieMode = strings.ToLower(v.S)
		return true, nil
	case "breed":
		if v.K != KindAgentset || v.Set == nil {
			return true, fmt.Errorf("breed must be a link breed")
		}
		l.Breed = v.Set.Breed
		return true, nil
	}
	l.Vars[name] = v
	return true, nil
}

func setPatchColor(p *Patch, v Value) error {
	if v.K == KindNum {
		p.PColor = v.N
		p.HasRGB = false
		return nil
	}
	c, _, err := ColorFromValue(v)
	if err != nil {
		return err
	}
	p.RGB = c
	p.HasRGB = true
	return nil
}

func readLiteralCheck(s string) bool {
	for _, r := range s {
		if unicode.IsSpace(r) {
			return true
		}
	}
	return false
}
