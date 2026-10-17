package nl

// Stmt is a command in a procedure or command block.
type Stmt interface {
	stmt()
	at() Pos
}

// Expr is a reporter.
type Expr interface {
	expr()
	at() Pos
}

type Program struct {
	Globals       []string
	InterfaceVars map[string]bool // widget variables; clear-all does not zero them
	TurtlesOwn    []string
	PatchesOwn    []string
	LinksOwn      []string
	Breeds        []BreedDecl
	LinkBreeds    []LinkBreedDecl
	BreedOwn      map[string][]string // plural -> extra turtle vars
	LinkBreedOwn  map[string][]string
	Extensions    []string
	Procedures    []*Procedure
	procs         map[string]*Procedure
}

type BreedDecl struct {
	Plural   string
	Singular string
}

type LinkBreedDecl struct {
	Plural   string
	Singular string
	Directed bool
}

type Procedure struct {
	Name       string
	Params     []string
	Body       []Stmt
	IsReporter bool
	Pos        Pos
}

func (p *Program) Proc(name string) *Procedure {
	if p == nil {
		return nil
	}
	return p.procs[name]
}

func (p *Program) indexProcs() {
	p.procs = map[string]*Procedure{}
	for _, pr := range p.Procedures {
		p.procs[pr.Name] = pr
	}
	if p.BreedOwn == nil {
		p.BreedOwn = map[string][]string{}
	}
	if p.LinkBreedOwn == nil {
		p.LinkBreedOwn = map[string][]string{}
	}
	if p.InterfaceVars == nil {
		p.InterfaceVars = map[string]bool{}
	}
}

// --- statements ---

type cmdStmt struct {
	Name string
	Args []Expr
	Pos  Pos
}

func (*cmdStmt) stmt()     {}
func (s *cmdStmt) at() Pos { return s.Pos }

type letStmt struct {
	Names []string
	Value Expr
	Pos   Pos
}

func (*letStmt) stmt()     {}
func (s *letStmt) at() Pos { return s.Pos }

type setStmt struct {
	// Vars are variable references. A single name, or several for
	// set [x y] list.
	Names []string
	Value Expr
	Pos   Pos
}

func (*setStmt) stmt()     {}
func (s *setStmt) at() Pos { return s.Pos }

type ifStmt struct {
	Cond Expr
	Body []Stmt
	Pos  Pos
}

func (*ifStmt) stmt()     {}
func (s *ifStmt) at() Pos { return s.Pos }

type ifelseStmt struct {
	Cond Expr
	Then []Stmt
	Else []Stmt
	Pos  Pos
}

func (*ifelseStmt) stmt()     {}
func (s *ifelseStmt) at() Pos { return s.Pos }

type whileStmt struct {
	Cond Expr // reporter, evaluated each time (already the inside of the brackets)
	Body []Stmt
	Pos  Pos
}

func (*whileStmt) stmt()     {}
func (s *whileStmt) at() Pos { return s.Pos }

type repeatStmt struct {
	Times Expr
	Body  []Stmt
	Pos   Pos
}

func (*repeatStmt) stmt()     {}
func (s *repeatStmt) at() Pos { return s.Pos }

type loopStmt struct {
	Body []Stmt
	Pos  Pos
}

func (*loopStmt) stmt()     {}
func (s *loopStmt) at() Pos { return s.Pos }

type askStmt struct {
	Agents Expr
	Body   []Stmt
	Pos    Pos
}

func (*askStmt) stmt()     {}
func (s *askStmt) at() Pos { return s.Pos }

type createStmt struct {
	Breed   string // empty means turtles
	Ordered bool
	Count   Expr
	Body    []Stmt
	Pos     Pos
}

func (*createStmt) stmt()     {}
func (s *createStmt) at() Pos { return s.Pos }

type sproutStmt struct {
	Breed string
	Count Expr
	Body  []Stmt
	Pos   Pos
}

func (*sproutStmt) stmt()     {}
func (s *sproutStmt) at() Pos { return s.Pos }

type hatchStmt struct {
	Breed string // empty means parent's breed
	Count Expr
	Body  []Stmt
	Pos   Pos
}

func (*hatchStmt) stmt()     {}
func (s *hatchStmt) at() Pos { return s.Pos }

type foreachStmt struct {
	Lists []Expr
	Task  Expr
	Pos   Pos
}

func (*foreachStmt) stmt()     {}
func (s *foreachStmt) at() Pos { return s.Pos }

type reportStmt struct {
	Value Expr
	Pos   Pos
}

func (*reportStmt) stmt()     {}
func (s *reportStmt) at() Pos { return s.Pos }

type stopStmt struct{ Pos Pos }

func (*stopStmt) stmt()     {}
func (s *stopStmt) at() Pos { return s.Pos }

// --- expressions ---

type identExpr struct {
	Name string
	Pos  Pos
}

func (*identExpr) expr()     {}
func (e *identExpr) at() Pos { return e.Pos }

type numExpr struct {
	N   float64
	Pos Pos
}

func (*numExpr) expr()     {}
func (e *numExpr) at() Pos { return e.Pos }

type strExpr struct {
	S   string
	Pos Pos
}

func (*strExpr) expr()     {}
func (e *strExpr) at() Pos { return e.Pos }

type boolExpr struct {
	B   bool
	Pos Pos
}

func (*boolExpr) expr()     {}
func (e *boolExpr) at() Pos { return e.Pos }

type nobodyExpr struct{ Pos Pos }

func (*nobodyExpr) expr()     {}
func (e *nobodyExpr) at() Pos { return e.Pos }

type listExpr struct {
	Elems []Expr
	Pos   Pos
}

func (*listExpr) expr()     {}
func (e *listExpr) at() Pos { return e.Pos }

type callExpr struct {
	Name     string
	Args     []Expr
	Variadic bool
	Pos      Pos
}

func (*callExpr) expr()     {}
func (e *callExpr) at() Pos { return e.Pos }

type infixExpr struct {
	Op          string
	Left, Right Expr
	Pos         Pos
}

func (*infixExpr) expr()     {}
func (e *infixExpr) at() Pos { return e.Pos }

type prefixExpr struct {
	Op    string
	Right Expr
	Pos   Pos
}

func (*prefixExpr) expr()     {}
func (e *prefixExpr) at() Pos { return e.Pos }

type cmdBlockExpr struct {
	Body []Stmt
	Pos  Pos
}

func (*cmdBlockExpr) expr()     {}
func (e *cmdBlockExpr) at() Pos { return e.Pos }

type repBlockExpr struct {
	Body Expr
	Pos  Pos
}

func (*repBlockExpr) expr()     {}
func (e *repBlockExpr) at() Pos { return e.Pos }

type lambdaExpr struct {
	Params    []string
	IsCommand bool
	Body      []Stmt
	Rep       Expr
	Pos       Pos
}

func (*lambdaExpr) expr()     {}
func (e *lambdaExpr) at() Pos { return e.Pos }

type ofExpr struct {
	Block  Expr // reporter block or reporter task
	Agents Expr
	Pos    Pos
}

func (*ofExpr) expr()     {}
func (e *ofExpr) at() Pos { return e.Pos }

type withExpr struct {
	Agents Expr
	Block  Expr
	Pos    Pos
}

func (*withExpr) expr()     {}
func (e *withExpr) at() Pos { return e.Pos }
