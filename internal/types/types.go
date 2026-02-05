package types

import "sort"

type Kind int

const (
	KindInvalid Kind = iota
	KindI64
	KindF64
	KindBool
	KindString
	KindJSON
	KindVoid
	KindFunc
	KindArray
	KindNull
	KindUndefined
	KindTuple
	KindObject
	KindUnion
	KindTypeParam
)

type Type struct {
	Kind         Kind
	Literal      bool
	LiteralValue interface{}
	TypeParams   []string
	Name         string
	Params       []*Type
	Ret          *Type
	Elem         *Type
	Tuple        []*Type
	Props        []Prop
	Union        []*Type
	Index        *Type
	propIx       map[string]*Type
}

type Prop struct {
	Name string
	Type *Type
}

func (t *Type) Equals(o *Type) bool {
	if t == nil || o == nil {
		return t == o
	}
	if t.Kind != o.Kind {
		// string型エイリアス(JSX等)もKindStringなら必ず等価とみなす
		if t.Kind == KindString || o.Kind == KindString {
			if t.Kind == KindString && o.Kind == KindString {
				return true
			}
		}
		return false
	}
	if t.Literal || o.Literal {
		if t.Literal != o.Literal {
			return false
		}
		return literalValuesEqual(t.Kind, t.LiteralValue, o.LiteralValue)
	}
	switch t.Kind {
	case KindArray:
		return t.Elem.Equals(o.Elem)
	case KindFunc:
		if len(t.Params) != len(o.Params) {
			return false
		}
		for i := range t.Params {
			if !t.Params[i].Equals(o.Params[i]) {
				return false
			}
		}
		return t.Ret.Equals(o.Ret)
	case KindTuple:
		if len(t.Tuple) != len(o.Tuple) {
			return false
		}
		for i := range t.Tuple {
			if !t.Tuple[i].Equals(o.Tuple[i]) {
				return false
			}
		}
		return true
	case KindObject:
		if len(t.Props) != len(o.Props) {
			return false
		}
		if (t.Index == nil) != (o.Index == nil) {
			return false
		}
		if t.Index != nil && !t.Index.Equals(o.Index) {
			return false
		}
		for i := range t.Props {
			if t.Props[i].Name != o.Props[i].Name {
				return false
			}
			if !t.Props[i].Type.Equals(o.Props[i].Type) {
				return false
			}
		}
		return true
	case KindUnion:
		if len(t.Union) != len(o.Union) {
			return false
		}
		used := make([]bool, len(o.Union))
		for _, member := range t.Union {
			found := false
			for i, other := range o.Union {
				if used[i] {
					continue
				}
				if member.Equals(other) {
					used[i] = true
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
		return true
	case KindTypeParam:
		return t == o
	default:
		return true
	}
}

func (t *Type) AssignableTo(dst *Type) bool {
	if t == nil || dst == nil {
		return false
	}
	// void と undefined は成功時に値を持たない型として相互代入を許可する。
	if (t.Kind == KindVoid && dst.Kind == KindUndefined) || (t.Kind == KindUndefined && dst.Kind == KindVoid) {
		return true
	}
	if dst.Kind == KindUnion {
		if t.Kind == KindUnion {
			for _, tMember := range t.Union {
				match := false
				for _, member := range dst.Union {
					if tMember.AssignableTo(member) {
						match = true
						break
					}
				}
				if !match {
					return false
				}
			}
			return true
		}
		for _, member := range dst.Union {
			if t.AssignableTo(member) {
				return true
			}
		}
		return false
	}
	if t.Kind == KindUnion {
		for _, member := range t.Union {
			if !member.AssignableTo(dst) {
				return false
			}
		}
		return true
	}
	if t.Kind != dst.Kind {
		return false
	}
	if t.Literal {
		if dst.Literal {
			return literalValuesEqual(t.Kind, t.LiteralValue, dst.LiteralValue)
		}
		return true
	}
	if dst.Literal {
		return false
	}
	switch t.Kind {
	case KindFunc:
		if len(t.Params) != len(dst.Params) {
			return false
		}
		if len(t.TypeParams) > 0 {
			bindings := map[string]*Type{}
			if !bindTypeParams(t, dst, bindings) {
				return false
			}
			return true
		}
		for i := range t.Params {
			if !t.Params[i].AssignableTo(dst.Params[i]) {
				return false
			}
		}
		return t.Ret.AssignableTo(dst.Ret)
	case KindArray:
		return t.Elem.AssignableTo(dst.Elem)
	case KindTuple:
		if len(t.Tuple) != len(dst.Tuple) {
			return false
		}
		for i := range t.Tuple {
			if !t.Tuple[i].AssignableTo(dst.Tuple[i]) {
				return false
			}
		}
		return true
	case KindObject:
		// Allow assigning object literals (with only explicit props) to map-like types
		if dst.Index != nil && len(dst.Props) == 0 {
			for _, prop := range t.Props {
				if !prop.Type.AssignableTo(dst.Index) {
					return false
				}
			}
			return true
		}
		if len(t.Props) != len(dst.Props) {
			return false
		}
		if (t.Index == nil) != (dst.Index == nil) {
			return false
		}
		if t.Index != nil && !t.Index.AssignableTo(dst.Index) {
			return false
		}
		for i := range t.Props {
			if t.Props[i].Name != dst.Props[i].Name {
				return false
			}
			if !t.Props[i].Type.AssignableTo(dst.Props[i].Type) {
				return false
			}
		}
		return true
	case KindTypeParam:
		return t == dst
	default:
		return t.Equals(dst)
	}
}

func bindTypeParams(template, actual *Type, bindings map[string]*Type) bool {
	if template == nil || actual == nil {
		return template == actual
	}
	if template.Kind == KindTypeParam {
		if bound, ok := bindings[template.Name]; ok {
			return actual.Equals(bound)
		}
		bindings[template.Name] = actual
		return true
	}
	if template.Kind != actual.Kind {
		return false
	}
	switch template.Kind {
	case KindArray:
		return bindTypeParams(template.Elem, actual.Elem, bindings)
	case KindFunc:
		if len(template.Params) != len(actual.Params) {
			return false
		}
		for i := range template.Params {
			if !bindTypeParams(template.Params[i], actual.Params[i], bindings) {
				return false
			}
		}
		return bindTypeParams(template.Ret, actual.Ret, bindings)
	case KindTuple:
		if len(template.Tuple) != len(actual.Tuple) {
			return false
		}
		for i := range template.Tuple {
			if !bindTypeParams(template.Tuple[i], actual.Tuple[i], bindings) {
				return false
			}
		}
		return true
	case KindObject:
		if len(template.Props) != len(actual.Props) {
			return false
		}
		for i := range template.Props {
			if template.Props[i].Name != actual.Props[i].Name {
				return false
			}
			if !bindTypeParams(template.Props[i].Type, actual.Props[i].Type, bindings) {
				return false
			}
		}
		if (template.Index == nil) != (actual.Index == nil) {
			return false
		}
		if template.Index != nil {
			return bindTypeParams(template.Index, actual.Index, bindings)
		}
		return true
	case KindUnion:
		if len(template.Union) != len(actual.Union) {
			return false
		}
		for i := range template.Union {
			if !bindTypeParams(template.Union[i], actual.Union[i], bindings) {
				return false
			}
		}
		return true
	default:
		return template.Equals(actual)
	}
}

func (t *Type) PropType(name string) *Type {
	if t.propIx != nil {
		if propType, ok := t.propIx[name]; ok {
			return propType
		}
	}
	if t.Index != nil {
		return t.Index
	}
	return nil
}

func NewObject(props []Prop) *Type {
	return NewObjectWithIndex(props, nil)
}

func NewObjectWithIndex(props []Prop, index *Type) *Type {
	sorted := make([]Prop, len(props))
	copy(sorted, props)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	propIx := make(map[string]*Type, len(sorted))
	for _, p := range sorted {
		propIx[p.Name] = p.Type
	}
	return &Type{Kind: KindObject, Props: sorted, Index: index, propIx: propIx}
}

func NewArray(elem *Type) *Type {
	return &Type{Kind: KindArray, Elem: elem}
}

func NewFunc(params []*Type, ret *Type) *Type {
	return &Type{Kind: KindFunc, Params: params, Ret: ret}
}

func NewTuple(elems []*Type) *Type {
	return &Type{Kind: KindTuple, Tuple: elems}
}

func NewUnion(members []*Type) *Type {
	var flat []*Type
	for _, member := range members {
		if member == nil {
			continue
		}
		if member.Kind == KindUnion {
			flat = append(flat, member.Union...)
			continue
		}
		flat = append(flat, member)
	}
	if len(flat) == 0 {
		return nil
	}
	var unique []*Type
	for _, member := range flat {
		found := false
		for _, existing := range unique {
			if member.Equals(existing) {
				found = true
				break
			}
		}
		if !found {
			unique = append(unique, member)
		}
	}
	if len(unique) == 1 {
		return unique[0]
	}
	return &Type{Kind: KindUnion, Union: unique}
}

func literalValuesEqual(kind Kind, a, b interface{}) bool {
	if a == nil || b == nil {
		return false
	}
	switch kind {
	case KindI64:
		return a.(int64) == b.(int64)
	case KindF64:
		return a.(float64) == b.(float64)
	case KindBool:
		return a.(bool) == b.(bool)
	case KindString:
		return a.(string) == b.(string)
	default:
		return false
	}
}

func baseType(typ *Type) *Type {
	if typ == nil {
		return nil
	}
	if !typ.Literal {
		return typ
	}
	switch typ.Kind {
	case KindI64:
		return I64()
	case KindF64:
		return F64()
	case KindBool:
		return Bool()
	case KindString:
		return String()
	default:
		return typ
	}
}

func typesEqual(a, b *Type) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equals(b)
}

func LiteralI64(value int64) *Type {
	return &Type{Kind: KindI64, Literal: true, LiteralValue: value}
}

func LiteralF64(value float64) *Type {
	return &Type{Kind: KindF64, Literal: true, LiteralValue: value}
}

func LiteralBool(value bool) *Type {
	return &Type{Kind: KindBool, Literal: true, LiteralValue: value}
}

func LiteralString(value string) *Type {
	return &Type{Kind: KindString, Literal: true, LiteralValue: value}
}

func NewTypeParam(name string) *Type {
	return &Type{Kind: KindTypeParam, Name: name}
}

var (
	i64Type    = &Type{Kind: KindI64}
	f64Type    = &Type{Kind: KindF64}
	boolType   = &Type{Kind: KindBool}
	stringType = &Type{Kind: KindString}
	jsonType   = &Type{Kind: KindJSON}
	voidType   = &Type{Kind: KindVoid}
	nullType   = &Type{Kind: KindNull}
	undefType  = &Type{Kind: KindUndefined}
)

func I64() *Type    { return i64Type }
func F64() *Type    { return f64Type }
func Bool() *Type   { return boolType }
func String() *Type { return stringType }
func JSON() *Type   { return jsonType }
func Void() *Type   { return voidType }
func Null() *Type   { return nullType }
func Undefined() *Type {
	return undefType
}

func Number() *Type {
	return F64()
}
