package nl

import (
	"fmt"
	"strings"
)

// Parse compiles a NetLogo program.
func Parse(src string) (*Program, error) {
	toks, err := cleanTokens(src)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks, prog: &Program{
		BreedOwn:      map[string][]string{},
		LinkBreedOwn:  map[string][]string{},
		InterfaceVars: map[string]bool{},
	}}
	if err := p.parseStructure(); err != nil {
		return nil, err
	}
	p.i = 0
	if err := p.parseBodies(); err != nil {
		return nil, err
	}
	p.prog.indexProcs()
	return p.prog, nil
}

// ParseCommands parses a command list (a button or the command center)
// against an existing program so breed and procedure names resolve.
func ParseCommands(src string, prog *Program) ([]Stmt, error) {
	toks, err := cleanTokens(src)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks, prog: prog}
	p.installDecls()
	stmts, err := p.parseStatements(func(t Token) bool { return t.Kind == TokEOF })
	if err != nil {
		return nil, err
	}
	if p.peek().Kind != TokEOF {
		return nil, p.errf(p.peek(), "expected end of commands")
	}
	return stmts, nil
}

// ParseReporter parses one reporter expression.
func ParseReporter(src string, prog *Program) (Expr, error) {
	toks, err := cleanTokens(src)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks, prog: prog}
	p.installDecls()
	e, err := p.parseExpr(precMin, goalAny, false)
	if err != nil {
		return nil, err
	}
	if p.peek().Kind != TokEOF {
		return nil, p.errf(p.peek(), "expected end of reporter")
	}
	return e, nil
}

func cleanTokens(src string) ([]Token, error) {
	toks := Lex(src)
	for _, t := range toks {
		if t.Kind == TokError {
			return nil, errorAt(t.Pos, "%s", t.Lit)
		}
	}
	return toks, nil
}

type parser struct {
	toks []Token
	i    int
	prog *Program

	breedPlural   map[string]string // plural -> singular
	breedSingular map[string]string
	linkPlural    map[string]LinkBreedDecl

	extraRep map[string]primSpec
	extraCmd map[string]primSpec

	// bodySpans records procedure body token ranges collected in pass 1.
	bodies []bodySpan
}

type bodySpan struct {
	proc       *Procedure
	start, end int // token indices, end exclusive, not including 'end'
}

func (p *parser) peek() Token {
	if p.i >= len(p.toks) {
		return Token{Kind: TokEOF}
	}
	return p.toks[p.i]
}

func (p *parser) next() Token {
	t := p.peek()
	if p.i < len(p.toks) {
		p.i++
	}
	return t
}

func (p *parser) errf(t Token, format string, args ...any) error {
	return errorAt(t.Pos, format, args...)
}

func (p *parser) parseStructure() error {
	p.breedPlural = map[string]string{}
	p.breedSingular = map[string]string{}
	p.linkPlural = map[string]LinkBreedDecl{}
	for p.peek().Kind != TokEOF {
		t := p.peek()
		if t.Kind != TokIdent {
			return p.errf(t, "expected a declaration or procedure, got %s", t.Text)
		}
		switch t.Lit {
		case "globals":
			p.next()
			names, err := p.parseNameList()
			if err != nil {
				return err
			}
			p.prog.Globals = append(p.prog.Globals, names...)
		case "turtles-own":
			p.next()
			names, err := p.parseNameList()
			if err != nil {
				return err
			}
			p.prog.TurtlesOwn = append(p.prog.TurtlesOwn, names...)
		case "patches-own":
			p.next()
			names, err := p.parseNameList()
			if err != nil {
				return err
			}
			p.prog.PatchesOwn = append(p.prog.PatchesOwn, names...)
		case "links-own":
			p.next()
			names, err := p.parseNameList()
			if err != nil {
				return err
			}
			p.prog.LinksOwn = append(p.prog.LinksOwn, names...)
		case "extensions":
			p.next()
			names, err := p.parseNameList()
			if err != nil {
				return err
			}
			p.prog.Extensions = append(p.prog.Extensions, names...)
		case "breed":
			p.next()
			pl, sg, err := p.parseBreedPair()
			if err != nil {
				return err
			}
			p.prog.Breeds = append(p.prog.Breeds, BreedDecl{Plural: pl, Singular: sg})
			p.breedPlural[pl] = sg
			p.breedSingular[sg] = pl
		case "directed-link-breed":
			p.next()
			pl, sg, err := p.parseBreedPair()
			if err != nil {
				return err
			}
			d := LinkBreedDecl{Plural: pl, Singular: sg, Directed: true}
			p.prog.LinkBreeds = append(p.prog.LinkBreeds, d)
			p.linkPlural[pl] = d
		case "undirected-link-breed":
			p.next()
			pl, sg, err := p.parseBreedPair()
			if err != nil {
				return err
			}
			d := LinkBreedDecl{Plural: pl, Singular: sg, Directed: false}
			p.prog.LinkBreeds = append(p.prog.LinkBreeds, d)
			p.linkPlural[pl] = d
		case "to", "to-report":
			if err := p.skipProcedure(); err != nil {
				return err
			}
		default:
			if strings.HasSuffix(t.Lit, "-own") {
				owner := strings.TrimSuffix(t.Lit, "-own")
				p.next()
				names, err := p.parseNameList()
				if err != nil {
					return err
				}
				if _, ok := p.breedPlural[owner]; ok {
					p.prog.BreedOwn[owner] = append(p.prog.BreedOwn[owner], names...)
				} else if _, ok := p.linkPlural[owner]; ok {
					p.prog.LinkBreedOwn[owner] = append(p.prog.LinkBreedOwn[owner], names...)
				} else {
					return p.errf(t, "nothing named %s to own variables", owner)
				}
				continue
			}
			return p.errf(t, "expected a declaration or procedure, got %s", t.Text)
		}
	}
	p.installDecls()
	return nil
}

func (p *parser) installDecls() {
	if p.extraRep == nil {
		p.extraRep = map[string]primSpec{}
	}
	if p.extraCmd == nil {
		p.extraCmd = map[string]primSpec{}
	}
	if p.breedPlural == nil {
		p.breedPlural = map[string]string{}
		p.breedSingular = map[string]string{}
		p.linkPlural = map[string]LinkBreedDecl{}
		if p.prog != nil {
			for _, b := range p.prog.Breeds {
				p.breedPlural[b.Plural] = b.Singular
				p.breedSingular[b.Singular] = b.Plural
			}
			for _, b := range p.prog.LinkBreeds {
				p.linkPlural[b.Plural] = b
			}
		}
	}
	for pl, sg := range p.breedPlural {
		p.extraRep[pl+"-here"] = primSpec{argc: 0}
		p.extraRep[pl+"-on"] = primSpec{argc: 1, prec: precNormal}
		p.extraRep[pl+"-at"] = primSpec{argc: 2, prec: precNormal}
		p.extraRep["is-"+sg+"?"] = primSpec{argc: 1, prec: precNormal}
		p.extraCmd["create-"+pl] = primSpec{argc: 1, optBlock: true}
		p.extraCmd["create-ordered-"+pl] = primSpec{argc: 1, optBlock: true}
		p.extraCmd["sprout-"+pl] = primSpec{argc: 1, optBlock: true}
		p.extraCmd["hatch-"+pl] = primSpec{argc: 1, optBlock: true}
	}
	for pl, b := range p.linkPlural {
		sg := b.Singular
		// singular: one turtle. plural: an agentset.
		if b.Directed {
			p.extraCmd["create-"+sg+"-to"] = primSpec{argc: 1, optBlock: true}
			p.extraCmd["create-"+sg+"-from"] = primSpec{argc: 1, optBlock: true}
			p.extraCmd["create-"+pl+"-to"] = primSpec{argc: 1, optBlock: true}
			p.extraCmd["create-"+pl+"-from"] = primSpec{argc: 1, optBlock: true}
		} else {
			p.extraCmd["create-"+sg+"-with"] = primSpec{argc: 1, optBlock: true}
			p.extraCmd["create-"+pl+"-with"] = primSpec{argc: 1, optBlock: true}
		}
		p.extraRep[pl] = primSpec{argc: 0}
	}
	for pl := range p.breedPlural {
		p.extraRep[pl] = primSpec{argc: 0}
	}
}

func (p *parser) parseNameList() ([]string, error) {
	if p.peek().Kind != TokLBrack {
		return nil, p.errf(p.peek(), "expected [")
	}
	p.next()
	var names []string
	for p.peek().Kind != TokRBrack {
		if p.peek().Kind == TokEOF {
			return nil, p.errf(p.peek(), "expected ]")
		}
		t := p.next()
		if t.Kind != TokIdent {
			return nil, p.errf(t, "expected a name")
		}
		names = append(names, t.Lit)
	}
	p.next()
	return names, nil
}

func (p *parser) parseBreedPair() (string, string, error) {
	if p.peek().Kind != TokLBrack {
		return "", "", p.errf(p.peek(), "expected [")
	}
	p.next()
	a := p.next()
	b := p.next()
	if a.Kind != TokIdent || b.Kind != TokIdent {
		return "", "", p.errf(a, "breed expected two names")
	}
	if p.peek().Kind != TokRBrack {
		return "", "", p.errf(p.peek(), "expected ]")
	}
	p.next()
	return a.Lit, b.Lit, nil
}

func (p *parser) skipProcedure() error {
	kw := p.next()
	nameTok := p.next()
	if nameTok.Kind != TokIdent {
		return p.errf(nameTok, "expected a procedure name")
	}
	proc := &Procedure{Name: nameTok.Lit, IsReporter: kw.Lit == "to-report", Pos: nameTok.Pos}
	if p.peek().Kind == TokLBrack {
		names, err := p.parseNameList()
		if err != nil {
			return err
		}
		proc.Params = names
	}
	start := p.i
	depth := 0
	for {
		t := p.peek()
		if t.Kind == TokEOF {
			return p.errf(kw, "expected end")
		}
		if t.Kind == TokLBrack || t.Kind == TokLParen || t.Kind == TokLBrace {
			depth++
		} else if t.Kind == TokRBrack || t.Kind == TokRParen || t.Kind == TokRBrace {
			if depth > 0 {
				depth--
			}
		}
		if t.Kind == TokIdent && t.Lit == "end" && depth == 0 {
			p.bodies = append(p.bodies, bodySpan{proc: proc, start: start, end: p.i})
			p.prog.Procedures = append(p.prog.Procedures, proc)
			p.next()
			return nil
		}
		p.next()
	}
}

func (p *parser) parseBodies() error {
	for _, b := range p.bodies {
		sub := &parser{
			toks:          append(append([]Token{}, p.toks[b.start:b.end]...), Token{Kind: TokEOF, Pos: p.toks[b.end].Pos}),
			prog:          p.prog,
			breedPlural:   p.breedPlural,
			breedSingular: p.breedSingular,
			linkPlural:    p.linkPlural,
			extraRep:      p.extraRep,
			extraCmd:      p.extraCmd,
		}
		stmts, err := sub.parseStatements(func(t Token) bool { return t.Kind == TokEOF })
		if err != nil {
			return err
		}
		b.proc.Body = stmts
	}
	return nil
}

func (p *parser) parseStatements(stop func(Token) bool) ([]Stmt, error) {
	var stmts []Stmt
	for !stop(p.peek()) {
		s, err := p.parseStatement(false)
		if err != nil {
			return nil, err
		}
		stmts = append(stmts, s)
	}
	return stmts, nil
}

func (p *parser) parseStatement(variadic bool) (Stmt, error) {
	t := p.peek()
	if t.Kind == TokLParen {
		p.next()
		s, err := p.parseStatement(true)
		if err != nil {
			return nil, err
		}
		if p.peek().Kind != TokRParen {
			return nil, p.errf(p.peek(), "expected )")
		}
		p.next()
		return s, nil
	}
	if t.Kind != TokIdent {
		return nil, p.errf(t, "expected a command")
	}
	name := t.Lit
	switch name {
	case "let":
		return p.parseLet()
	case "set":
		return p.parseSet()
	case "if":
		return p.parseIf()
	case "ifelse":
		return p.parseIfelse(variadic)
	case "while":
		return p.parseWhile()
	case "repeat":
		return p.parseRepeat()
	case "loop":
		return p.parseLoop()
	case "ask":
		return p.parseAsk()
	case "foreach":
		return p.parseForeach(variadic)
	case "report":
		p.next()
		e, err := p.parseExpr(precMin, goalAny, false)
		if err != nil {
			return nil, err
		}
		return &reportStmt{Value: e, Pos: t.Pos}, nil
	case "stop":
		p.next()
		return &stopStmt{Pos: t.Pos}, nil
	case "create-turtles", "crt":
		return p.parseCreate("", false)
	case "create-ordered-turtles", "cro":
		return p.parseCreate("", true)
	case "sprout":
		return p.parseSprout("")
	case "hatch":
		return p.parseHatch("")
	}
	if strings.HasPrefix(name, "create-ordered-") {
		br := strings.TrimPrefix(name, "create-ordered-")
		if _, ok := p.breedPlural[br]; ok {
			return p.parseCreate(br, true)
		}
	}
	if strings.HasPrefix(name, "create-") {
		br := strings.TrimPrefix(name, "create-")
		if _, ok := p.breedPlural[br]; ok {
			return p.parseCreate(br, false)
		}
	}
	if strings.HasPrefix(name, "sprout-") {
		br := strings.TrimPrefix(name, "sprout-")
		if _, ok := p.breedPlural[br]; ok {
			return p.parseSprout(br)
		}
	}
	if strings.HasPrefix(name, "hatch-") {
		br := strings.TrimPrefix(name, "hatch-")
		if _, ok := p.breedPlural[br]; ok {
			return p.parseHatch(br)
		}
	}
	if spec, ok := p.lookupCmd(name); ok {
		p.next()
		args, err := p.parsePrimArgs(spec, variadic, t.Pos)
		if err != nil {
			return nil, err
		}
		return &cmdStmt{Name: name, Args: args, Pos: t.Pos}, nil
	}
	if proc := p.lookupProc(name); proc != nil && !proc.IsReporter {
		p.next()
		args, err := p.parseFixedArgs(len(proc.Params), precMin, nil)
		if err != nil {
			return nil, err
		}
		return &cmdStmt{Name: name, Args: args, Pos: t.Pos}, nil
	}
	return nil, p.errf(t, "nothing named %s has been defined", strings.ToUpper(t.Text))
}

func (p *parser) lookupCmd(name string) (primSpec, bool) {
	if s, ok := p.extraCmd[name]; ok {
		return s, true
	}
	s, ok := commands[name]
	return s, ok
}

func (p *parser) lookupRep(name string) (primSpec, bool) {
	if s, ok := p.extraRep[name]; ok && (s.argc > 0 || s.variadic) {
		return s, true
	}
	s, ok := reporters[name]
	return s, ok
}

func (p *parser) lookupProc(name string) *Procedure {
	if p.prog == nil {
		return nil
	}
	for _, pr := range p.prog.Procedures {
		if pr.Name == name {
			return pr
		}
	}
	return nil
}

func (p *parser) parseLet() (Stmt, error) {
	t := p.next()
	names, err := p.parseVarNames()
	if err != nil {
		return nil, err
	}
	e, err := p.parseExpr(precMin, goalAny, false)
	if err != nil {
		return nil, err
	}
	return &letStmt{Names: names, Value: e, Pos: t.Pos}, nil
}

func (p *parser) parseSet() (Stmt, error) {
	t := p.next()
	names, err := p.parseVarNames()
	if err != nil {
		return nil, err
	}
	e, err := p.parseExpr(precMin, goalAny, false)
	if err != nil {
		return nil, err
	}
	return &setStmt{Names: names, Value: e, Pos: t.Pos}, nil
}

func (p *parser) parseVarNames() ([]string, error) {
	if p.peek().Kind == TokLBrack {
		return p.parseNameList()
	}
	t := p.next()
	if t.Kind != TokIdent {
		return nil, p.errf(t, "expected a variable name")
	}
	return []string{t.Lit}, nil
}

func (p *parser) parseIf() (Stmt, error) {
	t := p.next()
	cond, err := p.parseExpr(precMin, goalAny, false)
	if err != nil {
		return nil, err
	}
	body, err := p.expectCmdBlock()
	if err != nil {
		return nil, err
	}
	return &ifStmt{Cond: cond, Body: body, Pos: t.Pos}, nil
}

func (p *parser) parseIfelse(variadic bool) (Stmt, error) {
	t := p.next()
	if !variadic {
		cond, err := p.parseExpr(precMin, goalAny, false)
		if err != nil {
			return nil, err
		}
		thenB, err := p.expectCmdBlock()
		if err != nil {
			return nil, err
		}
		var elseB []Stmt
		if p.peek().Kind == TokLBrack {
			elseB, err = p.expectCmdBlock()
			if err != nil {
				return nil, err
			}
		}
		return &ifelseStmt{Cond: cond, Then: thenB, Else: elseB, Pos: t.Pos}, nil
	}
	// (ifelse c1 [cmds1] c2 [cmds2] ... [else])
	type branch struct {
		cond Expr
		body []Stmt
	}
	var pairs []branch
	for {
		if p.peek().Kind == TokRParen || p.peek().Kind == TokEOF {
			break
		}
		if p.peek().Kind == TokLBrack {
			elseB, err := p.expectCmdBlock()
			if err != nil {
				return nil, err
			}
			if p.peek().Kind != TokRParen && p.peek().Kind != TokEOF {
				return nil, p.errf(p.peek(), "ifelse else block must be the last input")
			}
			if len(pairs) == 0 {
				return nil, p.errf(t, "ifelse expected a condition")
			}
			stmt := &ifelseStmt{Cond: pairs[len(pairs)-1].cond, Then: pairs[len(pairs)-1].body, Else: elseB, Pos: t.Pos}
			for i := len(pairs) - 2; i >= 0; i-- {
				stmt = &ifelseStmt{Cond: pairs[i].cond, Then: pairs[i].body, Else: []Stmt{stmt}, Pos: t.Pos}
			}
			return stmt, nil
		}
		cond, err := p.parseExpr(precMin, goalAny, false)
		if err != nil {
			return nil, err
		}
		body, err := p.expectCmdBlock()
		if err != nil {
			return nil, err
		}
		pairs = append(pairs, branch{cond, body})
		if len(pairs) > 64 {
			return nil, p.errf(p.peek(), "too many arguments")
		}
	}
	return nil, p.errf(p.peek(), "ifelse expected an else block")
}

func (p *parser) parseWhile() (Stmt, error) {
	t := p.next()
	condE, err := p.parseExpr(precMin, goalRepBlock, false)
	if err != nil {
		return nil, err
	}
	rb, ok := condE.(*repBlockExpr)
	if !ok {
		return nil, p.errf(t, "while expected a reporter block")
	}
	body, err := p.expectCmdBlock()
	if err != nil {
		return nil, err
	}
	return &whileStmt{Cond: rb.Body, Body: body, Pos: t.Pos}, nil
}

func (p *parser) parseRepeat() (Stmt, error) {
	t := p.next()
	n, err := p.parseExpr(precMin, goalAny, false)
	if err != nil {
		return nil, err
	}
	body, err := p.expectCmdBlock()
	if err != nil {
		return nil, err
	}
	return &repeatStmt{Times: n, Body: body, Pos: t.Pos}, nil
}

func (p *parser) parseLoop() (Stmt, error) {
	t := p.next()
	body, err := p.expectCmdBlock()
	if err != nil {
		return nil, err
	}
	return &loopStmt{Body: body, Pos: t.Pos}, nil
}

func (p *parser) parseAsk() (Stmt, error) {
	t := p.next()
	agents, err := p.parseExpr(precMin, goalAny, false)
	if err != nil {
		return nil, err
	}
	body, err := p.expectCmdBlock()
	if err != nil {
		return nil, err
	}
	return &askStmt{Agents: agents, Body: body, Pos: t.Pos}, nil
}

func (p *parser) parseForeach(variadic bool) (Stmt, error) {
	t := p.next()
	var lists []Expr
	if variadic {
		for p.peek().Kind != TokRParen && p.peek().Kind != TokEOF {
			// The last argument is the command task, including a bare name
			// such as `(foreach xs print)`.
			g := goalAny
			if p.taskArgNext() {
				g = goalCmdTask
			}
			e, err := p.parseExpr(precMin, g, false)
			if err != nil {
				return nil, err
			}
			lists = append(lists, e)
		}
		if len(lists) < 2 {
			return nil, p.errf(t, "foreach expected at least a list and a command task")
		}
		task := lists[len(lists)-1]
		return &foreachStmt{Lists: lists[:len(lists)-1], Task: task, Pos: t.Pos}, nil
	}
	list, err := p.parseExpr(precMin, goalAny, false)
	if err != nil {
		return nil, err
	}
	task, err := p.parseExpr(precMin, goalCmdTask, false)
	if err != nil {
		return nil, err
	}
	return &foreachStmt{Lists: []Expr{list}, Task: task, Pos: t.Pos}, nil
}

func (p *parser) parseCreate(breed string, ordered bool) (Stmt, error) {
	t := p.next()
	n, err := p.parseExpr(precMin, goalAny, false)
	if err != nil {
		return nil, err
	}
	var body []Stmt
	if p.peek().Kind == TokLBrack {
		body, err = p.expectCmdBlock()
		if err != nil {
			return nil, err
		}
	}
	return &createStmt{Breed: breed, Ordered: ordered, Count: n, Body: body, Pos: t.Pos}, nil
}

func (p *parser) parseSprout(breed string) (Stmt, error) {
	t := p.next()
	n, err := p.parseExpr(precMin, goalAny, false)
	if err != nil {
		return nil, err
	}
	var body []Stmt
	if p.peek().Kind == TokLBrack {
		body, err = p.expectCmdBlock()
		if err != nil {
			return nil, err
		}
	}
	return &sproutStmt{Breed: breed, Count: n, Body: body, Pos: t.Pos}, nil
}

func (p *parser) parseHatch(breed string) (Stmt, error) {
	t := p.next()
	n, err := p.parseExpr(precMin, goalAny, false)
	if err != nil {
		return nil, err
	}
	var body []Stmt
	if p.peek().Kind == TokLBrack {
		body, err = p.expectCmdBlock()
		if err != nil {
			return nil, err
		}
	}
	return &hatchStmt{Breed: breed, Count: n, Body: body, Pos: t.Pos}, nil
}

func (p *parser) expectCmdBlock() ([]Stmt, error) {
	if p.peek().Kind != TokLBrack {
		return nil, p.errf(p.peek(), "expected a command block")
	}
	e, err := p.parseBracket(goalCmdBlock)
	if err != nil {
		return nil, err
	}
	cb, ok := e.(*cmdBlockExpr)
	if !ok {
		return nil, p.errf(p.peek(), "expected a command block")
	}
	return cb.Body, nil
}

func (p *parser) parsePrimArgs(spec primSpec, variadic bool, pos Pos) ([]Expr, error) {
	if spec.ref0 {
		t := p.next()
		if t.Kind != TokIdent {
			return nil, p.errf(t, "expected a variable name")
		}
		rest, err := p.parseFixedArgs(spec.argc-1, precMin, nil)
		if err != nil {
			return nil, err
		}
		args := []Expr{&identExpr{Name: t.Lit, Pos: t.Pos}}
		return append(args, rest...), nil
	}
	if variadic && spec.variadic {
		return p.parseVarArgs(spec)
	}
	args, err := p.parseFixedArgs(spec.argc, spec.prec, spec.goals)
	if err != nil {
		return nil, err
	}
	if spec.optBlock && p.peek().Kind == TokLBrack {
		blk, err := p.parseBracket(goalCmdBlock)
		if err != nil {
			return nil, err
		}
		args = append(args, blk)
	}
	return args, nil
}

func (p *parser) parseFixedArgs(n, prec int, goals []goal) ([]Expr, error) {
	args := make([]Expr, 0, n)
	for i := 0; i < n; i++ {
		g := goalAny
		if i < len(goals) {
			g = goals[i]
		}
		// Command arguments are parsed at the command precedence (0) when
		// prec is 0. Reporter arguments use the reporter precedence so that
		// looser infix operators stay outside the call.
		usePrec := prec
		if usePrec == 0 {
			usePrec = precMin
		}
		e, err := p.parseExpr(usePrec, g, false)
		if err != nil {
			return nil, err
		}
		args = append(args, e)
	}
	return args, nil
}

func (p *parser) parseVarArgs(spec primSpec) ([]Expr, error) {
	var args []Expr
	i := 0
	for {
		t := p.peek()
		if t.Kind == TokRParen || t.Kind == TokEOF {
			break
		}
		if t.Kind == TokIdent {
			if inf, ok := infixOps[t.Lit]; ok && len(args) >= spec.argc && inf.prec <= spec.prec {
				break
			}
		}
		g := spec.goalAt(i)
		// Repeating goals: after the declared ones, reuse the last non-block
		// or the pattern. ifelse-value alternates. For a generic variadic,
		// extra args are goalAny unless the last declared goal is a task
		// and this is still within the pattern.
		if i >= len(spec.goals) {
			g = goalAny
		}
		e, err := p.parseExpr(spec.prec, g, false)
		if err != nil {
			return nil, err
		}
		args = append(args, e)
		i++
		if i > 10000 {
			return nil, p.errf(t, "too many arguments")
		}
	}
	if len(args) < spec.argc && !spec.variadic {
		return nil, fmt.Errorf("expected %d inputs", spec.argc)
	}
	return args, nil
}

func (p *parser) parseExpr(minPrec int, g goal, variadic bool) (Expr, error) {
	left, err := p.parsePrimary(g, variadic)
	if err != nil {
		return nil, err
	}
	left, err = p.parseMore(left, minPrec)
	if err != nil {
		return nil, err
	}
	if pend, ok := left.(*pendingExpr); ok {
		left, err = p.resolvePending(pend, g)
		if err != nil {
			return nil, err
		}
	}
	return left, nil
}

func (p *parser) parsePrimary(g goal, variadic bool) (Expr, error) {
	t := p.peek()
	switch t.Kind {
	case TokNumber:
		p.next()
		return &numExpr{N: t.Num, Pos: t.Pos}, nil
	case TokString:
		p.next()
		return &strExpr{S: t.Lit, Pos: t.Pos}, nil
	case TokLParen:
		p.next()
		e, err := p.parseExpr(precMin, g, true)
		if err != nil {
			return nil, err
		}
		if p.peek().Kind != TokRParen {
			return nil, p.errf(p.peek(), "expected )")
		}
		p.next()
		return e, nil
	case TokLBrack:
		return p.parseBracket(g)
	case TokIdent:
		return p.parseIdentExpr(t, g, variadic)
	default:
		return nil, p.errf(t, "expected a reporter")
	}
}

func (p *parser) parseIdentExpr(t Token, g goal, variadic bool) (Expr, error) {
	name := t.Lit
	if name == "true" || name == "false" {
		p.next()
		return &boolExpr{B: name == "true", Pos: t.Pos}, nil
	}
	if name == "nobody" {
		p.next()
		return &nobodyExpr{Pos: t.Pos}, nil
	}
	// Concise task syntax: `map abs xs`, `reduce + xs`, `foreach xs print`.
	// The name is the task; its inputs come from the higher-order primitive.
	if g == goalRepTask || g == goalCmdTask {
		if e, ok := p.taskName(t, g); ok {
			return e, nil
		}
	}
	if inf, ok := infixOps[name]; ok {
		if name == "-" && (variadic || p.wantUnaryMinus()) {
			p.next()
			right, err := p.parseExpr(precNormal, goalAny, false)
			if err != nil {
				return nil, err
			}
			return &prefixExpr{Op: "-", Right: right, Pos: t.Pos}, nil
		}
		_ = inf
		return nil, p.errf(t, "expected a reporter, found %s", t.Text)
	}
	if spec, ok := p.lookupRep(name); ok {
		p.next()
		// Parenthesized ifelse-value repeats (condition, reporter block)
		// and ends with an else reporter block. Those brackets are not lists.
		if name == "ifelse-value" && variadic {
			args, err := p.parseIfelseValueVar()
			if err != nil {
				return nil, err
			}
			return &callExpr{Name: name, Args: args, Variadic: true, Pos: t.Pos}, nil
		}
		args, err := p.parsePrimArgs(spec, variadic, t.Pos)
		if err != nil {
			return nil, err
		}
		return &callExpr{Name: name, Args: args, Variadic: variadic && spec.variadic, Pos: t.Pos}, nil
	}
	if proc := p.lookupProc(name); proc != nil && proc.IsReporter && len(proc.Params) > 0 {
		p.next()
		args, err := p.parseFixedArgs(len(proc.Params), precNormal, nil)
		if err != nil {
			return nil, err
		}
		return &callExpr{Name: name, Args: args, Pos: t.Pos}, nil
	}
	p.next()
	return &identExpr{Name: name, Pos: t.Pos}, nil
}

func (p *parser) wantUnaryMinus() bool {
	// A minus with no left-hand side. Callers in primary position already
	// imply that. Always allow it; NetLogo is stricter inside non-paren
	// code, but accepting it does not reject any legal program.
	return true
}

func (p *parser) parseIfelseValueVar() ([]Expr, error) {
	var args []Expr
	for {
		if p.peek().Kind == TokRParen || p.peek().Kind == TokEOF {
			break
		}
		// A bracket where a condition would start is the else reporter.
		if p.peek().Kind == TokLBrack {
			e, err := p.parseBracket(goalRepBlock)
			if err != nil {
				return nil, err
			}
			args = append(args, e)
			if p.peek().Kind != TokRParen && p.peek().Kind != TokEOF {
				return nil, p.errf(p.peek(), "ifelse-value else block must be the last input")
			}
			break
		}
		cond, err := p.parseExpr(precMin, goalAny, false)
		if err != nil {
			return nil, err
		}
		args = append(args, cond)
		if p.peek().Kind != TokLBrack {
			return nil, p.errf(p.peek(), "ifelse-value expected a reporter block")
		}
		blk, err := p.parseBracket(goalRepBlock)
		if err != nil {
			return nil, err
		}
		args = append(args, blk)
		if len(args) > 64 {
			return nil, p.errf(p.peek(), "too many arguments")
		}
	}
	if len(args) < 3 || len(args)%2 == 0 {
		return nil, p.errf(p.peek(), "ifelse-value expected at least 3 inputs")
	}
	return args, nil
}

// taskArgNext reports whether the next foreach argument is the last one
// and should be parsed as a command task.
func (p *parser) taskArgNext() bool {
	if p.i >= len(p.toks) {
		return false
	}
	kind := p.toks[p.i].Kind
	switch kind {
	case TokIdent:
		return p.i+1 < len(p.toks) && p.toks[p.i+1].Kind == TokRParen
	case TokLBrack, TokLParen:
		closer := TokRBrack
		if kind == TokLParen {
			closer = TokRParen
		}
		depth := 0
		for i := p.i; i < len(p.toks); i++ {
			switch p.toks[i].Kind {
			case kind:
				depth++
			case closer:
				depth--
				if depth == 0 {
					return i+1 < len(p.toks) && p.toks[i+1].Kind == TokRParen
				}
			}
		}
	}
	return false
}

// taskName parses a bare reporter or command name used as a task.
// Unknown names are left alone so a variable holding a task still works.
func (p *parser) taskName(t Token, g goal) (Expr, bool) {
	name := t.Lit
	if g == goalRepTask {
		if infixTaskOp(name) {
			p.next()
			return infixTask(name, t.Pos), true
		}
		if spec, ok := p.lookupRep(name); ok {
			p.next()
			return reporterTask(name, spec.argc, t.Pos), true
		}
		if proc := p.lookupProc(name); proc != nil && proc.IsReporter {
			p.next()
			return reporterTask(name, len(proc.Params), t.Pos), true
		}
		if zeroArgReporters[name] {
			p.next()
			return reporterTask(name, 0, t.Pos), true
		}
		return nil, false
	}
	if spec, ok := p.lookupCmd(name); ok {
		p.next()
		return commandTask(name, spec.argc, t.Pos), true
	}
	if proc := p.lookupProc(name); proc != nil && !proc.IsReporter {
		p.next()
		return commandTask(name, len(proc.Params), t.Pos), true
	}
	return nil, false
}

func taskParams(argc int, pos Pos) ([]string, []Expr) {
	params := make([]string, argc)
	args := make([]Expr, argc)
	for i := 0; i < argc; i++ {
		params[i] = fmt.Sprintf("?%d", i+1)
		args[i] = &identExpr{Name: params[i], Pos: pos}
	}
	return params, args
}

func reporterTask(name string, argc int, pos Pos) Expr {
	params, args := taskParams(argc, pos)
	var body Expr
	if argc == 0 {
		body = &identExpr{Name: name, Pos: pos}
	} else {
		body = &callExpr{Name: name, Args: args, Pos: pos}
	}
	return &lambdaExpr{Params: params, Rep: body, Pos: pos}
}

func commandTask(name string, argc int, pos Pos) Expr {
	params, args := taskParams(argc, pos)
	return &lambdaExpr{
		Params:    params,
		IsCommand: true,
		Body:      []Stmt{&cmdStmt{Name: name, Args: args, Pos: pos}},
		Pos:       pos,
	}
}

func infixTask(op string, pos Pos) Expr {
	return &lambdaExpr{
		Params: []string{"?1", "?2"},
		Rep: &infixExpr{
			Op:    op,
			Left:  &identExpr{Name: "?1", Pos: pos},
			Right: &identExpr{Name: "?2", Pos: pos},
			Pos:   pos,
		},
		Pos: pos,
	}
}

func (p *parser) parseMore(left Expr, minPrec int) (Expr, error) {
	for {
		t := p.peek()
		if t.Kind != TokIdent {
			return left, nil
		}
		inf, ok := infixOps[t.Lit]
		if !ok {
			return left, nil
		}
		if !(inf.prec > minPrec || (inf.right && inf.prec == minPrec)) {
			return left, nil
		}
		p.next()
		if inf.repBlockL {
			if pend, ok := left.(*pendingExpr); ok {
				rb, err := p.resolvePending(pend, goalRepBlock)
				if err != nil {
					return nil, err
				}
				left = rb
			}
		}
		rightGoal := goalAny
		if inf.repBlockR {
			rightGoal = goalRepBlock
		}
		right, err := p.parseExpr(inf.prec, rightGoal, false)
		if err != nil {
			return nil, err
		}
		switch t.Lit {
		case "of":
			left = &ofExpr{Block: left, Agents: right, Pos: t.Pos}
		case "with":
			left = &withExpr{Agents: left, Block: right, Pos: t.Pos}
		default:
			// with-min, with-max, in-radius, and arithmetic.
			left = &infixExpr{Op: t.Lit, Left: left, Right: right, Pos: t.Pos}
		}
	}
}

type pendingExpr struct {
	toks []Token
	Pos  Pos
}

func (*pendingExpr) expr()     {}
func (e *pendingExpr) at() Pos { return e.Pos }

func (p *parser) parseBracket(g goal) (Expr, error) {
	open := p.next()
	if open.Kind != TokLBrack {
		return nil, p.errf(open, "expected [")
	}
	inner, err := p.collectBracket()
	if err != nil {
		return nil, err
	}
	if arrow, params, body, ok := splitArrow(inner); ok {
		return p.parseLambda(open.Pos, params, body, arrow)
	}
	switch g {
	case goalCmdBlock:
		return p.parseCmdBlockTokens(open.Pos, inner)
	case goalRepBlock:
		// Still delay so `[xcor] of turtles` inside a reporter block works
		// when this bracket is only the left of `of`. A reporter-block goal
		// on the bracket itself means the brackets ARE the block, and `of`
		// would be outside. Resolve now.
		return p.parseRepBlockTokens(open.Pos, inner)
	case goalCmdTask, goalRepTask:
		// No arrow: try the old "?" form as a single-argument task.
		if g == goalCmdTask {
			cb, err := p.parseCmdBlockTokens(open.Pos, inner)
			if err != nil {
				return nil, err
			}
			return &lambdaExpr{Params: []string{"?"}, IsCommand: true, Body: cb.(*cmdBlockExpr).Body, Pos: open.Pos}, nil
		}
		rb, err := p.parseRepBlockTokens(open.Pos, inner)
		if err != nil {
			return nil, err
		}
		return &lambdaExpr{Params: []string{"?"}, Rep: rb.(*repBlockExpr).Body, Pos: open.Pos}, nil
	default:
		return &pendingExpr{toks: inner, Pos: open.Pos}, nil
	}
}

func (p *parser) collectBracket() ([]Token, error) {
	var inner []Token
	depth := 1
	for {
		t := p.peek()
		if t.Kind == TokEOF {
			return nil, p.errf(t, "expected ]")
		}
		p.next()
		if t.Kind == TokLBrack {
			depth++
			inner = append(inner, t)
			continue
		}
		if t.Kind == TokRBrack {
			depth--
			if depth == 0 {
				return inner, nil
			}
		}
		inner = append(inner, t)
	}
}

func splitArrow(toks []Token) (arrow Token, params []string, body []Token, ok bool) {
	depth := 0
	for i, t := range toks {
		switch t.Kind {
		case TokLBrack, TokLParen, TokLBrace:
			depth++
		case TokRBrack, TokRParen, TokRBrace:
			depth--
		}
		if depth == 0 && t.Kind == TokIdent && t.Lit == "->" {
			head := toks[:i]
			body = toks[i+1:]
			if len(head) == 0 {
				return t, nil, body, true
			}
			// [ [x y] -> ... ] arrives here with head = [ x y ] because the
			// outer brackets were already consumed. head may itself be a
			// bracketed name list: first token '[' and last ']'.
			if head[0].Kind == TokLBrack {
				if head[len(head)-1].Kind != TokRBrack {
					return Token{}, nil, nil, false
				}
				for _, h := range head[1 : len(head)-1] {
					if h.Kind != TokIdent {
						return Token{}, nil, nil, false
					}
					params = append(params, h.Lit)
				}
				return t, params, body, true
			}
			for _, h := range head {
				if h.Kind != TokIdent {
					return Token{}, nil, nil, false
				}
				params = append(params, h.Lit)
			}
			return t, params, body, true
		}
	}
	return Token{}, nil, nil, false
}

func (p *parser) parseLambda(pos Pos, params []string, body []Token, arrow Token) (Expr, error) {
	if len(body) == 0 {
		return &lambdaExpr{Params: params, IsCommand: true, Pos: pos}, nil
	}
	sub := p.sub(body)
	if sub.startsWithCommand() {
		stmts, err := sub.parseStatements(func(t Token) bool { return t.Kind == TokEOF })
		if err != nil {
			return nil, err
		}
		return &lambdaExpr{Params: params, IsCommand: true, Body: stmts, Pos: pos}, nil
	}
	e, err := sub.parseExpr(precMin, goalAny, false)
	if err != nil {
		return nil, err
	}
	if sub.peek().Kind != TokEOF {
		return nil, sub.errf(sub.peek(), "expected end of reporter task")
	}
	return &lambdaExpr{Params: params, Rep: e, Pos: pos}, nil
}

func (p *parser) startsWithCommand() bool {
	t := p.peek()
	if t.Kind == TokLParen {
		// look at the next ident
		if p.i+1 < len(p.toks) && p.toks[p.i+1].Kind == TokIdent {
			t = p.toks[p.i+1]
		}
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
	if _, ok := p.lookupCmd(name); ok {
		return true
	}
	if proc := p.lookupProc(name); proc != nil && !proc.IsReporter {
		return true
	}
	if strings.HasPrefix(name, "create-") || strings.HasPrefix(name, "sprout-") || strings.HasPrefix(name, "hatch-") || strings.HasPrefix(name, "create-ordered-") {
		return true
	}
	return false
}

func (p *parser) sub(toks []Token) *parser {
	cp := append(append([]Token{}, toks...), Token{Kind: TokEOF})
	return &parser{
		toks:          cp,
		prog:          p.prog,
		breedPlural:   p.breedPlural,
		breedSingular: p.breedSingular,
		linkPlural:    p.linkPlural,
		extraRep:      p.extraRep,
		extraCmd:      p.extraCmd,
	}
}

func (p *parser) parseCmdBlockTokens(pos Pos, inner []Token) (Expr, error) {
	sub := p.sub(inner)
	stmts, err := sub.parseStatements(func(t Token) bool { return t.Kind == TokEOF })
	if err != nil {
		return nil, err
	}
	return &cmdBlockExpr{Body: stmts, Pos: pos}, nil
}

func (p *parser) parseRepBlockTokens(pos Pos, inner []Token) (Expr, error) {
	if len(inner) == 0 {
		return nil, errorAt(pos, "expected a reporter")
	}
	sub := p.sub(inner)
	e, err := sub.parseExpr(precMin, goalAny, false)
	if err != nil {
		return nil, err
	}
	if sub.peek().Kind != TokEOF {
		return nil, sub.errf(sub.peek(), "expected ]")
	}
	return &repBlockExpr{Body: e, Pos: pos}, nil
}

func (p *parser) resolvePending(pend *pendingExpr, g goal) (Expr, error) {
	if _, params, body, ok := splitArrow(pend.toks); ok {
		return p.parseLambda(pend.Pos, params, body, Token{})
	}
	switch g {
	case goalCmdBlock:
		return p.parseCmdBlockTokens(pend.Pos, pend.toks)
	case goalRepBlock:
		return p.parseRepBlockTokens(pend.Pos, pend.toks)
	case goalCmdTask:
		cb, err := p.parseCmdBlockTokens(pend.Pos, pend.toks)
		if err != nil {
			return nil, err
		}
		return &lambdaExpr{Params: []string{"?"}, IsCommand: true, Body: cb.(*cmdBlockExpr).Body, Pos: pend.Pos}, nil
	case goalRepTask:
		rb, err := p.parseRepBlockTokens(pend.Pos, pend.toks)
		if err != nil {
			return nil, err
		}
		return &lambdaExpr{Params: []string{"?"}, Rep: rb.(*repBlockExpr).Body, Pos: pend.Pos}, nil
	default:
		return p.parseLiteralList(pend.Pos, pend.toks)
	}
}

func (p *parser) parseLiteralList(pos Pos, toks []Token) (Expr, error) {
	sub := p.sub(toks)
	var elems []Expr
	for sub.peek().Kind != TokEOF {
		e, err := sub.parseLiteralElem()
		if err != nil {
			return nil, err
		}
		elems = append(elems, e)
	}
	return &listExpr{Elems: elems, Pos: pos}, nil
}

func (p *parser) parseLiteralElem() (Expr, error) {
	t := p.peek()
	switch t.Kind {
	case TokNumber:
		p.next()
		return &numExpr{N: t.Num, Pos: t.Pos}, nil
	case TokString:
		p.next()
		return &strExpr{S: t.Lit, Pos: t.Pos}, nil
	case TokLBrack:
		open := p.next()
		inner, err := p.collectBracket()
		if err != nil {
			return nil, err
		}
		return p.parseLiteralList(open.Pos, inner)
	case TokIdent:
		if t.Lit == "true" || t.Lit == "false" {
			p.next()
			return &boolExpr{B: t.Lit == "true", Pos: t.Pos}, nil
		}
		if t.Lit == "nobody" {
			p.next()
			return &nobodyExpr{Pos: t.Pos}, nil
		}
		if constantReporters[t.Lit] {
			p.next()
			return &identExpr{Name: t.Lit, Pos: t.Pos}, nil
		}
		return nil, p.errf(t, "expected a literal value, got %s (use the list primitive to build a list of variables)", t.Text)
	default:
		return nil, p.errf(t, "expected a literal value")
	}
}
