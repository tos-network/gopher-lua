package lexer

type Lexer struct {
	src  []byte
	idx  int
	line int
	col  int
}

func New(src []byte) *Lexer {
	return &Lexer{
		src:  src,
		line: 1,
		col:  1,
	}
}

func (l *Lexer) Next() Token {
	l.skipSpaceAndComments()
	start := l.pos()
	if l.eof() {
		return Token{Type: TokenEOF, Start: start, End: start}
	}

	ch := l.peek()
	switch ch {
	case '(':
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenLParen, Literal: "(", Start: start, End: end}
	case ')':
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenRParen, Literal: ")", Start: start, End: end}
	case '{':
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenLBrace, Literal: "{", Start: start, End: end}
	case '}':
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenRBrace, Literal: "}", Start: start, End: end}
	case '[':
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenLBracket, Literal: "[", Start: start, End: end}
	case ']':
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenRBracket, Literal: "]", Start: start, End: end}
	case ':':
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenColon, Literal: ":", Start: start, End: end}
	case ';':
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenSemicolon, Literal: ";", Start: start, End: end}
	case ',':
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenComma, Literal: ",", Start: start, End: end}
	case '.':
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenDot, Literal: ".", Start: start, End: end}
	case '-':
		if l.peekN(1) == '>' {
			l.advance()
			l.advance()
			end := l.lastPos()
			return Token{Type: TokenArrow, Literal: "->", Start: start, End: end}
		}
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenMinus, Literal: "-", Start: start, End: end}
	case '=':
		if l.peekN(1) == '>' {
			l.advance()
			l.advance()
			end := l.lastPos()
			return Token{Type: TokenFatArrow, Literal: "=>", Start: start, End: end}
		}
		if l.peekN(1) == '=' {
			l.advance()
			l.advance()
			end := l.lastPos()
			return Token{Type: TokenEq, Literal: "==", Start: start, End: end}
		}
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenAssign, Literal: "=", Start: start, End: end}
	case '!':
		if l.peekN(1) == '=' {
			l.advance()
			l.advance()
			end := l.lastPos()
			return Token{Type: TokenNe, Literal: "!=", Start: start, End: end}
		}
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenBang, Literal: "!", Start: start, End: end}
	case '<':
		if l.peekN(1) == '=' {
			l.advance()
			l.advance()
			end := l.lastPos()
			return Token{Type: TokenLE, Literal: "<=", Start: start, End: end}
		}
		if l.peekN(1) == '<' {
			l.advance()
			l.advance()
			end := l.lastPos()
			return Token{Type: TokenShl, Literal: "<<", Start: start, End: end}
		}
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenLT, Literal: "<", Start: start, End: end}
	case '>':
		if l.peekN(1) == '=' {
			l.advance()
			l.advance()
			end := l.lastPos()
			return Token{Type: TokenGE, Literal: ">=", Start: start, End: end}
		}
		if l.peekN(1) == '>' {
			l.advance()
			l.advance()
			end := l.lastPos()
			return Token{Type: TokenShr, Literal: ">>", Start: start, End: end}
		}
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenGT, Literal: ">", Start: start, End: end}
	case '&':
		if l.peekN(1) == '&' {
			l.advance()
			l.advance()
			end := l.lastPos()
			return Token{Type: TokenAndAnd, Literal: "&&", Start: start, End: end}
		}
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenBitAnd, Literal: "&", Start: start, End: end}
	case '|':
		if l.peekN(1) == '|' {
			l.advance()
			l.advance()
			end := l.lastPos()
			return Token{Type: TokenOrOr, Literal: "||", Start: start, End: end}
		}
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenBitOr, Literal: "|", Start: start, End: end}
	case '^':
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenBitXor, Literal: "^", Start: start, End: end}
	case '~':
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenBitNot, Literal: "~", Start: start, End: end}
	case '+':
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenPlus, Literal: "+", Start: start, End: end}
	case '*':
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenStar, Literal: "*", Start: start, End: end}
	case '/':
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenSlash, Literal: "/", Start: start, End: end}
	case '%':
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenPercent, Literal: "%", Start: start, End: end}
	case '@':
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenAt, Literal: "@", Start: start, End: end}
	case '"', '\'':
		lit := l.readString(ch)
		end := l.lastPos()
		return Token{Type: TokenString, Literal: lit, Start: start, End: end}
	}

	if isIdentStart(ch) {
		lit := l.readIdent()
		end := l.lastPos()
		return Token{Type: keywordType(lit), Literal: lit, Start: start, End: end}
	}

	if isDigit(ch) {
		lit := l.readNumber()
		end := l.lastPos()
		return Token{Type: TokenNumber, Literal: lit, Start: start, End: end}
	}

	l.advance()
	end := l.lastPos()
	return Token{Type: TokenIllegal, Literal: string([]byte{ch}), Start: start, End: end}
}

func (l *Lexer) skipSpaceAndComments() {
	for !l.eof() {
		ch := l.peek()
		if isSpace(ch) {
			l.advance()
			continue
		}
		if ch == '/' && l.peekN(1) == '/' {
			for !l.eof() && l.peek() != '\n' {
				l.advance()
			}
			continue
		}
		if ch == '/' && l.peekN(1) == '*' {
			l.advance()
			l.advance()
			for !l.eof() {
				if l.peek() == '*' && l.peekN(1) == '/' {
					l.advance()
					l.advance()
					break
				}
				l.advance()
			}
			continue
		}
		break
	}
}

func (l *Lexer) readIdent() string {
	start := l.idx
	for !l.eof() && isIdentPart(l.peek()) {
		l.advance()
	}
	return string(l.src[start:l.idx])
}

func (l *Lexer) readNumber() string {
	start := l.idx
	for !l.eof() {
		ch := l.peek()
		if isDigit(ch) || ch == '.' {
			l.advance()
			continue
		}
		break
	}
	return string(l.src[start:l.idx])
}

func (l *Lexer) readString(quote byte) string {
	start := l.idx
	l.advance() // opening quote
	for !l.eof() {
		ch := l.peek()
		if ch == '\\' {
			l.advance()
			if !l.eof() {
				l.advance()
			}
			continue
		}
		l.advance()
		if ch == quote {
			break
		}
	}
	return string(l.src[start:l.idx])
}

func (l *Lexer) eof() bool {
	return l.idx >= len(l.src)
}

func (l *Lexer) peek() byte {
	return l.src[l.idx]
}

func (l *Lexer) peekN(n int) byte {
	if l.idx+n >= len(l.src) {
		return 0
	}
	return l.src[l.idx+n]
}

func (l *Lexer) advance() {
	if l.eof() {
		return
	}
	ch := l.src[l.idx]
	l.idx++
	if ch == '\n' {
		l.line++
		l.col = 1
		return
	}
	l.col++
}

func (l *Lexer) pos() Position {
	return Position{
		Offset: l.idx,
		Line:   l.line,
		Column: l.col,
	}
}

func (l *Lexer) lastPos() Position {
	if l.col <= 1 {
		return Position{
			Offset: l.idx,
			Line:   l.line - 1,
			Column: 1,
		}
	}
	return Position{
		Offset: l.idx,
		Line:   l.line,
		Column: l.col - 1,
	}
}

func isSpace(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r'
}

func isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

func isIdentStart(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
}

func isIdentPart(ch byte) bool {
	return isIdentStart(ch) || isDigit(ch)
}
