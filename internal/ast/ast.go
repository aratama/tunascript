package ast

type Program struct {
	Modules []*Module
}

type Module struct {
	Path    string
	Imports []ImportDecl
	Decls   []Decl
}

type ImportItem struct {
	Name   string
	IsType bool // true if imported with "type" keyword
}

type ImportDecl struct {
	DefaultName string
	Items       []ImportItem
	From        string
	Span        Span
}

type Decl interface {
	declNode()
	GetSpan() Span
}

type ConstDecl struct {
	Name   string
	Export bool
	Type   TypeExpr
	Init   Expr
	Span   Span
}

func (*ConstDecl) declNode()       {}
func (d *ConstDecl) GetSpan() Span { return d.Span }

type FuncDecl struct {
	Name       string
	Export     bool
	TypeParams []string
	Params     []Param
	Ret        TypeExpr
	Body       *BlockStmt
	Span       Span
}

func (*FuncDecl) declNode()       {}
func (d *FuncDecl) GetSpan() Span { return d.Span }

type ExternFuncDecl struct {
	Name       string
	Export     bool
	TypeParams []string
	Params     []Param
	Ret        TypeExpr
	Span       Span
}

func (*ExternFuncDecl) declNode()       {}
func (d *ExternFuncDecl) GetSpan() Span { return d.Span }

// TableDecl represents a table definition: table tableName { column definitions }
type TableDecl struct {
	Name    string
	Columns []TableColumn
	Span    Span
}

func (*TableDecl) declNode()       {}
func (d *TableDecl) GetSpan() Span { return d.Span }

// TypeAliasDecl represents a type alias: type Name = TypeExpr
type TypeAliasDecl struct {
	Name       string
	Export     bool
	TypeParams []string
	Type       TypeExpr
	Span       Span
}

func (*TypeAliasDecl) declNode()       {}
func (d *TypeAliasDecl) GetSpan() Span { return d.Span }

// TableColumn represents a column definition in a table
type TableColumn struct {
	Name        string
	Type        string // INTEGER, TEXT, REAL, BLOB, etc.
	Constraints string // PRIMARY KEY, NOT NULL, DEFAULT, etc.
}

type Param struct {
	Name string
	Type TypeExpr
	Span Span
}

type Stmt interface {
	stmtNode()
	GetSpan() Span
}

type BlockStmt struct {
	Stmts []Stmt
	Span  Span
}

func (*BlockStmt) stmtNode()       {}
func (s *BlockStmt) GetSpan() Span { return s.Span }

type ConstStmt struct {
	Name string
	Type TypeExpr
	Init Expr
	Span Span
}

func (*ConstStmt) stmtNode()       {}
func (s *ConstStmt) GetSpan() Span { return s.Span }

// DestructureStmt represents array destructuring: const [a, b, c] = expr;
type DestructureStmt struct {
	Names []string   // Variable names to bind
	Types []TypeExpr // Optional type annotations for each variable
	Init  Expr       // The array/tuple expression
	Span  Span
}

func (*DestructureStmt) stmtNode()       {}
func (s *DestructureStmt) GetSpan() Span { return s.Span }

// ObjectDestructureStmt represents object destructuring: const { key1, key2 } = expr;
type ObjectDestructureStmt struct {
	Keys  []string   // Property keys to extract (also used as variable names)
	Types []TypeExpr // Optional type annotations for each variable
	Init  Expr       // The object expression
	Span  Span
}

func (*ObjectDestructureStmt) stmtNode()       {}
func (s *ObjectDestructureStmt) GetSpan() Span { return s.Span }

type ExprStmt struct {
	Expr Expr
	Span Span
}

func (*ExprStmt) stmtNode()       {}
func (s *ExprStmt) GetSpan() Span { return s.Span }

type IfStmt struct {
	Cond Expr
	Then *BlockStmt
	Else *BlockStmt
	Span Span
}

func (*IfStmt) stmtNode()       {}
func (s *IfStmt) GetSpan() Span { return s.Span }

type ForOfStmt struct {
	Var  ForOfVar
	Iter Expr
	Body *BlockStmt
	Span Span
}

func (*ForOfStmt) stmtNode()       {}
func (s *ForOfStmt) GetSpan() Span { return s.Span }

type ForOfVar interface {
	forOfVarNode()
}

type ForOfIdentVar struct {
	Name string
	Type TypeExpr
	Span Span
}

func (*ForOfIdentVar) forOfVarNode() {}

type ForOfArrayDestructureVar struct {
	Names []string
	Types []TypeExpr
	Span  Span
}

func (*ForOfArrayDestructureVar) forOfVarNode() {}

type ForOfObjectDestructureVar struct {
	Keys  []string
	Types []TypeExpr
	Span  Span
}

func (*ForOfObjectDestructureVar) forOfVarNode() {}

type ReturnStmt struct {
	Value Expr
	Span  Span
}

func (*ReturnStmt) stmtNode()       {}
func (s *ReturnStmt) GetSpan() Span { return s.Span }

type Expr interface {
	exprNode()
	GetSpan() Span
}

type IdentExpr struct {
	Name string
	Span Span
}

func (*IdentExpr) exprNode()       {}
func (e *IdentExpr) GetSpan() Span { return e.Span }

type IntLit struct {
	Value int64
	Span  Span
}

func (*IntLit) exprNode()       {}
func (e *IntLit) GetSpan() Span { return e.Span }

type FloatLit struct {
	Value float64
	Span  Span
}

func (*FloatLit) exprNode()       {}
func (e *FloatLit) GetSpan() Span { return e.Span }

type BoolLit struct {
	Value bool
	Span  Span
}

func (*BoolLit) exprNode()       {}
func (e *BoolLit) GetSpan() Span { return e.Span }

type NullLit struct {
	Span Span
}

func (*NullLit) exprNode()       {}
func (e *NullLit) GetSpan() Span { return e.Span }

type UndefinedLit struct {
	Span Span
}

func (*UndefinedLit) exprNode()       {}
func (e *UndefinedLit) GetSpan() Span { return e.Span }

type StringLit struct {
	Value string
	Span  Span
}

func (*StringLit) exprNode()       {}
func (e *StringLit) GetSpan() Span { return e.Span }

// TemplateLit represents a template literal: `hello ${name}`
// Segments length is always len(Exprs)+1.
type TemplateLit struct {
	Segments []string
	Exprs    []Expr
	Span     Span
}

func (*TemplateLit) exprNode()       {}
func (e *TemplateLit) GetSpan() Span { return e.Span }

// ArrayPatternExpr represents a destructuring binding pattern used in switch-as patterns:
// case [a, b] as [integer, string]: ...
type ArrayPatternExpr struct {
	Names []string
	Types []TypeExpr
	Span  Span
}

func (*ArrayPatternExpr) exprNode()       {}
func (e *ArrayPatternExpr) GetSpan() Span { return e.Span }

// ObjectPatternExpr represents a destructuring binding pattern used in switch-as patterns:
// case { message } as Error: ...
type ObjectPatternExpr struct {
	Keys  []string
	Types []TypeExpr
	Span  Span
}

func (*ObjectPatternExpr) exprNode()       {}
func (e *ObjectPatternExpr) GetSpan() Span { return e.Span }

type ArrayEntryKind int

const (
	ArrayValue ArrayEntryKind = iota
	ArraySpread
)

type ArrayEntry struct {
	Kind  ArrayEntryKind
	Value Expr
	Span  Span
}

type ArrayLit struct {
	Entries []ArrayEntry
	Span    Span
}

func (*ArrayLit) exprNode()       {}
func (e *ArrayLit) GetSpan() Span { return e.Span }

type ObjectLit struct {
	Entries []ObjectEntry
	Span    Span
}

func (*ObjectLit) exprNode()       {}
func (e *ObjectLit) GetSpan() Span { return e.Span }

type ObjectEntryKind int

const (
	ObjectProp ObjectEntryKind = iota
	ObjectSpread
)

type ObjectEntry struct {
	Kind      ObjectEntryKind
	Key       string
	KeyQuoted bool
	Value     Expr
	Span      Span
}

type CallExpr struct {
	Callee   Expr
	TypeArgs []TypeExpr
	Args     []Expr
	Span     Span
}

func (*CallExpr) exprNode()       {}
func (e *CallExpr) GetSpan() Span { return e.Span }

type MemberExpr struct {
	Object   Expr
	Property string
	Span     Span
}

func (*MemberExpr) exprNode()       {}
func (e *MemberExpr) GetSpan() Span { return e.Span }

type IndexExpr struct {
	Array Expr
	Index Expr
	Span  Span
}

func (*IndexExpr) exprNode()       {}
func (e *IndexExpr) GetSpan() Span { return e.Span }

// TryExpr represents short-hand operator for (T | Error): expr?
// If expr is Error at runtime, it returns from the current function.
type TryExpr struct {
	Expr Expr
	Span Span
}

func (*TryExpr) exprNode()       {}
func (e *TryExpr) GetSpan() Span { return e.Span }

type UnaryExpr struct {
	Op   string
	Expr Expr
	Span Span
}

func (*UnaryExpr) exprNode()       {}
func (e *UnaryExpr) GetSpan() Span { return e.Span }

// AsExpr represents a type assertion: expr as Type
type AsExpr struct {
	Expr Expr
	Type TypeExpr
	Span Span
}

func (*AsExpr) exprNode()       {}
func (e *AsExpr) GetSpan() Span { return e.Span }

type BinaryExpr struct {
	Op    string
	Left  Expr
	Right Expr
	Span  Span
}

func (*BinaryExpr) exprNode()       {}
func (e *BinaryExpr) GetSpan() Span { return e.Span }

// IfExpr represents an if expression: if (cond) thenExpr else elseExpr
// else is optional; if omitted the expression evaluates to undefined when the condition is false.
type IfExpr struct {
	Cond Expr
	Then Expr
	Else Expr // optional
	Span Span
}

func (*IfExpr) exprNode()       {}
func (e *IfExpr) GetSpan() Span { return e.Span }

// SwitchExpr represents a switch expression (like Rust's match)
// switch(expr) { case value: result, case value2: result2, default: result3 }
type SwitchExpr struct {
	Value   Expr
	Cases   []SwitchCase
	Default Expr // optional default case
	Span    Span
}

// SwitchCase represents a single case in a switch expression
type SwitchCase struct {
	Pattern Expr // The pattern to match (currently only literal values)
	Body    Expr // The expression to evaluate if matched
	Span    Span
}

func (*SwitchExpr) exprNode()       {}
func (e *SwitchExpr) GetSpan() Span { return e.Span }

// BlockExpr represents a block used as an expression (for switch case bodies)
// The block executes statements and returns void
type BlockExpr struct {
	Stmts []Stmt
	Span  Span
}

func (*BlockExpr) exprNode()       {}
func (e *BlockExpr) GetSpan() Span { return e.Span }

type ArrowFunc struct {
	Params []Param
	Ret    TypeExpr
	Body   *BlockStmt
	Expr   Expr
	Span   Span
}

func (*ArrowFunc) exprNode()       {}
func (e *ArrowFunc) GetSpan() Span { return e.Span }

// SQLQueryKind represents the type of SQL query operation
type SQLQueryKind int

const (
	SQLQueryFetchAll      SQLQueryKind = iota // sql { } or fetch_all { } - returns all rows
	SQLQueryExecute                           // execute { } - returns nothing (for INSERT/UPDATE/DELETE)
	SQLQueryFetchOptional                     // fetch_optional { } - returns 0 or 1 row
	SQLQueryFetchOne                          // fetch_one { } - returns exactly 1 row
	SQLQueryFetch                             // fetch { } - returns iterator (same as fetch_all for now)
)

// SQLExpr represents a raw SQL block: sql { SELECT * FROM ... }
// Parameters can be embedded using {expr} syntax, which are replaced with ? placeholders
type SQLExpr struct {
	Kind   SQLQueryKind // The kind of query (execute, fetch_optional, fetch_one, fetch, fetch_all)
	Query  string       // SQL query text with ? placeholders
	Params []Expr       // Parameter expressions extracted from {expr}
	Span   Span
}

func (*SQLExpr) exprNode()       {}
func (e *SQLExpr) GetSpan() Span { return e.Span }

// JSXElement represents a JSX element: <div className="foo">children</div>
type JSXElement struct {
	Tag        string         // Tag name (e.g., "div", "span", "html")
	Attributes []JSXAttribute // Attributes (e.g., className="foo")
	Children   []JSXChild     // Children (text, elements, or expressions)
	SelfClose  bool           // True if self-closing: <br />
	Span       Span
}

func (*JSXElement) exprNode()       {}
func (e *JSXElement) GetSpan() Span { return e.Span }

// JSXAttribute represents an attribute in a JSX element
type JSXAttribute struct {
	Name  string // Attribute name
	Value Expr   // Attribute value (StringLit or JSXExprContainer, nil for boolean attrs)
	Span  Span
}

// JSXChild represents a child of a JSX element
type JSXChildKind int

const (
	JSXChildText    JSXChildKind = iota // Plain text content
	JSXChildElement                     // Nested JSX element
	JSXChildExpr                        // Expression in braces: {expr}
)

type JSXChild struct {
	Kind    JSXChildKind
	Text    string      // For JSXChildText
	Element *JSXElement // For JSXChildElement
	Expr    Expr        // For JSXChildExpr
	Span    Span
}

// JSXFragment represents a JSX fragment: <>children</>
type JSXFragment struct {
	Children []JSXChild
	Span     Span
}

func (*JSXFragment) exprNode()       {}
func (e *JSXFragment) GetSpan() Span { return e.Span }

type TypeExpr interface {
	typeNode()
	GetSpan() Span
}

type NamedType struct {
	Name string
	Span Span
}

func (*NamedType) typeNode()       {}
func (t *NamedType) GetSpan() Span { return t.Span }

type GenericType struct {
	Name string
	Args []TypeExpr
	Span Span
}

func (*GenericType) typeNode()       {}
func (t *GenericType) GetSpan() Span { return t.Span }

type ArrayType struct {
	Elem TypeExpr
	Span Span
}

func (*ArrayType) typeNode()       {}
func (t *ArrayType) GetSpan() Span { return t.Span }

type TupleType struct {
	Elems []TypeExpr
	Span  Span
}

func (*TupleType) typeNode()       {}
func (t *TupleType) GetSpan() Span { return t.Span }

type UnionType struct {
	Types []TypeExpr
	Span  Span
}

func (*UnionType) typeNode()       {}
func (t *UnionType) GetSpan() Span { return t.Span }

type LiteralType struct {
	Value Expr
	Span  Span
}

func (*LiteralType) typeNode()       {}
func (t *LiteralType) GetSpan() Span { return t.Span }

type FuncType struct {
	TypeParams []string
	Params     []FuncTypeParam
	Ret        TypeExpr
	Span       Span
}

func (*FuncType) typeNode()       {}
func (t *FuncType) GetSpan() Span { return t.Span }

type FuncTypeParam struct {
	Name string
	Type TypeExpr
	Span Span
}

type ObjectType struct {
	Props []TypeProp
	Span  Span
}

func (*ObjectType) typeNode()       {}
func (t *ObjectType) GetSpan() Span { return t.Span }

type TypeProp struct {
	Key       string
	KeyQuoted bool
	Type      TypeExpr
	Span      Span
}

type Span struct {
	Start Position
	End   Position
}

type Position struct {
	Line int
	Col  int
}
