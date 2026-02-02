package lexer

import "fmt"

type TokenKind int

const (
	TokenEOF TokenKind = iota
	TokenIdent
	TokenInt
	TokenFloat
	TokenString
	TokenImport
	TokenFrom
	TokenExport
	TokenConst
	TokenIf
	TokenElse
	TokenFor
	TokenOf
	TokenReturn
	TokenTrue
	TokenFalse
	TokenFunction
	TokenLParen
	TokenRParen
	TokenLBrace
	TokenRBrace
	TokenLBracket
	TokenRBracket
	TokenComma
	TokenDot
	TokenSemicolon
	TokenColon
	TokenArrow
	TokenEllipsis
	TokenPlus
	TokenMinus
	TokenStar
	TokenSlash
	TokenPercent
	TokenEq
	TokenEqEq
	TokenNotEq
	TokenLT
	TokenLTE
	TokenGT
	TokenGTE
	TokenAmp
	TokenPipe
	TokenQuestion   // "?"
	TokenSwitch     // "switch" keyword
	TokenCase       // "case" keyword
	TokenDefault    // "default" keyword
	TokenTable      // "create_table" keyword
	TokenTableBlock // raw create_table definition block content
	// SQL query keywords (sqlx-style)
	TokenExecute       // "execute" keyword
	TokenExecuteBlock  // raw execute block content
	TokenFetchOptional // "fetch_optional" keyword
	TokenFetchOptionalBlock // raw fetch_optional block content
	TokenFetchOne      // "fetch_one" keyword
	TokenFetchOneBlock // raw fetch_one block content
	TokenFetch         // "fetch" keyword
	TokenFetchBlock    // raw fetch block content
	TokenFetchAll      // "fetch_all" keyword
	TokenFetchAllBlock // raw fetch_all block content
	TokenType          // "type" keyword
	// JSX tokens
	TokenJSXOpen      // "<" in JSX context (opening tag start)
	TokenJSXClose     // ">" in JSX context (closing tag end)
	TokenJSXSlashOpen // "</" in JSX context (closing tag start)
	TokenJSXSlashClose // "/>" in JSX context (self-closing tag end)
	TokenJSXText      // Plain text content inside JSX
	TokenJSXIdent     // Tag or attribute name in JSX
)

type Position struct {
	Line int
	Col  int
}

type Token struct {
	Kind      TokenKind
	Text      string
	Pos       Position
	SQLParams []string // For SQLBlock tokens: parameter expressions extracted from {expr}
}

func (k TokenKind) String() string {
	switch k {
	case TokenEOF:
		return "eof"
	case TokenIdent:
		return "ident"
	case TokenInt:
		return "int"
	case TokenFloat:
		return "float"
	case TokenString:
		return "string"
	case TokenImport:
		return "import"
	case TokenFrom:
		return "from"
	case TokenExport:
		return "export"
	case TokenConst:
		return "const"
	case TokenIf:
		return "if"
	case TokenElse:
		return "else"
	case TokenFor:
		return "for"
	case TokenOf:
		return "of"
	case TokenReturn:
		return "return"
	case TokenTrue:
		return "true"
	case TokenFalse:
		return "false"
	case TokenFunction:
		return "function"
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
	case TokenComma:
		return ","
	case TokenDot:
		return "."
	case TokenSemicolon:
		return ";"
	case TokenColon:
		return ":"
	case TokenArrow:
		return "=>"
	case TokenEllipsis:
		return "..."
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
	case TokenEq:
		return "="
	case TokenEqEq:
		return "=="
	case TokenNotEq:
		return "!="
	case TokenLT:
		return "<"
	case TokenLTE:
		return "<="
	case TokenGT:
		return ">"
	case TokenGTE:
		return ">="
	case TokenAmp:
		return "&"
	case TokenPipe:
		return "|"
	case TokenQuestion:
		return "?"
	case TokenSwitch:
		return "switch"
	case TokenCase:
		return "case"
	case TokenDefault:
		return "default"
	case TokenTable:
		return "create_table"
	case TokenTableBlock:
		return "create_table_block"
	case TokenExecute:
		return "execute"
	case TokenExecuteBlock:
		return "execute_block"
	case TokenFetchOptional:
		return "fetch_optional"
	case TokenFetchOptionalBlock:
		return "fetch_optional_block"
	case TokenFetchOne:
		return "fetch_one"
	case TokenFetchOneBlock:
		return "fetch_one_block"
	case TokenFetch:
		return "fetch"
	case TokenFetchBlock:
		return "fetch_block"
	case TokenFetchAll:
		return "fetch_all"
	case TokenFetchAllBlock:
		return "fetch_all_block"
	case TokenType:
		return "type"
	case TokenJSXOpen:
		return "jsx_open"
	case TokenJSXClose:
		return "jsx_close"
	case TokenJSXSlashOpen:
		return "jsx_slash_open"
	case TokenJSXSlashClose:
		return "jsx_slash_close"
	case TokenJSXText:
		return "jsx_text"
	case TokenJSXIdent:
		return "jsx_ident"
	default:
		return fmt.Sprintf("token(%d)", int(k))
	}
}

var keywords = map[string]TokenKind{
	"import":   TokenImport,
	"from":     TokenFrom,
	"export":   TokenExport,
	"const":    TokenConst,
	"type":     TokenType,
	"if":       TokenIf,
	"else":     TokenElse,
	"for":      TokenFor,
	"of":       TokenOf,
	"return":   TokenReturn,
	"true":     TokenTrue,
	"false":    TokenFalse,
	"function": TokenFunction,
	"switch":         TokenSwitch,
	"case":           TokenCase,
	"default":        TokenDefault,
	"create_table":   TokenTable,
	"execute":        TokenExecute,
	"fetch_optional": TokenFetchOptional,
	"fetch_one":      TokenFetchOne,
	"fetch":          TokenFetch,
	"fetch_all":      TokenFetchAll,
}
