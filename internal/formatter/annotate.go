package formatter

import (
	"fmt"

	"tuna/internal/ast"
	ttypes "tuna/internal/types"
)

// AnnotateModuleTypes runs the type checker and fills missing type annotations
// for local variables, destructuring, for-of loop variables and function literals.
func (f *Formatter) AnnotateModuleTypes(mod *ast.Module) error {
	checker := ttypes.NewChecker()
	checker.AddModule(mod)
	ok := checker.Check()
	if !ok {
		// return first error
		if len(checker.Errors) > 0 {
			return checker.Errors[0]
		}
		return fmt.Errorf("type check failed")
	}

	// Walk declarations
	for _, decl := range mod.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			annotateBlock(d.Body, checker)
		case *ast.ConstDecl:
			// top-level consts have Type required by parser
		}
	}

	return nil
}

func annotateBlock(block *ast.BlockStmt, checker *ttypes.Checker) {
	if block == nil {
		return
	}
	for _, stmt := range block.Stmts {
		annotateStmt(stmt, checker)
	}
}

func annotateStmt(stmt ast.Stmt, checker *ttypes.Checker) {
	switch s := stmt.(type) {
	case *ast.ConstStmt:
		if s.Type == nil {
			if typ, ok := checker.ExprTypes[s.Init]; ok && typ != nil {
				s.Type = typeToTypeExpr(typ)
			}
		}
		annotateExpr(s.Init, checker)
	case *ast.DestructureStmt:
		annotateExpr(s.Init, checker)
		if typ, ok := checker.ExprTypes[s.Init]; ok && typ != nil {
			switch typ.Kind {
			case ttypes.KindArray:
				for i := range s.Names {
					if s.Types == nil {
						s.Types = make([]ast.TypeExpr, len(s.Names))
					}
					if s.Types[i] == nil {
						s.Types[i] = typeToTypeExpr(typ.Elem)
					}
				}
			case ttypes.KindTuple:
				for i := range s.Names {
					if s.Types == nil {
						s.Types = make([]ast.TypeExpr, len(s.Names))
					}
					if i < len(typ.Tuple) && s.Types[i] == nil {
						s.Types[i] = typeToTypeExpr(typ.Tuple[i])
					}
				}
			}
		}
	case *ast.ObjectDestructureStmt:
		annotateExpr(s.Init, checker)
		if typ, ok := checker.ExprTypes[s.Init]; ok && typ != nil {
			for i, key := range s.Keys {
				if s.Types == nil {
					s.Types = make([]ast.TypeExpr, len(s.Keys))
				}
				if s.Types[i] == nil {
					prop := typ.PropType(key)
					if prop != nil {
						s.Types[i] = typeToTypeExpr(prop)
					}
				}
			}
		}
	case *ast.ExprStmt:
		annotateExpr(s.Expr, checker)
	case *ast.IfStmt:
		annotateExpr(s.Cond, checker)
		annotateBlock(s.Then, checker)
		if s.Else != nil {
			annotateBlock(s.Else, checker)
		}
	case *ast.ForOfStmt:
		annotateExpr(s.Iter, checker)
		var elemType *ttypes.Type
		if typ, ok := checker.ExprTypes[s.Iter]; ok && typ != nil && typ.Kind == ttypes.KindArray {
			elemType = typ.Elem
		}
		switch v := s.Var.(type) {
		case *ast.ForOfIdentVar:
			if v.Type == nil && elemType != nil {
				v.Type = typeToTypeExpr(elemType)
			}
		case *ast.ForOfArrayDestructureVar:
			if elemType != nil {
				switch elemType.Kind {
				case ttypes.KindArray:
					for i := range v.Names {
						if v.Types == nil {
							v.Types = make([]ast.TypeExpr, len(v.Names))
						}
						if v.Types[i] == nil {
							v.Types[i] = typeToTypeExpr(elemType.Elem)
						}
					}
				case ttypes.KindTuple:
					for i := range v.Names {
						if v.Types == nil {
							v.Types = make([]ast.TypeExpr, len(v.Names))
						}
						if i < len(elemType.Tuple) && v.Types[i] == nil {
							v.Types[i] = typeToTypeExpr(elemType.Tuple[i])
						}
					}
				}
			}
		case *ast.ForOfObjectDestructureVar:
			if elemType != nil && elemType.Kind == ttypes.KindObject {
				for i, key := range v.Keys {
					if v.Types == nil {
						v.Types = make([]ast.TypeExpr, len(v.Keys))
					}
					if v.Types[i] == nil {
						if prop := elemType.PropType(key); prop != nil {
							v.Types[i] = typeToTypeExpr(prop)
						}
					}
				}
			}
		}
		annotateBlock(s.Body, checker)
	case *ast.ReturnStmt:
		if s.Value != nil {
			annotateExpr(s.Value, checker)
		}
	case *ast.BlockStmt:
		annotateBlock(s, checker)
	}
}

func annotateExpr(expr ast.Expr, checker *ttypes.Checker) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *ast.IdentExpr, *ast.IntLit, *ast.FloatLit, *ast.BoolLit, *ast.StringLit:
		return
	case *ast.ArrayLit:
		for _, entry := range e.Entries {
			annotateExpr(entry.Value, checker)
		}
	case *ast.ObjectLit:
		for _, entry := range e.Entries {
			annotateExpr(entry.Value, checker)
		}
	case *ast.CallExpr:
		annotateExpr(e.Callee, checker)
		for _, a := range e.Args {
			annotateExpr(a, checker)
		}
	case *ast.MemberExpr:
		annotateExpr(e.Object, checker)
	case *ast.IndexExpr:
		annotateExpr(e.Array, checker)
		annotateExpr(e.Index, checker)
	case *ast.UnaryExpr:
		annotateExpr(e.Expr, checker)
	case *ast.AsExpr:
		annotateExpr(e.Expr, checker)
	case *ast.BinaryExpr:
		annotateExpr(e.Left, checker)
		annotateExpr(e.Right, checker)
	case *ast.TernaryExpr:
		annotateExpr(e.Cond, checker)
		annotateExpr(e.Then, checker)
		annotateExpr(e.Else, checker)
	case *ast.SwitchExpr:
		annotateExpr(e.Value, checker)
		for _, c := range e.Cases {
			annotateExpr(c.Pattern, checker)
			annotateExpr(c.Body, checker)
		}
		if e.Default != nil {
			annotateExpr(e.Default, checker)
		}
	case *ast.BlockExpr:
		for _, s := range e.Stmts {
			annotateStmt(s, checker)
		}
	case *ast.ArrowFunc:
		// Use checker-inferred function signature
		if sig, ok := checker.ExprTypes[e]; ok && sig != nil && sig.Kind == ttypes.KindFunc {
			for i := range e.Params {
				if e.Params[i].Type == nil && i < len(sig.Params) {
					e.Params[i].Type = typeToTypeExpr(sig.Params[i])
				}
			}
			if e.Ret == nil && sig.Ret != nil {
				e.Ret = typeToTypeExpr(sig.Ret)
			}
		}
		// annotate body
		if e.Body != nil {
			annotateBlock(e.Body, checker)
		} else if e.Expr != nil {
			annotateExpr(e.Expr, checker)
		}
	case *ast.SQLExpr:
		for _, p := range e.Params {
			annotateExpr(p, checker)
		}
	case *ast.JSXElement:
		for _, attr := range e.Attributes {
			if attr.Value != nil {
				annotateExpr(attr.Value, checker)
			}
		}
		for _, child := range e.Children {
			if child.Expr != nil {
				annotateExpr(child.Expr, checker)
			} else if child.Element != nil {
				annotateExpr(child.Element, checker)
			}
		}
	case *ast.JSXFragment:
		for _, child := range e.Children {
			if child.Expr != nil {
				annotateExpr(child.Expr, checker)
			} else if child.Element != nil {
				annotateExpr(child.Element, checker)
			}
		}
	}
}

func typeToTypeExpr(t *ttypes.Type) ast.TypeExpr {
	if t == nil {
		return nil
	}
	switch t.Kind {
	case ttypes.KindI64:
		return &ast.NamedType{Name: "integer"}
	case ttypes.KindF64:
		return &ast.NamedType{Name: "number"}
	case ttypes.KindBool:
		return &ast.NamedType{Name: "boolean"}
	case ttypes.KindString:
		return &ast.NamedType{Name: "string"}
	case ttypes.KindVoid:
		return &ast.NamedType{Name: "void"}
	case ttypes.KindArray:
		return &ast.ArrayType{Elem: typeToTypeExpr(t.Elem)}
	case ttypes.KindTuple:
		elems := make([]ast.TypeExpr, len(t.Tuple))
		for i, te := range t.Tuple {
			elems[i] = typeToTypeExpr(te)
		}
		return &ast.TupleType{Elems: elems}
	case ttypes.KindUnion:
		parts := make([]ast.TypeExpr, len(t.Union))
		for i, m := range t.Union {
			parts[i] = typeToTypeExpr(m)
		}
		return &ast.UnionType{Types: parts}
	case ttypes.KindFunc:
		params := make([]ast.FuncTypeParam, len(t.Params))
		for i, p := range t.Params {
			params[i] = ast.FuncTypeParam{Type: typeToTypeExpr(p)}
		}
		return &ast.FuncType{Params: params, Ret: typeToTypeExpr(t.Ret)}
	case ttypes.KindObject:
		props := make([]ast.TypeProp, len(t.Props))
		for i, p := range t.Props {
			props[i] = ast.TypeProp{Key: p.Name, Type: typeToTypeExpr(p.Type)}
		}
		return &ast.ObjectType{Props: props}
	default:
		return &ast.NamedType{Name: "string"}
	}
}
