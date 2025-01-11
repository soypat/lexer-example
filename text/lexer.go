package text

import (
	"bufio"
	"errors"
	"io"
	"strconv"
	"unicode/utf8"
)

// This lexer implementation was ripped from https://github.com/soypat/goeda/blob/main/io/dsn/lexer.go
// The above lexer is in turn an implementation of the Lexer as described in "Writing An Interpreter In Go" by Thorsten Ball https://monkeylang.org/

type Token int

// Install stringer tool:
//  go install golang.org/x/tools/cmd/stringer@latest

//go:generate stringer -type=token -linecomment -output stringers.go .

// Token definitions.
const (
	tokUndefined Token = iota // undefined_or_uninitialized_token
	TokILLEGAL                // ILLEGAL
	TokEOF                    // EOF

	// Add literals below this line.

	TokLBRACE  // {
	TokRBRACE  // }
	TokIDENT   // IDENT
	TokEQ      // =
	TokINTEGER // INTEGER
	TokFLOAT   // FLOAT
	TokNewline // \n

	numToks = iota
)

// IsLiteral checks if the token is represented by text in the source.
func (tok Token) IsLiteral() bool {
	return tok > TokEOF && tok < numToks
}

// Lexer is a basic text file lexer for demonstrative purposes.
//
//	var l text.Lexer
//	l.Reset("source.go", file)
//	for {
//		tok, literalStart, literal := l.NextToken()
//		if !tok.IsLiteral() || l.Err() != nil {
//			break
//		}
//		// Do stuff with token and literals.
//	}
type Lexer struct {
	input bufio.Reader
	ch    rune // current character (utf8)
	err   error
	idbuf []byte // accumulation buffer.

	// Higher level statistics fields:

	source string // filename or source name.
	line   int    // file line number
	col    int    // column number in line
	pos    int    // byte position.
	braces int    // '{','}' braces counter to pick up on unbalanced braces early.
}

// Reset discards all state and buffered data and begins a new lexing
// procedure on the input r. It performs a single utf8 read to initialize.
func (l *Lexer) Reset(source string, r io.Reader) error {
	if r == nil {
		return errors.New("nil reader")
	}
	*l = Lexer{
		input:  l.input,
		line:   1,
		idbuf:  l.idbuf,
		source: source,
	}
	l.input.Reset(r)
	if l.idbuf == nil {
		l.idbuf = make([]byte, 0, 1024)
	}
	l.readChar()
	return l.err
}

// Source returns the name the lexer was reset/initialized with. Usually a filename.
func (l *Lexer) Source() string {
	return l.source
}

// Err returns the lexer error.
func (l *Lexer) Err() error {
	if l.err == io.EOF {
		return nil
	}
	return l.err
}

// LineCol returns the current line number and column number (utf8 relative).
func (l *Lexer) LineCol() (line, col int) {
	return l.line, l.col
}

// Pos returns the absolute position of the lexer in bytes from the start of the file.
func (l *Lexer) Pos() int { return l.pos }

// Braces returns the parentheses/braces depth at the current position.
func (l *Lexer) Braces() int { return l.braces }

// Next token parses the upcoming token and returns the literal representation
// of the token for identifiers, strings and numbers.
// The returned byte slice is reused between calls to NextToken.
func (l *Lexer) NextToken() (tok Token, startPos int, literal []byte) {
	l.skipWhitespace()
	startPos = l.pos
	if l.err == io.EOF {
		return TokEOF, startPos, nil
	} else if l.err != nil {
		return TokILLEGAL, startPos, nil
	}
	ch := l.ch
	switch ch {
	case '\n':
		tok = TokNewline
		l.readChar()
	case '{':
		tok = TokLBRACE
		l.readChar()
		l.braces++
	case '}':
		tok = TokRBRACE
		l.braces--
		if l.braces < 0 {
			l.err = errors.New("unbalanced parentheses")
		}
		l.readChar()
	case '=':
		tok = TokEQ
		l.readChar()
	default:
		if isDigit(ch) || ch == '-' {
			var isFloat bool
			tok = TokINTEGER // Handle floats later on.
			literal, isFloat = l.readNumber()
			if isFloat {
				tok = TokFLOAT
			}
		} else if isIdentifierChar(ch) {
			tok = TokIDENT
			literal = l.readIdentifier()
		} else {
			tok = TokILLEGAL
		}
	}
	return tok, startPos, literal
}

func (l *Lexer) readIdentifier() []byte {
	start := l.bufstart()
	for isIdentifierChar(l.ch) || isDigit(l.ch) {
		l.idbuf = utf8.AppendRune(l.idbuf, l.ch)
		l.readChar()
	}
	return l.idbuf[start:]
}

func (l *Lexer) readNumber() ([]byte, bool) {
	start := l.bufstart()
	seenDot := false
	if l.ch == '-' {
		// Consume leading negative character.
		l.idbuf = utf8.AppendRune(l.idbuf, l.ch)
		l.readChar()
	}
	for {
		ch := l.ch
		if !isDigit(ch) {
			if !seenDot && ch == '.' {
				seenDot = true
			} else {
				break
			}
		}
		l.idbuf = utf8.AppendRune(l.idbuf, l.ch)
		l.readChar()
	}
	return l.idbuf[start:], seenDot
}

func (l *Lexer) bufstart() int {
	const reuseMem = true
	if reuseMem {
		l.idbuf = l.idbuf[:0]
		return 0
	}
	return len(l.idbuf)
}

func (l *Lexer) skipWhitespace() {
	for isWhitespace(l.ch) {
		l.readChar()
	}
}

func (l *Lexer) readChar() {
	if l.err != nil {
		l.ch = 0 // Just in case annihilate char.
		return
	}
	ch, sz, err := l.input.ReadRune()
	if err != nil {
		l.ch = 0
		l.err = err
		return
	}
	if ch == '\n' {
		l.line++
		l.col = 0
	} else {
		l.col++
	}
	l.pos += sz
	l.ch = ch
}

func (l *Lexer) peekChar() rune {
	posstart := l.pos
	linestart := l.line
	colstart := l.col
	l.readChar()
	if l.err != nil {
		return 0
	}
	l.line = linestart
	l.err = l.input.UnreadRune()
	l.pos = posstart
	l.col = colstart
	return l.ch
}

// PositionString returns the "source:line:column" representation of the lexer's current position.
func (l *Lexer) PositionString() string {
	return string(l.AppendPositionString(make([]byte, 0, len(l.source)+1+3+1+1))) // Enough space for 3 digit line error
}

// AppendPositionString appends [Lexer.PositionString] to the buffer and returns the result.
func (l *Lexer) AppendPositionString(b []byte) []byte {
	b = append(b, l.source...)
	b = append(b, ':')
	b = strconv.AppendInt(b, int64(l.line), 10)
	if l.col > 0 {
		b = append(b, ':')
		b = strconv.AppendInt(b, int64(l.col), 10)
	}
	return b
}

func isIdentifierChar(ch rune) bool {
	return 'a' <= ch && ch <= 'z' || 'A' <= ch && ch <= 'Z' || ch == '_'
}

func isDigit(ch rune) bool {
	return '0' <= ch && ch <= '9'
}

func isDigitOrDecimal(ch rune) bool {
	return ch == '.' || isDigit(ch)
}

func isWhitespace(ch rune) bool {
	return ch == ' ' || ch == '\t' || ch == '\r'
}
