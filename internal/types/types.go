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
	KindAny
)

type Type struct {
	Kind   Kind
	Params []*Type
	Ret    *Type
	Elem   *Type
	Tuple  []*Type
	Props  []Prop
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
	if t.Kind == KindAny || o.Kind == KindAny {
		return true
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
		for i := range t.Props {
			if t.Props[i].Name != o.Props[i].Name {
				return false
			}
			if !t.Props[i].Type.Equals(o.Props[i].Type) {
				return false
			}
		}
		return true
	default:
		return true
	}
}

func (t *Type) PropType(name string) *Type {
	if t.propIx == nil {
		return nil
	}
	return t.propIx[name]
}

func NewObject(props []Prop) *Type {
	sorted := make([]Prop, len(props))
	copy(sorted, props)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	propIx := make(map[string]*Type, len(sorted))
	for _, p := range sorted {
		propIx[p.Name] = p.Type
	}
	return &Type{Kind: KindObject, Props: sorted, propIx: propIx}
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

var (
	anyType    = &Type{Kind: KindAny}
	i64Type    = &Type{Kind: KindI64}
	f64Type    = &Type{Kind: KindF64}
	boolType   = &Type{Kind: KindBool}
	stringType = &Type{Kind: KindString}
	voidType   = &Type{Kind: KindVoid}
)

func Any() *Type    { return anyType }
func I64() *Type    { return i64Type }
func F64() *Type    { return f64Type }
func Bool() *Type   { return boolType }
func String() *Type { return stringType }
func Void() *Type   { return voidType }
