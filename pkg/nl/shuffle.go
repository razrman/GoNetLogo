package nl

// agentShuffler is NetLogo's shufflerator: a lazy Fisher-Yates walk that
// draws nextInt(len - i) for each slot but the last. It fetches the next
// agent before the caller runs code for the current one, so random draws
// inside an ask body interleave with the shuffle exactly as in NetLogo.
//
// skipDead mirrors ArrayAgentSet's shufflerator: a dead or nil agent is
// skipped at fetch time and the next slot is drawn at once. Without it the
// walk mirrors TreeAgentSet (turtles, links, breeds), which copies only
// live agents and never skips; callers drop agents that died meanwhile.
type agentShuffler struct {
	w        *World
	copy     []Agent
	i        int
	next     Agent
	skipDead bool
}

func (w *World) newShuffler(ags []Agent, skipDead bool) *agentShuffler {
	s := &agentShuffler{w: w, copy: append([]Agent(nil), ags...), skipDead: skipDead}
	if skipDead {
		for s.i < len(s.copy) && s.copy[s.i] == nil {
			s.i++
		}
	}
	s.fetch()
	return s
}

// shuffleSet starts a shufflerator over an agentset value. The canonical
// turtles, links, and breed sets are TreeAgentSets in NetLogo; every other
// set, including patches, is an ArrayAgentSet that keeps dead slots.
func (w *World) shuffleSet(set *AgentSet) *agentShuffler {
	ags, skipDead := shuffleSource(set)
	return w.newShuffler(ags, skipDead)
}

// shuffleSource returns the agents a shufflerator over set walks and
// whether it skips dead slots, following shuffleSet.
func shuffleSource(set *AgentSet) ([]Agent, bool) {
	if set == nil {
		return nil, false
	}
	if (set.All || set.Named) && set.Kind != KindPatch {
		return set.Alive(), false
	}
	return set.Agents, true
}

func (s *agentShuffler) fetch() {
	for {
		n := len(s.copy)
		if s.i >= n {
			s.next = nil
			return
		}
		if s.i < n-1 {
			r := s.i + s.w.nextInt(n-s.i)
			s.next = s.copy[r]
			s.copy[r] = s.copy[s.i]
		} else {
			s.next = s.copy[s.i]
		}
		s.i++
		if !s.skipDead || (s.next != nil && !s.next.IsDead()) {
			return
		}
	}
}

// Next returns the next agent. ok is false when the walk is done.
func (s *agentShuffler) Next() (Agent, bool) {
	if s.next == nil {
		return nil, false
	}
	a := s.next
	s.fetch()
	return a, true
}

// shuffledLive drains a shufflerator over set and returns the live agents
// in the order NetLogo visits them. Use it where NetLogo walks the whole
// shufflerator before running any user code that might draw.
func (w *World) shuffledLive(set *AgentSet) []Agent {
	it := w.shuffleSet(set)
	out := make([]Agent, 0, set.Count())
	for {
		a, ok := it.Next()
		if !ok {
			return out
		}
		if a != nil && !a.IsDead() {
			out = append(out, a)
		}
	}
}

// shuffleValues shuffles in place like java.util.Collections.shuffle with
// NetLogo's RNG, which is what the shuffle primitive uses.
func (w *World) shuffleValues(vs []Value) {
	for i := len(vs); i > 1; i-- {
		j := w.nextInt(i)
		vs[i-1], vs[j] = vs[j], vs[i-1]
	}
}

// randomSubsetValues is NetLogo's list n-of: selection sampling (Knuth's
// algorithm S) that keeps item i when nextInt(size - i) < n - j. The
// result keeps list order.
func (w *World) randomSubsetValues(vs []Value, n int) []Value {
	out := make([]Value, 0, n)
	size := len(vs)
	for i := 0; i < size && len(out) < n; i++ {
		if w.nextInt(size-i) < n-len(out) {
			out = append(out, vs[i])
		}
	}
	return out
}

// randomSubsetAgents is AgentSet.randomSubset over the live agents in
// iteration order. NetLogo special-cases one and two agents, so their
// draws differ from the general selection sampling.
func (w *World) randomSubsetAgents(live []Agent, n int) []Agent {
	count := len(live)
	switch n {
	case 0:
		return []Agent{}
	case 1:
		return []Agent{live[w.nextInt(count)]}
	case 2:
		r1 := w.nextInt(count)
		r2 := w.nextInt(count - 1)
		small, big := r2, r1
		if r2 >= r1 {
			small, big = r1, r2+1
		}
		return []Agent{live[small], live[big]}
	}
	out := make([]Agent, 0, n)
	for i := 0; len(out) < n; i++ {
		if w.nextInt(count-i) < n-len(out) {
			out = append(out, live[i])
		}
	}
	return out
}
