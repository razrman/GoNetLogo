package nl

import (
	"fmt"
	"strings"
)

// Kind is the runtime type of a NetLogo value.
type Kind int

const (
	KindNobody Kind = iota
	KindBool
	KindNum
	KindString
	KindList
	KindAgent
	KindAgentset
	KindTask
)

// AgentKind distinguishes turtles, patches, and links.
type AgentKind int

const (
	KindObserver AgentKind = iota
	KindTurtle
	KindPatch
	KindLink
)

func (k AgentKind) String() string {
	switch k {
	case KindTurtle:
		return "turtle"
	case KindPatch:
		return "patch"
	case KindLink:
		return "link"
	default:
		return "observer"
	}
}

// Agent is a turtle, patch, or link.
type Agent interface {
	AgentKind() AgentKind
	AgentString() string
	IsDead() bool
}

// Task is an anonymous command or reporter (NetLogo arrow lambda).
type Task struct {
	Params    []string
	CmdBody   []Stmt
	RepBody   Expr
	IsCommand bool
	Scope     *scope
}

// Value is a NetLogo value. Unused fields are zero.
type Value struct {
	K      Kind
	N      float64
	B      bool
	S      string
	L      []Value
	A      Agent
	Set    *AgentSet
	Task   *Task
	quoted bool // string that should be printed with quotes (write vs print)
}

func Nobody() Value          { return Value{K: KindNobody} }
func Bool(b bool) Value      { return Value{K: KindBool, B: b} }
func Num(n float64) Value    { return Value{K: KindNum, N: n} }
func Str(s string) Value     { return Value{K: KindString, S: s} }
func List(xs ...Value) Value { return Value{K: KindList, L: xs} }
func AgentVal(a Agent) Value { return Value{K: KindAgent, A: a} }
func SetVal(s *AgentSet) Value {
	return Value{K: KindAgentset, Set: s}
}

func (v Value) IsNobody() bool { return v.K == KindNobody }

func (v Value) Truth() (bool, error) {
	if v.K != KindBool {
		return false, fmt.Errorf("expected a true/false value, got %s", v.TypeName())
	}
	return v.B, nil
}

func (v Value) Num() (float64, error) {
	if v.K != KindNum {
		return 0, fmt.Errorf("expected a number, got %s", v.TypeName())
	}
	return v.N, nil
}

func (v Value) TypeName() string {
	switch v.K {
	case KindNobody:
		return "nobody"
	case KindBool:
		return "true/false"
	case KindNum:
		return "a number"
	case KindString:
		return "a string"
	case KindList:
		return "a list"
	case KindAgent:
		if v.A == nil {
			return "an agent"
		}
		return "a " + v.A.AgentKind().String()
	case KindAgentset:
		return "an agentset"
	case KindTask:
		if v.Task != nil && v.Task.IsCommand {
			return "a command task"
		}
		return "a reporter task"
	default:
		return "a value"
	}
}

// Equals implements NetLogo "=". Strings compare case-insensitively.
func (v Value) Equals(o Value) bool {
	if v.K != o.K {
		return false
	}
	switch v.K {
	case KindNobody:
		return true
	case KindBool:
		return v.B == o.B
	case KindNum:
		return v.N == o.N
	case KindString:
		return strings.EqualFold(v.S, o.S)
	case KindList:
		if len(v.L) != len(o.L) {
			return false
		}
		for i := range v.L {
			if !v.L[i].Equals(o.L[i]) {
				return false
			}
		}
		return true
	case KindAgent:
		return v.A == o.A
	case KindAgentset:
		return v.Set.sameMembers(o.Set)
	case KindTask:
		return v.Task == o.Task
	default:
		return false
	}
}

// Format prints a value the way NetLogo's print does.
func (v Value) Format(quoted bool) string {
	switch v.K {
	case KindNobody:
		return "nobody"
	case KindBool:
		if v.B {
			return "true"
		}
		return "false"
	case KindNum:
		return formatNum(v.N)
	case KindString:
		if quoted {
			return `"` + escapeString(v.S) + `"`
		}
		return v.S
	case KindList:
		parts := make([]string, len(v.L))
		for i, e := range v.L {
			parts[i] = e.Format(true)
		}
		return "[" + strings.Join(parts, " ") + "]"
	case KindAgent:
		if v.A == nil || v.A.IsDead() {
			return "nobody"
		}
		return v.A.AgentString()
	case KindAgentset:
		if v.Set == nil {
			return "(agentset, 0 agents)"
		}
		return v.Set.String()
	case KindTask:
		if v.Task != nil && v.Task.IsCommand {
			return "(command task)"
		}
		return "(reporter task)"
	default:
		return "?"
	}
}

func (v Value) String() string { return v.Format(false) }

func formatNum(n float64) string {
	if n == float64(int64(n)) && n < 1e15 && n > -1e15 {
		return fmt.Sprintf("%d", int64(n))
	}
	s := fmt.Sprintf("%.10g", n)
	return s
}

func escapeString(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// AgentSet is an ordered collection of agents of one kind.
// All is true only for the canonical turtles, patches, or links agentset
// returned by those reporters. NetLogo forbids a turtle from asking that
// exact agentset.
type AgentSet struct {
	Kind   AgentKind
	Breed  string // empty means all of that kind; otherwise the breed plural
	All    bool
	Agents []Agent
}

func (s *AgentSet) Count() int {
	if s == nil {
		return 0
	}
	n := 0
	for _, a := range s.Agents {
		if a != nil && !a.IsDead() {
			n++
		}
	}
	return n
}

func (s *AgentSet) Alive() []Agent {
	if s == nil {
		return nil
	}
	out := make([]Agent, 0, len(s.Agents))
	for _, a := range s.Agents {
		if a != nil && !a.IsDead() {
			out = append(out, a)
		}
	}
	return out
}

func (s *AgentSet) String() string {
	kind := "agents"
	switch s.Kind {
	case KindTurtle:
		kind = "turtles"
	case KindPatch:
		kind = "patches"
	case KindLink:
		kind = "links"
	}
	name := kind
	if s.Breed != "" {
		name = s.Breed
	}
	return fmt.Sprintf("(agentset, %d %s)", s.Count(), name)
}

func (s *AgentSet) sameMembers(o *AgentSet) bool {
	if s == nil || o == nil {
		return s == o
	}
	if s.Kind != o.Kind {
		return false
	}
	a := s.Alive()
	b := o.Alive()
	if len(a) != len(b) {
		return false
	}
	seen := map[Agent]int{}
	for _, x := range a {
		seen[x]++
	}
	for _, x := range b {
		seen[x]--
		if seen[x] < 0 {
			return false
		}
	}
	return true
}

func (s *AgentSet) Copy() *AgentSet {
	if s == nil {
		return nil
	}
	cp := *s
	cp.Agents = append([]Agent(nil), s.Agents...)
	return &cp
}
