package parser

import (
	"fmt"
	"strconv"
	"strings"

	"tuna/internal/ast"
	"tuna/internal/lexer"
)

type Parser struct {
	lex  *lexer.Lexer
	curr lexer.Token
	path string
	errs []error
}

func New(path, src string) *Parser {
	lex := lexer.New(src)
	p := &Parser{lex: lex, path: path}
	p.curr = lex.Next()
	return p
}

func (p *Parser) ParseModule() (*ast.Module, error) {
	mod := &ast.Module{Path: p.path}
	for p.curr.Kind == lexer.TokenImport {
		mod.Imports = append(mod.Imports, p.parseImport())
	}
	for p.curr.Kind != lexer.TokenEOF {
		decl := p.parseDecl()
		if decl != nil {
			mod.Decls = append(mod.Decls, decl)
		}
	}
	if len(p.errs) > 0 {
		return nil, p.errs[0]
	}
	return mod, nil
}

func (p *Parser) parseImport() ast.ImportDecl {
	start := p.curr.Pos
	p.expect(lexer.TokenImport)
	p.expect(lexer.TokenLBrace)
	var items []ast.ImportItem
	for {
		if p.curr.Kind == lexer.TokenRBrace {
			break
		}
		isType := false
		if p.curr.Kind == lexer.TokenType {
			isType = true
			p.next()
		}
		name := p.expect(lexer.TokenIdent)
		items = append(items, ast.ImportItem{Name: name.Text, IsType: isType})
		if p.curr.Kind == lexer.TokenComma {
			p.next()
			continue
		}
		break
	}
	p.expect(lexer.TokenRBrace)
	p.expect(lexer.TokenFrom)
	modTok := p.expect(lexer.TokenString)
	p.optional(lexer.TokenSemicolon)
	end := p.curr.Pos
	return ast.ImportDecl{Items: items, From: modTok.Text, Span: spanFrom(start, end)}
}

func (p *Parser) parseDecl() ast.Decl {
	export := false
	if p.curr.Kind == lexer.TokenExport {
		export = true
		p.next()
	}
	switch p.curr.Kind {
	case lexer.TokenConst:
		return p.parseConstDecl(export)
	case lexer.TokenFunction:
		return p.parseFuncDecl(export)
	case lexer.TokenType:
		return p.parseTypeAliasDecl(export)
	case lexer.TokenTableBlock:
		return p.parseTableDecl()
	default:
		p.err("top-level declaration required")
		p.sync()
		return nil
	}
}

func (p *Parser) parseConstDecl(export bool) ast.Decl {
	start := p.curr.Pos
	p.expect(lexer.TokenConst)
	nameTok := p.expect(lexer.TokenIdent)
	p.expect(lexer.TokenColon)
	texpr := p.parseType()
	p.expect(lexer.TokenEq)
	init := p.parseExpr(0)
	p.optional(lexer.TokenSemicolon)
	end := p.curr.Pos
	return &ast.ConstDecl{Name: nameTok.Text, Export: export, Type: texpr, Init: init, Span: spanFrom(start, end)}
}

func (p *Parser) parseFuncDecl(export bool) ast.Decl {
	start := p.curr.Pos
	p.expect(lexer.TokenFunction)
	nameTok := p.expect(lexer.TokenIdent)
	params := p.parseParamList()
	p.expect(lexer.TokenColon)
	ret := p.parseType()
	body := p.parseBlock()
	end := p.curr.Pos
	return &ast.FuncDecl{Name: nameTok.Text, Export: export, Params: params, Ret: ret, Body: body, Span: spanFrom(start, end)}
}

func (p *Parser) parseTableDecl() ast.Decl {
	start := p.curr.Pos
	tok := p.curr
	p.next() // consume TableBlock token

	// Parse table name and content from token text (separated by \x00)
	parts := strings.SplitN(tok.Text, "\x00", 2)
	tableName := parts[0]
	tableContent := ""
	if len(parts) > 1 {
		tableContent = parts[1]
	}

	// Parse columns from content
	columns := parseTableColumns(tableContent)

	end := p.curr.Pos
	return &ast.TableDecl{Name: tableName, Columns: columns, Span: spanFrom(start, end)}
}

func (p *Parser) parseTypeAliasDecl(export bool) ast.Decl {
	start := p.curr.Pos
	p.expect(lexer.TokenType)
	nameTok := p.expect(lexer.TokenIdent)
	var typeParams []string
	if p.curr.Kind == lexer.TokenLT {
		typeParams = p.parseTypeParamList()
	}
	p.expect(lexer.TokenEq)
	typeExpr := p.parseType()
	p.optional(lexer.TokenSemicolon)
	end := p.curr.Pos
	return &ast.TypeAliasDecl{Name: nameTok.Text, Export: export, TypeParams: typeParams, Type: typeExpr, Span: spanFrom(start, end)}
}

// parseTableColumns parses column definitions from table block content
func parseTableColumns(content string) []ast.TableColumn {
	var columns []ast.TableColumn
	// Split by comma, handling parentheses for constraints
	var current strings.Builder
	parenDepth := 0
	for _, ch := range content {
		if ch == '(' {
			parenDepth++
			current.WriteRune(ch)
		} else if ch == ')' {
			parenDepth--
			current.WriteRune(ch)
		} else if ch == ',' && parenDepth == 0 {
			col := parseColumnDef(strings.TrimSpace(current.String()))
			if col.Name != "" {
				columns = append(columns, col)
			}
			current.Reset()
		} else {
			current.WriteRune(ch)
		}
	}
	// Last column
	if current.Len() > 0 {
		col := parseColumnDef(strings.TrimSpace(current.String()))
		if col.Name != "" {
			columns = append(columns, col)
		}
	}
	return columns
}

// parseColumnDef parses a single column definition
func parseColumnDef(def string) ast.TableColumn {
	if def == "" {
		return ast.TableColumn{}
	}
	// Split into words
	words := strings.Fields(def)
	if len(words) < 2 {
		return ast.TableColumn{}
	}
	name := words[0]
	typ := words[1]
	constraints := ""
	if len(words) > 2 {
		constraints = strings.Join(words[2:], " ")
	}
	return ast.TableColumn{Name: name, Type: typ, Constraints: constraints}
}

func (p *Parser) parseStmt() ast.Stmt {
	switch p.curr.Kind {
	case lexer.TokenConst:
		return p.parseConstStmt()
	case lexer.TokenIf:
		return p.parseIf()
	case lexer.TokenFor:
		return p.parseForOf()
	case lexer.TokenReturn:
		return p.parseReturn()
	case lexer.TokenLBrace:
		return p.parseBlock()
	default:
		expr := p.parseExpr(0)
		p.optional(lexer.TokenSemicolon)
		return &ast.ExprStmt{Expr: expr, Span: expr.GetSpan()}
	}
}

func (p *Parser) parseConstStmt() ast.Stmt {
	start := p.curr.Pos
	p.expect(lexer.TokenConst)

	// Check for array destructuring: const [a, b, c] = expr
	if p.curr.Kind == lexer.TokenLBracket {
		names, types := p.parseArrayPatternNames()
		p.expect(lexer.TokenEq)
		init := p.parseExpr(0)
		p.optional(lexer.TokenSemicolon)
		end := p.curr.Pos
		return &ast.DestructureStmt{Names: names, Types: types, Init: init, Span: spanFrom(start, end)}
	}

	// Check for object destructuring: const { key1, key2 } = expr
	if p.curr.Kind == lexer.TokenLBrace {
		keys, types := p.parseObjectPatternKeys()
		p.expect(lexer.TokenEq)
		init := p.parseExpr(0)
		p.optional(lexer.TokenSemicolon)
		end := p.curr.Pos
		return &ast.ObjectDestructureStmt{Keys: keys, Types: types, Init: init, Span: spanFrom(start, end)}
	}

	nameTok := p.expect(lexer.TokenIdent)
	var texpr ast.TypeExpr
	// 型注釈は省略可能（ローカル変数のみ）
	if p.curr.Kind == lexer.TokenColon {
		p.next()
		texpr = p.parseType()
	}
	p.expect(lexer.TokenEq)
	init := p.parseExpr(0)
	p.optional(lexer.TokenSemicolon)
	end := p.curr.Pos
	return &ast.ConstStmt{Name: nameTok.Text, Type: texpr, Init: init, Span: spanFrom(start, end)}
}

func (p *Parser) parseDestructureStmt(start lexer.Position) ast.Stmt {
	names, types := p.parseArrayPatternNames()
	p.expect(lexer.TokenEq)
	init := p.parseExpr(0)
	p.optional(lexer.TokenSemicolon)
	end := p.curr.Pos
	return &ast.DestructureStmt{Names: names, Types: types, Init: init, Span: spanFrom(start, end)}
}

func (p *Parser) parseObjectDestructureStmt(start lexer.Position) ast.Stmt {
	keys, types := p.parseObjectPatternKeys()
	p.expect(lexer.TokenEq)
	init := p.parseExpr(0)
	p.optional(lexer.TokenSemicolon)
	end := p.curr.Pos
	return &ast.ObjectDestructureStmt{Keys: keys, Types: types, Init: init, Span: spanFrom(start, end)}
}

func (p *Parser) parseArrayPatternNames() ([]string, []ast.TypeExpr) {
	p.expect(lexer.TokenLBracket)
	var names []string
	var types []ast.TypeExpr

	for p.curr.Kind != lexer.TokenRBracket && p.curr.Kind != lexer.TokenEOF {
		nameTok := p.expect(lexer.TokenIdent)
		names = append(names, nameTok.Text)

		var texpr ast.TypeExpr
		if p.curr.Kind == lexer.TokenColon {
			p.next()
			texpr = p.parseType()
		}
		types = append(types, texpr)

		if p.curr.Kind != lexer.TokenComma {
			break
		}
		p.next()
	}

	p.expect(lexer.TokenRBracket)
	return names, types
}

func (p *Parser) parseObjectPatternKeys() ([]string, []ast.TypeExpr) {
	p.expect(lexer.TokenLBrace)
	var keys []string
	var types []ast.TypeExpr

	for p.curr.Kind != lexer.TokenRBrace && p.curr.Kind != lexer.TokenEOF {
		keyTok := p.expect(lexer.TokenIdent)
		keys = append(keys, keyTok.Text)

		var texpr ast.TypeExpr
		if p.curr.Kind == lexer.TokenColon {
			p.next()
			texpr = p.parseType()
		}
		types = append(types, texpr)

		if p.curr.Kind != lexer.TokenComma {
			break
		}
		p.next()
	}

	p.expect(lexer.TokenRBrace)
	return keys, types
}

func (p *Parser) parseIf() ast.Stmt {
	start := p.curr.Pos
	p.expect(lexer.TokenIf)
	p.expect(lexer.TokenLParen)
	cond := p.parseExpr(0)
	p.expect(lexer.TokenRParen)
	thenBlock := p.parseBlock()
	var elseBlock *ast.BlockStmt
	if p.curr.Kind == lexer.TokenElse {
		p.next()
		elseBlock = p.parseBlock()
	}
	end := p.curr.Pos
	return &ast.IfStmt{Cond: cond, Then: thenBlock, Else: elseBlock, Span: spanFrom(start, end)}
}

func (p *Parser) parseForOf() ast.Stmt {
	start := p.curr.Pos
	p.expect(lexer.TokenFor)
	p.expect(lexer.TokenLParen)
	p.expect(lexer.TokenConst)
	var binding ast.ForOfVar
	switch p.curr.Kind {
	case lexer.TokenIdent:
		nameTok := p.expect(lexer.TokenIdent)
		var varType ast.TypeExpr
		if p.curr.Kind == lexer.TokenColon {
			p.next()
			varType = p.parseType()
		}
		binding = &ast.ForOfIdentVar{Name: nameTok.Text, Type: varType, Span: spanFrom(nameTok.Pos, p.curr.Pos)}
	case lexer.TokenLBracket:
		patternStart := p.curr.Pos
		names, types := p.parseArrayPatternNames()
		binding = &ast.ForOfArrayDestructureVar{Names: names, Types: types, Span: spanFrom(patternStart, p.curr.Pos)}
	case lexer.TokenLBrace:
		patternStart := p.curr.Pos
		keys, types := p.parseObjectPatternKeys()
		binding = &ast.ForOfObjectDestructureVar{Keys: keys, Types: types, Span: spanFrom(patternStart, p.curr.Pos)}
	default:
		p.err("expected identifier or destructuring pattern in for-of")
		nameTok := p.expect(lexer.TokenIdent)
		binding = &ast.ForOfIdentVar{Name: nameTok.Text, Span: spanFrom(nameTok.Pos, nameTok.Pos)}
	}
	p.expect(lexer.TokenOf)
	iter := p.parseExpr(0)
	p.expect(lexer.TokenRParen)
	body := p.parseBlock()
	end := p.curr.Pos
	return &ast.ForOfStmt{Var: binding, Iter: iter, Body: body, Span: spanFrom(start, end)}
}

func (p *Parser) parseReturn() ast.Stmt {
	start := p.curr.Pos
	p.expect(lexer.TokenReturn)
	var expr ast.Expr
	if p.curr.Kind != lexer.TokenSemicolon && p.curr.Kind != lexer.TokenRBrace {
		expr = p.parseExpr(0)
	}
	p.optional(lexer.TokenSemicolon)
	end := p.curr.Pos
	return &ast.ReturnStmt{Value: expr, Span: spanFrom(start, end)}
}

func (p *Parser) parseBlock() *ast.BlockStmt {
	start := p.curr.Pos
	p.expect(lexer.TokenLBrace)
	var stmts []ast.Stmt
	for p.curr.Kind != lexer.TokenRBrace && p.curr.Kind != lexer.TokenEOF {
		stmts = append(stmts, p.parseStmt())
	}
	p.expect(lexer.TokenRBrace)
	end := p.curr.Pos
	return &ast.BlockStmt{Stmts: stmts, Span: spanFrom(start, end)}
}

func (p *Parser) parseParamList() []ast.Param {
	p.expect(lexer.TokenLParen)
	var params []ast.Param
	if p.curr.Kind != lexer.TokenRParen {
		for {
			nameTok := p.expect(lexer.TokenIdent)
			var typeExpr ast.TypeExpr
			endPos := posFromLex(nameTok.Pos)
			if p.curr.Kind == lexer.TokenColon {
				p.next()
				typeExpr = p.parseType()
				endPos = typeExpr.GetSpan().End
			} else {
				endPos = posFromLex(p.curr.Pos)
			}
			params = append(params, ast.Param{Name: nameTok.Text, Type: typeExpr, Span: spanFromPos(posFromLex(nameTok.Pos), endPos)})
			if p.curr.Kind != lexer.TokenComma {
				break
			}
			p.next()
		}
	}
	p.expect(lexer.TokenRParen)
	return params
}

func (p *Parser) parseArrowFunc() ast.Expr {
	start := p.curr.Pos
	params := p.parseParamList()
	var ret ast.TypeExpr
	if p.curr.Kind == lexer.TokenColon {
		p.next()
		ret = p.parseType()
	}
	p.expect(lexer.TokenArrow)
	var body *ast.BlockStmt
	var expr ast.Expr
	if p.curr.Kind == lexer.TokenLBrace {
		body = p.parseBlock()
	} else {
		expr = p.parseExpr(0)
	}
	end := p.curr.Pos
	return &ast.ArrowFunc{Params: params, Ret: ret, Body: body, Expr: expr, Span: spanFrom(start, end)}
}

func (p *Parser) parseFunctionLiteral() ast.Expr {
	start := p.curr.Pos
	p.expect(lexer.TokenFunction)
	params := p.parseParamList()
	var ret ast.TypeExpr
	if p.curr.Kind == lexer.TokenColon {
		p.next()
		ret = p.parseType()
	}
	var body *ast.BlockStmt
	var expr ast.Expr
	if p.curr.Kind == lexer.TokenArrow {
		p.next()
		if p.curr.Kind == lexer.TokenLBrace {
			body = p.parseBlock()
		} else {
			expr = p.parseExpr(0)
		}
	} else {
		body = p.parseBlock()
	}
	end := p.curr.Pos
	return &ast.ArrowFunc{Params: params, Ret: ret, Body: body, Expr: expr, Span: spanFrom(start, end)}
}

func (p *Parser) parseExpr(precedence int) ast.Expr {
	expr := p.parseUnary()
	for {
		// 三項演算子のチェック (最低優先度)
		if p.curr.Kind == lexer.TokenQuestion && precedence <= 0 {
			p.next()
			thenExpr := p.parseExpr(0)
			p.expect(lexer.TokenColon)
			elseExpr := p.parseExpr(0)
			expr = &ast.TernaryExpr{Cond: expr, Then: thenExpr, Else: elseExpr, Span: spanFromPos(expr.GetSpan().Start, elseExpr.GetSpan().End)}
			continue
		}
		prec := p.binaryPrecedence(p.curr.Kind)
		if prec < precedence {
			break
		}
		op := p.curr.Text
		kind := p.curr.Kind
		p.next()
		right := p.parseExpr(prec + 1)
		expr = &ast.BinaryExpr{Op: opFromToken(kind, op), Left: expr, Right: right, Span: spanFromPos(expr.GetSpan().Start, right.GetSpan().End)}
	}
	return expr
}

func (p *Parser) parseUnary() ast.Expr {
	switch p.curr.Kind {
	case lexer.TokenPlus, lexer.TokenMinus:
		op := p.curr.Text
		start := p.curr.Pos
		p.next()
		expr := p.parseUnary()
		return &ast.UnaryExpr{Op: op, Expr: expr, Span: spanFromPos(posFromLex(start), expr.GetSpan().End)}
	}
	return p.parsePostfix()
}

func (p *Parser) parsePostfix() ast.Expr {
	expr := p.parsePrimary()
	for {
		switch p.curr.Kind {
		case lexer.TokenLParen:
			args := p.parseArgs()
			expr = &ast.CallExpr{Callee: expr, Args: args, Span: spanFromPos(expr.GetSpan().Start, argsEnd(args, expr.GetSpan().End))}
		case lexer.TokenDot:
			p.next()
			propTok := p.expect(lexer.TokenIdent)
			expr = &ast.MemberExpr{Object: expr, Property: propTok.Text, Span: spanFromPos(expr.GetSpan().Start, posFromLex(propTok.Pos))}
		case lexer.TokenLBracket:
			p.next()
			idx := p.parseExpr(0)
			p.expect(lexer.TokenRBracket)
			expr = &ast.IndexExpr{Array: expr, Index: idx, Span: spanFromPos(expr.GetSpan().Start, idx.GetSpan().End)}
		case lexer.TokenAs:
			p.next()
			typeExpr := p.parseType()
			expr = &ast.AsExpr{Expr: expr, Type: typeExpr, Span: spanFromPos(expr.GetSpan().Start, typeExpr.GetSpan().End)}
		default:
			return expr
		}
	}
}

func (p *Parser) parsePrimary() ast.Expr {
	switch p.curr.Kind {
	case lexer.TokenIdent:
		tok := p.curr
		p.next()
		return &ast.IdentExpr{Name: tok.Text, Span: spanFrom(tok.Pos, tok.Pos)}
	case lexer.TokenInt:
		tok := p.curr
		p.next()
		v, _ := strconv.ParseInt(tok.Text, 10, 64)
		return &ast.IntLit{Value: v, Span: spanFrom(tok.Pos, tok.Pos)}
	case lexer.TokenFloat:
		tok := p.curr
		p.next()
		v, _ := strconv.ParseFloat(tok.Text, 64)
		return &ast.FloatLit{Value: v, Span: spanFrom(tok.Pos, tok.Pos)}
	case lexer.TokenTrue, lexer.TokenFalse:
		tok := p.curr
		p.next()
		return &ast.BoolLit{Value: tok.Kind == lexer.TokenTrue, Span: spanFrom(tok.Pos, tok.Pos)}
	case lexer.TokenString:
		tok := p.curr
		p.next()
		return &ast.StringLit{Value: tok.Text, Span: spanFrom(tok.Pos, tok.Pos)}
	case lexer.TokenExecuteBlock:
		tok := p.curr
		p.next()
		var params []ast.Expr
		for _, paramStr := range tok.SQLParams {
			paramLexer := lexer.New(paramStr)
			paramParser := &Parser{lex: paramLexer, curr: paramLexer.Next(), path: p.path}
			paramExpr := paramParser.parseExpr(0)
			params = append(params, paramExpr)
		}
		return &ast.SQLExpr{Kind: ast.SQLQueryExecute, Query: tok.Text, Params: params, Span: spanFrom(tok.Pos, tok.Pos)}
	case lexer.TokenFetchOptionalBlock:
		tok := p.curr
		p.next()
		var params []ast.Expr
		for _, paramStr := range tok.SQLParams {
			paramLexer := lexer.New(paramStr)
			paramParser := &Parser{lex: paramLexer, curr: paramLexer.Next(), path: p.path}
			paramExpr := paramParser.parseExpr(0)
			params = append(params, paramExpr)
		}
		return &ast.SQLExpr{Kind: ast.SQLQueryFetchOptional, Query: tok.Text, Params: params, Span: spanFrom(tok.Pos, tok.Pos)}
	case lexer.TokenFetchOneBlock:
		tok := p.curr
		p.next()
		var params []ast.Expr
		for _, paramStr := range tok.SQLParams {
			paramLexer := lexer.New(paramStr)
			paramParser := &Parser{lex: paramLexer, curr: paramLexer.Next(), path: p.path}
			paramExpr := paramParser.parseExpr(0)
			params = append(params, paramExpr)
		}
		return &ast.SQLExpr{Kind: ast.SQLQueryFetchOne, Query: tok.Text, Params: params, Span: spanFrom(tok.Pos, tok.Pos)}
	case lexer.TokenFetchBlock:
		tok := p.curr
		p.next()
		var params []ast.Expr
		for _, paramStr := range tok.SQLParams {
			paramLexer := lexer.New(paramStr)
			paramParser := &Parser{lex: paramLexer, curr: paramLexer.Next(), path: p.path}
			paramExpr := paramParser.parseExpr(0)
			params = append(params, paramExpr)
		}
		return &ast.SQLExpr{Kind: ast.SQLQueryFetch, Query: tok.Text, Params: params, Span: spanFrom(tok.Pos, tok.Pos)}
	case lexer.TokenFetchAllBlock:
		tok := p.curr
		p.next()
		var params []ast.Expr
		for _, paramStr := range tok.SQLParams {
			paramLexer := lexer.New(paramStr)
			paramParser := &Parser{lex: paramLexer, curr: paramLexer.Next(), path: p.path}
			paramExpr := paramParser.parseExpr(0)
			params = append(params, paramExpr)
		}
		return &ast.SQLExpr{Kind: ast.SQLQueryFetchAll, Query: tok.Text, Params: params, Span: spanFrom(tok.Pos, tok.Pos)}
	case lexer.TokenLParen:
		p.next()
		expr := p.parseExpr(0)
		p.expect(lexer.TokenRParen)
		return expr
	case lexer.TokenFunction:
		return p.parseFunctionLiteral()
	case lexer.TokenLBracket:
		return p.parseArrayLit()
	case lexer.TokenLBrace:
		return p.parseObjectLit()
	case lexer.TokenSwitch:
		return p.parseSwitchExpr()
	case lexer.TokenLT:
		// JSX element: <div>...</div> or <div />
		return p.parseJSXElement()
	default:
		p.err("expression required")
		p.next()
		return &ast.IdentExpr{Name: "", Span: spanFrom(p.curr.Pos, p.curr.Pos)}
	}
}

func (p *Parser) parseArrayLit() ast.Expr {
	start := p.curr.Pos
	p.expect(lexer.TokenLBracket)
	var entries []ast.ArrayEntry
	if p.curr.Kind != lexer.TokenRBracket {
		for {
			if p.curr.Kind == lexer.TokenEllipsis {
				spreadStart := p.curr.Pos
				p.next()
				value := p.parseExpr(0)
				entries = append(entries, ast.ArrayEntry{Kind: ast.ArraySpread, Value: value, Span: spanFromPos(posFromLex(spreadStart), value.GetSpan().End)})
			} else {
				value := p.parseExpr(0)
				entries = append(entries, ast.ArrayEntry{Kind: ast.ArrayValue, Value: value, Span: value.GetSpan()})
			}
			if p.curr.Kind != lexer.TokenComma {
				break
			}
			p.next()
		}
	}
	p.expect(lexer.TokenRBracket)
	end := p.curr.Pos
	return &ast.ArrayLit{Entries: entries, Span: spanFrom(start, end)}
}

func (p *Parser) parseObjectLit() ast.Expr {
	start := p.curr.Pos
	p.expect(lexer.TokenLBrace)
	var entries []ast.ObjectEntry
	if p.curr.Kind != lexer.TokenRBrace {
		for {
			if p.curr.Kind == lexer.TokenEllipsis {
				p.next()
				value := p.parseExpr(0)
				entries = append(entries, ast.ObjectEntry{Kind: ast.ObjectSpread, Value: value, Span: value.GetSpan()})
			} else {
				keyTok := p.curr
				found := true
				switch keyTok.Kind {
				case lexer.TokenString, lexer.TokenIdent:
					p.next()
					key := keyTok.Text
					p.expect(lexer.TokenColon)
					value := p.parseExpr(0)
					entries = append(entries, ast.ObjectEntry{Kind: ast.ObjectProp, Key: key, Value: value, Span: spanFromPos(posFromLex(keyTok.Pos), value.GetSpan().End)})
				default:
					p.err("string literal key required")
					found = false
				}
				if !found {
					break
				}
			}
			if p.curr.Kind != lexer.TokenComma {
				break
			}
			p.next()
		}
	}
	p.expect(lexer.TokenRBrace)
	end := p.curr.Pos
	return &ast.ObjectLit{Entries: entries, Span: spanFrom(start, end)}
}

// parseSwitchExpr parses a switch expression: switch(expr) { case pat: expr, ... default: expr }
func (p *Parser) parseSwitchExpr() ast.Expr {
	start := p.curr.Pos
	p.expect(lexer.TokenSwitch)
	p.expect(lexer.TokenLParen)
	value := p.parseExpr(0)
	p.expect(lexer.TokenRParen)
	p.expect(lexer.TokenLBrace)

	var cases []ast.SwitchCase
	var defaultExpr ast.Expr

	for p.curr.Kind != lexer.TokenRBrace && p.curr.Kind != lexer.TokenEOF {
		if p.curr.Kind == lexer.TokenCase {
			caseStart := p.curr.Pos
			p.next()
			pattern := p.parseExpr(0)
			p.expect(lexer.TokenColon)
			body := p.parseSwitchCaseBody()
			cases = append(cases, ast.SwitchCase{
				Pattern: pattern,
				Body:    body,
				Span:    spanFromPos(posFromLex(caseStart), body.GetSpan().End),
			})
		} else if p.curr.Kind == lexer.TokenDefault {
			p.next()
			p.expect(lexer.TokenColon)
			defaultExpr = p.parseSwitchCaseBody()
		} else {
			p.err("expected 'case' or 'default'")
			break
		}
	}

	p.expect(lexer.TokenRBrace)
	end := p.curr.Pos
	return &ast.SwitchExpr{Value: value, Cases: cases, Default: defaultExpr, Span: spanFrom(start, end)}
}

// parseSwitchCaseBody parses the body of a switch case (either a block or an expression)
func (p *Parser) parseSwitchCaseBody() ast.Expr {
	if p.curr.Kind == lexer.TokenLBrace {
		return p.parseBlockExpr()
	}
	return p.parseExpr(0)
}

// parseBlockExpr parses a block as an expression (returns void)
func (p *Parser) parseBlockExpr() ast.Expr {
	start := p.curr.Pos
	p.expect(lexer.TokenLBrace)
	var stmts []ast.Stmt
	for p.curr.Kind != lexer.TokenRBrace && p.curr.Kind != lexer.TokenEOF {
		stmts = append(stmts, p.parseStmt())
	}
	p.expect(lexer.TokenRBrace)
	end := p.curr.Pos
	return &ast.BlockExpr{Stmts: stmts, Span: spanFrom(start, end)}
}

func (p *Parser) parseArgs() []ast.Expr {
	p.expect(lexer.TokenLParen)
	var args []ast.Expr
	if p.curr.Kind != lexer.TokenRParen {
		for {
			args = append(args, p.parseExpr(0))
			if p.curr.Kind != lexer.TokenComma {
				break
			}
			p.next()
		}
	}
	p.expect(lexer.TokenRParen)
	return args
}

func (p *Parser) parseType() ast.TypeExpr {
	return p.parseUnionType()
}

func (p *Parser) parseUnionType() ast.TypeExpr {
	left := p.parseTypePrimary()
	if p.curr.Kind != lexer.TokenPipe {
		return left
	}
	types := []ast.TypeExpr{left}
	start := left.GetSpan().Start
	for p.curr.Kind == lexer.TokenPipe {
		p.next()
		types = append(types, p.parseTypePrimary())
	}
	end := types[len(types)-1].GetSpan().End
	return &ast.UnionType{Types: types, Span: spanFromPos(start, end)}
}

func (p *Parser) parseTypePrimary() ast.TypeExpr {
	switch p.curr.Kind {
	case lexer.TokenIdent:
		start := p.curr.Pos
		name := p.curr.Text
		p.next()
		base := ast.TypeExpr(&ast.NamedType{Name: name, Span: spanFrom(start, start)})
		base = p.parseTypeApplication(base)
		return p.parseTypeSuffix(base)
	case lexer.TokenLBracket:
		start := p.curr.Pos
		p.next()
		var elems []ast.TypeExpr
		if p.curr.Kind != lexer.TokenRBracket {
			for {
				elems = append(elems, p.parseType())
				if p.curr.Kind != lexer.TokenComma {
					break
				}
				p.next()
			}
		}
		p.expect(lexer.TokenRBracket)
		base := ast.TypeExpr(&ast.TupleType{Elems: elems, Span: spanFrom(start, p.curr.Pos)})
		return p.parseTypeSuffix(base)
	case lexer.TokenLT:
		start := p.curr.Pos
		typeParams := p.parseTypeParamList()
		if p.curr.Kind != lexer.TokenLParen {
			p.err("function type expected after type parameters")
			end := p.curr.Pos
			return &ast.NamedType{Name: "", Span: spanFromPos(posFromLex(start), posFromLex(end))}
		}
		p.next()
		base := p.parseFuncTypeBody(start, typeParams)
		return p.parseTypeSuffix(base)
	case lexer.TokenLParen:
		start := p.curr.Pos
		p.next()
		base := p.parseFuncTypeBody(start, nil)
		return p.parseTypeSuffix(base)
	case lexer.TokenLBrace:
		start := p.curr.Pos
		p.next()
		var props []ast.TypeProp
		if p.curr.Kind != lexer.TokenRBrace {
			for {
				keyTok := p.curr
				if keyTok.Kind != lexer.TokenString {
					p.err("type key must be string literal")
					break
				}
				p.next()
				key := keyTok.Text
				p.expect(lexer.TokenColon)
				typeExpr := p.parseType()
				props = append(props, ast.TypeProp{Key: key, Type: typeExpr, Span: spanFromPos(posFromLex(keyTok.Pos), typeExpr.GetSpan().End)})
				if p.curr.Kind != lexer.TokenComma {
					break
				}
				p.next()
			}
		}
		p.expect(lexer.TokenRBrace)
		base := ast.TypeExpr(&ast.ObjectType{Props: props, Span: spanFrom(start, p.curr.Pos)})
		return p.parseTypeSuffix(base)
	case lexer.TokenInt, lexer.TokenFloat, lexer.TokenString, lexer.TokenTrue, lexer.TokenFalse:
		base := p.parseLiteralType()
		return p.parseTypeSuffix(base)
	default:
		p.err("type required")
		p.next()
		return &ast.NamedType{Name: "", Span: spanFrom(p.curr.Pos, p.curr.Pos)}
	}
}

func (p *Parser) parseLiteralType() ast.TypeExpr {
	tok := p.curr
	var lit ast.Expr
	switch tok.Kind {
	case lexer.TokenInt:
		value, _ := strconv.ParseInt(tok.Text, 10, 64)
		lit = &ast.IntLit{Value: value, Span: spanFrom(tok.Pos, tok.Pos)}
	case lexer.TokenFloat:
		value, _ := strconv.ParseFloat(tok.Text, 64)
		lit = &ast.FloatLit{Value: value, Span: spanFrom(tok.Pos, tok.Pos)}
	case lexer.TokenTrue, lexer.TokenFalse:
		lit = &ast.BoolLit{Value: tok.Kind == lexer.TokenTrue, Span: spanFrom(tok.Pos, tok.Pos)}
	case lexer.TokenString:
		lit = &ast.StringLit{Value: tok.Text, Span: spanFrom(tok.Pos, tok.Pos)}
	default:
		lit = &ast.StringLit{Value: tok.Text, Span: spanFrom(tok.Pos, tok.Pos)}
	}
	p.next()
	return &ast.LiteralType{Value: lit, Span: lit.GetSpan()}
}

func (p *Parser) parseFuncTypeBody(start lexer.Position, typeParams []string) ast.TypeExpr {
	var params []ast.FuncTypeParam
	if p.curr.Kind != lexer.TokenRParen {
		for {
			paramStart := p.curr.Pos
			name := ""
			if p.curr.Kind == lexer.TokenIdent && p.lex.Peek().Kind == lexer.TokenColon {
				nameTok := p.expect(lexer.TokenIdent)
				name = nameTok.Text
				p.expect(lexer.TokenColon)
			}
			typeExpr := p.parseType()
			params = append(params, ast.FuncTypeParam{Name: name, Type: typeExpr, Span: spanFromPos(posFromLex(paramStart), typeExpr.GetSpan().End)})
			if p.curr.Kind != lexer.TokenComma {
				break
			}
			p.next()
		}
	}
	p.expect(lexer.TokenRParen)
	if p.curr.Kind == lexer.TokenArrow {
		p.next()
		ret := p.parseType()
		if ret == nil {
			ret = &ast.NamedType{Name: "", Span: spanFromPos(posFromLex(start), posFromLex(start))}
		}
		retSpan := ret.GetSpan()
		funcType := &ast.FuncType{
			TypeParams: typeParams,
			Params:     params,
			Ret:        ret,
			Span:       spanFromPos(posFromLex(start), retSpan.End),
		}
		return funcType
	}
	if len(params) == 1 && params[0].Name == "" {
		return params[0].Type
	}
	p.err("function type arrow required")
	return &ast.NamedType{Name: "", Span: spanFrom(start, start)}
}

func (p *Parser) parseTypeParamList() []string {
	var params []string
	p.expect(lexer.TokenLT)
	for {
		if p.curr.Kind != lexer.TokenIdent {
			p.err("type parameter name expected")
			break
		}
		params = append(params, p.curr.Text)
		p.next()
		if p.curr.Kind != lexer.TokenComma {
			break
		}
		p.next()
	}
	p.expect(lexer.TokenGT)
	return params
}

func (p *Parser) parseTypeSuffix(base ast.TypeExpr) ast.TypeExpr {
	for p.curr.Kind == lexer.TokenLBracket {
		start := base.GetSpan().Start
		p.next()
		p.expect(lexer.TokenRBracket)
		base = &ast.ArrayType{Elem: base, Span: spanFromPos(start, posFromLex(p.curr.Pos))}
	}
	return base
}

func (p *Parser) parseTypeApplication(base ast.TypeExpr) ast.TypeExpr {
	if p.curr.Kind != lexer.TokenLT {
		return base
	}
	named, ok := base.(*ast.NamedType)
	if !ok {
		return base
	}
	start := base.GetSpan().Start
	var args []ast.TypeExpr
	for {
		p.next()
		args = append(args, p.parseType())
		if p.curr.Kind != lexer.TokenComma {
			break
		}
		p.next()
	}
	if p.curr.Kind != lexer.TokenGT {
		p.err("expected '>' in generic type")
	}
	end := posFromLex(p.curr.Pos)
	p.expect(lexer.TokenGT)
	return &ast.GenericType{Name: named.Name, Args: args, Span: spanFromPos(start, end)}
}

func (p *Parser) binaryPrecedence(kind lexer.TokenKind) int {
	switch kind {
	case lexer.TokenPipe:
		return 1
	case lexer.TokenAmp:
		return 2
	case lexer.TokenEqEq, lexer.TokenNotEq:
		return 3
	case lexer.TokenLT, lexer.TokenLTE, lexer.TokenGT, lexer.TokenGTE:
		return 4
	case lexer.TokenPlus, lexer.TokenMinus:
		return 5
	case lexer.TokenStar, lexer.TokenSlash, lexer.TokenPercent:
		return 6
	default:
		return -1
	}
}

func opFromToken(kind lexer.TokenKind, text string) string {
	switch kind {
	case lexer.TokenEqEq:
		return "=="
	case lexer.TokenNotEq:
		return "!="
	case lexer.TokenLT:
		return "<"
	case lexer.TokenLTE:
		return "<="
	case lexer.TokenGT:
		return ">"
	case lexer.TokenGTE:
		return ">="
	case lexer.TokenAmp:
		return "&"
	case lexer.TokenPipe:
		return "|"
	default:
		return text
	}
}

func (p *Parser) optional(kind lexer.TokenKind) {
	if p.curr.Kind == kind {
		p.next()
	}
}

func (p *Parser) expect(kind lexer.TokenKind) lexer.Token {
	if p.curr.Kind != kind {
		p.err(fmt.Sprintf("%s expected", kind.String()))
		return p.curr
	}
	tok := p.curr
	p.next()
	return tok
}

func (p *Parser) next() {
	p.curr = p.lex.Next()
}

func (p *Parser) err(msg string) {
	p.errs = append(p.errs, fmt.Errorf("%s:%d:%d: %s", p.path, p.curr.Pos.Line, p.curr.Pos.Col, msg))
}

func (p *Parser) sync() {
	for p.curr.Kind != lexer.TokenEOF {
		switch p.curr.Kind {
		case lexer.TokenSemicolon, lexer.TokenRBrace:
			p.next()
			return
		default:
			p.next()
		}
	}
}

func spanFrom(start lexer.Position, end lexer.Position) ast.Span {
	return ast.Span{Start: ast.Position{Line: start.Line, Col: start.Col}, End: ast.Position{Line: end.Line, Col: end.Col}}
}

func spanFromPos(start ast.Position, end ast.Position) ast.Span {
	return ast.Span{Start: start, End: end}
}

func posFromLex(pos lexer.Position) ast.Position {
	return ast.Position{Line: pos.Line, Col: pos.Col}
}

func argsEnd(args []ast.Expr, fallback ast.Position) ast.Position {
	if len(args) == 0 {
		return fallback
	}
	return args[len(args)-1].GetSpan().End
}

// parseJSXElement parses a JSX element: <tag attr="value">children</tag> or <tag />
func (p *Parser) parseJSXElement() ast.Expr {
	start := p.curr.Pos
	p.expect(lexer.TokenLT) // consume '<'

	// Check for fragment: <>...</>
	if p.curr.Kind == lexer.TokenGT {
		return p.parseJSXFragment(start)
	}

	// Parse tag name
	tagTok := p.expect(lexer.TokenIdent)
	tag := tagTok.Text

	// Parse attributes
	var attrs []ast.JSXAttribute
	for p.isJSXAttributeName() {
		attrName := p.curr.Text
		attrStart := p.curr.Pos
		p.next()
		var attrValue ast.Expr
		if p.curr.Kind == lexer.TokenEq {
			p.next()
			if p.curr.Kind == lexer.TokenString {
				attrValue = &ast.StringLit{Value: p.curr.Text, Span: spanFrom(p.curr.Pos, p.curr.Pos)}
				p.next()
			} else if p.curr.Kind == lexer.TokenLBrace {
				// Dynamic attribute: attr={expr}
				p.next()
				attrValue = p.parseExpr(0)
				p.expect(lexer.TokenRBrace)
			} else {
				p.err("attribute value expected (string or {expr})")
			}
		}
		attrs = append(attrs, ast.JSXAttribute{Name: attrName, Value: attrValue, Span: spanFrom(attrStart, p.curr.Pos)})
	}

	// Check for self-closing tag: />
	if p.curr.Kind == lexer.TokenSlash {
		p.next()
		p.expect(lexer.TokenGT)
		end := p.curr.Pos
		return &ast.JSXElement{Tag: tag, Attributes: attrs, SelfClose: true, Span: spanFrom(start, end)}
	}

	// Expect closing >
	p.expect(lexer.TokenGT)

	// Parse children until closing tag </tag>
	children := p.parseJSXChildren(tag)

	end := p.curr.Pos
	return &ast.JSXElement{Tag: tag, Attributes: attrs, Children: children, Span: spanFrom(start, end)}
}

// parseJSXFragment parses a JSX fragment: <>children</>
func (p *Parser) parseJSXFragment(start lexer.Position) ast.Expr {
	p.expect(lexer.TokenGT) // consume '>' after '<'

	// Parse children until closing </>
	var children []ast.JSXChild
	for {
		if p.isJSXClosingFragment() {
			break
		}
		child := p.parseJSXChild("")
		if child != nil {
			children = append(children, *child)
		}
	}

	// Expect </>
	p.expect(lexer.TokenLT)
	p.expect(lexer.TokenSlash)
	p.expect(lexer.TokenGT)

	end := p.curr.Pos
	return &ast.JSXFragment{Children: children, Span: spanFrom(start, end)}
}

// parseJSXChildren parses children of a JSX element until the closing tag
func (p *Parser) parseJSXChildren(tag string) []ast.JSXChild {
	// Raw text tags: style, script, textarea - their content should not be parsed as JSX
	if tag == "style" || tag == "script" || tag == "textarea" {
		children := p.parseJSXRawContent(tag)
		// Expect closing tag: </tag> (raw content parsing leaves us at the closing tag)
		p.expect(lexer.TokenLT)
		p.expect(lexer.TokenSlash)
		p.expect(lexer.TokenIdent) // tag name
		p.expect(lexer.TokenGT)
		return children
	}

	var children []ast.JSXChild
	for {
		if p.isJSXClosingTag(tag) {
			break
		}
		child := p.parseJSXChild(tag)
		if child != nil {
			children = append(children, *child)
		}
	}

	// Expect closing tag: </tag>
	p.expect(lexer.TokenLT)
	p.expect(lexer.TokenSlash)
	p.expect(lexer.TokenIdent) // tag name
	p.expect(lexer.TokenGT)

	return children
}

// isJSXClosingTag checks if current position is </tag>
func (p *Parser) isJSXClosingTag(tag string) bool {
	if p.curr.Kind != lexer.TokenLT {
		return false
	}
	peek := p.lex.Peek()
	if peek.Kind != lexer.TokenSlash {
		return false
	}
	return true
}

// isJSXClosingFragment checks if current position is </>
func (p *Parser) isJSXClosingFragment() bool {
	if p.curr.Kind != lexer.TokenLT {
		return false
	}
	peek := p.lex.Peek()
	if peek.Kind != lexer.TokenSlash {
		return false
	}
	return true
}

// parseJSXChild parses a single child of a JSX element
func (p *Parser) parseJSXChild(parentTag string) *ast.JSXChild {
	switch p.curr.Kind {
	case lexer.TokenLT:
		// Check if it's a closing tag
		if p.isJSXClosingTag(parentTag) || p.isJSXClosingFragment() {
			return nil
		}
		// Nested JSX element
		elem := p.parseJSXElement()
		jsxElem, ok := elem.(*ast.JSXElement)
		if ok {
			return &ast.JSXChild{Kind: ast.JSXChildElement, Element: jsxElem, Span: elem.GetSpan()}
		}
		// Could be a fragment
		if frag, ok := elem.(*ast.JSXFragment); ok {
			// Convert fragment children to parent's children
			// For simplicity, we'll wrap it as an element child
			return &ast.JSXChild{Kind: ast.JSXChildElement, Element: &ast.JSXElement{
				Tag:      "",
				Children: frag.Children,
				Span:     frag.Span,
			}, Span: frag.Span}
		}
		return nil
	case lexer.TokenLBrace:
		// Expression: {expr}
		start := p.curr.Pos
		p.next()
		expr := p.parseExpr(0)
		p.expect(lexer.TokenRBrace)
		return &ast.JSXChild{Kind: ast.JSXChildExpr, Expr: expr, Span: spanFromPos(posFromLex(start), expr.GetSpan().End)}
	case lexer.TokenEOF:
		return nil
	default:
		// Text content - read everything until < or {
		return p.parseJSXText()
	}
}

// parseJSXText parses text content inside a JSX element
func (p *Parser) parseJSXText() *ast.JSXChild {
	start := p.curr.Pos
	var text strings.Builder

	// Read tokens until we hit < or { or EOF
	// We need to be careful here - regular tokens won't work well for arbitrary text
	// Instead, we'll concatenate token text until we hit a JSX boundary
	for p.curr.Kind != lexer.TokenLT && p.curr.Kind != lexer.TokenLBrace && p.curr.Kind != lexer.TokenEOF {
		if text.Len() > 0 {
			text.WriteString(" ")
		}
		text.WriteString(p.curr.Text)
		p.next()
	}

	if text.Len() == 0 {
		return nil
	}

	return &ast.JSXChild{Kind: ast.JSXChildText, Text: text.String(), Span: spanFrom(start, p.curr.Pos)}
}

// parseJSXRawContent parses raw content for tags like style, script, textarea
// that should not have their content parsed as JSX
func (p *Parser) parseJSXRawContent(tag string) []ast.JSXChild {
	start := p.curr.Pos
	closingTag := "</" + tag + ">"
	closingTagLower := strings.ToLower(closingTag)

	// Get raw source from lexer and find the closing tag
	src := p.lex.GetSource()
	// Calculate the byte position for the start of content
	// The current token position tells us where we are in terms of line/col,
	// but we need the byte position. We'll scan forward from lexer's position.
	bytePos := p.lex.GetBytePosition()

	// If there's a peeked token, we need to account for it
	// Actually, the current token (p.curr) was already consumed from lexer,
	// so bytePos might be past it. We need to find position from token position.

	// Find closing tag in source starting from current lexer position
	// But we need to include the current token's content too

	// Simpler approach: search for closing tag from current byte position backwards
	// adjusted to start of current token

	// We need to find where the content starts. Since p.curr points to the first
	// token inside the raw content tag, we need to read raw text until </tag>

	// Use lexer method to read raw content
	content, _ := p.lex.ReadRawUntilClosingTag(tag)

	// The content from ReadRawUntilClosingTag starts at lexer's current position
	// but we already have p.curr which contains the first token
	// So we need to prepend that token's literal representation

	// Actually, let's take a different approach:
	// Find the closing tag position in source and extract everything before it
	searchStart := bytePos
	// Adjust for any peeked token that was consumed
	if searchStart > 0 && searchStart < len(src) {
		idx := strings.Index(strings.ToLower(src[searchStart:]), closingTagLower)
		if idx >= 0 {
			// But we also need to include the current token which was already lexed
			// The current token text is p.curr.Text, prepend it
			rawContent := p.curr.Text + src[searchStart:searchStart+idx]
			content = strings.TrimSpace(rawContent)
		}
	}

	// Re-sync the parser's current token after reading raw content
	p.next()

	// If content is empty, return empty children
	if content == "" {
		return nil
	}

	return []ast.JSXChild{{
		Kind: ast.JSXChildText,
		Text: content,
		Span: spanFrom(start, p.curr.Pos),
	}}
}

// isJSXAttributeName checks if the current token can be used as a JSX attribute name.
// This includes identifiers and keywords that can be valid attribute names (like "type", "for", "class", etc.)
func (p *Parser) isJSXAttributeName() bool {
	switch p.curr.Kind {
	case lexer.TokenIdent, lexer.TokenType, lexer.TokenFor, lexer.TokenIf, lexer.TokenElse,
		lexer.TokenConst, lexer.TokenReturn, lexer.TokenTrue, lexer.TokenFalse,
		lexer.TokenSwitch, lexer.TokenCase, lexer.TokenDefault, lexer.TokenExport,
		lexer.TokenImport, lexer.TokenFrom, lexer.TokenOf, lexer.TokenFunction:
		return true
	default:
		return false
	}
}
