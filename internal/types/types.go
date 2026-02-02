package types

import "sort"

type Kind int

const (
	KindInvalid Kind = iota
	KindI64
	KindF64
	KindBool
	KindString
	KindVoid
	KindFunc
	KindArray
	KindTuple
	KindObject
	KindUnion
)

type Type struct {
	Kind   Kind
	Params []*Type
	Ret    *Type
	Elem   *Type
	Tuple  []*Type
	Props  []Prop
	Union  []*Type
	Index  *Type
	propIx map[string]*Type
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
	default:
		return true
	}
}

func (t *Type) AssignableTo(dst *Type) bool {
	if t == nil || dst == nil {
		return false
	}
	if dst.Kind == KindUnion {
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
	switch t.Kind {
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
	default:
		return t.Equals(dst)
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

var (
	i64Type    = &Type{Kind: KindI64}
	f64Type    = &Type{Kind: KindF64}
	boolType   = &Type{Kind: KindBool}
	stringType = &Type{Kind: KindString}
	voidType   = &Type{Kind: KindVoid}
)

func I64() *Type    { return i64Type }
func F64() *Type    { return f64Type }
func Bool() *Type   { return boolType }
func String() *Type { return stringType }
func Void() *Type   { return voidType }
