package lexer

type Type int

const (
	TokenIllegal Type = iota
	TokenEOF
	TokenIdent
	TokenNumber
	TokenString
	TokenLParen
	TokenRParen
	TokenLBrace
	TokenRBrace
	TokenLBracket
	TokenRBracket
	TokenColon
	TokenSemicolon
	TokenComma
	TokenDot
	TokenArrow
	TokenFatArrow
	TokenAssign
	TokenEq
	TokenNe
	TokenLT
	TokenLE
	TokenGT
	TokenGE
	TokenBang
	TokenAndAnd
	TokenOrOr
	TokenPlus
	TokenMinus
	TokenStar
	TokenSlash
	TokenPercent
	TokenAt
	TokenBitAnd
	TokenBitOr
	TokenBitXor
	TokenBitNot
	TokenShl
	TokenShr
	TokenKwTol
	TokenKwContract
	TokenKwInterface
	TokenKwLibrary
	TokenKwStorage
	TokenKwSlot
	TokenKwEvent
	TokenKwFn
	TokenKwConstructor
	TokenKwFallback
	TokenKwError
	TokenKwEnum
	TokenKwModifier
	TokenKwLet
	TokenKwSet
	TokenKwIf
	TokenKwElse
	TokenKwWhile
	TokenKwFor
	TokenKwBreak
	TokenKwContinue
	TokenKwReturn
	TokenKwRequire
	TokenKwAssert
	TokenKwRevert
	TokenKwEmit
)

func (t Type) String() string {
	switch t {
	case TokenIllegal:
		return "ILLEGAL"
	case TokenEOF:
		return "EOF"
	case TokenIdent:
		return "IDENT"
	case TokenNumber:
		return "NUMBER"
	case TokenString:
		return "STRING"
	case TokenLParen:
		return "("
	case TokenRParen:
		return ")"
	case TokenLBrace:
		return "{"
	case TokenRBrace:
		return "}"
	case TokenLBracket:
		return "["
	case TokenRBracket:
		return "]"
	case TokenColon:
		return ":"
	case TokenSemicolon:
		return ";"
	case TokenComma:
		return ","
	case TokenDot:
		return "."
	case TokenArrow:
		return "->"
	case TokenFatArrow:
		return "=>"
	case TokenAssign:
		return "="
	case TokenEq:
		return "=="
	case TokenNe:
		return "!="
	case TokenLT:
		return "<"
	case TokenLE:
		return "<="
	case TokenGT:
		return ">"
	case TokenGE:
		return ">="
	case TokenBang:
		return "!"
	case TokenAndAnd:
		return "&&"
	case TokenOrOr:
		return "||"
	case TokenPlus:
		return "+"
	case TokenMinus:
		return "-"
	case TokenStar:
		return "*"
	case TokenSlash:
		return "/"
	case TokenPercent:
		return "%"
	case TokenAt:
		return "@"
	case TokenBitAnd:
		return "&"
	case TokenBitOr:
		return "|"
	case TokenBitXor:
		return "^"
	case TokenBitNot:
		return "~"
	case TokenShl:
		return "<<"
	case TokenShr:
		return ">>"
	case TokenKwTol:
		return "tol"
	case TokenKwContract:
		return "contract"
	case TokenKwInterface:
		return "interface"
	case TokenKwLibrary:
		return "library"
	case TokenKwStorage:
		return "storage"
	case TokenKwSlot:
		return "slot"
	case TokenKwEvent:
		return "event"
	case TokenKwFn:
		return "fn"
	case TokenKwConstructor:
		return "constructor"
	case TokenKwFallback:
		return "fallback"
	case TokenKwError:
		return "error"
	case TokenKwEnum:
		return "enum"
	case TokenKwModifier:
		return "modifier"
	default:
		return "UNKNOWN"
	}
}

type Position struct {
	Offset int
	Line   int
	Column int
}

type Token struct {
	Type    Type
	Literal string
	Start   Position
	End     Position
}

func keywordType(lit string) Type {
	switch lit {
	case "tol":
		return TokenKwTol
	case "contract":
		return TokenKwContract
	case "interface":
		return TokenKwInterface
	case "library":
		return TokenKwLibrary
	case "storage":
		return TokenKwStorage
	case "slot":
		return TokenKwSlot
	case "event":
		return TokenKwEvent
	case "fn":
		return TokenKwFn
	case "constructor":
		return TokenKwConstructor
	case "fallback":
		return TokenKwFallback
	case "error":
		return TokenKwError
	case "enum":
		return TokenKwEnum
	case "modifier":
		return TokenKwModifier
	case "let":
		return TokenKwLet
	case "set":
		return TokenKwSet
	case "if":
		return TokenKwIf
	case "else":
		return TokenKwElse
	case "while":
		return TokenKwWhile
	case "for":
		return TokenKwFor
	case "break":
		return TokenKwBreak
	case "continue":
		return TokenKwContinue
	case "return":
		return TokenKwReturn
	case "require":
		return TokenKwRequire
	case "assert":
		return TokenKwAssert
	case "revert":
		return TokenKwRevert
	case "emit":
		return TokenKwEmit
	default:
		return TokenIdent
	}
}
