package lexer

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

type Lexer struct {
	src      string
	pos      int
	line     int
	col      int
	peeked   *Token
	comments []Comment
}

func New(src string) *Lexer {
	return &Lexer{src: src, line: 1, col: 1}
}

// GetSource returns the source code string
func (l *Lexer) GetSource() string {
	return l.src
}

// GetBytePosition returns the current byte position in the source
func (l *Lexer) GetBytePosition() int {
	return l.pos
}

// Comments returns all comments encountered during lexing in source order.
func (l *Lexer) Comments() []Comment {
	if len(l.comments) == 0 {
		return nil
	}
	out := make([]Comment, len(l.comments))
	copy(out, l.comments)
	return out
}

func (l *Lexer) Next() Token {
	if l.peeked != nil {
		tok := *l.peeked
		l.peeked = nil
		return tok
	}
	l.skipSpace()
	startPos := Position{Line: l.line, Col: l.col}
	if l.eof() {
		return Token{Kind: TokenEOF, Pos: startPos}
	}
	ch := l.peek()
	if isIdentStart(ch) {
		text := l.readIdent()
		if kind, ok := keywords[text]; ok {
			// Special handling for execute keyword: check for execute { ... } block
			if kind == TokenExecute {
				l.skipSpace()
				if !l.eof() && l.peek() == '{' {
					l.advance() // consume '{'
					sqlContent, params := l.readSQLBlock()
					return Token{Kind: TokenExecuteBlock, Text: sqlContent, Pos: startPos, SQLParams: params}
				}
			}
			// Special handling for fetch_optional keyword: check for fetch_optional { ... } block
			if kind == TokenFetchOptional {
				l.skipSpace()
				if !l.eof() && l.peek() == '{' {
					l.advance() // consume '{'
					sqlContent, params := l.readSQLBlock()
					return Token{Kind: TokenFetchOptionalBlock, Text: sqlContent, Pos: startPos, SQLParams: params}
				}
			}
			// Special handling for fetch_one keyword: check for fetch_one { ... } block
			if kind == TokenFetchOne {
				l.skipSpace()
				if !l.eof() && l.peek() == '{' {
					l.advance() // consume '{'
					sqlContent, params := l.readSQLBlock()
					return Token{Kind: TokenFetchOneBlock, Text: sqlContent, Pos: startPos, SQLParams: params}
				}
			}
			// Special handling for fetch keyword: check for fetch { ... } block
			if kind == TokenFetch {
				l.skipSpace()
				if !l.eof() && l.peek() == '{' {
					l.advance() // consume '{'
					sqlContent, params := l.readSQLBlock()
					return Token{Kind: TokenFetchBlock, Text: sqlContent, Pos: startPos, SQLParams: params}
				}
			}
			// Special handling for fetch_all keyword: check for fetch_all { ... } block
			if kind == TokenFetchAll {
				l.skipSpace()
				if !l.eof() && l.peek() == '{' {
					l.advance() // consume '{'
					sqlContent, params := l.readSQLBlock()
					return Token{Kind: TokenFetchAllBlock, Text: sqlContent, Pos: startPos, SQLParams: params}
				}
			}
			// Special handling for create_table keyword: check for create_table name { ... } block
			if kind == TokenTable {
				l.skipSpace()
				// Read the table name
				if !l.eof() && isIdentStart(l.peek()) {
					tableName := l.readIdent()
					l.skipSpace()
					if !l.eof() && l.peek() == '{' {
						l.advance() // consume '{'
						tableContent := l.readTableBlock()
						return Token{Kind: TokenTableBlock, Text: tableName + "\x00" + tableContent, Pos: startPos}
					}
				}
			}
			return Token{Kind: kind, Text: text, Pos: startPos}
		}
		return Token{Kind: TokenIdent, Text: text, Pos: startPos}
	}
	if isDigit(ch) {
		text, isFloat := l.readNumber()
		kind := TokenInt
		if isFloat {
			kind = TokenFloat
		}
		return Token{Kind: kind, Text: text, Pos: startPos}
	}
	switch ch {
	case '"', '\'':
		text := l.readString(ch)
		return Token{Kind: TokenString, Text: text, Pos: startPos}
	case '`':
		text, exprs := l.readTemplateLiteral()
		return Token{Kind: TokenTemplate, Text: text, Pos: startPos, SQLParams: exprs}
	case '(':
		l.advance()
		return Token{Kind: TokenLParen, Text: "(", Pos: startPos}
	case ')':
		l.advance()
		return Token{Kind: TokenRParen, Text: ")", Pos: startPos}
	case '{':
		l.advance()
		return Token{Kind: TokenLBrace, Text: "{", Pos: startPos}
	case '}':
		l.advance()
		return Token{Kind: TokenRBrace, Text: "}", Pos: startPos}
	case '[':
		l.advance()
		return Token{Kind: TokenLBracket, Text: "[", Pos: startPos}
	case ']':
		l.advance()
		return Token{Kind: TokenRBracket, Text: "]", Pos: startPos}
	case ',':
		l.advance()
		return Token{Kind: TokenComma, Text: ",", Pos: startPos}
	case '.':
		if l.match("...") {
			return Token{Kind: TokenEllipsis, Text: "...", Pos: startPos}
		}
		l.advance()
		return Token{Kind: TokenDot, Text: ".", Pos: startPos}
	case ';':
		l.advance()
		return Token{Kind: TokenSemicolon, Text: ";", Pos: startPos}
	case ':':
		l.advance()
		return Token{Kind: TokenColon, Text: ":", Pos: startPos}
	case '?':
		l.advance()
		return Token{Kind: TokenQuestion, Text: "?", Pos: startPos}
	case '+':
		l.advance()
		return Token{Kind: TokenPlus, Text: "+", Pos: startPos}
	case '-':
		l.advance()
		return Token{Kind: TokenMinus, Text: "-", Pos: startPos}
	case '*':
		l.advance()
		return Token{Kind: TokenStar, Text: "*", Pos: startPos}
	case '/':
		l.advance()
		return Token{Kind: TokenSlash, Text: "/", Pos: startPos}
	case '%':
		l.advance()
		return Token{Kind: TokenPercent, Text: "%", Pos: startPos}
	case '=':
		if l.match("==") {
			return Token{Kind: TokenEqEq, Text: "==", Pos: startPos}
		}
		if l.match("=>") {
			return Token{Kind: TokenArrow, Text: "=>", Pos: startPos}
		}
		l.advance()
		return Token{Kind: TokenEq, Text: "=", Pos: startPos}
	case '!':
		if l.match("!=") {
			return Token{Kind: TokenNotEq, Text: "!=", Pos: startPos}
		}
		l.advance()
		return Token{Kind: TokenIdent, Text: "!", Pos: startPos}
	case '<':
		if l.match("<=") {
			return Token{Kind: TokenLTE, Text: "<=", Pos: startPos}
		}
		l.advance()
		return Token{Kind: TokenLT, Text: "<", Pos: startPos}
	case '>':
		if l.match(">=") {
			return Token{Kind: TokenGTE, Text: ">=", Pos: startPos}
		}
		l.advance()
		return Token{Kind: TokenGT, Text: ">", Pos: startPos}
	case '&':
		l.advance()
		return Token{Kind: TokenAmp, Text: "&", Pos: startPos}
	case '|':
		l.advance()
		return Token{Kind: TokenPipe, Text: "|", Pos: startPos}
	default:
		l.advance()
		return Token{Kind: TokenIdent, Text: string(ch), Pos: startPos}
	}
}

func (l *Lexer) Peek() Token {
	if l.peeked == nil {
		tok := l.Next()
		l.peeked = &tok
	}
	return *l.peeked
}

func (l *Lexer) skipSpace() {
	for {
		if l.eof() {
			return
		}
		ch := l.peek()
		if ch == '/' && l.peekN(1) == '/' {
			startByte := l.pos
			startPos := Position{Line: l.line, Col: l.col}
			inline := l.hasCodeBeforeInLine(startByte)
			l.advance()
			l.advance()
			for !l.eof() && l.peek() != '\n' {
				l.advance()
			}
			endPos := Position{Line: l.line, Col: l.col}
			l.comments = append(l.comments, Comment{
				Kind:   CommentLine,
				Text:   l.src[startByte:l.pos],
				Pos:    startPos,
				End:    endPos,
				Inline: inline,
			})
			continue
		}
		if ch == '/' && l.peekN(1) == '*' {
			startByte := l.pos
			startPos := Position{Line: l.line, Col: l.col}
			inline := l.hasCodeBeforeInLine(startByte)
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
			endPos := Position{Line: l.line, Col: l.col}
			l.comments = append(l.comments, Comment{
				Kind:   CommentBlock,
				Text:   l.src[startByte:l.pos],
				Pos:    startPos,
				End:    endPos,
				Inline: inline,
			})
			continue
		}
		if unicode.IsSpace(ch) {
			l.advance()
			continue
		}
		return
	}
}

func (l *Lexer) hasCodeBeforeInLine(commentStart int) bool {
	for i := commentStart - 1; i >= 0; i-- {
		ch := l.src[i]
		if ch == '\n' || ch == '\r' {
			return false
		}
		if ch != ' ' && ch != '\t' {
			return true
		}
	}
	return false
}

func (l *Lexer) readIdent() string {
	start := l.pos
	for !l.eof() {
		ch := l.peek()
		if !isIdentPart(ch) {
			break
		}
		l.advance()
	}
	return l.src[start:l.pos]
}

func (l *Lexer) readNumber() (string, bool) {
	start := l.pos
	isFloat := false
	for !l.eof() {
		ch := l.peek()
		if isDigit(ch) {
			l.advance()
			continue
		}
		if ch == '.' && !isFloat {
			isFloat = true
			l.advance()
			continue
		}
		if ch == 'e' || ch == 'E' {
			isFloat = true
			l.advance()
			if !l.eof() {
				if l.peek() == '+' || l.peek() == '-' {
					l.advance()
				}
			}
			continue
		}
		break
	}
	return l.src[start:l.pos], isFloat
}

func (l *Lexer) readString(quote rune) string {
	l.advance()
	var b strings.Builder
	for !l.eof() {
		ch := l.peek()
		if ch == quote {
			l.advance()
			break
		}
		if ch == '\\' {
			l.advance()
			if l.eof() {
				break
			}
			esc := l.peek()
			l.advance()
			switch esc {
			case 'n':
				b.WriteByte('\n')
			case 'r':
				b.WriteByte('\r')
			case 't':
				b.WriteByte('\t')
			case '\\':
				b.WriteByte('\\')
			case '\'':
				b.WriteByte('\'')
			case '"':
				b.WriteByte('"')
			case 'u':
				var hex strings.Builder
				for i := 0; i < 4 && !l.eof(); i++ {
					hex.WriteRune(l.peek())
					l.advance()
				}
				code, ok := parseHex(hex.String())
				if ok {
					b.WriteRune(rune(code))
				}
			default:
				b.WriteRune(esc)
			}
			continue
		}
		b.WriteRune(ch)
		l.advance()
	}
	return b.String()
}

func (l *Lexer) readTemplateLiteral() (string, []string) {
	l.advance() // consume opening backtick
	var segment strings.Builder
	var segments []string
	var exprs []string

	for !l.eof() {
		ch := l.peek()
		switch ch {
		case '`':
			l.advance()
			segments = append(segments, segment.String())
			return strings.Join(segments, "\x00"), exprs
		case '\\':
			l.advance()
			if l.eof() {
				break
			}
			esc := l.peek()
			l.advance()
			switch esc {
			case 'n':
				segment.WriteByte('\n')
			case 'r':
				segment.WriteByte('\r')
			case 't':
				segment.WriteByte('\t')
			case '\\':
				segment.WriteByte('\\')
			case '\'':
				segment.WriteByte('\'')
			case '"':
				segment.WriteByte('"')
			case '`':
				segment.WriteByte('`')
			case 'u':
				var hex strings.Builder
				for i := 0; i < 4 && !l.eof(); i++ {
					hex.WriteRune(l.peek())
					l.advance()
				}
				code, ok := parseHex(hex.String())
				if ok {
					segment.WriteRune(rune(code))
				}
			default:
				segment.WriteRune(esc)
			}
		case '$':
			if l.peekN(1) == '{' {
				l.advance() // $
				l.advance() // {
				segments = append(segments, segment.String())
				segment.Reset()
				exprs = append(exprs, l.readParamExpr())
				continue
			}
			segment.WriteRune(ch)
			l.advance()
		default:
			segment.WriteRune(ch)
			l.advance()
		}
	}

	segments = append(segments, segment.String())
	return strings.Join(segments, "\x00"), exprs
}

// readSQLBlock reads raw SQL content until matching closing brace
// It extracts parameter expressions from {expr} and replaces them with ?
func (l *Lexer) readSQLBlock() (string, []string) {
	var b strings.Builder
	var params []string
	depth := 1
	for !l.eof() && depth > 0 {
		ch := l.peek()
		switch ch {
		case '{':
			// Check if this is a parameter expression {expr}
			// We need to distinguish between SQL's use of {} and our parameter syntax
			// Parameters start with { followed by an identifier or expression
			l.advance() // consume '{'
			l.skipSpace()
			if !l.eof() && (isIdentStart(l.peek()) || l.peek() == '(' || l.peek() == '"' || l.peek() == '\'' || isDigit(l.peek())) {
				// This is a parameter expression
				paramExpr := l.readParamExpr()
				params = append(params, paramExpr)
				b.WriteRune('?') // Replace with placeholder
			} else {
				// Not a parameter, just regular SQL brace
				depth++
				b.WriteRune('{')
			}
		case '}':
			depth--
			if depth > 0 {
				b.WriteRune(ch)
			}
			l.advance()
		case '\'', '"':
			// Handle string literals inside SQL
			quote := ch
			b.WriteRune(ch)
			l.advance()
			for !l.eof() {
				c := l.peek()
				b.WriteRune(c)
				l.advance()
				if c == quote {
					break
				}
				if c == '\\' && !l.eof() {
					c2 := l.peek()
					b.WriteRune(c2)
					l.advance()
				}
			}
		case '-':
			// Handle SQL single-line comments
			b.WriteRune(ch)
			l.advance()
			if !l.eof() && l.peek() == '-' {
				b.WriteRune(l.peek())
				l.advance()
				for !l.eof() && l.peek() != '\n' {
					b.WriteRune(l.peek())
					l.advance()
				}
			}
		default:
			b.WriteRune(ch)
			l.advance()
		}
	}
	return strings.TrimSpace(b.String()), params
}

// readParamExpr reads a parameter expression from inside {expr}
// It handles nested braces, parentheses, and string literals
func (l *Lexer) readParamExpr() string {
	var b strings.Builder
	depth := 1 // We've already consumed the opening {
	for !l.eof() && depth > 0 {
		ch := l.peek()
		switch ch {
		case '{':
			depth++
			b.WriteRune(ch)
			l.advance()
		case '}':
			depth--
			if depth > 0 {
				b.WriteRune(ch)
			}
			l.advance()
		case '\'', '"':
			// Handle string literals
			quote := ch
			b.WriteRune(ch)
			l.advance()
			for !l.eof() {
				c := l.peek()
				b.WriteRune(c)
				l.advance()
				if c == quote {
					break
				}
				if c == '\\' && !l.eof() {
					c2 := l.peek()
					b.WriteRune(c2)
					l.advance()
				}
			}
		case '(':
			b.WriteRune(ch)
			l.advance()
		case ')':
			b.WriteRune(ch)
			l.advance()
		default:
			b.WriteRune(ch)
			l.advance()
		}
	}
	return strings.TrimSpace(b.String())
}

// readTableBlock reads the content of a table definition block
func (l *Lexer) readTableBlock() string {
	var b strings.Builder
	depth := 1
	for !l.eof() && depth > 0 {
		ch := l.peek()
		switch ch {
		case '{':
			depth++
			b.WriteRune(ch)
			l.advance()
		case '}':
			depth--
			if depth > 0 {
				b.WriteRune(ch)
			}
			l.advance()
		default:
			b.WriteRune(ch)
			l.advance()
		}
	}
	return strings.TrimSpace(b.String())
}

func (l *Lexer) match(s string) bool {
	if strings.HasPrefix(l.src[l.pos:], s) {
		for range s {
			l.advance()
		}
		return true
	}
	return false
}

func (l *Lexer) advance() {
	if l.eof() {
		return
	}
	_, size := utf8.DecodeRuneInString(l.src[l.pos:])
	ch := l.src[l.pos : l.pos+size]
	l.pos += size
	if ch == "\n" {
		l.line++
		l.col = 1
		return
	}
	l.col++
}

func (l *Lexer) peek() rune {
	if l.eof() {
		return 0
	}
	ch, _ := utf8.DecodeRuneInString(l.src[l.pos:])
	return ch
}

func (l *Lexer) peekN(n int) rune {
	idx := l.pos
	for i := 0; i < n; i++ {
		if idx >= len(l.src) {
			return 0
		}
		_, size := utf8.DecodeRuneInString(l.src[idx:])
		idx += size
	}
	if idx >= len(l.src) {
		return 0
	}
	ch, _ := utf8.DecodeRuneInString(l.src[idx:])
	return ch
}

func (l *Lexer) eof() bool {
	return l.pos >= len(l.src)
}

func isIdentStart(ch rune) bool {
	return ch == '_' || ch == '$' || unicode.IsLetter(ch)
}

func isIdentPart(ch rune) bool {
	return isIdentStart(ch) || unicode.IsDigit(ch)
}

func isDigit(ch rune) bool {
	return ch >= '0' && ch <= '9'
}

func parseHex(s string) (int64, bool) {
	var v int64
	for _, ch := range s {
		v <<= 4
		switch {
		case ch >= '0' && ch <= '9':
			v |= int64(ch - '0')
		case ch >= 'a' && ch <= 'f':
			v |= int64(ch-'a') + 10
		case ch >= 'A' && ch <= 'F':
			v |= int64(ch-'A') + 10
		default:
			return 0, false
		}
	}
	return v, true
}

// ReadRawUntilClosingTag reads raw content until the closing tag </tag> is found.
// It returns the raw content without the closing tag.
// After calling this, the current position will be at the character after </tag>
func (l *Lexer) ReadRawUntilClosingTag(tag string) (string, Position) {
	startPos := Position{Line: l.line, Col: l.col}
	closingTag := "</" + tag + ">"
	closingTagLower := strings.ToLower(closingTag)

	var content strings.Builder
	start := l.pos

	for !l.eof() {
		// Check if we're at the closing tag (case-insensitive for HTML tags)
		remaining := l.src[l.pos:]
		if len(remaining) >= len(closingTag) {
			prefix := remaining[:len(closingTag)]
			if strings.ToLower(prefix) == closingTagLower {
				// Found closing tag, capture content before it
				content.WriteString(l.src[start:l.pos])
				break
			}
		}
		l.advance()
	}

	// Clear any peeked token since we've manually advanced
	l.peeked = nil

	return content.String(), startPos
}
