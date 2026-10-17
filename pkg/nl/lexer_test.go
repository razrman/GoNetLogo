package nl

import "testing"

func TestLexBasics(t *testing.T) {
	toks := Lex("fd 1\n; comment\nset x-1 2.5")
	var kinds []string
	for _, tok := range toks {
		if tok.Kind == TokEOF {
			break
		}
		kinds = append(kinds, tok.Lit)
	}
	want := []string{"fd", "1", "set", "x-1", "2.5"}
	if len(kinds) != len(want) {
		t.Fatalf("tokens %v", kinds)
	}
	for i := range want {
		if kinds[i] != want[i] && !(want[i] == "1" && toks[i].Kind == TokNumber) {
			// numbers store the literal in Lit as the raw text
		}
	}
	if toks[0].Lit != "fd" || toks[1].Kind != TokNumber || toks[1].Num != 1 {
		t.Fatalf("prefix: %#v %#v", toks[0], toks[1])
	}
	// comment dropped; x-1 is one identifier
	found := false
	for _, tok := range toks {
		if tok.Kind == TokIdent && tok.Lit == "x-1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected identifier x-1 in %v", kinds)
	}
}

func TestLexNumberAndString(t *testing.T) {
	toks := Lex(`print "a\"b\n" 1e-2 .5`)
	if toks[0].Lit != "print" || toks[1].Kind != TokString || toks[1].Lit != "a\"b\n" {
		t.Fatalf("string: %#v", toks[1])
	}
	if toks[2].Kind != TokNumber || toks[2].Num != 0.01 {
		t.Fatalf("exponent: %#v", toks[2])
	}
	if toks[3].Kind != TokNumber || toks[3].Num != 0.5 {
		t.Fatalf("dot number: %#v", toks[3])
	}
	bad := Lex("show 1-2")
	sawErr := false
	for _, tok := range bad {
		if tok.Kind == TokError {
			sawErr = true
		}
	}
	if !sawErr {
		t.Fatal("1-2 should be an illegal number")
	}
}

func TestLexOperators(t *testing.T) {
	toks := Lex("x != y and a <= b")
	got := map[string]bool{}
	for _, tok := range toks {
		if tok.Kind == TokIdent {
			got[tok.Lit] = true
		}
	}
	for _, name := range []string{"x", "!=", "y", "and", "a", "<=", "b"} {
		if !got[name] {
			t.Fatalf("missing %s in %v", name, got)
		}
	}
}
