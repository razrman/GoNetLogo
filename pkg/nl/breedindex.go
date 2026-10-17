package nl

import "strings"

// rememberTurtle inserts t into its breed's who-ordered list.
func (w *World) rememberTurtle(t *Turtle) {
	if t == nil {
		return
	}
	if w.breedLive == nil {
		w.breedLive = map[string][]*Turtle{}
	}
	w.breedLive[t.Breed] = insertWho(w.breedLive[t.Breed], t)
}

// forgetTurtle removes t from its breed list. Breed must still be the old one.
func (w *World) forgetTurtle(t *Turtle) {
	if t == nil || w.breedLive == nil {
		return
	}
	w.breedLive[t.Breed] = removeWho(w.breedLive[t.Breed], t)
}

func insertWho(ts []*Turtle, t *Turtle) []*Turtle {
	i := sortSearchWho(ts, t.Who)
	if i < len(ts) && ts[i] == t {
		return ts
	}
	ts = append(ts, nil)
	copy(ts[i+1:], ts[i:])
	ts[i] = t
	return ts
}

func removeWho(ts []*Turtle, t *Turtle) []*Turtle {
	i := sortSearchWho(ts, t.Who)
	if i < len(ts) && ts[i] == t {
		copy(ts[i:], ts[i+1:])
		return ts[:len(ts)-1]
	}
	for i, o := range ts {
		if o == t {
			copy(ts[i:], ts[i+1:])
			return ts[:len(ts)-1]
		}
	}
	return ts
}

func sortSearchWho(ts []*Turtle, who int) int {
	lo, hi := 0, len(ts)
	for lo < hi {
		mid := lo + (hi-lo)/2
		if ts[mid] == nil || ts[mid].Who < who {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo
}

// shuffleTurtles copies ts into an agent slice and shuffles that copy.
// The input order must already be who order so the permutation matches
// Shuffle(Alive(snapshot)).
func (w *World) shuffleTurtles(ts []*Turtle) []Agent {
	out := make([]Agent, 0, len(ts))
	for _, t := range ts {
		if t != nil && !t.dead {
			out = append(out, t)
		}
	}
	w.rng.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}

func (w *World) shufflePatches() []Agent {
	out := make([]Agent, 0, len(w.patchOrder))
	for _, p := range w.patchOrder {
		if p != nil && !p.dead {
			out = append(out, p)
		}
	}
	w.rng.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}

// fastAgentQuery answers any?, count, and one-of on a breed, breed-here,
// turtles, turtles-here, or patches identifier without building an agentset.
// ok is false when the expression has to go through the normal evaluator.
func (vm *VM) fastAgentQuery(name string, arg Expr) (Value, bool, error) {
	if name != "any?" && name != "count" && name != "one-of" {
		return Value{}, false, nil
	}
	id, ok := arg.(*identExpr)
	if !ok {
		return Value{}, false, nil
	}
	ts, kind, handled := vm.agentIdent(id.Name)
	if !handled {
		return Value{}, false, nil
	}
	switch name {
	case "any?":
		if kind == KindPatch {
			return Bool(vm.World.patchCount() > 0), true, nil
		}
		return Bool(liveCount(ts) > 0), true, nil
	case "count":
		if kind == KindPatch {
			return Num(float64(vm.World.patchCount())), true, nil
		}
		return Num(float64(liveCount(ts))), true, nil
	default:
		if kind == KindPatch {
			ps := vm.World.livePatches()
			if len(ps) == 0 {
				return Nobody(), true, nil
			}
			return AgentVal(ps[vm.World.rng.Intn(len(ps))]), true, nil
		}
		t := pickLive(ts, vm.World.rng.Intn)
		if t == nil {
			return Nobody(), true, nil
		}
		return AgentVal(t), true, nil
	}
}

// agentIdent resolves a name the way evalIdent does, but only for the
// agentsets that have an index. handled is false when a local, agent
// variable, or global owns the name, or it is not one of those sets.
func (vm *VM) agentIdent(name string) (ts []*Turtle, kind AgentKind, handled bool) {
	if vm.scope != nil {
		if _, ok := vm.scope.get(name); ok {
			return nil, 0, false
		}
	}
	if vm.self != nil {
		if _, ok, err := vm.getAgentVar(name); ok || err != nil {
			return nil, 0, false
		}
	}
	if _, ok := vm.World.Global(name); ok {
		return nil, 0, false
	}
	switch name {
	case "turtles":
		return vm.World.liveTurtles(), KindTurtle, true
	case "patches":
		return nil, KindPatch, true
	case "turtles-here":
		return vm.World.TurtlesHere(vm.patchUnderSelf()), KindTurtle, true
	}
	if _, ok := vm.World.breeds[name]; ok {
		return vm.World.breedLive[name], KindTurtle, true
	}
	if strings.HasSuffix(name, "-here") {
		br := strings.TrimSuffix(name, "-here")
		if _, ok := vm.World.breeds[br]; ok {
			return turtlesBreedHere(vm.patchUnderSelf(), br), KindTurtle, true
		}
	}
	return nil, 0, false
}

func turtlesBreedHere(p *Patch, breed string) []*Turtle {
	if p == nil {
		return nil
	}
	var out []*Turtle
	for _, t := range p.turtles {
		if t != nil && !t.dead && t.Breed == breed {
			out = append(out, t)
		}
	}
	return out
}

func liveCount(ts []*Turtle) int {
	n := 0
	for _, t := range ts {
		if t != nil && !t.dead {
			n++
		}
	}
	return n
}

// pickLive chooses the k-th living turtle, with k drawn by the caller so
// empty lists do not consume a random number. intn is rng.Intn.
func pickLive(ts []*Turtle, intn func(int) int) *Turtle {
	n := liveCount(ts)
	if n == 0 {
		return nil
	}
	k := intn(n)
	for _, t := range ts {
		if t == nil || t.dead {
			continue
		}
		if k == 0 {
			return t
		}
		k--
	}
	return nil
}

func (w *World) patchCount() int {
	n := 0
	for _, p := range w.patchOrder {
		if p != nil && !p.dead {
			n++
		}
	}
	return n
}

func (w *World) livePatches() []*Patch {
	out := make([]*Patch, 0, len(w.patchOrder))
	for _, p := range w.patchOrder {
		if p != nil && !p.dead {
			out = append(out, p)
		}
	}
	return out
}

// askAgents shuffles the agentset named by an identifier. ok is false when
// the caller should evaluate the expression normally. The observer check
// matches ask of the canonical turtles or patches agentset.
func (vm *VM) askAgents(name string, pos Pos) (list []Agent, ok bool) {
	if vm.scope != nil {
		if _, ok := vm.scope.get(name); ok {
			return nil, false
		}
	}
	if vm.self != nil {
		if _, ok, err := vm.getAgentVar(name); ok || err != nil {
			return nil, false
		}
	}
	if _, ok := vm.World.Global(name); ok {
		return nil, false
	}
	switch name {
	case "turtles":
		if vm.self != nil {
			vm.fail(pos, "only the observer can ASK the set of all %ss", KindTurtle)
		}
		return vm.World.shuffleTurtles(vm.World.liveTurtles()), true
	case "patches":
		if vm.self != nil {
			vm.fail(pos, "only the observer can ASK the set of all %ss", KindPatch)
		}
		return vm.World.shufflePatches(), true
	case "turtles-here":
		return vm.World.shuffleTurtles(vm.World.TurtlesHere(vm.patchUnderSelf())), true
	}
	if _, ok := vm.World.breeds[name]; ok {
		return vm.World.shuffleTurtles(vm.World.breedLive[name]), true
	}
	if strings.HasSuffix(name, "-here") {
		br := strings.TrimSuffix(name, "-here")
		if _, ok := vm.World.breeds[br]; ok {
			return vm.World.shuffleTurtles(turtlesBreedHere(vm.patchUnderSelf(), br)), true
		}
	}
	return nil, false
}
