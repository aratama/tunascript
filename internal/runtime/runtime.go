//go:build cgo
// +build cgo

package runtime

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/bytecodealliance/wasmtime-go"
	_ "modernc.org/sqlite"
)

type Kind int

const (
	KindI64 Kind = iota
	KindF64
	KindBool
	KindString
	KindObject
	KindArray
)

type Value struct {
	Kind Kind
	I64  int64
	F64  float64
	Bool bool
	Str  string
	Obj  *Object
	Arr  *Array
}

type Object struct {
	Order []string
	Props map[string]int32
}

type Array struct {
	Elems []int32
}

// TableDef represents a table definition for validation
type TableDef struct {
	Name    string      `json:"name"`
	Columns []ColumnDef `json:"columns"`
}

// ColumnDef represents a column definition
type ColumnDef struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Constraints string `json:"constraints"`
}

// HTTPServer represents an HTTP server instance
type HTTPServer struct {
	mux    *http.ServeMux
	routes map[string]int32 // path -> handler handle
}

// HTTPRequest represents an HTTP request
type HTTPRequest struct {
	Method string
	Path   string
}

// HTTPResponse represents an HTTP response
type HTTPResponse struct {
	Body        string
	ContentType string
	StatusCode  int
}

type Runtime struct {
	heap            []Value
	output          bytes.Buffer
	db              *sql.DB
	args            []string
	tableDefs       []TableDef // Table definitions for validation
	httpServers     map[int32]*HTTPServer
	httpMu          sync.Mutex
	store           *wasmtime.Store
	instance        *wasmtime.Instance
	internedStrings map[uint64]int32 // 文字列リテラルのインターンキャッシュ
	// pendingServer is set when http_listen is called, actual server starts after WASM execution
	pendingServer *pendingHTTPServer
}

// pendingHTTPServer holds info for starting HTTP server after WASM execution completes
//
// ============================================================================
// HTTPサーバー実装における重要な設計上の注意点
// ============================================================================
//
// 【問題の背景】
// 当初、http_listen関数内で直接http.ListenAndServeを呼び出していた。
// これにより、WASMの関数呼び出しスタックがhttp_listenの実行中にアクティブな
// 状態のまま、HTTPハンドラーが呼び出されることになっていた。
//
// 【発生したエラー】
// "wasm trap: call stack exhausted" - スタックオーバーフロー
//
// 【原因の詳細】
//  1. negitoroプログラムのmain関数がlisten()を呼び出す
//  2. listen()はWASMのimport関数としてhttp_listen(Go関数)を呼び出す
//  3. http_listenがhttp.ListenAndServe()でブロックする
//  4. この時点でWASMのコールスタックは以下の状態:
//     [_start] -> [main_impl] -> [http_listen(import)]
//  5. HTTPリクエストが来ると、ハンドラー内でhandlerFunc.Call()を実行
//  6. これは同じWASM instanceとstoreを使用して新しいWASM関数を呼び出す
//  7. wasmtimeは既存のスタックフレーム上に新しい呼び出しを追加しようとする
//  8. しかし、スタックは既にhttp_listen呼び出しで使用中のため、
//     スタックの再入(reentrant)が発生し、即座にスタックオーバーフロー
//
// 【解決策】
// http_listenではサーバー情報をpendingServerに保存するだけにし、
// 実際のサーバー起動はWASM実行が完全に終了した後に行う。
// これにより、HTTPハンドラーからWASM関数を呼び出す際に、
// WASMのコールスタックがクリアな状態になる。
//
// 【実行フロー（修正後）】
// 1. _start() -> main_impl() -> http_listen() : pendingServerに保存して即return
// 2. WASM実行完了、スタックがクリアになる
// 3. runner.goでStartPendingServer()を呼び出し
// 4. http.ListenAndServe()でサーバー起動
// 5. HTTPリクエスト到着時、handlerFunc.Call()はクリアなスタックで実行可能
// ============================================================================
type pendingHTTPServer struct {
	server *HTTPServer
	port   string
}

func NewRuntime() *Runtime {
	r := &Runtime{
		httpServers:     make(map[int32]*HTTPServer),
		internedStrings: make(map[uint64]int32),
	}
	r.heap = append(r.heap, Value{})
	// Initialize in-memory SQLite database
	db, err := sql.Open("sqlite", ":memory:")
	if err == nil {
		r.db = db
	}
	return r
}

// SetWasmContext sets the store and instance for callback invocation
func (r *Runtime) SetWasmContext(store *wasmtime.Store, instance *wasmtime.Instance) {
	r.store = store
	r.instance = instance
}

func (r *Runtime) Output() string {
	return r.output.String()
}

func (r *Runtime) SetArgs(args []string) {
	r.args = args
}

func (r *Runtime) Define(linker *wasmtime.Linker, store *wasmtime.Store) error {
	define := func(name string, fn interface{}) error {
		return linker.DefineFunc(store, "prelude", name, fn)
	}
	if err := define("str_from_utf8", func(caller *wasmtime.Caller, ptr int32, length int32) int32 {
		return must(r.strFromUTF8(caller, ptr, length))
	}); err != nil {
		return err
	}
	if err := define("intern_string", func(caller *wasmtime.Caller, ptr int32, length int32) int32 {
		return must(r.internString(caller, ptr, length))
	}); err != nil {
		return err
	}
	if err := define("str_concat", func(a int32, b int32) int32 {
		return must(r.strConcat(a, b))
	}); err != nil {
		return err
	}
	if err := define("str_eq", func(a int32, b int32) int32 {
		return must(r.strEq(a, b))
	}); err != nil {
		return err
	}
	if err := define("val_from_i64", func(v int64) int32 {
		return must(r.valFromI64(v))
	}); err != nil {
		return err
	}
	if err := define("val_from_f64", func(v float64) int32 {
		return must(r.valFromF64(v))
	}); err != nil {
		return err
	}
	if err := define("val_from_bool", func(v int32) int32 {
		return must(r.valFromBool(v))
	}); err != nil {
		return err
	}
	if err := define("val_to_i64", func(handle int32) int64 {
		return must(r.valToI64(handle))
	}); err != nil {
		return err
	}
	if err := define("val_to_f64", func(handle int32) float64 {
		return must(r.valToF64(handle))
	}); err != nil {
		return err
	}
	if err := define("val_to_bool", func(handle int32) int32 {
		return must(r.valToBool(handle))
	}); err != nil {
		return err
	}
	if err := define("val_kind", func(handle int32) int32 {
		return must(r.valKind(handle))
	}); err != nil {
		return err
	}
	if err := define("obj_new", func(count int32) int32 {
		return must(r.objNew(count))
	}); err != nil {
		return err
	}
	if err := define("obj_set", func(objHandle int32, keyHandle int32, valHandle int32) {
		must0(r.objSet(objHandle, keyHandle, valHandle))
	}); err != nil {
		return err
	}
	if err := define("obj_get", func(objHandle int32, keyHandle int32) int32 {
		return must(r.objGet(objHandle, keyHandle))
	}); err != nil {
		return err
	}
	if err := define("arr_new", func(count int32) int32 {
		return must(r.arrNew(count))
	}); err != nil {
		return err
	}
	if err := define("arr_set", func(arrHandle int32, index int32, valHandle int32) {
		must0(r.arrSet(arrHandle, index, valHandle))
	}); err != nil {
		return err
	}
	if err := define("arr_get", func(arrHandle int32, index int32) int32 {
		return must(r.arrGet(arrHandle, index))
	}); err != nil {
		return err
	}
	if err := define("arr_len", func(arrHandle int32) int32 {
		return must(r.arrLen(arrHandle))
	}); err != nil {
		return err
	}
	if err := define("arr_join", func(arrHandle int32) int32 {
		return must(r.arrJoin(arrHandle))
	}); err != nil {
		return err
	}
	if err := define("range", func(start int64, end int64) int32 {
		return must(r.rangeFunc(start, end))
	}); err != nil {
		return err
	}
	if err := define("val_eq", func(a int32, b int32) int32 {
		return must(r.valEq(a, b))
	}); err != nil {
		return err
	}
	if err := define("print", func(handle int32) {
		must0(r.print(handle))
	}); err != nil {
		return err
	}
	if err := define("stringify", func(handle int32) int32 {
		return must(r.stringify(handle))
	}); err != nil {
		return err
	}
	if err := define("parse", func(handle int32) int32 {
		return must(r.parse(handle))
	}); err != nil {
		return err
	}
	if err := define("toString", func(handle int32) int32 {
		return must(r.toString(handle))
	}); err != nil {
		return err
	}
	if err := define("sql_exec", func(caller *wasmtime.Caller, ptr int32, length int32) int32 {
		return must(r.sqlExec(caller, ptr, length))
	}); err != nil {
		return err
	}
	if err := define("db_save", func(strHandle int32) {
		must0(r.dbSaveHandle(strHandle))
	}); err != nil {
		return err
	}
	if err := define("db_open", func(strHandle int32) {
		must0(r.dbOpenHandle(strHandle))
	}); err != nil {
		return err
	}
	if err := define("register_tables", func(caller *wasmtime.Caller, ptr int32, length int32) {
		must0(r.registerTables(caller, ptr, length))
	}); err != nil {
		return err
	}
	if err := define("get_args", func() int32 {
		return must(r.getArgs())
	}); err != nil {
		return err
	}
	if err := define("sql_query", func(caller *wasmtime.Caller, ptr int32, length int32, paramsHandle int32) int32 {
		return must(r.sqlQuery(caller, ptr, length, paramsHandle))
	}); err != nil {
		return err
	}
	if err := define("sql_fetch_one", func(caller *wasmtime.Caller, ptr int32, length int32, paramsHandle int32) int32 {
		return must(r.sqlFetchOne(caller, ptr, length, paramsHandle))
	}); err != nil {
		return err
	}
	if err := define("sql_fetch_optional", func(caller *wasmtime.Caller, ptr int32, length int32, paramsHandle int32) int32 {
		return must(r.sqlFetchOptional(caller, ptr, length, paramsHandle))
	}); err != nil {
		return err
	}
	if err := define("sql_execute", func(caller *wasmtime.Caller, ptr int32, length int32, paramsHandle int32) {
		must0(r.sqlExecute(caller, ptr, length, paramsHandle))
	}); err != nil {
		return err
	}
	// HTTP server functions
	if err := define("http_create_server", func() int32 {
		return must(r.httpCreateServer())
	}); err != nil {
		return err
	}
	if err := define("http_add_route", func(caller *wasmtime.Caller, serverHandle int32, pathPtr int32, pathLen int32, handlerHandle int32) {
		must0(r.httpAddRoute(caller, serverHandle, pathPtr, pathLen, handlerHandle))
	}); err != nil {
		return err
	}
	if err := define("http_listen", func(caller *wasmtime.Caller, serverHandle int32, portPtr int32, portLen int32) {
		must0(r.httpListen(caller, serverHandle, portPtr, portLen))
	}); err != nil {
		return err
	}
	if err := define("http_response_text", func(caller *wasmtime.Caller, textPtr int32, textLen int32) int32 {
		return must(r.httpResponseText(caller, textPtr, textLen))
	}); err != nil {
		return err
	}
	if err := define("http_response_html", func(caller *wasmtime.Caller, htmlPtr int32, htmlLen int32) int32 {
		return must(r.httpResponseHtml(caller, htmlPtr, htmlLen))
	}); err != nil {
		return err
	}
	if err := define("http_response_text_str", func(strHandle int32) int32 {
		return must(r.httpResponseTextStr(strHandle))
	}); err != nil {
		return err
	}
	if err := define("http_response_html_str", func(strHandle int32) int32 {
		return must(r.httpResponseHtmlStr(strHandle))
	}); err != nil {
		return err
	}
	if err := define("http_response_json", func(dataHandle int32) int32 {
		return must(r.httpResponseJson(dataHandle))
	}); err != nil {
		return err
	}
	if err := define("http_response_redirect", func(caller *wasmtime.Caller, urlPtr int32, urlLen int32) int32 {
		return must(r.httpResponseRedirect(caller, urlPtr, urlLen))
	}); err != nil {
		return err
	}
	if err := define("http_response_redirect_str", func(strHandle int32) int32 {
		return must(r.httpResponseRedirectStr(strHandle))
	}); err != nil {
		return err
	}
	if err := define("http_get_path", func(reqHandle int32) int32 {
		return must(r.httpGetPath(reqHandle))
	}); err != nil {
		return err
	}
	if err := define("http_get_method", func(reqHandle int32) int32 {
		return must(r.httpGetMethod(reqHandle))
	}); err != nil {
		return err
	}
	return nil
}

func must[T any](value T, err error) T {
	if err != nil {
		panic(wasmtime.NewTrap(err.Error()))
	}
	return value
}

func must0(err error) {
	if err != nil {
		panic(wasmtime.NewTrap(err.Error()))
	}
}

func (r *Runtime) newValue(v Value) int32 {
	r.heap = append(r.heap, v)
	return int32(len(r.heap) - 1)
}

func (r *Runtime) getValue(handle int32) (*Value, error) {
	if handle < 0 || int(handle) >= len(r.heap) {
		return nil, fmt.Errorf("invalid handle: %d", handle)
	}
	return &r.heap[handle], nil
}

func (r *Runtime) strFromUTF8(caller *wasmtime.Caller, ptr int32, length int32) (int32, error) {
	ext := caller.GetExport("memory")
	if ext == nil {
		return 0, errors.New("memory not found")
	}
	memory := ext.Memory()
	if memory == nil {
		return 0, errors.New("memory not found")
	}
	data := memory.UnsafeData(caller)
	start := int(ptr)
	end := start + int(length)
	if start < 0 || end > len(data) {
		return 0, errors.New("string out of bounds")
	}
	return r.newValue(Value{Kind: KindString, Str: string(data[start:end])}), nil
}

// internString は文字列リテラル（offset, length）をヒープハンドルに変換します。
// 同じリテラルは同じハンドルを返します（インターン）。
func (r *Runtime) internString(caller *wasmtime.Caller, ptr int32, length int32) (int32, error) {
	// キャッシュをチェック
	key := uint64(ptr)<<32 | uint64(uint32(length))
	if handle, ok := r.internedStrings[key]; ok {
		return handle, nil
	}

	// メモリから文字列を読み取り
	ext := caller.GetExport("memory")
	if ext == nil {
		return 0, errors.New("memory not found")
	}
	memory := ext.Memory()
	if memory == nil {
		return 0, errors.New("memory not found")
	}
	data := memory.UnsafeData(caller)
	start := int(ptr)
	end := start + int(length)
	if start < 0 || end > len(data) {
		return 0, errors.New("string out of bounds")
	}
	str := string(data[start:end])

	// ヒープに登録
	handle := r.newValue(Value{Kind: KindString, Str: str})

	// キャッシュに保存
	r.internedStrings[key] = handle
	return handle, nil
}

func (r *Runtime) strConcat(a int32, b int32) (int32, error) {
	va, err := r.getValue(a)
	if err != nil {
		return 0, err
	}
	vb, err := r.getValue(b)
	if err != nil {
		return 0, err
	}
	if va.Kind != KindString || vb.Kind != KindString {
		return 0, errors.New("str_concat type error")
	}
	return r.newValue(Value{Kind: KindString, Str: va.Str + vb.Str}), nil
}

func (r *Runtime) strEq(a int32, b int32) (int32, error) {
	va, err := r.getValue(a)
	if err != nil {
		return 0, err
	}
	vb, err := r.getValue(b)
	if err != nil {
		return 0, err
	}
	if va.Kind != KindString || vb.Kind != KindString {
		return 0, errors.New("str_eq type error")
	}
	if va.Str == vb.Str {
		return 1, nil
	}
	return 0, nil
}

func (r *Runtime) valFromI64(v int64) (int32, error) {
	return r.newValue(Value{Kind: KindI64, I64: v}), nil
}

func (r *Runtime) valFromF64(v float64) (int32, error) {
	return r.newValue(Value{Kind: KindF64, F64: v}), nil
}

func (r *Runtime) valFromBool(v int32) (int32, error) {
	return r.newValue(Value{Kind: KindBool, Bool: v != 0}), nil
}

func (r *Runtime) valToI64(handle int32) (int64, error) {
	v, err := r.getValue(handle)
	if err != nil {
		return 0, err
	}
	if v.Kind != KindI64 {
		return 0, errors.New("not integer")
	}
	return v.I64, nil
}

func (r *Runtime) valToF64(handle int32) (float64, error) {
	v, err := r.getValue(handle)
	if err != nil {
		return 0, err
	}
	if v.Kind != KindF64 {
		return 0, errors.New("not float")
	}
	return v.F64, nil
}

func (r *Runtime) valToBool(handle int32) (int32, error) {
	v, err := r.getValue(handle)
	if err != nil {
		return 0, err
	}
	if v.Kind != KindBool {
		return 0, errors.New("not boolean")
	}
	if v.Bool {
		return 1, nil
	}
	return 0, nil
}

func (r *Runtime) valKind(handle int32) (int32, error) {
	v, err := r.getValue(handle)
	if err != nil {
		return 0, err
	}
	return int32(v.Kind), nil
}

func (r *Runtime) objNew(count int32) (int32, error) {
	return r.newValue(Value{Kind: KindObject, Obj: &Object{Order: []string{}, Props: map[string]int32{}}}), nil
}

func (r *Runtime) objSet(objHandle int32, keyHandle int32, valHandle int32) error {
	objVal, err := r.getValue(objHandle)
	if err != nil {
		return err
	}
	keyVal, err := r.getValue(keyHandle)
	if err != nil {
		return err
	}
	if objVal.Kind != KindObject || keyVal.Kind != KindString {
		return errors.New("obj_set type error")
	}
	key := keyVal.Str
	if _, ok := objVal.Obj.Props[key]; !ok {
		objVal.Obj.Order = append(objVal.Obj.Order, key)
	}
	objVal.Obj.Props[key] = valHandle
	return nil
}

func (r *Runtime) objGet(objHandle int32, keyHandle int32) (int32, error) {
	objVal, err := r.getValue(objHandle)
	if err != nil {
		return 0, err
	}
	keyVal, err := r.getValue(keyHandle)
	if err != nil {
		return 0, err
	}
	if objVal.Kind != KindObject || keyVal.Kind != KindString {
		return 0, errors.New("obj_get type error")
	}
	key := keyVal.Str
	val, ok := objVal.Obj.Props[key]
	if !ok {
		// Return empty string for missing keys (useful for optional query params, form fields, etc.)
		return r.newValue(Value{Kind: KindString, Str: ""}), nil
	}
	return val, nil
}

func (r *Runtime) arrNew(count int32) (int32, error) {
	arr := make([]int32, int(count))
	return r.newValue(Value{Kind: KindArray, Arr: &Array{Elems: arr}}), nil
}

func (r *Runtime) arrSet(arrHandle int32, index int32, valHandle int32) error {
	arrVal, err := r.getValue(arrHandle)
	if err != nil {
		return err
	}
	if arrVal.Kind != KindArray {
		return errors.New("arr_set type error")
	}
	if index < 0 || int(index) >= len(arrVal.Arr.Elems) {
		return errors.New("index out of range")
	}
	arrVal.Arr.Elems[index] = valHandle
	return nil
}

func (r *Runtime) arrGet(arrHandle int32, index int32) (int32, error) {
	arrVal, err := r.getValue(arrHandle)
	if err != nil {
		return 0, err
	}
	if arrVal.Kind != KindArray {
		return 0, errors.New("arr_get type error")
	}
	if index < 0 || int(index) >= len(arrVal.Arr.Elems) {
		return 0, errors.New("index out of range")
	}
	return arrVal.Arr.Elems[index], nil
}

func (r *Runtime) arrLen(arrHandle int32) (int32, error) {
	arrVal, err := r.getValue(arrHandle)
	if err != nil {
		return 0, err
	}
	if arrVal.Kind != KindArray {
		return 0, errors.New("arr_len type error")
	}
	return int32(len(arrVal.Arr.Elems)), nil
}

func (r *Runtime) arrJoin(arrHandle int32) (int32, error) {
	arrVal, err := r.getValue(arrHandle)
	if err != nil {
		return 0, err
	}
	if arrVal.Kind != KindArray {
		return 0, errors.New("arr_join type error")
	}
	var parts []string
	for _, elemHandle := range arrVal.Arr.Elems {
		elemVal, err := r.getValue(elemHandle)
		if err != nil {
			continue
		}
		if elemVal.Kind == KindString {
			parts = append(parts, elemVal.Str)
		}
	}
	result := strings.Join(parts, "")
	return r.newValue(Value{Kind: KindString, Str: result}), nil
}

func (r *Runtime) rangeFunc(start int64, end int64) (int32, error) {
	if end < start {
		return 0, errors.New("range end must be >= start")
	}
	delta := end - start
	if delta < 0 {
		return 0, errors.New("range too large")
	}
	if delta >= int64(math.MaxInt32) {
		return 0, errors.New("range too large")
	}
	length := delta + 1
	elems := make([]int32, int(length))
	for i := int64(0); i < length; i++ {
		val, err := r.valFromI64(start + i)
		if err != nil {
			return 0, err
		}
		elems[int(i)] = val
	}
	return r.newValue(Value{Kind: KindArray, Arr: &Array{Elems: elems}}), nil
}

func (r *Runtime) valEq(a int32, b int32) (int32, error) {
	va, err := r.getValue(a)
	if err != nil {
		return 0, err
	}
	vb, err := r.getValue(b)
	if err != nil {
		return 0, err
	}
	eq := r.valueEqual(va, vb)
	if eq {
		return 1, nil
	}
	return 0, nil
}

func (r *Runtime) valueEqual(a *Value, b *Value) bool {
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case KindI64:
		return a.I64 == b.I64
	case KindF64:
		return a.F64 == b.F64
	case KindBool:
		return a.Bool == b.Bool
	case KindString:
		return a.Str == b.Str
	case KindArray:
		if len(a.Arr.Elems) != len(b.Arr.Elems) {
			return false
		}
		for i := range a.Arr.Elems {
			va, err := r.getValue(a.Arr.Elems[i])
			if err != nil {
				return false
			}
			vb, err := r.getValue(b.Arr.Elems[i])
			if err != nil {
				return false
			}
			if !r.valueEqual(va, vb) {
				return false
			}
		}
		return true
	case KindObject:
		if len(a.Obj.Props) != len(b.Obj.Props) {
			return false
		}
		for key, av := range a.Obj.Props {
			bv, ok := b.Obj.Props[key]
			if !ok {
				return false
			}
			va, err := r.getValue(av)
			if err != nil {
				return false
			}
			vb, err := r.getValue(bv)
			if err != nil {
				return false
			}
			if !r.valueEqual(va, vb) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func (r *Runtime) print(handle int32) error {
	v, err := r.getValue(handle)
	if err != nil {
		return err
	}
	if v.Kind == KindString {
		r.output.WriteString(v.Str)
		r.output.WriteString("\n")
		return nil
	}
	text, err := r.stringifyValue(handle)
	if err != nil {
		return err
	}
	r.output.WriteString(text)
	r.output.WriteString("\n")
	return nil
}

func (r *Runtime) stringify(handle int32) (int32, error) {
	text, err := r.stringifyValue(handle)
	if err != nil {
		return 0, err
	}
	return r.newValue(Value{Kind: KindString, Str: text}), nil
}

func (r *Runtime) toString(handle int32) (int32, error) {
	v, err := r.getValue(handle)
	if err != nil {
		return 0, err
	}
	switch v.Kind {
	case KindString:
		return handle, nil
	case KindI64:
		return r.newValue(Value{Kind: KindString, Str: strconv.FormatInt(v.I64, 10)}), nil
	case KindF64:
		if math.IsNaN(v.F64) || math.IsInf(v.F64, 0) {
			return 0, errors.New("invalid float")
		}
		return r.newValue(Value{Kind: KindString, Str: strconv.FormatFloat(v.F64, 'g', -1, 64)}), nil
	case KindBool:
		if v.Bool {
			return r.newValue(Value{Kind: KindString, Str: "true"}), nil
		}
		return r.newValue(Value{Kind: KindString, Str: "false"}), nil
	default:
		return 0, errors.New("toString expects primitive")
	}
}

func (r *Runtime) parse(handle int32) (int32, error) {
	v, err := r.getValue(handle)
	if err != nil {
		return 0, err
	}
	if v.Kind != KindString {
		return 0, errors.New("parse expects string")
	}
	dec := json.NewDecoder(strings.NewReader(v.Str))
	dec.UseNumber()
	var data interface{}
	if err := dec.Decode(&data); err != nil {
		return 0, err
	}
	return r.fromInterface(data)
}

func (r *Runtime) fromInterface(v interface{}) (int32, error) {
	switch val := v.(type) {
	case string:
		return r.newValue(Value{Kind: KindString, Str: val}), nil
	case bool:
		return r.newValue(Value{Kind: KindBool, Bool: val}), nil
	case json.Number:
		str := val.String()
		if strings.ContainsAny(str, ".eE") {
			f, err := val.Float64()
			if err != nil {
				return 0, err
			}
			return r.newValue(Value{Kind: KindF64, F64: f}), nil
		}
		i, err := val.Int64()
		if err != nil {
			f, ferr := val.Float64()
			if ferr != nil {
				return 0, ferr
			}
			return r.newValue(Value{Kind: KindF64, F64: f}), nil
		}
		return r.newValue(Value{Kind: KindI64, I64: i}), nil
	case []interface{}:
		arr := make([]int32, len(val))
		for i, elem := range val {
			child, err := r.fromInterface(elem)
			if err != nil {
				return 0, err
			}
			arr[i] = child
		}
		return r.newValue(Value{Kind: KindArray, Arr: &Array{Elems: arr}}), nil
	case map[string]interface{}:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		props := map[string]int32{}
		for _, k := range keys {
			child, err := r.fromInterface(val[k])
			if err != nil {
				return 0, err
			}
			props[k] = child
		}
		return r.newValue(Value{Kind: KindObject, Obj: &Object{Order: keys, Props: props}}), nil
	default:
		return 0, errors.New("unsupported json")
	}
}

func (r *Runtime) stringifyValue(handle int32) (string, error) {
	var buf bytes.Buffer
	if err := r.writeJSON(handle, &buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (r *Runtime) writeJSON(handle int32, buf *bytes.Buffer) error {
	v, err := r.getValue(handle)
	if err != nil {
		return err
	}
	switch v.Kind {
	case KindString:
		buf.WriteString(strconv.Quote(v.Str))
	case KindI64:
		buf.WriteString(strconv.FormatInt(v.I64, 10))
	case KindF64:
		if math.IsNaN(v.F64) || math.IsInf(v.F64, 0) {
			return errors.New("invalid float")
		}
		buf.WriteString(strconv.FormatFloat(v.F64, 'g', -1, 64))
	case KindBool:
		if v.Bool {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case KindArray:
		buf.WriteByte('[')
		for i, child := range v.Arr.Elems {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := r.writeJSON(child, buf); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	case KindObject:
		buf.WriteByte('{')
		for i, key := range v.Obj.Order {
			if i > 0 {
				buf.WriteByte(',')
			}
			buf.WriteString(strconv.Quote(key))
			buf.WriteByte(':')
			child := v.Obj.Props[key]
			if err := r.writeJSON(child, buf); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
	default:
		return errors.New("unsupported type")
	}
	return nil
}

// sqlExec executes a SQL query and returns the result as an object with columns and rows
func (r *Runtime) sqlExec(caller *wasmtime.Caller, ptr int32, length int32) (int32, error) {
	if r.db == nil {
		return 0, errors.New("database not initialized")
	}

	ext := caller.GetExport("memory")
	if ext == nil {
		return 0, errors.New("memory not found")
	}
	memory := ext.Memory()
	if memory == nil {
		return 0, errors.New("memory not found")
	}
	data := memory.UnsafeData(caller)
	start := int(ptr)
	end := start + int(length)
	if start < 0 || end > len(data) {
		return 0, errors.New("sql string out of bounds")
	}
	query := string(data[start:end])

	// Determine if it's a SELECT query or a modification query
	trimmed := strings.TrimSpace(strings.ToUpper(query))
	isSelect := strings.HasPrefix(trimmed, "SELECT")

	if isSelect {
		return r.execSelectQuery(query)
	}
	return r.execModifyQuery(query)
}

func (r *Runtime) execSelectQuery(query string) (int32, error) {
	rows, err := r.db.Query(query)
	if err != nil {
		return 0, fmt.Errorf("sql query error: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return 0, fmt.Errorf("sql columns error: %w", err)
	}

	// Create columns array
	colHandles := make([]int32, len(cols))
	for i, col := range cols {
		colHandles[i] = r.newValue(Value{Kind: KindString, Str: col})
	}
	columnsArr := r.newValue(Value{Kind: KindArray, Arr: &Array{Elems: colHandles}})

	// Read all rows as objects
	var rowHandles []int32
	values := make([]interface{}, len(cols))
	valuePtrs := make([]interface{}, len(cols))
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	for rows.Next() {
		if err := rows.Scan(valuePtrs...); err != nil {
			return 0, fmt.Errorf("sql scan error: %w", err)
		}
		// Create row object with column names as keys
		rowObj := r.newValue(Value{Kind: KindObject, Obj: &Object{Order: []string{}, Props: map[string]int32{}}})
		for i, v := range values {
			var str string
			if v == nil {
				str = ""
			} else {
				str = fmt.Sprintf("%v", v)
			}
			colName := strings.ToLower(cols[i])
			keyHandle := r.newValue(Value{Kind: KindString, Str: colName})
			valueHandle := r.newValue(Value{Kind: KindString, Str: str})
			if err := r.objSet(rowObj, keyHandle, valueHandle); err != nil {
				return 0, err
			}
		}
		rowHandles = append(rowHandles, rowObj)
	}

	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("sql rows error: %w", err)
	}

	rowsArr := r.newValue(Value{Kind: KindArray, Arr: &Array{Elems: rowHandles}})

	// Create result object { "columns": [...], "rows": [...] }
	columnsKey := r.newValue(Value{Kind: KindString, Str: "columns"})
	rowsKey := r.newValue(Value{Kind: KindString, Str: "rows"})

	objHandle := r.newValue(Value{Kind: KindObject, Obj: &Object{Order: []string{}, Props: map[string]int32{}}})
	if err := r.objSet(objHandle, columnsKey, columnsArr); err != nil {
		return 0, err
	}
	if err := r.objSet(objHandle, rowsKey, rowsArr); err != nil {
		return 0, err
	}

	return objHandle, nil
}

func (r *Runtime) execModifyQuery(query string) (int32, error) {
	result, err := r.db.Exec(query)
	if err != nil {
		return 0, fmt.Errorf("sql exec error: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()

	// Return object with columns: [] and rows: [] for non-SELECT queries
	// Include rowsAffected info as well
	columnsArr := r.newValue(Value{Kind: KindArray, Arr: &Array{Elems: []int32{}}})

	// For INSERT/UPDATE/DELETE, return empty rows but we can include metadata
	rowsArr := r.newValue(Value{Kind: KindArray, Arr: &Array{Elems: []int32{}}})

	columnsKey := r.newValue(Value{Kind: KindString, Str: "columns"})
	rowsKey := r.newValue(Value{Kind: KindString, Str: "rows"})
	affectedKey := r.newValue(Value{Kind: KindString, Str: "rowsAffected"})

	objHandle := r.newValue(Value{Kind: KindObject, Obj: &Object{Order: []string{}, Props: map[string]int32{}}})
	if err := r.objSet(objHandle, columnsKey, columnsArr); err != nil {
		return 0, err
	}
	if err := r.objSet(objHandle, rowsKey, rowsArr); err != nil {
		return 0, err
	}
	affectedHandle, _ := r.valFromI64(rowsAffected)
	if err := r.objSet(objHandle, affectedKey, affectedHandle); err != nil {
		return 0, err
	}

	return objHandle, nil
}

// dbSave saves the in-memory database to a file
// dbSaveHandle saves the database using a string handle from the heap
func (r *Runtime) dbSaveHandle(strHandle int32) error {
	if r.db == nil {
		return errors.New("database not initialized")
	}
	val, err := r.getValue(strHandle)
	if err != nil {
		return err
	}
	if val.Kind != KindString {
		return errors.New("dbSaveHandle expects a string")
	}
	filename := val.Str
	os.Remove(filename)
	_, err = r.db.Exec(fmt.Sprintf("VACUUM INTO '%s'", filename))
	if err != nil {
		return fmt.Errorf("db save error: %w", err)
	}
	return nil
}

// dbOpenHandle opens a database using a string handle from the heap
func (r *Runtime) dbOpenHandle(strHandle int32) error {
	val, err := r.getValue(strHandle)
	if err != nil {
		return err
	}
	if val.Kind != KindString {
		return errors.New("dbOpenHandle expects a string")
	}
	filename := val.Str

	// Close existing database if any
	if r.db != nil {
		r.db.Close()
	}

	// Check if file exists and has content
	fileInfo, err := os.Stat(filename)
	fileExists := err == nil && fileInfo.Size() > 0

	// Always use in-memory database for full read/write access
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return fmt.Errorf("db open error: %w", err)
	}
	r.db = db

	if fileExists {
		// Restore the in-memory database from the file
		_, err := r.db.Exec(fmt.Sprintf("ATTACH DATABASE '%s' AS backup", filename))
		if err != nil {
			return fmt.Errorf("db attach error: %w", err)
		}

		// Get all tables from the backup database
		rows, err := r.db.Query("SELECT name FROM backup.sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'")
		if err != nil {
			return fmt.Errorf("db query tables error: %w", err)
		}
		defer rows.Close()

		var tables []string
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				return fmt.Errorf("db scan error: %w", err)
			}
			tables = append(tables, name)
		}

		// Copy each table from backup to main
		for _, table := range tables {
			var createSQL string
			err := r.db.QueryRow(fmt.Sprintf("SELECT sql FROM backup.sqlite_master WHERE type='table' AND name='%s'", table)).Scan(&createSQL)
			if err != nil {
				return fmt.Errorf("db get create sql error: %w", err)
			}
			_, err = r.db.Exec(createSQL)
			if err != nil {
				return fmt.Errorf("db create table error: %w", err)
			}
			_, err = r.db.Exec(fmt.Sprintf("INSERT INTO main.%s SELECT * FROM backup.%s", table, table))
			if err != nil {
				return fmt.Errorf("db copy data error: %w", err)
			}
		}

		_, err = r.db.Exec("DETACH DATABASE backup")
		if err != nil {
			return fmt.Errorf("db detach error: %w", err)
		}
	}

	// Initialize and validate tables based on registered table definitions
	if err := r.initAndValidateTables(); err != nil {
		return err
	}

	return nil
}

// registerTables registers table definitions from JSON
func (r *Runtime) registerTables(caller *wasmtime.Caller, ptr int32, length int32) error {
	ext := caller.GetExport("memory")
	if ext == nil {
		return errors.New("memory not found")
	}
	memory := ext.Memory()
	if memory == nil {
		return errors.New("memory not found")
	}
	data := memory.UnsafeData(caller)
	start := int(ptr)
	end := start + int(length)
	if start < 0 || end > len(data) {
		return errors.New("table defs out of bounds")
	}
	jsonData := string(data[start:end])

	var tableDefs []TableDef
	if err := json.Unmarshal([]byte(jsonData), &tableDefs); err != nil {
		return fmt.Errorf("failed to parse table definitions: %w", err)
	}
	r.tableDefs = tableDefs
	return nil
}

// initAndValidateTables creates or validates tables based on registered definitions
func (r *Runtime) initAndValidateTables() error {
	if r.db == nil || len(r.tableDefs) == 0 {
		return nil
	}

	for _, tableDef := range r.tableDefs {
		// Check if table exists
		exists, err := r.tableExists(tableDef.Name)
		if err != nil {
			return err
		}

		if exists {
			// Validate table structure
			if err := r.validateTableStructure(tableDef); err != nil {
				return err
			}
		} else {
			// Create table
			if err := r.createTable(tableDef); err != nil {
				return err
			}
		}
	}
	return nil
}

// tableExists checks if a table exists in the database
func (r *Runtime) tableExists(tableName string) (bool, error) {
	var count int
	err := r.db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", tableName).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check table existence: %w", err)
	}
	return count > 0, nil
}

// validateTableStructure validates that an existing table matches the definition
func (r *Runtime) validateTableStructure(tableDef TableDef) error {
	rows, err := r.db.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableDef.Name))
	if err != nil {
		return fmt.Errorf("failed to get table info: %w", err)
	}
	defer rows.Close()

	existingColumns := make(map[string]string) // name -> type
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dfltValue interface{}
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err != nil {
			return fmt.Errorf("failed to scan table info: %w", err)
		}
		existingColumns[strings.ToLower(name)] = strings.ToUpper(colType)
	}

	// Check that all defined columns exist with correct types
	for _, col := range tableDef.Columns {
		colName := strings.ToLower(col.Name)
		existingType, exists := existingColumns[colName]
		if !exists {
			return fmt.Errorf("table '%s' is missing column '%s'", tableDef.Name, col.Name)
		}
		expectedType := strings.ToUpper(col.Type)
		if existingType != expectedType {
			return fmt.Errorf("table '%s' column '%s' has type '%s' but expected '%s'", tableDef.Name, col.Name, existingType, expectedType)
		}
	}

	return nil
}

// createTable creates a new table based on the definition
func (r *Runtime) createTable(tableDef TableDef) error {
	var b strings.Builder
	b.WriteString("CREATE TABLE ")
	b.WriteString(tableDef.Name)
	b.WriteString(" (")
	for i, col := range tableDef.Columns {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(col.Name)
		b.WriteString(" ")
		b.WriteString(col.Type)
		if col.Constraints != "" {
			b.WriteString(" ")
			b.WriteString(col.Constraints)
		}
	}
	b.WriteString(")")

	_, err := r.db.Exec(b.String())
	if err != nil {
		return fmt.Errorf("failed to create table '%s': %w", tableDef.Name, err)
	}
	return nil
}

// getArgs returns the command line arguments as a string array
func (r *Runtime) getArgs() (int32, error) {
	argHandles := make([]int32, len(r.args))
	for i, arg := range r.args {
		argHandles[i] = r.newValue(Value{Kind: KindString, Str: arg})
	}
	return r.newValue(Value{Kind: KindArray, Arr: &Array{Elems: argHandles}}), nil
}

// sqlQuery executes a SQL query with parameters and returns the result
func (r *Runtime) sqlQuery(caller *wasmtime.Caller, ptr int32, length int32, paramsHandle int32) (int32, error) {
	if r.db == nil {
		return 0, errors.New("database not initialized")
	}

	ext := caller.GetExport("memory")
	if ext == nil {
		return 0, errors.New("memory not found")
	}
	memory := ext.Memory()
	if memory == nil {
		return 0, errors.New("memory not found")
	}
	data := memory.UnsafeData(caller)
	start := int(ptr)
	end := start + int(length)
	if start < 0 || end > len(data) {
		return 0, errors.New("sql string out of bounds")
	}
	query := string(data[start:end])

	// Extract parameters from the array handle
	var params []interface{}
	paramsVal, err := r.getValue(paramsHandle)
	if err != nil {
		return 0, fmt.Errorf("invalid params handle: %w", err)
	}
	if paramsVal.Kind == KindArray {
		for _, elemHandle := range paramsVal.Arr.Elems {
			elemVal, err := r.getValue(elemHandle)
			if err != nil {
				return 0, fmt.Errorf("invalid param element: %w", err)
			}
			switch elemVal.Kind {
			case KindString:
				params = append(params, elemVal.Str)
			case KindI64:
				params = append(params, elemVal.I64)
			case KindF64:
				params = append(params, elemVal.F64)
			case KindBool:
				if elemVal.Bool {
					params = append(params, 1)
				} else {
					params = append(params, 0)
				}
			default:
				return 0, errors.New("unsupported parameter type")
			}
		}
	}

	// Determine if it's a SELECT query or a modification query
	trimmed := strings.TrimSpace(strings.ToUpper(query))
	isSelect := strings.HasPrefix(trimmed, "SELECT")

	if isSelect {
		return r.execSelectQueryWithParams(query, params)
	}
	return r.execModifyQueryWithParams(query, params)
}

func (r *Runtime) execSelectQueryWithParams(query string, params []interface{}) (int32, error) {
	rows, err := r.db.Query(query, params...)
	if err != nil {
		return 0, fmt.Errorf("sql query error: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return 0, fmt.Errorf("sql columns error: %w", err)
	}

	// Read all rows as objects
	var rowHandles []int32
	values := make([]interface{}, len(cols))
	valuePtrs := make([]interface{}, len(cols))
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	for rows.Next() {
		if err := rows.Scan(valuePtrs...); err != nil {
			return 0, fmt.Errorf("sql scan error: %w", err)
		}
		// Create row object with column names as keys
		rowObj := r.newValue(Value{Kind: KindObject, Obj: &Object{Order: []string{}, Props: map[string]int32{}}})
		for i, v := range values {
			var str string
			if v == nil {
				str = ""
			} else {
				str = fmt.Sprintf("%v", v)
			}
			colName := strings.ToLower(cols[i])
			keyHandle := r.newValue(Value{Kind: KindString, Str: colName})
			valueHandle := r.newValue(Value{Kind: KindString, Str: str})
			if err := r.objSet(rowObj, keyHandle, valueHandle); err != nil {
				return 0, err
			}
		}
		rowHandles = append(rowHandles, rowObj)
	}

	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("sql rows error: %w", err)
	}

	// Return the rows array directly (no columns wrapper)
	rowsArr := r.newValue(Value{Kind: KindArray, Arr: &Array{Elems: rowHandles}})
	return rowsArr, nil
}

func (r *Runtime) execModifyQueryWithParams(query string, params []interface{}) (int32, error) {
	result, err := r.db.Exec(query, params...)
	if err != nil {
		return 0, fmt.Errorf("sql exec error: %w", err)
	}

	_ = result // rowsAffected not used in array return

	// Return empty array for non-SELECT queries
	rowsArr := r.newValue(Value{Kind: KindArray, Arr: &Array{Elems: []int32{}}})
	return rowsArr, nil
}

// sqlFetchOne executes a SQL query and returns exactly one row as an object
// If no row is found, it returns an error
func (r *Runtime) sqlFetchOne(caller *wasmtime.Caller, ptr int32, length int32, paramsHandle int32) (int32, error) {
	if r.db == nil {
		return 0, errors.New("database not initialized")
	}

	ext := caller.GetExport("memory")
	if ext == nil {
		return 0, errors.New("memory not found")
	}
	memory := ext.Memory()
	if memory == nil {
		return 0, errors.New("memory not found")
	}
	data := memory.UnsafeData(caller)
	start := int(ptr)
	end := start + int(length)
	if start < 0 || end > len(data) {
		return 0, errors.New("sql string out of bounds")
	}
	query := string(data[start:end])

	// Extract parameters from the array handle
	params, err := r.extractSQLParams(paramsHandle)
	if err != nil {
		return 0, err
	}

	rows, err := r.db.Query(query, params...)
	if err != nil {
		return 0, fmt.Errorf("sql query error: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return 0, fmt.Errorf("sql columns error: %w", err)
	}

	values := make([]interface{}, len(cols))
	valuePtrs := make([]interface{}, len(cols))
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	if !rows.Next() {
		return 0, errors.New("fetch_one: no row found")
	}

	if err := rows.Scan(valuePtrs...); err != nil {
		return 0, fmt.Errorf("sql scan error: %w", err)
	}

	// Create row object with column names as keys
	rowObj := r.newValue(Value{Kind: KindObject, Obj: &Object{Order: []string{}, Props: map[string]int32{}}})
	for i, v := range values {
		var str string
		if v == nil {
			str = ""
		} else {
			str = fmt.Sprintf("%v", v)
		}
		colName := strings.ToLower(cols[i])
		keyHandle := r.newValue(Value{Kind: KindString, Str: colName})
		valueHandle := r.newValue(Value{Kind: KindString, Str: str})
		if err := r.objSet(rowObj, keyHandle, valueHandle); err != nil {
			return 0, err
		}
	}

	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("sql rows error: %w", err)
	}

	return rowObj, nil
}

// sqlFetchOptional executes a SQL query and returns 0 or 1 row as an object
// If no row is found, it returns a null/empty object
func (r *Runtime) sqlFetchOptional(caller *wasmtime.Caller, ptr int32, length int32, paramsHandle int32) (int32, error) {
	if r.db == nil {
		return 0, errors.New("database not initialized")
	}

	ext := caller.GetExport("memory")
	if ext == nil {
		return 0, errors.New("memory not found")
	}
	memory := ext.Memory()
	if memory == nil {
		return 0, errors.New("memory not found")
	}
	data := memory.UnsafeData(caller)
	start := int(ptr)
	end := start + int(length)
	if start < 0 || end > len(data) {
		return 0, errors.New("sql string out of bounds")
	}
	query := string(data[start:end])

	// Extract parameters from the array handle
	params, err := r.extractSQLParams(paramsHandle)
	if err != nil {
		return 0, err
	}

	rows, err := r.db.Query(query, params...)
	if err != nil {
		return 0, fmt.Errorf("sql query error: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return 0, fmt.Errorf("sql columns error: %w", err)
	}

	values := make([]interface{}, len(cols))
	valuePtrs := make([]interface{}, len(cols))
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	// If no row found, return null (handle 0)
	if !rows.Next() {
		return 0, nil
	}

	if err := rows.Scan(valuePtrs...); err != nil {
		return 0, fmt.Errorf("sql scan error: %w", err)
	}

	// Create row object with column names as keys
	rowObj := r.newValue(Value{Kind: KindObject, Obj: &Object{Order: []string{}, Props: map[string]int32{}}})
	for i, v := range values {
		var str string
		if v == nil {
			str = ""
		} else {
			str = fmt.Sprintf("%v", v)
		}
		colName := strings.ToLower(cols[i])
		keyHandle := r.newValue(Value{Kind: KindString, Str: colName})
		valueHandle := r.newValue(Value{Kind: KindString, Str: str})
		if err := r.objSet(rowObj, keyHandle, valueHandle); err != nil {
			return 0, err
		}
	}

	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("sql rows error: %w", err)
	}

	return rowObj, nil
}

// sqlExecute executes a SQL query (INSERT, UPDATE, DELETE) without returning results
func (r *Runtime) sqlExecute(caller *wasmtime.Caller, ptr int32, length int32, paramsHandle int32) error {
	if r.db == nil {
		return errors.New("database not initialized")
	}

	ext := caller.GetExport("memory")
	if ext == nil {
		return errors.New("memory not found")
	}
	memory := ext.Memory()
	if memory == nil {
		return errors.New("memory not found")
	}
	data := memory.UnsafeData(caller)
	start := int(ptr)
	end := start + int(length)
	if start < 0 || end > len(data) {
		return errors.New("sql string out of bounds")
	}
	query := string(data[start:end])

	// Extract parameters from the array handle
	params, err := r.extractSQLParams(paramsHandle)
	if err != nil {
		return err
	}

	_, err = r.db.Exec(query, params...)
	if err != nil {
		return fmt.Errorf("sql exec error: %w", err)
	}

	return nil
}

// extractSQLParams extracts SQL parameters from an array handle
func (r *Runtime) extractSQLParams(paramsHandle int32) ([]interface{}, error) {
	var params []interface{}
	paramsVal, err := r.getValue(paramsHandle)
	if err != nil {
		return nil, fmt.Errorf("invalid params handle: %w", err)
	}
	if paramsVal.Kind == KindArray {
		for _, elemHandle := range paramsVal.Arr.Elems {
			elemVal, err := r.getValue(elemHandle)
			if err != nil {
				return nil, fmt.Errorf("invalid param element: %w", err)
			}
			switch elemVal.Kind {
			case KindString:
				params = append(params, elemVal.Str)
			case KindI64:
				params = append(params, elemVal.I64)
			case KindF64:
				params = append(params, elemVal.F64)
			case KindBool:
				if elemVal.Bool {
					params = append(params, 1)
				} else {
					params = append(params, 0)
				}
			default:
				return nil, errors.New("unsupported parameter type")
			}
		}
	}
	return params, nil
}

// HTTP Server methods

// httpCreateServer creates a new HTTP server instance
func (r *Runtime) httpCreateServer() (int32, error) {
	r.httpMu.Lock()
	defer r.httpMu.Unlock()

	server := &HTTPServer{
		mux:    http.NewServeMux(),
		routes: make(map[string]int32),
	}
	handle := r.newValue(Value{Kind: KindI64, I64: int64(len(r.httpServers))})
	r.httpServers[handle] = server
	return handle, nil
}

// httpAddRoute adds a route to the HTTP server
func (r *Runtime) httpAddRoute(caller *wasmtime.Caller, serverHandle int32, pathPtr int32, pathLen int32, handlerHandle int32) error {
	r.httpMu.Lock()
	defer r.httpMu.Unlock()

	server, ok := r.httpServers[serverHandle]
	if !ok {
		return errors.New("invalid server handle")
	}

	// Get path from memory
	ext := caller.GetExport("memory")
	if ext == nil {
		return errors.New("memory not found")
	}
	memory := ext.Memory()
	if memory == nil {
		return errors.New("memory not found")
	}
	data := memory.UnsafeData(caller)
	start := int(pathPtr)
	end := start + int(pathLen)
	if start < 0 || end > len(data) {
		return errors.New("path string out of bounds")
	}
	path := string(data[start:end])

	server.routes[path] = handlerHandle
	return nil
}

// httpListen prepares the HTTP server for listening (actual start happens after WASM execution)
//
// 注意: この関数は実際にサーバーを起動しない。サーバー情報をpendingServerに保存し、
// WASM実行完了後にStartPendingServer()で起動する。
// 詳細はpendingHTTPServer構造体のコメントを参照。
func (r *Runtime) httpListen(caller *wasmtime.Caller, serverHandle int32, portPtr int32, portLen int32) error {
	r.httpMu.Lock()
	server, ok := r.httpServers[serverHandle]
	r.httpMu.Unlock()

	if !ok {
		return errors.New("invalid server handle")
	}

	// Get port from memory
	ext := caller.GetExport("memory")
	if ext == nil {
		return errors.New("memory not found")
	}
	memory := ext.Memory()
	if memory == nil {
		return errors.New("memory not found")
	}
	data := memory.UnsafeData(caller)
	start := int(portPtr)
	end := start + int(portLen)
	if start < 0 || end > len(data) {
		return errors.New("port string out of bounds")
	}
	port := string(data[start:end])

	// Store pending server info - actual startup happens after WASM execution completes
	r.pendingServer = &pendingHTTPServer{
		server: server,
		port:   port,
	}

	return nil
}

// StartPendingServer starts the HTTP server if one was registered via http_listen
//
// この関数はWASM実行が完全に終了した後にrunner.goから呼び出される。
// これにより、HTTPハンドラー内でWASM関数を呼び出す際に、
// コールスタックがクリアな状態であることが保証される。
//
// 重要: WASM実行中にこの関数を呼び出すと、スタックオーバーフローが発生する可能性がある。
func (r *Runtime) StartPendingServer() error {
	if r.pendingServer == nil {
		return nil
	}

	server := r.pendingServer.server
	port := r.pendingServer.port

	// Set up handler for all routes
	server.mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		// Look up the handler for this path
		r.httpMu.Lock()
		handlerHandle, ok := server.routes[req.URL.Path]
		r.httpMu.Unlock()

		if !ok {
			http.NotFound(w, req)
			return
		}

		// Create request object
		reqObj := r.newValue(Value{Kind: KindObject, Obj: &Object{Order: []string{}, Props: map[string]int32{}}})
		pathHandle := r.newValue(Value{Kind: KindString, Str: req.URL.Path})
		methodHandle := r.newValue(Value{Kind: KindString, Str: req.Method})
		pathKeyHandle := r.newValue(Value{Kind: KindString, Str: "path"})
		methodKeyHandle := r.newValue(Value{Kind: KindString, Str: "method"})
		_ = r.objSet(reqObj, pathKeyHandle, pathHandle)
		_ = r.objSet(reqObj, methodKeyHandle, methodHandle)

		// Add query parameters as an object
		queryObj := r.newValue(Value{Kind: KindObject, Obj: &Object{Order: []string{}, Props: map[string]int32{}}})
		for key, values := range req.URL.Query() {
			if len(values) > 0 {
				keyHandle := r.newValue(Value{Kind: KindString, Str: key})
				valueHandle := r.newValue(Value{Kind: KindString, Str: values[0]})
				_ = r.objSet(queryObj, keyHandle, valueHandle)
			}
		}
		queryKeyHandle := r.newValue(Value{Kind: KindString, Str: "query"})
		_ = r.objSet(reqObj, queryKeyHandle, queryObj)

		// Add form data as an object (for POST requests)
		formObj := r.newValue(Value{Kind: KindObject, Obj: &Object{Order: []string{}, Props: map[string]int32{}}})
		if req.Method == "POST" {
			req.ParseForm()
			for key, values := range req.PostForm {
				if len(values) > 0 {
					keyHandle := r.newValue(Value{Kind: KindString, Str: key})
					valueHandle := r.newValue(Value{Kind: KindString, Str: values[0]})
					_ = r.objSet(formObj, keyHandle, valueHandle)
				}
			}
		}
		formKeyHandle := r.newValue(Value{Kind: KindString, Str: "form"})
		_ = r.objSet(reqObj, formKeyHandle, formObj)

		// Call the handler function
		if r.instance != nil && r.store != nil {
			handlerVal, err := r.getValue(handlerHandle)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error getting handler value: %v\n", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			if handlerVal.Kind != KindString {
				fmt.Fprintf(os.Stderr, "Handler is not a string, kind=%d\n", handlerVal.Kind)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			handlerName := handlerVal.Str
			handlerFunc := r.instance.GetFunc(r.store, handlerName)
			if handlerFunc == nil {
				fmt.Fprintf(os.Stderr, "Handler function not found: %s\n", handlerName)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			result, err := handlerFunc.Call(r.store, reqObj)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Handler call error: %v\n", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			if result == nil {
				fmt.Fprintf(os.Stderr, "Handler returned nil\n")
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			resHandle, ok := result.(int32)
			if !ok {
				fmt.Fprintf(os.Stderr, "Result is not int32: %T\n", result)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			resVal, err := r.getValue(resHandle)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error getting result value: %v\n", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			if resVal.Kind != KindObject {
				fmt.Fprintf(os.Stderr, "Result is not object, kind=%d\n", resVal.Kind)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			// Get body from response object
			bodyKey := r.newValue(Value{Kind: KindString, Str: "body"})
			bodyHandle, err := r.objGet(resHandle, bodyKey)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error getting body: %v\n", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			bodyVal, err := r.getValue(bodyHandle)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error getting body value: %v\n", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			if bodyVal.Kind != KindString {
				fmt.Fprintf(os.Stderr, "Body is not string, kind=%d\n", bodyVal.Kind)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			// Get contentType from response object (default to text/plain if not set)
			contentType := "text/plain; charset=utf-8"
			contentTypeKey := r.newValue(Value{Kind: KindString, Str: "contentType"})
			if ctHandle, err := r.objGet(resHandle, contentTypeKey); err == nil {
				if ctVal, err := r.getValue(ctHandle); err == nil && ctVal.Kind == KindString {
					contentType = ctVal.Str
				}
			}

			// Check if it's a redirect response
			if contentType == "redirect" {
				redirectUrlKey := r.newValue(Value{Kind: KindString, Str: "redirectUrl"})
				if urlHandle, err := r.objGet(resHandle, redirectUrlKey); err == nil {
					if urlVal, err := r.getValue(urlHandle); err == nil && urlVal.Kind == KindString {
						http.Redirect(w, req, urlVal.Str, http.StatusFound)
						return
					}
				}
				http.Error(w, "Invalid redirect response", http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", contentType)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(bodyVal.Str))
			return
		}
		http.Error(w, "Internal Server Error: no instance", http.StatusInternalServerError)
	})

	// Flush accumulated output to stdout before blocking on ListenAndServe
	fmt.Print(r.output.String())
	r.output.Reset()
	return http.ListenAndServe(port, server.mux)
}

// httpResponseText creates a text response (from raw memory)
func (r *Runtime) httpResponseText(caller *wasmtime.Caller, textPtr int32, textLen int32) (int32, error) {
	return r.httpResponse(caller, textPtr, textLen, "text/plain; charset=utf-8")
}

// httpResponseHtml creates an HTML response (from raw memory)
func (r *Runtime) httpResponseHtml(caller *wasmtime.Caller, htmlPtr int32, htmlLen int32) (int32, error) {
	return r.httpResponse(caller, htmlPtr, htmlLen, "text/html; charset=utf-8")
}

// httpResponseTextStr creates a text response from a string object handle
func (r *Runtime) httpResponseTextStr(strHandle int32) (int32, error) {
	return r.httpResponseStr(strHandle, "text/plain; charset=utf-8")
}

// httpResponseHtmlStr creates an HTML response from a string object handle
func (r *Runtime) httpResponseHtmlStr(strHandle int32) (int32, error) {
	return r.httpResponseStr(strHandle, "text/html; charset=utf-8")
}

// httpResponseStr creates a response with the specified content type from a string handle
func (r *Runtime) httpResponseStr(strHandle int32, contentType string) (int32, error) {
	strVal, err := r.getValue(strHandle)
	if err != nil {
		return 0, err
	}
	if strVal.Kind != KindString {
		return 0, errors.New("expected string")
	}
	text := strVal.Str

	// Create response object with body and contentType
	resObj := r.newValue(Value{Kind: KindObject, Obj: &Object{Order: []string{}, Props: map[string]int32{}}})
	bodyKey := r.newValue(Value{Kind: KindString, Str: "body"})
	bodyValue := r.newValue(Value{Kind: KindString, Str: text})
	_ = r.objSet(resObj, bodyKey, bodyValue)
	contentTypeKey := r.newValue(Value{Kind: KindString, Str: "contentType"})
	contentTypeValue := r.newValue(Value{Kind: KindString, Str: contentType})
	_ = r.objSet(resObj, contentTypeKey, contentTypeValue)

	return resObj, nil
}

// httpResponse creates a response with the specified content type
func (r *Runtime) httpResponse(caller *wasmtime.Caller, textPtr int32, textLen int32, contentType string) (int32, error) {
	// Get text from memory
	ext := caller.GetExport("memory")
	if ext == nil {
		return 0, errors.New("memory not found")
	}
	memory := ext.Memory()
	if memory == nil {
		return 0, errors.New("memory not found")
	}
	data := memory.UnsafeData(caller)
	start := int(textPtr)
	end := start + int(textLen)
	if start < 0 || end > len(data) {
		return 0, errors.New("text string out of bounds")
	}
	text := string(data[start:end])

	// Create response object with body and contentType
	resObj := r.newValue(Value{Kind: KindObject, Obj: &Object{Order: []string{}, Props: map[string]int32{}}})
	bodyKey := r.newValue(Value{Kind: KindString, Str: "body"})
	bodyValue := r.newValue(Value{Kind: KindString, Str: text})
	_ = r.objSet(resObj, bodyKey, bodyValue)
	contentTypeKey := r.newValue(Value{Kind: KindString, Str: "contentType"})
	contentTypeValue := r.newValue(Value{Kind: KindString, Str: contentType})
	_ = r.objSet(resObj, contentTypeKey, contentTypeValue)

	return resObj, nil
}

// httpGetPath gets the path from a request object
func (r *Runtime) httpGetPath(reqHandle int32) (int32, error) {
	pathKey := r.newValue(Value{Kind: KindString, Str: "path"})
	return r.objGet(reqHandle, pathKey)
}

// httpGetMethod gets the method from a request object
func (r *Runtime) httpGetMethod(reqHandle int32) (int32, error) {
	methodKey := r.newValue(Value{Kind: KindString, Str: "method"})
	return r.objGet(reqHandle, methodKey)
}

// httpResponseJson creates a JSON response from a data handle
func (r *Runtime) httpResponseJson(dataHandle int32) (int32, error) {
	val, err := r.getValue(dataHandle)
	if err != nil {
		return 0, err
	}

	// Convert to JSON
	jsonStr := r.valueToJSON(val)

	// Create response object
	resObj := r.newValue(Value{Kind: KindObject, Obj: &Object{Order: []string{}, Props: map[string]int32{}}})
	bodyKey := r.newValue(Value{Kind: KindString, Str: "body"})
	bodyValue := r.newValue(Value{Kind: KindString, Str: jsonStr})
	_ = r.objSet(resObj, bodyKey, bodyValue)
	contentTypeKey := r.newValue(Value{Kind: KindString, Str: "contentType"})
	contentTypeValue := r.newValue(Value{Kind: KindString, Str: "application/json"})
	_ = r.objSet(resObj, contentTypeKey, contentTypeValue)

	return resObj, nil
}

// valueToJSON converts a runtime value to JSON string
func (r *Runtime) valueToJSON(val *Value) string {
	switch val.Kind {
	case KindString:
		escaped := strings.ReplaceAll(val.Str, "\\", "\\\\")
		escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
		escaped = strings.ReplaceAll(escaped, "\n", "\\n")
		escaped = strings.ReplaceAll(escaped, "\r", "\\r")
		escaped = strings.ReplaceAll(escaped, "\t", "\\t")
		return "\"" + escaped + "\""
	case KindI64:
		return fmt.Sprintf("%d", val.I64)
	case KindF64:
		return fmt.Sprintf("%g", val.F64)
	case KindBool:
		if val.Bool {
			return "true"
		}
		return "false"
	case KindArray:
		var parts []string
		for _, elemHandle := range val.Arr.Elems {
			elemVal, err := r.getValue(elemHandle)
			if err == nil {
				parts = append(parts, r.valueToJSON(elemVal))
			}
		}
		return "[" + strings.Join(parts, ",") + "]"
	case KindObject:
		var parts []string
		for _, key := range val.Obj.Order {
			propHandle := val.Obj.Props[key]
			propVal, err := r.getValue(propHandle)
			if err == nil {
				escaped := strings.ReplaceAll(key, "\\", "\\\\")
				escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
				parts = append(parts, "\""+escaped+"\":"+r.valueToJSON(propVal))
			}
		}
		return "{" + strings.Join(parts, ",") + "}"
	default:
		return "null"
	}
}

// httpResponseRedirect creates a redirect response
func (r *Runtime) httpResponseRedirect(caller *wasmtime.Caller, urlPtr int32, urlLen int32) (int32, error) {
	// Get URL from memory
	ext := caller.GetExport("memory")
	if ext == nil {
		return 0, errors.New("memory not found")
	}
	memory := ext.Memory()
	if memory == nil {
		return 0, errors.New("memory not found")
	}
	data := memory.UnsafeData(caller)
	start := int(urlPtr)
	end := start + int(urlLen)
	if start < 0 || end > len(data) {
		return 0, errors.New("url string out of bounds")
	}
	url := string(data[start:end])

	return r.createRedirectResponse(url)
}

// httpResponseRedirectStr creates a redirect response from a string handle
func (r *Runtime) httpResponseRedirectStr(strHandle int32) (int32, error) {
	strVal, err := r.getValue(strHandle)
	if err != nil {
		return 0, err
	}
	if strVal.Kind != KindString {
		return 0, errors.New("expected string")
	}
	return r.createRedirectResponse(strVal.Str)
}

// createRedirectResponse creates a response object for redirects
func (r *Runtime) createRedirectResponse(url string) (int32, error) {
	resObj := r.newValue(Value{Kind: KindObject, Obj: &Object{Order: []string{}, Props: map[string]int32{}}})
	bodyKey := r.newValue(Value{Kind: KindString, Str: "body"})
	bodyValue := r.newValue(Value{Kind: KindString, Str: ""})
	_ = r.objSet(resObj, bodyKey, bodyValue)
	contentTypeKey := r.newValue(Value{Kind: KindString, Str: "contentType"})
	contentTypeValue := r.newValue(Value{Kind: KindString, Str: "redirect"})
	_ = r.objSet(resObj, contentTypeKey, contentTypeValue)
	redirectUrlKey := r.newValue(Value{Kind: KindString, Str: "redirectUrl"})
	redirectUrlValue := r.newValue(Value{Kind: KindString, Str: url})
	_ = r.objSet(resObj, redirectUrlKey, redirectUrlValue)

	return resObj, nil
}
