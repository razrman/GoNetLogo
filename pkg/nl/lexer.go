package nl

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Pos is a source location. Lines and columns are 1-based.
type Pos struct {
	Offset int
	Line   int
	Col    int
}

func (p Pos) String() string {
	if p.Line == 0 {
		return ""
	}
	return fmt.Sprintf("%d:%d", p.Line, p.Col)
}

// TokKind classifies tokens.
type TokKind int

const (
	TokEOF TokKind = iota
	TokIdent
	TokNumber
	TokString
	TokLBrack
	TokRBrack
	TokLParen
	TokRParen
	TokLBrace
	TokRBrace
	TokComma
	TokError
)

// Token is a lexer token. Identifiers are stored lowercased in Lit;
// Text keeps the original spelling for errors.
type Token struct {
	Kind TokKind
	Lit  string
	Text string
	Num  float64
	Pos  Pos
}

func (t Token) String() string {
	switch t.Kind {
	case TokEOF:
		return "EOF"
	case TokNumber:
		return formatNum(t.Num)
	case TokString:
		return strconv.Quote(t.Lit)
	default:
		if t.Text != "" {
			return t.Text
		}
		return t.Lit
	}
}

// identPunct matches NetLogo's identifier character set:
// letters, digits, and _.?=*!<>#+/%$^'&-
const identPunct = "_.?=*!<>#+/%$^'&-"

func isIdentChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune(identPunct, r)
}

// Lex tokenizes NetLogo source. Comments (from ';' to end of line) are dropped.
// Numbers follow NetLogo: a leading '-' or '.' can start a numeric literal, and
// a number that runs into identifier characters (as in 1-2 or 2*3) is an error.
// Identifiers may contain '-' and other punctuation, so subtraction and the
// other operators must be surrounded by whitespace (x - 1, not x-1).
func Lex(src string) []Token {
	l := &lexer{src: src, line: 1, col: 1}
	var toks []Token
	for {
		t := l.next()
		toks = append(toks, t)
		if t.Kind == TokEOF || t.Kind == TokError {
			// Keep scanning after an error so the parser can report it,
			// but stop on a hard unterminated token that consumed the rest.
			if t.Kind == TokError && (strings.Contains(t.Lit, "end of line") || strings.Contains(t.Lit, "end of file")) {
				toks = append(toks, Token{Kind: TokEOF, Pos: t.Pos})
				break
			}
			if t.Kind == TokEOF {
				break
			}
		}
	}
	return toks
}

type lexer struct {
	src  string
	i    int
	line int
	col  int
}

func (l *lexer) pos() Pos {
	return Pos{Offset: l.i, Line: l.line, Col: l.col}
}

func (l *lexer) peek() rune {
	if l.i >= len(l.src) {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(l.src[l.i:])
	return r
}

func (l *lexer) peekAt(n int) rune {
	i := l.i
	for k := 0; k < n; k++ {
		if i >= len(l.src) {
			return 0
		}
		_, w := utf8.DecodeRuneInString(l.src[i:])
		i += w
	}
	if i >= len(l.src) {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(l.src[i:])
	return r
}

func (l *lexer) bump() rune {
	if l.i >= len(l.src) {
		return 0
	}
	r, w := utf8.DecodeRuneInString(l.src[l.i:])
	l.i += w
	if r == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return r
}

func (l *lexer) next() Token {
	l.skipSpaceAndComments()
	if l.i >= len(l.src) {
		return Token{Kind: TokEOF, Pos: l.pos()}
	}
	p := l.pos()
	r := l.peek()
	switch r {
	case '[':
		l.bump()
		return Token{Kind: TokLBrack, Lit: "[", Text: "[", Pos: p}
	case ']':
		l.bump()
		return Token{Kind: TokRBrack, Lit: "]", Text: "]", Pos: p}
	case '(':
		l.bump()
		return Token{Kind: TokLParen, Lit: "(", Text: "(", Pos: p}
	case ')':
		l.bump()
		return Token{Kind: TokRParen, Lit: ")", Text: ")", Pos: p}
	case '{':
		l.bump()
		return Token{Kind: TokLBrace, Lit: "{", Text: "{", Pos: p}
	case '}':
		l.bump()
		return Token{Kind: TokRBrace, Lit: "}", Text: "}", Pos: p}
	case ',':
		l.bump()
		return Token{Kind: TokComma, Lit: ",", Text: ",", Pos: p}
	case '"':
		return l.lexString(p)
	}
	if l.numberStart() {
		return l.lexNumber(p)
	}
	if isIdentChar(r) {
		return l.lexIdent(p)
	}
	l.bump()
	return Token{Kind: TokError, Lit: fmt.Sprintf("this non-standard character is not allowed: %q", r), Text: string(r), Pos: p}
}

func (l *lexer) skipSpaceAndComments() {
	for l.i < len(l.src) {
		r := l.peek()
		if r == ';' {
			for l.i < len(l.src) && l.peek() != '\n' {
				l.bump()
			}
			continue
		}
		if unicode.IsSpace(r) {
			l.bump()
			continue
		}
		break
	}
}

func (l *lexer) numberStart() bool {
	r := l.peek()
	if unicode.IsDigit(r) {
		return true
	}
	if r == '.' && unicode.IsDigit(l.peekAt(1)) {
		return true
	}
	if r == '-' {
		n1 := l.peekAt(1)
		if unicode.IsDigit(n1) {
			return true
		}
		if n1 == '.' && unicode.IsDigit(l.peekAt(2)) {
			return true
		}
	}
	return false
}

func (l *lexer) lexNumber(p Pos) Token {
	start := l.i
	// Consume a syntactically valid float, then if identifier characters
	// continue, the whole run is an illegal number (NetLogo's rule that
	// makes "1-2" and "2*3" errors rather than arithmetic).
	if l.peek() == '-' {
		l.bump()
	}
	sawDigit := false
	sawDot := false
	for {
		r := l.peek()
		if unicode.IsDigit(r) {
			sawDigit = true
			l.bump()
			continue
		}
		if r == '.' && !sawDot {
			// A dot is part of the number only when a digit follows,
			// or when we already have digits (so "5." is accepted by ParseFloat).
			n1 := l.peekAt(1)
			if unicode.IsDigit(n1) || sawDigit {
				sawDot = true
				l.bump()
				continue
			}
		}
		break
	}
	if l.peek() == 'e' || l.peek() == 'E' {
		// look ahead for a valid exponent; if it isn't valid, don't consume it
		j := 1
		n := l.peekAt(j)
		if n == '+' || n == '-' {
			j++
			n = l.peekAt(j)
		}
		if unicode.IsDigit(n) {
			l.bump() // e
			if l.peek() == '+' || l.peek() == '-' {
				l.bump()
			}
			for unicode.IsDigit(l.peek()) {
				l.bump()
			}
		}
	}
	// Trailing identifier junk makes the token illegal.
	if isIdentChar(l.peek()) {
		for isIdentChar(l.peek()) {
			l.bump()
		}
		text := l.src[start:l.i]
		return Token{Kind: TokError, Lit: "illegal number format: " + text, Text: text, Pos: p}
	}
	text := l.src[start:l.i]
	if !sawDigit {
		return Token{Kind: TokError, Lit: "illegal number format: " + text, Text: text, Pos: p}
	}
	n, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return Token{Kind: TokError, Lit: "illegal number format: " + text, Text: text, Pos: p}
	}
	return Token{Kind: TokNumber, Lit: text, Text: text, Num: n, Pos: p}
}

func (l *lexer) lexIdent(p Pos) Token {
	start := l.i
	for isIdentChar(l.peek()) {
		l.bump()
	}
	text := l.src[start:l.i]
	return Token{Kind: TokIdent, Lit: strings.ToLower(text), Text: text, Pos: p}
}

func (l *lexer) lexString(p Pos) Token {
	l.bump() // opening quote
	var b strings.Builder
	for {
		if l.i >= len(l.src) {
			return Token{Kind: TokError, Lit: "end of file reached before closing quote", Text: b.String(), Pos: p}
		}
		r := l.peek()
		if r == '\n' {
			return Token{Kind: TokError, Lit: "end of line reached before closing quote", Text: b.String(), Pos: p}
		}
		l.bump()
		if r == '"' {
			return Token{Kind: TokString, Lit: b.String(), Text: b.String(), Pos: p}
		}
		if r == '\\' {
			if l.i >= len(l.src) {
				return Token{Kind: TokError, Lit: "end of file reached before closing quote", Pos: p}
			}
			esc := l.bump()
			switch esc {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			case '\\', '"':
				b.WriteRune(esc)
			default:
				return Token{Kind: TokError, Lit: "illegal character after backslash", Pos: p}
			}
			continue
		}
		b.WriteRune(r)
	}
}
