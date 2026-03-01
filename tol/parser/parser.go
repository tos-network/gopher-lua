package parser

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/tos-network/tolang/tol/ast"
	"github.com/tos-network/tolang/tol/diag"
	"github.com/tos-network/tolang/tol/lexer"
)

type Parser struct {
	filename string
	lex      *lexer.Lexer
	cur      lexer.Token
	diags    diag.Diagnostics
}

func ParseFile(filename string, src []byte) (*ast.Module, diag.Diagnostics) {
	p := &Parser{
		filename: filename,
		lex:      lexer.New(src),
	}
	p.next()
	mod := p.parseModule()
	return mod, p.diags
}

func (p *Parser) parseModule() *ast.Module {
	mod := &ast.Module{}

	if !p.expect(lexer.TokenKwTol, diag.CodeParseUnexpected, "expected 'tol' header") {
		return mod
	}

	versionTok := p.cur
	if !p.expect(lexer.TokenNumber, diag.CodeParseUnexpected, "expected language version after 'tol'") {
		return mod
	}
	mod.Version = versionTok.Literal

	for p.cur.Type == lexer.TokenKwInterface || p.cur.Type == lexer.TokenKwLibrary {
		p.parseSkippedTopDecl(mod)
	}

	if !p.expect(lexer.TokenKwContract, diag.CodeParseUnexpected, "expected 'contract' declaration") {
		return mod
	}

	contractName := p.cur
	if !p.expect(lexer.TokenIdent, diag.CodeParseUnexpected, "expected contract name") {
		return mod
	}
	mod.Contract = &ast.ContractDecl{Name: contractName.Literal}

	if !p.expect(lexer.TokenLBrace, diag.CodeParseUnexpected, "expected '{' after contract name") {
		return mod
	}

	for p.cur.Type != lexer.TokenRBrace && p.cur.Type != lexer.TokenEOF {
		p.parseContractMember(mod.Contract)
	}
	if !p.expect(lexer.TokenRBrace, diag.CodeParseUnexpected, "expected '}' to close contract body") {
		return mod
	}

	if p.cur.Type != lexer.TokenEOF {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: fmt.Sprintf("unexpected token '%s' after contract declaration", p.cur.Literal),
			Span:    p.span(p.cur),
		})
	}

	return mod
}

func (p *Parser) parseSkippedTopDecl(mod *ast.Module) {
	kind := p.cur.Literal
	p.next()

	nameTok := p.cur
	if !p.expect(lexer.TokenIdent, diag.CodeParseUnexpected, fmt.Sprintf("expected %s name", kind)) {
		return
	}

	if !p.consumeBlock(kind + " body") {
		return
	}

	mod.SkippedTopDecls = append(mod.SkippedTopDecls, ast.SkippedTopDecl{
		Kind: kind,
		Name: nameTok.Literal,
	})
}

func (p *Parser) parseContractMember(contract *ast.ContractDecl) {
	if p.cur.Type == lexer.TokenAt {
		selectorOverride, ok := p.parseFunctionAttributes()
		if !ok {
			return
		}
		if p.cur.Type != lexer.TokenKwFn {
			p.addDiag(diag.Diagnostic{
				Code:    diag.CodeParseUnsupported,
				Message: "attributes are currently supported only before function declarations",
				Span:    p.span(p.cur),
			})
			p.syncUnknownMember()
			return
		}
		fn := p.parseFunctionDecl(selectorOverride)
		if fn != nil {
			contract.Functions = append(contract.Functions, *fn)
		}
		return
	}

	switch p.cur.Type {
	case lexer.TokenKwStorage:
		st := p.parseStorageDecl()
		if st == nil {
			return
		}
		if contract.Storage != nil {
			p.addDiag(diag.Diagnostic{
				Code:    diag.CodeParseUnsupported,
				Message: "duplicate storage block is not supported",
				Span:    p.span(p.cur),
			})
			return
		}
		contract.Storage = st
	case lexer.TokenKwEvent:
		ev := p.parseEventDecl()
		if ev != nil {
			contract.Events = append(contract.Events, *ev)
		}
	case lexer.TokenKwFn:
		fn := p.parseFunctionDecl("")
		if fn != nil {
			contract.Functions = append(contract.Functions, *fn)
		}
	case lexer.TokenKwConstructor:
		ctor := p.parseConstructorDecl()
		if ctor == nil {
			return
		}
		if contract.Constructor != nil {
			p.addDiag(diag.Diagnostic{
				Code:    diag.CodeParseUnsupported,
				Message: "multiple constructors are not supported",
				Span:    p.span(p.cur),
			})
			return
		}
		contract.Constructor = ctor
	case lexer.TokenKwFallback:
		fb := p.parseFallbackDecl()
		if fb == nil {
			return
		}
		if contract.Fallback != nil {
			p.addDiag(diag.Diagnostic{
				Code:    diag.CodeParseUnsupported,
				Message: "multiple fallbacks are not supported",
				Span:    p.span(p.cur),
			})
			return
		}
		contract.Fallback = fb
	case lexer.TokenKwError:
		p.parseSkippedContractDecl(contract, "error")
	case lexer.TokenKwEnum:
		p.parseSkippedContractDecl(contract, "enum")
	case lexer.TokenKwModifier:
		p.parseSkippedContractDecl(contract, "modifier")
	default:
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnsupported,
			Message: fmt.Sprintf("unsupported contract member starting at token '%s'", p.cur.Literal),
			Span:    p.span(p.cur),
		})
		p.syncUnknownMember()
	}
}

func (p *Parser) parseFunctionAttributes() (string, bool) {
	selectorOverride := ""
	for p.cur.Type == lexer.TokenAt {
		if !p.expect(lexer.TokenAt, diag.CodeParseUnexpected, "expected '@'") {
			return "", false
		}
		attrName := p.cur
		if !p.expect(lexer.TokenIdent, diag.CodeParseUnexpected, "expected attribute name after '@'") {
			return "", false
		}
		if !p.expect(lexer.TokenLParen, diag.CodeParseUnexpected, "expected '(' after attribute name") {
			return "", false
		}

		switch attrName.Literal {
		case "selector":
			if p.cur.Type != lexer.TokenString {
				p.addDiag(diag.Diagnostic{
					Code:    diag.CodeParseUnexpected,
					Message: "expected selector string literal",
					Span:    p.span(p.cur),
				})
			} else {
				val, err := strconv.Unquote(p.cur.Literal)
				if err != nil {
					p.addDiag(diag.Diagnostic{
						Code:    diag.CodeParseUnexpected,
						Message: "invalid selector string literal",
						Span:    p.span(p.cur),
					})
				} else {
					if selectorOverride != "" {
						p.addDiag(diag.Diagnostic{
							Code:    diag.CodeParseUnsupported,
							Message: "duplicate @selector attribute",
							Span:    p.span(attrName),
						})
					}
					selectorOverride = val
				}
				p.next()
			}
		default:
			p.addDiag(diag.Diagnostic{
				Code:    diag.CodeParseUnsupported,
				Message: fmt.Sprintf("unsupported attribute '@%s'", attrName.Literal),
				Span:    p.span(attrName),
			})
			if p.cur.Type != lexer.TokenRParen {
				p.syncUntil(lexer.TokenRParen, lexer.TokenEOF)
			}
		}
		if !p.expect(lexer.TokenRParen, diag.CodeParseUnexpected, "expected ')' after attribute") {
			return "", false
		}
	}
	return selectorOverride, true
}

func (p *Parser) parseSkippedContractDecl(contract *ast.ContractDecl, kind string) {
	p.next() // skip keyword

	name := "<anonymous>"
	if p.cur.Type == lexer.TokenIdent {
		name = p.cur.Literal
		p.next()
	}

	if p.cur.Type == lexer.TokenLParen {
		if !p.consumePaired(lexer.TokenLParen, lexer.TokenRParen, kind+" parameter list") {
			return
		}
	}

	if p.cur.Type == lexer.TokenLBrace {
		_ = p.consumeBlock(kind + " body")
	} else if p.cur.Type == lexer.TokenSemicolon {
		p.next()
	} else {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: "expected '{' or ';' after " + kind + " declaration",
			Span:    p.span(p.cur),
		})
	}

	contract.SkippedDecls = append(contract.SkippedDecls, ast.SkippedContractDecl{
		Kind: kind,
		Name: name,
	})
}

func (p *Parser) parseStorageDecl() *ast.StorageDecl {
	if !p.expect(lexer.TokenKwStorage, diag.CodeParseUnexpected, "expected 'storage'") {
		return nil
	}
	if !p.expect(lexer.TokenLBrace, diag.CodeParseUnexpected, "expected '{' after storage") {
		return nil
	}

	st := &ast.StorageDecl{}
	for p.cur.Type != lexer.TokenRBrace && p.cur.Type != lexer.TokenEOF {
		if p.cur.Type != lexer.TokenKwSlot {
			p.addDiag(diag.Diagnostic{
				Code:    diag.CodeParseUnsupported,
				Message: "only 'slot <name>: <type>;' is supported inside storage block",
				Span:    p.span(p.cur),
			})
			p.syncUntil(lexer.TokenSemicolon, lexer.TokenRBrace, lexer.TokenEOF)
			if p.cur.Type == lexer.TokenSemicolon {
				p.next()
			}
			continue
		}
		slot := p.parseStorageSlot()
		if slot != nil {
			st.Slots = append(st.Slots, *slot)
		}
	}

	if !p.expect(lexer.TokenRBrace, diag.CodeParseUnexpected, "expected '}' to close storage block") {
		return nil
	}
	return st
}

func (p *Parser) parseStorageSlot() *ast.StorageSlot {
	if !p.expect(lexer.TokenKwSlot, diag.CodeParseUnexpected, "expected 'slot'") {
		return nil
	}
	nameTok := p.cur
	if !p.expect(lexer.TokenIdent, diag.CodeParseUnexpected, "expected slot name") {
		return nil
	}
	if !p.expect(lexer.TokenColon, diag.CodeParseUnexpected, "expected ':' after slot name") {
		return nil
	}
	typ := p.parseTypeUntil(map[lexer.Type]bool{
		lexer.TokenSemicolon: true,
	})
	if typ == "" {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: "expected slot type",
			Span:    p.span(p.cur),
		})
	}
	if !p.expect(lexer.TokenSemicolon, diag.CodeParseUnexpected, "expected ';' after slot declaration") {
		return nil
	}
	return &ast.StorageSlot{Name: nameTok.Literal, Type: typ}
}

func (p *Parser) parseEventDecl() *ast.EventDecl {
	if !p.expect(lexer.TokenKwEvent, diag.CodeParseUnexpected, "expected 'event'") {
		return nil
	}
	nameTok := p.cur
	if !p.expect(lexer.TokenIdent, diag.CodeParseUnexpected, "expected event name") {
		return nil
	}
	params, ok := p.parseFieldList(true)
	if !ok {
		return nil
	}
	if p.cur.Type == lexer.TokenSemicolon {
		p.next()
	}
	return &ast.EventDecl{
		Name:   nameTok.Literal,
		Params: params,
	}
}

func (p *Parser) parseFunctionDecl(selectorOverride string) *ast.FunctionDecl {
	if !p.expect(lexer.TokenKwFn, diag.CodeParseUnexpected, "expected 'fn'") {
		return nil
	}
	nameTok := p.cur
	if !p.expect(lexer.TokenIdent, diag.CodeParseUnexpected, "expected function name") {
		return nil
	}

	params, ok := p.parseFieldList(false)
	if !ok {
		return nil
	}
	var returns []ast.FieldDecl
	if p.cur.Type == lexer.TokenArrow {
		p.next()
		ret, rok := p.parseFieldList(false)
		if !rok {
			return nil
		}
		returns = ret
	}

	modifiers := p.parseModifiersUntilBlock()
	body, ok := p.parseStatementBlock("function body")
	if !ok {
		return nil
	}

	return &ast.FunctionDecl{
		Name:             nameTok.Literal,
		SelectorOverride: selectorOverride,
		Params:           params,
		Returns:          returns,
		Modifiers:        modifiers,
		Body:             body,
	}
}

func (p *Parser) parseConstructorDecl() *ast.ConstructorDecl {
	if !p.expect(lexer.TokenKwConstructor, diag.CodeParseUnexpected, "expected 'constructor'") {
		return nil
	}

	params := []ast.FieldDecl{}
	if p.cur.Type == lexer.TokenLParen {
		var ok bool
		params, ok = p.parseFieldList(false)
		if !ok {
			return nil
		}
	}

	modifiers := p.parseModifiersUntilBlock()
	body, ok := p.parseStatementBlock("constructor body")
	if !ok {
		return nil
	}

	return &ast.ConstructorDecl{
		Params:    params,
		Modifiers: modifiers,
		Body:      body,
	}
}

func (p *Parser) parseFallbackDecl() *ast.FallbackDecl {
	if !p.expect(lexer.TokenKwFallback, diag.CodeParseUnexpected, "expected 'fallback'") {
		return nil
	}
	if p.cur.Type == lexer.TokenLParen {
		params, ok := p.parseFieldList(false)
		if !ok {
			return nil
		}
		if len(params) > 0 {
			p.addDiag(diag.Diagnostic{
				Code:    diag.CodeParseUnsupported,
				Message: "fallback parameters are not supported",
				Span:    p.span(p.cur),
			})
		}
	}
	_ = p.parseModifiersUntilBlock()
	body, ok := p.parseStatementBlock("fallback body")
	if !ok {
		return nil
	}
	return &ast.FallbackDecl{Body: body}
}

func (p *Parser) parseFieldList(allowIndexed bool) ([]ast.FieldDecl, bool) {
	if !p.expect(lexer.TokenLParen, diag.CodeParseUnexpected, "expected '('") {
		return nil, false
	}
	fields := []ast.FieldDecl{}
	if p.cur.Type == lexer.TokenRParen {
		p.next()
		return fields, true
	}

	for {
		field, ok := p.parseField(allowIndexed)
		if ok {
			fields = append(fields, field)
		} else {
			p.syncUntil(lexer.TokenComma, lexer.TokenRParen, lexer.TokenEOF)
		}
		if p.cur.Type == lexer.TokenComma {
			p.next()
			continue
		}
		break
	}
	if !p.expect(lexer.TokenRParen, diag.CodeParseUnexpected, "expected ')'") {
		return nil, false
	}
	return fields, true
}

func (p *Parser) parseField(allowIndexed bool) (ast.FieldDecl, bool) {
	nameTok := p.cur
	if !p.expect(lexer.TokenIdent, diag.CodeParseUnexpected, "expected parameter name") {
		return ast.FieldDecl{}, false
	}
	if !p.expect(lexer.TokenColon, diag.CodeParseUnexpected, "expected ':' after parameter name") {
		return ast.FieldDecl{}, false
	}

	indexed := false
	var typeTokens []string
	depthParen := 0
	depthBracket := 0

	for p.cur.Type != lexer.TokenEOF {
		if depthParen == 0 && depthBracket == 0 {
			if allowIndexed && p.cur.Type == lexer.TokenIdent && p.cur.Literal == "indexed" {
				indexed = true
				p.next()
				break
			}
			if p.cur.Type == lexer.TokenComma || p.cur.Type == lexer.TokenRParen {
				break
			}
		}

		switch p.cur.Type {
		case lexer.TokenLParen:
			depthParen++
		case lexer.TokenRParen:
			if depthParen > 0 {
				depthParen--
			}
		case lexer.TokenLBracket:
			depthBracket++
		case lexer.TokenRBracket:
			if depthBracket > 0 {
				depthBracket--
			}
		}

		typeTokens = append(typeTokens, p.cur.Literal)
		p.next()
	}

	typ := strings.TrimSpace(strings.Join(typeTokens, " "))
	if typ == "" {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: "expected parameter type",
			Span:    p.span(p.cur),
		})
		return ast.FieldDecl{}, false
	}

	return ast.FieldDecl{
		Name:    nameTok.Literal,
		Type:    typ,
		Indexed: indexed,
	}, true
}

func (p *Parser) parseTypeUntil(stop map[lexer.Type]bool) string {
	var tokens []string
	depthParen := 0
	depthBracket := 0

	for p.cur.Type != lexer.TokenEOF {
		if depthParen == 0 && depthBracket == 0 && stop[p.cur.Type] {
			break
		}
		switch p.cur.Type {
		case lexer.TokenLParen:
			depthParen++
		case lexer.TokenRParen:
			if depthParen > 0 {
				depthParen--
			}
		case lexer.TokenLBracket:
			depthBracket++
		case lexer.TokenRBracket:
			if depthBracket > 0 {
				depthBracket--
			}
		}
		tokens = append(tokens, p.cur.Literal)
		p.next()
	}
	return strings.TrimSpace(strings.Join(tokens, " "))
}

func (p *Parser) parseModifiersUntilBlock() []string {
	var mods []string
	for p.cur.Type != lexer.TokenEOF && p.cur.Type != lexer.TokenLBrace {
		// Function declarations without body are intentionally unsupported for now.
		if p.cur.Type == lexer.TokenSemicolon {
			p.addDiag(diag.Diagnostic{
				Code:    diag.CodeParseUnsupported,
				Message: "declaration-only functions are not supported in this milestone",
				Span:    p.span(p.cur),
			})
			p.next()
			return mods
		}
		mods = append(mods, p.cur.Literal)
		p.next()
	}
	return mods
}

func (p *Parser) parseStatementBlock(what string) ([]ast.Statement, bool) {
	if p.cur.Type != lexer.TokenLBrace {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: "expected '{' before " + what,
			Span:    p.span(p.cur),
		})
		return nil, false
	}
	p.next()

	stmts := []ast.Statement{}
	for p.cur.Type != lexer.TokenRBrace && p.cur.Type != lexer.TokenEOF {
		stmt, ok := p.parseStatement()
		if ok {
			stmts = append(stmts, stmt)
			continue
		}
		p.syncStatement()
	}
	if p.cur.Type == lexer.TokenEOF {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: "unexpected EOF while parsing " + what,
			Span:    p.span(p.cur),
		})
		return nil, false
	}
	p.next()
	return stmts, true
}

func (p *Parser) parseStatement() (ast.Statement, bool) {
	switch p.cur.Type {
	case lexer.TokenSemicolon:
		p.next()
		return ast.Statement{}, false
	case lexer.TokenKwLet:
		return p.parseLetStatement(lexer.TokenSemicolon)
	case lexer.TokenKwSet:
		return p.parseSetStatement(lexer.TokenSemicolon)
	case lexer.TokenKwReturn:
		return p.parseReturnStatement()
	case lexer.TokenKwBreak:
		if !p.expect(lexer.TokenKwBreak, diag.CodeParseUnexpected, "expected 'break'") {
			return ast.Statement{}, false
		}
		if !p.expect(lexer.TokenSemicolon, diag.CodeParseUnexpected, "expected ';' after break") {
			return ast.Statement{}, false
		}
		return ast.Statement{Kind: "break"}, true
	case lexer.TokenKwContinue:
		if !p.expect(lexer.TokenKwContinue, diag.CodeParseUnexpected, "expected 'continue'") {
			return ast.Statement{}, false
		}
		if !p.expect(lexer.TokenSemicolon, diag.CodeParseUnexpected, "expected ';' after continue") {
			return ast.Statement{}, false
		}
		return ast.Statement{Kind: "continue"}, true
	case lexer.TokenKwRequire:
		return p.parseRequireAssertStatement("require", lexer.TokenKwRequire)
	case lexer.TokenKwAssert:
		return p.parseRequireAssertStatement("assert", lexer.TokenKwAssert)
	case lexer.TokenKwRevert:
		return p.parseUnaryCallLikeStatement("revert", lexer.TokenKwRevert)
	case lexer.TokenKwEmit:
		return p.parseUnaryCallLikeStatement("emit", lexer.TokenKwEmit)
	case lexer.TokenKwIf:
		return p.parseIfStatement()
	case lexer.TokenKwWhile:
		return p.parseWhileStatement()
	case lexer.TokenKwFor:
		return p.parseForStatement()
	default:
		return p.parseExprSemicolonStmt()
	}
}

func (p *Parser) parseLetStatement(terminator lexer.Type) (ast.Statement, bool) {
	if !p.expect(lexer.TokenKwLet, diag.CodeParseUnexpected, "expected 'let'") {
		return ast.Statement{}, false
	}
	nameTok := p.cur
	if !p.expect(lexer.TokenIdent, diag.CodeParseUnexpected, "expected variable name after 'let'") {
		return ast.Statement{}, false
	}
	stmt := ast.Statement{
		Kind: "let",
		Name: nameTok.Literal,
	}

	if p.cur.Type == lexer.TokenColon {
		p.next()
		typ := p.parseTypeUntil(map[lexer.Type]bool{
			lexer.TokenAssign: true,
			terminator:        true,
		})
		if typ == "" {
			p.addDiag(diag.Diagnostic{
				Code:    diag.CodeParseUnexpected,
				Message: "expected type in let statement",
				Span:    p.span(p.cur),
			})
			return ast.Statement{}, false
		}
		stmt.Type = typ
	}

	if p.cur.Type == lexer.TokenAssign {
		p.next()
		expr, ok := p.parseExpression(map[lexer.Type]bool{terminator: true})
		if !ok {
			return ast.Statement{}, false
		}
		stmt.Expr = expr
	}

	if !p.expect(terminator, diag.CodeParseUnexpected, "expected statement terminator after let") {
		return ast.Statement{}, false
	}
	return stmt, true
}

func (p *Parser) parseSetStatement(terminator lexer.Type) (ast.Statement, bool) {
	if !p.expect(lexer.TokenKwSet, diag.CodeParseUnexpected, "expected 'set'") {
		return ast.Statement{}, false
	}
	target, ok := p.parseExpression(map[lexer.Type]bool{lexer.TokenAssign: true})
	if !ok {
		return ast.Statement{}, false
	}
	if !p.expect(lexer.TokenAssign, diag.CodeParseUnexpected, "expected '=' in set statement") {
		return ast.Statement{}, false
	}
	value, ok := p.parseExpression(map[lexer.Type]bool{terminator: true})
	if !ok {
		return ast.Statement{}, false
	}
	if !p.expect(terminator, diag.CodeParseUnexpected, "expected statement terminator after set") {
		return ast.Statement{}, false
	}
	return ast.Statement{
		Kind:   "set",
		Target: target,
		Expr:   value,
	}, true
}

func (p *Parser) parseReturnStatement() (ast.Statement, bool) {
	if !p.expect(lexer.TokenKwReturn, diag.CodeParseUnexpected, "expected 'return'") {
		return ast.Statement{}, false
	}
	if p.cur.Type == lexer.TokenSemicolon {
		p.next()
		return ast.Statement{Kind: "return"}, true
	}
	expr, ok := p.parseExpression(map[lexer.Type]bool{lexer.TokenSemicolon: true})
	if !ok {
		return ast.Statement{}, false
	}
	if !p.expect(lexer.TokenSemicolon, diag.CodeParseUnexpected, "expected ';' after return statement") {
		return ast.Statement{}, false
	}
	return ast.Statement{
		Kind: "return",
		Expr: expr,
	}, true
}

func (p *Parser) parseUnaryCallLikeStatement(kind string, kw lexer.Type) (ast.Statement, bool) {
	if !p.expect(kw, diag.CodeParseUnexpected, "expected '"+kind+"'") {
		return ast.Statement{}, false
	}
	if p.cur.Type == lexer.TokenSemicolon {
		p.next()
		return ast.Statement{Kind: kind}, true
	}
	expr, ok := p.parseExpression(map[lexer.Type]bool{lexer.TokenSemicolon: true})
	if !ok {
		return ast.Statement{}, false
	}
	if !p.expect(lexer.TokenSemicolon, diag.CodeParseUnexpected, "expected ';' after "+kind+" statement") {
		return ast.Statement{}, false
	}
	return ast.Statement{
		Kind: kind,
		Expr: expr,
	}, true
}

// parseRequireAssertStatement parses: require(cond, "msg"); or assert(cond, "msg");
// The condition expression is stored in Stmt.Expr; the message string in Stmt.Text.
func (p *Parser) parseRequireAssertStatement(kind string, kw lexer.Type) (ast.Statement, bool) {
	if !p.expect(kw, diag.CodeParseUnexpected, "expected '"+kind+"'") {
		return ast.Statement{}, false
	}
	if !p.expect(lexer.TokenLParen, diag.CodeParseUnexpected, "expected '(' after '"+kind+"'") {
		return ast.Statement{}, false
	}
	cond, ok := p.parseExpression(map[lexer.Type]bool{lexer.TokenComma: true})
	if !ok {
		return ast.Statement{}, false
	}
	if !p.expect(lexer.TokenComma, diag.CodeParseUnexpected, "expected ',' between "+kind+" condition and message") {
		return ast.Statement{}, false
	}
	if p.cur.Type != lexer.TokenString {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: "expected string literal as " + kind + " message",
			Span:    p.span(p.cur),
		})
		return ast.Statement{}, false
	}
	msg := p.cur.Literal
	p.next()
	if !p.expect(lexer.TokenRParen, diag.CodeParseUnexpected, "expected ')' after "+kind+" message") {
		return ast.Statement{}, false
	}
	if !p.expect(lexer.TokenSemicolon, diag.CodeParseUnexpected, "expected ';' after "+kind+" statement") {
		return ast.Statement{}, false
	}
	return ast.Statement{Kind: kind, Expr: cond, Text: msg}, true
}

func (p *Parser) parseExprSemicolonStmt() (ast.Statement, bool) {
	expr, ok := p.parseExpression(map[lexer.Type]bool{lexer.TokenSemicolon: true})
	if !ok {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: "unexpected EOF while parsing expression statement",
			Span:    p.span(p.cur),
		})
		return ast.Statement{}, false
	}
	if !p.expect(lexer.TokenSemicolon, diag.CodeParseUnexpected, "expected ';' after expression statement") {
		return ast.Statement{}, false
	}
	return ast.Statement{
		Kind: "expr",
		Expr: expr,
	}, true
}

func (p *Parser) parseIfStatement() (ast.Statement, bool) {
	if !p.expect(lexer.TokenKwIf, diag.CodeParseUnexpected, "expected 'if'") {
		return ast.Statement{}, false
	}
	cond, ok := p.parseExpression(map[lexer.Type]bool{lexer.TokenLBrace: true})
	if !ok {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: "unexpected EOF while parsing if condition",
			Span:    p.span(p.cur),
		})
		return ast.Statement{}, false
	}
	thenBlock, ok := p.parseStatementBlock("if body")
	if !ok {
		return ast.Statement{}, false
	}

	stmt := ast.Statement{
		Kind: "if",
		Cond: cond,
		Then: thenBlock,
	}
	if p.cur.Type == lexer.TokenKwElse {
		p.next()
		if p.cur.Type == lexer.TokenKwIf {
			nested, ok := p.parseIfStatement()
			if !ok {
				return ast.Statement{}, false
			}
			stmt.Else = []ast.Statement{nested}
			return stmt, true
		}
		elseBlock, ok := p.parseStatementBlock("else body")
		if !ok {
			return ast.Statement{}, false
		}
		stmt.Else = elseBlock
	}
	return stmt, true
}

func (p *Parser) parseWhileStatement() (ast.Statement, bool) {
	if !p.expect(lexer.TokenKwWhile, diag.CodeParseUnexpected, "expected 'while'") {
		return ast.Statement{}, false
	}
	cond, ok := p.parseExpression(map[lexer.Type]bool{lexer.TokenLBrace: true})
	if !ok {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: "unexpected EOF while parsing while condition",
			Span:    p.span(p.cur),
		})
		return ast.Statement{}, false
	}
	body, ok := p.parseStatementBlock("while body")
	if !ok {
		return ast.Statement{}, false
	}
	return ast.Statement{
		Kind: "while",
		Cond: cond,
		Body: body,
	}, true
}

func (p *Parser) parseForStatement() (ast.Statement, bool) {
	if !p.expect(lexer.TokenKwFor, diag.CodeParseUnexpected, "expected 'for'") {
		return ast.Statement{}, false
	}

	var init *ast.Statement
	if p.cur.Type != lexer.TokenSemicolon {
		switch p.cur.Type {
		case lexer.TokenKwLet:
			s, ok := p.parseLetStatement(lexer.TokenSemicolon)
			if !ok {
				return ast.Statement{}, false
			}
			init = &s
		case lexer.TokenKwSet:
			s, ok := p.parseSetStatement(lexer.TokenSemicolon)
			if !ok {
				return ast.Statement{}, false
			}
			init = &s
		default:
			expr, ok := p.parseExpression(map[lexer.Type]bool{lexer.TokenSemicolon: true})
			if !ok {
				return ast.Statement{}, false
			}
			if !p.expect(lexer.TokenSemicolon, diag.CodeParseUnexpected, "expected ';' after for init expression") {
				return ast.Statement{}, false
			}
			s := ast.Statement{Kind: "expr", Expr: expr}
			init = &s
		}
	} else {
		p.next()
	}

	var cond *ast.Expr
	if p.cur.Type != lexer.TokenSemicolon {
		var ok bool
		cond, ok = p.parseExpression(map[lexer.Type]bool{lexer.TokenSemicolon: true})
		if !ok {
			return ast.Statement{}, false
		}
	}
	if !p.expect(lexer.TokenSemicolon, diag.CodeParseUnexpected, "expected ';' after for condition") {
		return ast.Statement{}, false
	}

	var post *ast.Expr
	if p.cur.Type != lexer.TokenLBrace {
		var ok bool
		post, ok = p.parseExpression(map[lexer.Type]bool{lexer.TokenLBrace: true})
		if !ok {
			return ast.Statement{}, false
		}
	}

	body, ok := p.parseStatementBlock("for body")
	if !ok {
		return ast.Statement{}, false
	}
	return ast.Statement{
		Kind: "for",
		Init: init,
		Cond: cond,
		Post: post,
		Body: body,
	}, true
}

const (
	exprPrecLowest  = 1
	exprPrecAssign  = 2
	exprPrecOr      = 3
	exprPrecAnd     = 4
	exprPrecBitOr   = 5
	exprPrecBitXor  = 6
	exprPrecBitAnd  = 7
	exprPrecCmp     = 8
	exprPrecShift   = 9
	exprPrecAdd     = 10
	exprPrecMul     = 11
	exprPrecPrefix  = 12
	exprPrecPostfix = 13
)

func (p *Parser) parseExpression(stop map[lexer.Type]bool) (*ast.Expr, bool) {
	return p.parseExprPrec(exprPrecLowest, stop)
}

func (p *Parser) parseExprPrec(minPrec int, stop map[lexer.Type]bool) (*ast.Expr, bool) {
	left, ok := p.parsePrefixExpr(stop)
	if !ok {
		return nil, false
	}

	for {
		if p.cur.Type == lexer.TokenEOF || stop[p.cur.Type] {
			break
		}

		if p.cur.Type == lexer.TokenLParen || p.cur.Type == lexer.TokenDot || p.cur.Type == lexer.TokenLBracket {
			if exprPrecPostfix < minPrec {
				break
			}
			left, ok = p.parsePostfixExpr(left)
			if !ok {
				return nil, false
			}
			continue
		}

		prec, rightAssoc := infixPrecedence(p.cur.Type)
		if prec < minPrec || prec == 0 {
			break
		}

		opTok := p.cur
		p.next()
		nextMin := prec + 1
		if rightAssoc {
			nextMin = prec
		}
		right, ok := p.parseExprPrec(nextMin, stop)
		if !ok {
			return nil, false
		}

		kind := "binary"
		if opTok.Type == lexer.TokenAssign {
			kind = "assign"
		}
		left = &ast.Expr{
			Kind:  kind,
			Op:    opTok.Literal,
			Left:  left,
			Right: right,
		}
	}
	return left, true
}

func (p *Parser) parsePrefixExpr(stop map[lexer.Type]bool) (*ast.Expr, bool) {
	if p.cur.Type == lexer.TokenEOF || stop[p.cur.Type] {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: "expected expression",
			Span:    p.span(p.cur),
		})
		return nil, false
	}

	switch p.cur.Type {
	case lexer.TokenIdent:
		tok := p.cur
		p.next()
		return &ast.Expr{Kind: "ident", Value: tok.Literal}, true
	case lexer.TokenNumber:
		tok := p.cur
		p.next()
		return &ast.Expr{Kind: "number", Value: tok.Literal}, true
	case lexer.TokenString:
		tok := p.cur
		p.next()
		return &ast.Expr{Kind: "string", Value: tok.Literal}, true
	case lexer.TokenLParen:
		p.next()
		inner, ok := p.parseExpression(map[lexer.Type]bool{lexer.TokenRParen: true})
		if !ok {
			return nil, false
		}
		if !p.expect(lexer.TokenRParen, diag.CodeParseUnexpected, "expected ')' to close expression") {
			return nil, false
		}
		return &ast.Expr{Kind: "paren", Left: inner}, true
	case lexer.TokenPlus, lexer.TokenMinus, lexer.TokenBang, lexer.TokenBitNot:
		op := p.cur.Literal
		p.next()
		right, ok := p.parseExprPrec(exprPrecPrefix, stop)
		if !ok {
			return nil, false
		}
		return &ast.Expr{Kind: "unary", Op: op, Right: right}, true
	default:
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: fmt.Sprintf("unexpected token '%s' in expression", p.cur.Literal),
			Span:    p.span(p.cur),
		})
		return nil, false
	}
}

func (p *Parser) parsePostfixExpr(left *ast.Expr) (*ast.Expr, bool) {
	switch p.cur.Type {
	case lexer.TokenLParen:
		p.next()
		args := []*ast.Expr{}
		if p.cur.Type != lexer.TokenRParen {
			for {
				arg, ok := p.parseExpression(map[lexer.Type]bool{
					lexer.TokenComma:  true,
					lexer.TokenRParen: true,
				})
				if !ok {
					return nil, false
				}
				args = append(args, arg)
				if p.cur.Type == lexer.TokenComma {
					p.next()
					continue
				}
				break
			}
		}
		if !p.expect(lexer.TokenRParen, diag.CodeParseUnexpected, "expected ')' after argument list") {
			return nil, false
		}
		return &ast.Expr{Kind: "call", Callee: left, Args: args}, true
	case lexer.TokenDot:
		p.next()
		memberTok := p.cur
		if !p.expect(lexer.TokenIdent, diag.CodeParseUnexpected, "expected member name after '.'") {
			return nil, false
		}
		return &ast.Expr{Kind: "member", Object: left, Member: memberTok.Literal}, true
	case lexer.TokenLBracket:
		p.next()
		idx, ok := p.parseExpression(map[lexer.Type]bool{lexer.TokenRBracket: true})
		if !ok {
			return nil, false
		}
		if !p.expect(lexer.TokenRBracket, diag.CodeParseUnexpected, "expected ']' after index expression") {
			return nil, false
		}
		return &ast.Expr{Kind: "index", Object: left, Index: idx}, true
	default:
		return left, true
	}
}

func infixPrecedence(tt lexer.Type) (int, bool) {
	switch tt {
	case lexer.TokenAssign:
		return exprPrecAssign, true
	case lexer.TokenOrOr:
		return exprPrecOr, false
	case lexer.TokenAndAnd:
		return exprPrecAnd, false
	case lexer.TokenBitOr:
		return exprPrecBitOr, false
	case lexer.TokenBitXor:
		return exprPrecBitXor, false
	case lexer.TokenBitAnd:
		return exprPrecBitAnd, false
	case lexer.TokenEq, lexer.TokenNe, lexer.TokenLT, lexer.TokenLE, lexer.TokenGT, lexer.TokenGE:
		return exprPrecCmp, false
	case lexer.TokenShl, lexer.TokenShr:
		return exprPrecShift, false
	case lexer.TokenPlus, lexer.TokenMinus:
		return exprPrecAdd, false
	case lexer.TokenStar, lexer.TokenSlash, lexer.TokenPercent:
		return exprPrecMul, false
	default:
		return 0, false
	}
}

func (p *Parser) syncStatement() {
	for p.cur.Type != lexer.TokenEOF && p.cur.Type != lexer.TokenRBrace {
		if p.cur.Type == lexer.TokenSemicolon {
			p.next()
			return
		}
		if p.cur.Type == lexer.TokenLBrace {
			_ = p.consumeBlock("invalid nested block")
			return
		}
		p.next()
	}
}

func (p *Parser) consumeBlock(what string) bool {
	if p.cur.Type != lexer.TokenLBrace {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: "expected '{' before " + what,
			Span:    p.span(p.cur),
		})
		return false
	}
	p.next()
	depth := 1
	for p.cur.Type != lexer.TokenEOF {
		switch p.cur.Type {
		case lexer.TokenLBrace:
			depth++
		case lexer.TokenRBrace:
			depth--
			if depth == 0 {
				p.next()
				return true
			}
		}
		p.next()
	}
	p.addDiag(diag.Diagnostic{
		Code:    diag.CodeParseUnexpected,
		Message: "unexpected EOF while parsing " + what,
		Span:    p.span(p.cur),
	})
	return false
}

func (p *Parser) consumePaired(open, close lexer.Type, what string) bool {
	if p.cur.Type != open {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: "expected opening token before " + what,
			Span:    p.span(p.cur),
		})
		return false
	}
	p.next()
	depth := 1
	for p.cur.Type != lexer.TokenEOF {
		switch p.cur.Type {
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				p.next()
				return true
			}
		}
		p.next()
	}
	p.addDiag(diag.Diagnostic{
		Code:    diag.CodeParseUnexpected,
		Message: "unexpected EOF while parsing " + what,
		Span:    p.span(p.cur),
	})
	return false
}

func (p *Parser) syncUnknownMember() {
	for p.cur.Type != lexer.TokenEOF && p.cur.Type != lexer.TokenRBrace {
		if p.isContractMemberStart(p.cur.Type) {
			return
		}
		if p.cur.Type == lexer.TokenLBrace {
			_ = p.consumeBlock("unknown member")
			return
		}
		if p.cur.Type == lexer.TokenSemicolon {
			p.next()
			return
		}
		p.next()
	}
}

func (p *Parser) isContractMemberStart(tt lexer.Type) bool {
	switch tt {
	case lexer.TokenKwStorage,
		lexer.TokenKwEvent,
		lexer.TokenKwFn,
		lexer.TokenKwConstructor,
		lexer.TokenKwFallback,
		lexer.TokenKwError,
		lexer.TokenKwEnum,
		lexer.TokenKwModifier:
		return true
	default:
		return false
	}
}

func (p *Parser) expect(tt lexer.Type, code, message string) bool {
	if p.cur.Type != tt {
		p.addDiag(diag.Diagnostic{
			Code:    code,
			Message: message,
			Span:    p.span(p.cur),
		})
		return false
	}
	p.next()
	return true
}

func (p *Parser) syncUntil(types ...lexer.Type) {
	for p.cur.Type != lexer.TokenEOF {
		for _, tt := range types {
			if p.cur.Type == tt {
				return
			}
		}
		p.next()
	}
}

func (p *Parser) next() { p.cur = p.lex.Next() }

func (p *Parser) addDiag(d diag.Diagnostic) {
	p.diags = append(p.diags, d)
}

func (p *Parser) span(tok lexer.Token) diag.Span {
	return diag.Span{
		File: p.filename,
		Start: diag.Position{
			Line:   tok.Start.Line,
			Column: tok.Start.Column,
		},
		End: diag.Position{
			Line:   tok.End.Line,
			Column: tok.End.Column,
		},
	}
}
