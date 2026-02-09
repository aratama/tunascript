//go:build cgo
// +build cgo

package runtime

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	goruntime "runtime"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/bytecodealliance/wasmtime-go/v41"
	_ "modernc.org/sqlite"
	"tuna/internal/compiler"
	"tuna/internal/formatter"
)

type Kind int

const (
	KindI64 Kind = iota
	KindF64
	KindBool
	KindString
	KindObject
	KindArray
	KindNull
	KindUndefined
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
	Props map[string]*Value
}

type Array struct {
	Elems []*Value
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

const (
	routeMethodAny = "*"

	gcRequestInterval    uint64 = 100
	gcHeapThresholdBytes uint64 = 64 << 20 // 64 MiB
	gcMaxInterval               = time.Minute
)

// HTTPServer represents an HTTP server instance
type HTTPServer struct {
	mux    *http.ServeMux
	routes map[string]map[string]*Value // method -> (path -> handler handle)
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
	RedirectURL string
}

type Runtime struct {
	output          bytes.Buffer
	htmlOutput      bytes.Buffer
	db              *sql.DB
	handlerMu       sync.Mutex
	currentTx       *sql.Tx
	args            []string
	tableDefs       []TableDef // Table definitions for validation
	httpServers     map[int64]*HTTPServer
	httpMu          sync.Mutex
	store           *wasmtime.Store
	instance        *wasmtime.Instance
	internedStrings map[uint64]*Value // 文字列リテラルのインターンキャッシュ
	// pendingServer is set when http_listen is called, actual server starts after WASM execution
	pendingServer *pendingHTTPServer
	gcReqCount    uint64
	gcLastHeap    uint64
	gcLastAt      time.Time
}

var (
	nullValue      = &Value{Kind: KindNull}
	undefinedValue = &Value{Kind: KindUndefined}
)

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
//  1. TunaScriptプログラムのmain関数がlisten()を呼び出す
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
	now := time.Now()
	r := &Runtime{
		httpServers:     make(map[int64]*HTTPServer),
		internedStrings: make(map[uint64]*Value),
		gcLastHeap:      currentHeapAlloc(),
		gcLastAt:        now,
	}
	return r
}

// SetWasmContext sets the store and instance for callback invocation
func (r *Runtime) SetWasmContext(store *wasmtime.Store, instance *wasmtime.Instance) {
	r.store = store
	r.instance = instance
}

func currentHeapAlloc() uint64 {
	var ms goruntime.MemStats
	goruntime.ReadMemStats(&ms)
	return ms.HeapAlloc
}

func (r *Runtime) maybeStoreGC(force bool) {
	if r.store == nil {
		return
	}

	now := time.Now()
	if r.gcLastAt.IsZero() {
		r.gcLastAt = now
		r.gcLastHeap = currentHeapAlloc()
	}
	if force {
		r.store.GC()
		r.gcReqCount = 0
		r.gcLastAt = now
		r.gcLastHeap = currentHeapAlloc()
		return
	}

	r.gcReqCount++
	heapNow := currentHeapAlloc()
	heapDelta := uint64(0)
	if heapNow >= r.gcLastHeap {
		heapDelta = heapNow - r.gcLastHeap
	} else {
		// Goランタイム側のGCで減った場合は新しい基準値に追従する。
		r.gcLastHeap = heapNow
	}

	if r.gcReqCount < gcRequestInterval && heapDelta < gcHeapThresholdBytes && now.Sub(r.gcLastAt) < gcMaxInterval {
		return
	}

	r.store.GC()
	r.gcReqCount = 0
	r.gcLastAt = now
	r.gcLastHeap = currentHeapAlloc()
}

func (r *Runtime) Output() string {
	return r.output.String()
}

func (r *Runtime) appendOutputChunk(chunk string) error {
	if chunk == "" {
		return nil
	}
	r.output.WriteString(chunk)
	return nil
}

func (r *Runtime) appendHTMLChunk(chunk string) error {
	if chunk == "" {
		return nil
	}
	r.htmlOutput.WriteString(chunk)
	return nil
}

func (r *Runtime) SetArgs(args []string) {
	r.args = args
}

func (r *Runtime) Define(linker *wasmtime.Linker, store *wasmtime.Store) error {
	defineRuntime := func(name string, fn interface{}) error {
		return linker.DefineFunc(store, "runtime", name, fn)
	}
	defineServer := func(name string, fn interface{}) error {
		return linker.DefineFunc(store, "server", name, fn)
	}
	defineHost := func(name string, fn interface{}) error {
		return linker.DefineFunc(store, "host", name, fn)
	}

	if err := defineRuntime("run_formatter", func(sourceHandle *Value) *Value {
		value, err := r.run_formatter(sourceHandle)
		return r.resultValue(value, err)
	}); err != nil {
		return err
	}
	if err := defineRuntime("run_sandbox", func(sourceHandle *Value) *Value {
		value, err := r.run_sandbox(sourceHandle)
		return r.resultValue(value, err)
	}); err != nil {
		return err
	}
	if err := defineServer("sql_exec", func(caller *wasmtime.Caller, ptr int32, length int32) *Value {
		return must(r.sqlExec(caller, ptr, length))
	}); err != nil {
		return err
	}
	if err := defineServer("register_tables", func(caller *wasmtime.Caller, ptr int32, length int32) {
		must0(r.registerTables(caller, ptr, length))
	}); err != nil {
		return err
	}
	if err := defineServer("sql_query", func(caller *wasmtime.Caller, ptr int32, length int32, paramsHandle *Value) *Value {
		value, err := r.sqlQuery(caller, ptr, length, paramsHandle)
		return r.resultValue(value, err)
	}); err != nil {
		return err
	}
	if err := defineServer("sql_fetch_one", func(caller *wasmtime.Caller, ptr int32, length int32, paramsHandle *Value) *Value {
		value, err := r.sqlFetchOne(caller, ptr, length, paramsHandle)
		return r.resultValue(value, err)
	}); err != nil {
		return err
	}
	if err := defineServer("sql_fetch_optional", func(caller *wasmtime.Caller, ptr int32, length int32, paramsHandle *Value) *Value {
		value, err := r.sqlFetchOptional(caller, ptr, length, paramsHandle)
		return r.resultValue(value, err)
	}); err != nil {
		return err
	}
	if err := defineServer("sql_execute", func(caller *wasmtime.Caller, ptr int32, length int32, paramsHandle *Value) *Value {
		return r.resultError(r.sqlExecute(caller, ptr, length, paramsHandle))
	}); err != nil {
		return err
	}
	if err := defineServer("get_args", func() *Value {
		return must(r.get_args())
	}); err != nil {
		return err
	}
	if err := defineServer("get_env", func(nameHandle *Value) *Value {
		return must(r.get_env(nameHandle))
	}); err != nil {
		return err
	}
	if err := defineServer("gc", func() {
		r.maybeStoreGC(true)
	}); err != nil {
		return err
	}

	// Host bridge functions (externref-based)
	if err := defineHost("val_from_i64", func(v int64) *Value {
		return must(r.valFromI64(v))
	}); err != nil {
		return err
	}
	if err := defineHost("val_from_f64", func(v float64) *Value {
		return must(r.valFromF64(v))
	}); err != nil {
		return err
	}
	if err := defineHost("val_from_bool", func(v int32) *Value {
		return must(r.valFromBool(v))
	}); err != nil {
		return err
	}
	if err := defineHost("val_null", func() *Value {
		return nullValue
	}); err != nil {
		return err
	}
	if err := defineHost("val_undefined", func() *Value {
		return undefinedValue
	}); err != nil {
		return err
	}
	if err := defineHost("val_to_i64", func(handle *Value) int64 {
		return must(r.valToI64(handle))
	}); err != nil {
		return err
	}
	if err := defineHost("val_to_f64", func(handle *Value) float64 {
		return must(r.valToF64(handle))
	}); err != nil {
		return err
	}
	if err := defineHost("val_to_bool", func(handle *Value) int32 {
		return must(r.valToBool(handle))
	}); err != nil {
		return err
	}
	if err := defineHost("val_kind", func(handle *Value) int32 {
		return must(r.valKind(handle))
	}); err != nil {
		return err
	}
	if err := defineHost("str_from_utf8", func(caller *wasmtime.Caller, ptr int32, length int32) *Value {
		return must(r.strFromUTF8(caller, ptr, length))
	}); err != nil {
		return err
	}
	if err := defineHost("str_byte_len", func(handle *Value) int32 {
		return must(r.strByteLen(handle))
	}); err != nil {
		return err
	}
	if err := defineHost("str_copy", func(caller *wasmtime.Caller, handle *Value, ptr int32, length int32) {
		must0(r.strCopy(caller, handle, ptr, length))
	}); err != nil {
		return err
	}
	if err := defineHost("arr_new", func(count int32) *Value {
		return must(r.arrNew(count))
	}); err != nil {
		return err
	}
	if err := defineHost("arr_len", func(arrHandle *Value) int32 {
		return must(r.arrLen(arrHandle))
	}); err != nil {
		return err
	}
	if err := defineHost("arr_get", func(arrHandle *Value, index int32) *Value {
		return must(r.arrGet(arrHandle, index))
	}); err != nil {
		return err
	}
	if err := defineHost("arr_set", func(arrHandle *Value, index int32, valHandle *Value) {
		must0(r.arrSet(arrHandle, index, valHandle))
	}); err != nil {
		return err
	}
	if err := defineHost("obj_new", func(count int32) *Value {
		return must(r.objNew(count))
	}); err != nil {
		return err
	}
	if err := defineHost("obj_get", func(objHandle *Value, keyHandle *Value) *Value {
		return must(r.objGet(objHandle, keyHandle))
	}); err != nil {
		return err
	}
	if err := defineHost("obj_set", func(objHandle *Value, keyHandle *Value, valHandle *Value) {
		must0(r.objSet(objHandle, keyHandle, valHandle))
	}); err != nil {
		return err
	}
	if err := defineHost("obj_keys", func(objHandle *Value) *Value {
		return must(r.objKeys(objHandle))
	}); err != nil {
		return err
	}
	if err := defineHost("sqlite_db_open", func(strHandle *Value) *Value {
		return r.resultError(r.dbOpenHandle(strHandle))
	}); err != nil {
		return err
	}
	if err := defineHost("file_read_text", func(pathHandle *Value) *Value {
		value, err := r.fileReadText(pathHandle)
		return r.resultValue(value, err)
	}); err != nil {
		return err
	}
	if err := defineHost("file_write_text", func(pathHandle *Value, contentHandle *Value) *Value {
		return r.resultError(r.fileWriteText(pathHandle, contentHandle))
	}); err != nil {
		return err
	}
	if err := defineHost("file_append_text", func(pathHandle *Value, contentHandle *Value) *Value {
		return r.resultError(r.fileAppendText(pathHandle, contentHandle))
	}); err != nil {
		return err
	}
	if err := defineHost("file_read_dir", func(pathHandle *Value) *Value {
		value, err := r.fileReadDir(pathHandle)
		return r.resultValue(value, err)
	}); err != nil {
		return err
	}
	if err := defineHost("file_exists", func(pathHandle *Value) int32 {
		return r.fileExists(pathHandle)
	}); err != nil {
		return err
	}
	if err := defineHost("http_create_server", func() *Value {
		return must(r.httpCreateServer())
	}); err != nil {
		return err
	}
	if err := defineHost("http_add_route", func(caller *wasmtime.Caller, serverHandle *Value, methodHandle *Value, pathPtr int32, pathLen int32, handlerHandle *Value) {
		must0(r.httpAddRoute(caller, serverHandle, methodHandle, pathPtr, pathLen, handlerHandle))
	}); err != nil {
		return err
	}
	if err := defineHost("http_listen", func(caller *wasmtime.Caller, serverHandle *Value, portHandle *Value) {
		must0(r.httpListen(caller, serverHandle, portHandle))
	}); err != nil {
		return err
	}
	if err := defineHost("http_response_text", func(caller *wasmtime.Caller, textPtr int32, textLen int32) *Value {
		return must(r.httpResponseText(caller, textPtr, textLen))
	}); err != nil {
		return err
	}
	if err := defineHost("http_response_html", func(caller *wasmtime.Caller, htmlPtr int32, htmlLen int32) *Value {
		return must(r.httpResponseHtml(caller, htmlPtr, htmlLen))
	}); err != nil {
		return err
	}
	if err := defineHost("http_response_text_str", func(strHandle *Value) *Value {
		return must(r.httpResponseTextStr(strHandle))
	}); err != nil {
		return err
	}
	if err := defineHost("http_response_html_str", func(strHandle *Value) *Value {
		return must(r.httpResponseHtmlStr(strHandle))
	}); err != nil {
		return err
	}
	if err := defineHost("http_response_json", func(dataHandle *Value) *Value {
		return must(r.httpResponseJson(dataHandle))
	}); err != nil {
		return err
	}
	if err := defineHost("http_response_redirect", func(caller *wasmtime.Caller, urlPtr int32, urlLen int32) *Value {
		return must(r.httpResponseRedirect(caller, urlPtr, urlLen))
	}); err != nil {
		return err
	}
	if err := defineHost("http_response_redirect_str", func(strHandle *Value) *Value {
		return must(r.httpResponseRedirectStr(strHandle))
	}); err != nil {
		return err
	}
	if err := defineHost("http_get_path", func(reqHandle *Value) *Value {
		return must(r.httpGetPath(reqHandle))
	}); err != nil {
		return err
	}
	if err := defineHost("http_get_method", func(reqHandle *Value) *Value {
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

func (r *Runtime) resultValue(value *Value, err error) *Value {
	if err != nil {
		return r.decodeError(err.Error())
	}
	return value
}

func (r *Runtime) resultError(err error) *Value {
	if err != nil {
		return r.decodeError(err.Error())
	}
	// undefined を (undefined | error) の成功値として返す
	return undefinedValue
}

func (r *Runtime) newValue(v Value) *Value {
	switch v.Kind {
	case KindNull:
		return nullValue
	case KindUndefined:
		return undefinedValue
	default:
		vv := v
		return &vv
	}
}

func (r *Runtime) getValue(handle *Value) (*Value, error) {
	if handle == nil {
		return nullValue, nil
	}
	return handle, nil
}

func (r *Runtime) strFromUTF8(caller *wasmtime.Caller, ptr int32, length int32) (*Value, error) {
	ext := caller.GetExport("memory")
	if ext == nil {
		return nil, errors.New("memory not found")
	}
	memory := ext.Memory()
	if memory == nil {
		return nil, errors.New("memory not found")
	}
	data := memory.UnsafeData(caller)
	start := int(ptr)
	end := start + int(length)
	if start < 0 || end > len(data) {
		return nil, errors.New("string out of bounds")
	}
	return r.newValue(Value{Kind: KindString, Str: string(data[start:end])}), nil
}

func (r *Runtime) get_env(nameHandle *Value) (*Value, error) {
	valueHandle, err := r.getValue(nameHandle)
	if err != nil {
		return nil, err
	}
	if valueHandle.Kind != KindString {
		return nil, errors.New("get_env expects string")
	}
	value := os.Getenv(valueHandle.Str)
	return r.newValue(Value{Kind: KindString, Str: value}), nil
}

func (r *Runtime) fileReadText(pathHandle *Value) (*Value, error) {
	pathValue, err := r.getValue(pathHandle)
	if err != nil {
		return nil, err
	}
	if pathValue.Kind != KindString {
		return nil, errors.New("read_text expects string")
	}
	data, err := os.ReadFile(pathValue.Str)
	if err != nil {
		return nil, err
	}
	if !utf8.Valid(data) {
		return nil, errors.New("read_text expects UTF-8 text")
	}
	text := string(data)
	text = strings.TrimPrefix(text, "\uFEFF")
	return r.newValue(Value{Kind: KindString, Str: text}), nil
}

func (r *Runtime) fileWriteText(pathHandle *Value, contentHandle *Value) error {
	pathValue, err := r.getValue(pathHandle)
	if err != nil {
		return err
	}
	if pathValue.Kind != KindString {
		return errors.New("write_text expects string path")
	}
	contentValue, err := r.getValue(contentHandle)
	if err != nil {
		return err
	}
	if contentValue.Kind != KindString {
		return errors.New("write_text expects string content")
	}
	if !utf8.ValidString(contentValue.Str) {
		return errors.New("write_text expects UTF-8 text")
	}
	return os.WriteFile(pathValue.Str, []byte(contentValue.Str), 0644)
}

func (r *Runtime) fileAppendText(pathHandle *Value, contentHandle *Value) error {
	pathValue, err := r.getValue(pathHandle)
	if err != nil {
		return err
	}
	if pathValue.Kind != KindString {
		return errors.New("append_text expects string path")
	}
	contentValue, err := r.getValue(contentHandle)
	if err != nil {
		return err
	}
	if contentValue.Kind != KindString {
		return errors.New("append_text expects string content")
	}
	if !utf8.ValidString(contentValue.Str) {
		return errors.New("append_text expects UTF-8 text")
	}
	f, err := os.OpenFile(pathValue.Str, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(contentValue.Str)
	return err
}

func (r *Runtime) fileReadDir(pathHandle *Value) (*Value, error) {
	pathValue, err := r.getValue(pathHandle)
	if err != nil {
		return nil, err
	}
	if pathValue.Kind != KindString {
		return nil, errors.New("read_dir expects string")
	}
	entries, err := os.ReadDir(pathValue.Str)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	elems := make([]*Value, len(names))
	for i, name := range names {
		elems[i] = r.newValue(Value{Kind: KindString, Str: name})
	}
	return r.newValue(Value{Kind: KindArray, Arr: &Array{Elems: elems}}), nil
}

func (r *Runtime) fileExists(pathHandle *Value) int32 {
	pathValue, err := r.getValue(pathHandle)
	if err != nil || pathValue.Kind != KindString {
		return 0
	}
	if _, err := os.Stat(pathValue.Str); err == nil {
		return 1
	}
	return 0
}

func (r *Runtime) run_formatter(sourceHandle *Value) (*Value, error) {
	sourceValue, err := r.getValue(sourceHandle)
	if err != nil {
		return nil, err
	}
	if sourceValue.Kind != KindString {
		return nil, errors.New("run_formatter expects string")
	}
	formatted, err := formatter.New().Format("<runtime>", sourceValue.Str)
	if err != nil {
		return nil, err
	}
	return r.newValue(Value{Kind: KindString, Str: formatted}), nil
}

func (r *Runtime) sandboxResultValue(stdout string, htmlOut string) *Value {
	props := map[string]*Value{
		"stdout": r.newValue(Value{Kind: KindString, Str: stdout}),
		"html":   r.newValue(Value{Kind: KindString, Str: htmlOut}),
	}
	return r.newValue(Value{
		Kind: KindObject,
		Obj: &Object{
			Order: []string{"stdout", "html"},
			Props: props,
		},
	})
}

func (r *Runtime) run_sandbox(sourceHandle *Value) (*Value, error) {
	sourceValue, err := r.getValue(sourceHandle)
	if err != nil {
		return nil, err
	}
	if sourceValue.Kind != KindString {
		return nil, errors.New("run_sandbox expects string")
	}

	dir, err := os.MkdirTemp("", "tunascript-sandbox-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	entry := filepath.Join(dir, "main.tuna")
	if err := os.WriteFile(entry, []byte(sourceValue.Str), 0644); err != nil {
		return nil, err
	}

	comp := compiler.New()
	if err := comp.SetBackend(compiler.BackendGC); err != nil {
		return nil, err
	}
	res, err := comp.Compile(entry)
	if err != nil {
		return nil, err
	}

	runner := NewRunner()
	rt, err := runner.runWithArgs(res.Wasm, nil)
	if err != nil {
		return nil, err
	}
	return r.sandboxResultValue(rt.Output(), rt.htmlOutput.String()), nil
}

// internString は文字列リテラル（offset, length）をヒープハンドルに変換します。
// 同じリテラルは同じハンドルを返します（インターン）。
func (r *Runtime) internString(caller *wasmtime.Caller, ptr int32, length int32) (*Value, error) {
	// キャッシュをチェック
	key := uint64(ptr)<<32 | uint64(uint32(length))
	if handle, ok := r.internedStrings[key]; ok {
		return handle, nil
	}

	// メモリから文字列を読み取り
	ext := caller.GetExport("memory")
	if ext == nil {
		return nil, errors.New("memory not found")
	}
	memory := ext.Memory()
	if memory == nil {
		return nil, errors.New("memory not found")
	}
	data := memory.UnsafeData(caller)
	start := int(ptr)
	end := start + int(length)
	if start < 0 || end > len(data) {
		return nil, errors.New("string out of bounds")
	}
	str := string(data[start:end])

	// ヒープに登録
	handle := r.newValue(Value{Kind: KindString, Str: str})

	// キャッシュに保存
	r.internedStrings[key] = handle
	return handle, nil
}

func (r *Runtime) valFromI64(v int64) (*Value, error) {
	return r.newValue(Value{Kind: KindI64, I64: v}), nil
}

func (r *Runtime) valFromF64(v float64) (*Value, error) {
	return r.newValue(Value{Kind: KindF64, F64: v}), nil
}

func (r *Runtime) valFromBool(v int32) (*Value, error) {
	return r.newValue(Value{Kind: KindBool, Bool: v != 0}), nil
}

func (r *Runtime) valToI64(handle *Value) (int64, error) {
	v, err := r.getValue(handle)
	if err != nil {
		return 0, err
	}
	if v.Kind != KindI64 {
		return 0, errors.New("not integer")
	}
	return v.I64, nil
}

func (r *Runtime) valToF64(handle *Value) (float64, error) {
	v, err := r.getValue(handle)
	if err != nil {
		return 0, err
	}
	if v.Kind != KindF64 {
		return 0, errors.New("not number")
	}
	return v.F64, nil
}

func (r *Runtime) valToBool(handle *Value) (int32, error) {
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

func (r *Runtime) valKind(handle *Value) (int32, error) {
	v, err := r.getValue(handle)
	if err != nil {
		return 0, err
	}
	return int32(v.Kind), nil
}

func (r *Runtime) objNew(count int32) (*Value, error) {
	return r.newValue(Value{Kind: KindObject, Obj: &Object{Order: []string{}, Props: map[string]*Value{}}}), nil
}

func (r *Runtime) objSet(objHandle *Value, keyHandle *Value, valHandle *Value) error {
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

func (r *Runtime) objGet(objHandle *Value, keyHandle *Value) (*Value, error) {
	objVal, err := r.getValue(objHandle)
	if err != nil {
		return nil, err
	}
	keyVal, err := r.getValue(keyHandle)
	if err != nil {
		return nil, err
	}
	if objVal.Kind != KindObject || keyVal.Kind != KindString {
		return nil, errors.New("obj_get type error")
	}
	key := keyVal.Str
	val, ok := objVal.Obj.Props[key]
	if !ok {
		// Return empty string for missing keys (useful for optional query params, form fields, etc.)
		return r.newValue(Value{Kind: KindString, Str: ""}), nil
	}
	return val, nil
}

func (r *Runtime) objKeys(objHandle *Value) (*Value, error) {
	objVal, err := r.getValue(objHandle)
	if err != nil {
		return nil, err
	}
	if objVal.Kind != KindObject {
		return nil, errors.New("obj_keys expects object")
	}
	keys := make([]*Value, len(objVal.Obj.Order))
	for i, key := range objVal.Obj.Order {
		keys[i] = r.newValue(Value{Kind: KindString, Str: key})
	}
	return r.newValue(Value{Kind: KindArray, Arr: &Array{Elems: keys}}), nil
}

func (r *Runtime) arrNew(count int32) (*Value, error) {
	arr := make([]*Value, int(count))
	for i := range arr {
		arr[i] = nullValue
	}
	return r.newValue(Value{Kind: KindArray, Arr: &Array{Elems: arr}}), nil
}

func (r *Runtime) arrSet(arrHandle *Value, index int32, valHandle *Value) error {
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

func (r *Runtime) arrGet(arrHandle *Value, index int32) (*Value, error) {
	arrVal, err := r.getValue(arrHandle)
	if err != nil {
		return nil, err
	}
	if arrVal.Kind != KindArray {
		return nil, errors.New("arr_get type error")
	}
	if index < 0 || int(index) >= len(arrVal.Arr.Elems) {
		return nil, errors.New("index out of range")
	}
	return arrVal.Arr.Elems[index], nil
}

func (r *Runtime) arrLen(arrHandle *Value) (int32, error) {
	arrVal, err := r.getValue(arrHandle)
	if err != nil {
		return 0, err
	}
	if arrVal.Kind != KindArray {
		return 0, errors.New("arr_len type error")
	}
	return int32(len(arrVal.Arr.Elems)), nil
}

func (r *Runtime) strByteLen(handle *Value) (int32, error) {
	v, err := r.getValue(handle)
	if err != nil {
		return 0, err
	}
	if v.Kind != KindString {
		return 0, errors.New("str_byte_len expects string")
	}
	return int32(len(v.Str)), nil
}

func (r *Runtime) strCopy(caller *wasmtime.Caller, handle *Value, ptr int32, length int32) error {
	v, err := r.getValue(handle)
	if err != nil {
		return err
	}
	if v.Kind != KindString {
		return errors.New("str_copy expects string")
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
		return errors.New("string out of bounds")
	}
	if len(v.Str) != int(length) {
		return errors.New("str_copy length mismatch")
	}
	copy(data[start:end], []byte(v.Str))
	return nil
}

func (r *Runtime) decodeError(message string) *Value {
	props := map[string]*Value{
		"message":    r.newValue(Value{Kind: KindString, Str: message}),
		"stacktrace": r.newValue(Value{Kind: KindArray, Arr: &Array{Elems: []*Value{}}}),
		"type":       r.newValue(Value{Kind: KindString, Str: "error"}),
	}
	return r.newValue(Value{Kind: KindObject, Obj: &Object{Order: sortedKeys(props), Props: props}})
}

// resultErrorMessage returns (message, true, nil) when handle is an error object.
func (r *Runtime) resultErrorMessage(handle *Value) (string, bool, error) {
	v, err := r.getValue(handle)
	if err != nil {
		return "", false, err
	}
	if v.Kind != KindObject || v.Obj == nil {
		return "", false, nil
	}
	typeHandle, ok := v.Obj.Props["type"]
	if !ok {
		return "", false, nil
	}
	typeVal, err := r.getValue(typeHandle)
	if err != nil {
		return "", false, err
	}
	if typeVal.Kind != KindString || typeVal.Str != "error" {
		return "", false, nil
	}
	msg := "error"
	if msgHandle, ok := v.Obj.Props["message"]; ok {
		msgVal, err := r.getValue(msgHandle)
		if err != nil {
			return "", false, err
		}
		if msgVal.Kind == KindString {
			msg = msgVal.Str
		}
	}
	return msg, true, nil
}

func sortedKeys(m map[string]*Value) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// sqlExec executes a SQL query and returns the result as an object with columns and rows
func (r *Runtime) sqlExec(caller *wasmtime.Caller, ptr int32, length int32) (*Value, error) {
	if r.db == nil {
		return nil, errors.New("database not initialized")
	}

	ext := caller.GetExport("memory")
	if ext == nil {
		return nil, errors.New("memory not found")
	}
	memory := ext.Memory()
	if memory == nil {
		return nil, errors.New("memory not found")
	}
	data := memory.UnsafeData(caller)
	start := int(ptr)
	end := start + int(length)
	if start < 0 || end > len(data) {
		return nil, errors.New("sql string out of bounds")
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

func (r *Runtime) execSelectQuery(query string) (*Value, error) {
	rows, err := r.dbQuery(query)
	if err != nil {
		return nil, fmt.Errorf("sql query error: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("sql columns error: %w", err)
	}

	// Create columns array
	colHandles := make([]*Value, len(cols))
	for i, col := range cols {
		colHandles[i] = r.newValue(Value{Kind: KindString, Str: col})
	}
	columnsArr := r.newValue(Value{Kind: KindArray, Arr: &Array{Elems: colHandles}})

	// Read all rows as objects
	var rowHandles []*Value
	values := make([]interface{}, len(cols))
	valuePtrs := make([]interface{}, len(cols))
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	for rows.Next() {
		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("sql scan error: %w", err)
		}
		// Create row object with column names as keys
		rowObj := r.newValue(Value{Kind: KindObject, Obj: &Object{Order: []string{}, Props: map[string]*Value{}}})
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
				return nil, err
			}
		}
		rowHandles = append(rowHandles, rowObj)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sql rows error: %w", err)
	}

	rowsArr := r.newValue(Value{Kind: KindArray, Arr: &Array{Elems: rowHandles}})

	// Create result object { "columns": [...], "rows": [...] }
	columnsKey := r.newValue(Value{Kind: KindString, Str: "columns"})
	rowsKey := r.newValue(Value{Kind: KindString, Str: "rows"})

	objHandle := r.newValue(Value{Kind: KindObject, Obj: &Object{Order: []string{}, Props: map[string]*Value{}}})
	if err := r.objSet(objHandle, columnsKey, columnsArr); err != nil {
		return nil, err
	}
	if err := r.objSet(objHandle, rowsKey, rowsArr); err != nil {
		return nil, err
	}

	return objHandle, nil
}

func (r *Runtime) execModifyQuery(query string) (*Value, error) {
	result, err := r.dbExec(query)
	if err != nil {
		return nil, fmt.Errorf("sql exec error: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()

	// Return object with columns: [] and rows: [] for non-SELECT queries
	// Include rowsAffected info as well
	columnsArr := r.newValue(Value{Kind: KindArray, Arr: &Array{Elems: []*Value{}}})

	// For INSERT/UPDATE/DELETE, return empty rows but we can include metadata
	rowsArr := r.newValue(Value{Kind: KindArray, Arr: &Array{Elems: []*Value{}}})

	columnsKey := r.newValue(Value{Kind: KindString, Str: "columns"})
	rowsKey := r.newValue(Value{Kind: KindString, Str: "rows"})
	affectedKey := r.newValue(Value{Kind: KindString, Str: "rowsAffected"})

	objHandle := r.newValue(Value{Kind: KindObject, Obj: &Object{Order: []string{}, Props: map[string]*Value{}}})
	if err := r.objSet(objHandle, columnsKey, columnsArr); err != nil {
		return nil, err
	}
	if err := r.objSet(objHandle, rowsKey, rowsArr); err != nil {
		return nil, err
	}
	affectedHandle, _ := r.valFromI64(rowsAffected)
	if err := r.objSet(objHandle, affectedKey, affectedHandle); err != nil {
		return nil, err
	}

	return objHandle, nil
}

// dbOpenHandle opens a database file using a string handle from the heap
func (r *Runtime) dbOpenHandle(strHandle *Value) error {
	val, err := r.getValue(strHandle)
	if err != nil {
		return err
	}
	if val.Kind != KindString {
		return errors.New("dbOpenHandle expects a string")
	}
	filename := val.Str

	return r.openDB(filename)
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
	if r.db != nil {
		if err := r.initAndValidateTables(); err != nil {
			return err
		}
	}
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

func (r *Runtime) openDB(filename string) error {
	if r.currentTx != nil {
		r.currentTx.Rollback()
		r.currentTx = nil
	}
	if r.db != nil {
		r.db.Close()
		r.db = nil
	}

	db, err := sql.Open("sqlite", filename)
	if err != nil {
		return fmt.Errorf("db open error: %w", err)
	}
	r.db = db

	if err := r.initAndValidateTables(); err != nil {
		r.db.Close()
		r.db = nil
		return err
	}

	return nil
}

func (r *Runtime) ensureDefaultDB() error {
	if r.db != nil {
		return nil
	}
	return r.openDB(":memory:")
}

// tableExists checks if a table exists in the database
func (r *Runtime) tableExists(tableName string) (bool, error) {
	var count int
	exec := r.currentExecutor()
	if exec == nil {
		return false, errors.New("database not initialized")
	}
	err := exec.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", tableName).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check table existence: %w", err)
	}
	return count > 0, nil
}

// validateTableStructure validates that an existing table matches the definition
func (r *Runtime) validateTableStructure(tableDef TableDef) error {
	rows, err := r.dbQuery(fmt.Sprintf("PRAGMA table_info(%s)", tableDef.Name))
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

	_, err := r.dbExec(b.String())
	if err != nil {
		return fmt.Errorf("failed to create table '%s': %w", tableDef.Name, err)
	}
	return nil
}

// get_args returns the command line arguments as a string array
func (r *Runtime) get_args() (*Value, error) {
	argHandles := make([]*Value, len(r.args))
	for i, arg := range r.args {
		argHandles[i] = r.newValue(Value{Kind: KindString, Str: arg})
	}
	return r.newValue(Value{Kind: KindArray, Arr: &Array{Elems: argHandles}}), nil
}

// sqlQuery executes a SQL query with parameters and returns the result
func (r *Runtime) sqlQuery(caller *wasmtime.Caller, ptr int32, length int32, paramsHandle *Value) (*Value, error) {
	if r.db == nil {
		return nil, errors.New("database not initialized")
	}

	ext := caller.GetExport("memory")
	if ext == nil {
		return nil, errors.New("memory not found")
	}
	memory := ext.Memory()
	if memory == nil {
		return nil, errors.New("memory not found")
	}
	data := memory.UnsafeData(caller)
	start := int(ptr)
	end := start + int(length)
	if start < 0 || end > len(data) {
		return nil, errors.New("sql string out of bounds")
	}
	query := string(data[start:end])

	// Extract parameters from the array handle
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

	// Determine if it's a SELECT query or a modification query
	trimmed := strings.TrimSpace(strings.ToUpper(query))
	isSelect := strings.HasPrefix(trimmed, "SELECT")

	if isSelect {
		return r.execSelectQueryWithParams(query, params)
	}
	return r.execModifyQueryWithParams(query, params)
}

func (r *Runtime) execSelectQueryWithParams(query string, params []interface{}) (*Value, error) {
	rows, err := r.dbQuery(query, params...)
	if err != nil {
		return nil, fmt.Errorf("sql query error: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("sql columns error: %w", err)
	}

	// Read all rows as objects
	var rowHandles []*Value
	values := make([]interface{}, len(cols))
	valuePtrs := make([]interface{}, len(cols))
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	for rows.Next() {
		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("sql scan error: %w", err)
		}
		// Create row object with column names as keys
		rowObj := r.newValue(Value{Kind: KindObject, Obj: &Object{Order: []string{}, Props: map[string]*Value{}}})
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
				return nil, err
			}
		}
		rowHandles = append(rowHandles, rowObj)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sql rows error: %w", err)
	}

	// Return the rows array directly (no columns wrapper)
	rowsArr := r.newValue(Value{Kind: KindArray, Arr: &Array{Elems: rowHandles}})
	return rowsArr, nil
}

func (r *Runtime) execModifyQueryWithParams(query string, params []interface{}) (*Value, error) {
	result, err := r.dbExec(query, params...)
	if err != nil {
		return nil, fmt.Errorf("sql exec error: %w", err)
	}

	_ = result // rowsAffected not used in array return

	// Return empty array for non-SELECT queries
	rowsArr := r.newValue(Value{Kind: KindArray, Arr: &Array{Elems: []*Value{}}})
	return rowsArr, nil
}

// sqlFetchOne executes a SQL query and returns exactly one row as an object
// If no row is found, it returns an error
func (r *Runtime) sqlFetchOne(caller *wasmtime.Caller, ptr int32, length int32, paramsHandle *Value) (*Value, error) {
	if r.db == nil {
		return nil, errors.New("database not initialized")
	}

	ext := caller.GetExport("memory")
	if ext == nil {
		return nil, errors.New("memory not found")
	}
	memory := ext.Memory()
	if memory == nil {
		return nil, errors.New("memory not found")
	}
	data := memory.UnsafeData(caller)
	start := int(ptr)
	end := start + int(length)
	if start < 0 || end > len(data) {
		return nil, errors.New("sql string out of bounds")
	}
	query := string(data[start:end])

	// Extract parameters from the array handle
	params, err := r.extractSQLParams(paramsHandle)
	if err != nil {
		return nil, err
	}

	rows, err := r.dbQuery(query, params...)
	if err != nil {
		return nil, fmt.Errorf("sql query error: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("sql columns error: %w", err)
	}

	values := make([]interface{}, len(cols))
	valuePtrs := make([]interface{}, len(cols))
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	if !rows.Next() {
		return nil, errors.New("fetch_one: no row found")
	}

	if err := rows.Scan(valuePtrs...); err != nil {
		return nil, fmt.Errorf("sql scan error: %w", err)
	}

	// Create row object with column names as keys
	rowObj := r.newValue(Value{Kind: KindObject, Obj: &Object{Order: []string{}, Props: map[string]*Value{}}})
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
			return nil, err
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sql rows error: %w", err)
	}

	return rowObj, nil
}

// sqlFetchOptional executes a SQL query and returns 0 or 1 row as an object
// If no row is found, it returns a null/empty object
func (r *Runtime) sqlFetchOptional(caller *wasmtime.Caller, ptr int32, length int32, paramsHandle *Value) (*Value, error) {
	if r.db == nil {
		return nil, errors.New("database not initialized")
	}

	ext := caller.GetExport("memory")
	if ext == nil {
		return nil, errors.New("memory not found")
	}
	memory := ext.Memory()
	if memory == nil {
		return nil, errors.New("memory not found")
	}
	data := memory.UnsafeData(caller)
	start := int(ptr)
	end := start + int(length)
	if start < 0 || end > len(data) {
		return nil, errors.New("sql string out of bounds")
	}
	query := string(data[start:end])

	// Extract parameters from the array handle
	params, err := r.extractSQLParams(paramsHandle)
	if err != nil {
		return nil, err
	}

	rows, err := r.dbQuery(query, params...)
	if err != nil {
		return nil, fmt.Errorf("sql query error: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("sql columns error: %w", err)
	}

	values := make([]interface{}, len(cols))
	valuePtrs := make([]interface{}, len(cols))
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	// If no row found, return null (handle 0)
	if !rows.Next() {
		return nil, nil
	}

	if err := rows.Scan(valuePtrs...); err != nil {
		return nil, fmt.Errorf("sql scan error: %w", err)
	}

	// Create row object with column names as keys
	rowObj := r.newValue(Value{Kind: KindObject, Obj: &Object{Order: []string{}, Props: map[string]*Value{}}})
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
			return nil, err
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sql rows error: %w", err)
	}

	return rowObj, nil
}

// sqlExecute executes a SQL query (INSERT, UPDATE, DELETE) without returning results
func (r *Runtime) sqlExecute(caller *wasmtime.Caller, ptr int32, length int32, paramsHandle *Value) error {
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

	_, err = r.dbExec(query, params...)
	if err != nil {
		return fmt.Errorf("sql exec error: %w", err)
	}

	return nil
}

// extractSQLParams extracts SQL parameters from an array handle
func (r *Runtime) extractSQLParams(paramsHandle *Value) ([]interface{}, error) {
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

type dbExecutor interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
	Query(query string, args ...interface{}) (*sql.Rows, error)
	QueryRow(query string, args ...interface{}) *sql.Row
}

func (r *Runtime) currentExecutor() dbExecutor {
	if r.currentTx != nil {
		return r.currentTx
	}
	return r.db
}

func (r *Runtime) dbExec(query string, args ...interface{}) (sql.Result, error) {
	exec := r.currentExecutor()
	if exec == nil {
		return nil, errors.New("database not initialized")
	}
	return exec.Exec(query, args...)
}

func (r *Runtime) dbQuery(query string, args ...interface{}) (*sql.Rows, error) {
	exec := r.currentExecutor()
	if exec == nil {
		return nil, errors.New("database not initialized")
	}
	return exec.Query(query, args...)
}

// HTTP Server methods

// httpCreateServer creates a new HTTP server instance
func (r *Runtime) httpCreateServer() (*Value, error) {
	r.httpMu.Lock()
	defer r.httpMu.Unlock()

	server := &HTTPServer{
		mux:    http.NewServeMux(),
		routes: make(map[string]map[string]*Value),
	}
	serverID := int64(len(r.httpServers))
	handle := r.newValue(Value{Kind: KindI64, I64: serverID})
	r.httpServers[serverID] = server
	return handle, nil
}

// httpAddRoute adds a route to the HTTP server
func (r *Runtime) httpAddRoute(caller *wasmtime.Caller, serverHandle *Value, methodHandle *Value, pathPtr int32, pathLen int32, handlerHandle *Value) error {
	r.httpMu.Lock()
	defer r.httpMu.Unlock()

	serverVal, err := r.getValue(serverHandle)
	if err != nil || serverVal.Kind != KindI64 {
		return errors.New("invalid server handle")
	}
	server, ok := r.httpServers[serverVal.I64]
	if !ok {
		return errors.New("invalid server handle")
	}

	methodVal, err := r.getValue(methodHandle)
	if err != nil {
		return err
	}
	if methodVal.Kind != KindString {
		return errors.New("route method must be string")
	}
	routeMethod, err := normalizeRouteMethod(methodVal.Str)
	if err != nil {
		return err
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

	if _, exists := server.routes[routeMethod]; !exists {
		server.routes[routeMethod] = map[string]*Value{}
	}
	server.routes[routeMethod][path] = handlerHandle
	return nil
}

// httpListen prepares the HTTP server for listening (actual start happens after WASM execution)
//
// 注意: この関数は実際にサーバーを起動しない。サーバー情報をpendingServerに保存し、
// WASM実行完了後にStartPendingServer()で起動する。
// 詳細はpendingHTTPServer構造体のコメントを参照。
func (r *Runtime) httpListen(caller *wasmtime.Caller, serverHandle *Value, portHandle *Value) error {
	serverVal, err := r.getValue(serverHandle)
	if err != nil || serverVal.Kind != KindI64 {
		return errors.New("invalid server handle")
	}

	r.httpMu.Lock()
	server, ok := r.httpServers[serverVal.I64]
	r.httpMu.Unlock()

	if !ok {
		return errors.New("invalid server handle")
	}

	portVal, err := r.getValue(portHandle)
	if err != nil {
		return err
	}
	if portVal.Kind != KindString {
		return errors.New("port must be string")
	}
	port := portVal.Str

	// Store pending server info - actual startup happens after WASM execution completes
	r.pendingServer = &pendingHTTPServer{
		server: server,
		port:   port,
	}

	return nil
}

func (r *Runtime) buildRequestObject(path string, method string, query map[string]string, form map[string]string) (*Value, error) {
	reqObj := r.newValue(Value{Kind: KindObject, Obj: &Object{Order: []string{}, Props: map[string]*Value{}}})
	pathHandle := r.newValue(Value{Kind: KindString, Str: path})
	methodHandle := r.newValue(Value{Kind: KindString, Str: method})
	pathKeyHandle := r.newValue(Value{Kind: KindString, Str: "path"})
	methodKeyHandle := r.newValue(Value{Kind: KindString, Str: "method"})
	if err := r.objSet(reqObj, pathKeyHandle, pathHandle); err != nil {
		return nil, err
	}
	if err := r.objSet(reqObj, methodKeyHandle, methodHandle); err != nil {
		return nil, err
	}

	queryObj := r.newValue(Value{Kind: KindObject, Obj: &Object{Order: []string{}, Props: map[string]*Value{}}})
	queryKeys := make([]string, 0, len(query))
	for key := range query {
		queryKeys = append(queryKeys, key)
	}
	sort.Strings(queryKeys)
	for _, key := range queryKeys {
		keyHandle := r.newValue(Value{Kind: KindString, Str: key})
		valueHandle := r.newValue(Value{Kind: KindString, Str: query[key]})
		if err := r.objSet(queryObj, keyHandle, valueHandle); err != nil {
			return nil, err
		}
	}
	queryKeyHandle := r.newValue(Value{Kind: KindString, Str: "query"})
	if err := r.objSet(reqObj, queryKeyHandle, queryObj); err != nil {
		return nil, err
	}

	formObj := r.newValue(Value{Kind: KindObject, Obj: &Object{Order: []string{}, Props: map[string]*Value{}}})
	formKeys := make([]string, 0, len(form))
	for key := range form {
		formKeys = append(formKeys, key)
	}
	sort.Strings(formKeys)
	for _, key := range formKeys {
		keyHandle := r.newValue(Value{Kind: KindString, Str: key})
		valueHandle := r.newValue(Value{Kind: KindString, Str: form[key]})
		if err := r.objSet(formObj, keyHandle, valueHandle); err != nil {
			return nil, err
		}
	}
	formKeyHandle := r.newValue(Value{Kind: KindString, Str: "form"})
	if err := r.objSet(reqObj, formKeyHandle, formObj); err != nil {
		return nil, err
	}

	return reqObj, nil
}

func resolveRoute(path string, routes map[string]*Value) (*Value, map[string]string, bool) {
	if handler, ok := routes[path]; ok {
		return handler, map[string]string{}, true
	}

	bestScore := -1
	bestPattern := ""
	var bestHandler *Value
	bestParams := map[string]string{}

	for pattern, handler := range routes {
		params, score, matched := matchRoutePattern(pattern, path)
		if !matched {
			continue
		}
		// Prefer routes with more static segments, then longer patterns, then lexicographically.
		if score > bestScore || (score == bestScore && (len(pattern) > len(bestPattern) || (len(pattern) == len(bestPattern) && pattern < bestPattern))) {
			bestScore = score
			bestPattern = pattern
			bestHandler = handler
			bestParams = params
		}
	}

	if bestScore == -1 {
		return nil, nil, false
	}
	return bestHandler, bestParams, true
}

func resolveRouteByMethod(path string, method string, routes map[string]map[string]*Value) (*Value, map[string]string, bool) {
	if methodRoutes, ok := routes[method]; ok {
		if handler, params, found := resolveRoute(path, methodRoutes); found {
			return handler, params, true
		}
	}
	if wildcardRoutes, ok := routes[routeMethodAny]; ok {
		return resolveRoute(path, wildcardRoutes)
	}
	return nil, nil, false
}

func normalizeRouteMethod(method string) (string, error) {
	trimmed := strings.TrimSpace(method)
	if trimmed == "" || trimmed == routeMethodAny {
		return routeMethodAny, nil
	}
	upper := strings.ToUpper(trimmed)
	switch upper {
	case "GET", "POST":
		return upper, nil
	default:
		return "", fmt.Errorf("unsupported HTTP method for add_route: %s (expected get or post)", method)
	}
}

func matchRoutePattern(pattern string, path string) (map[string]string, int, bool) {
	patternSegs := splitRouteSegments(pattern)
	pathSegs := splitRouteSegments(path)
	if len(patternSegs) != len(pathSegs) {
		return nil, 0, false
	}

	params := map[string]string{}
	staticCount := 0
	hasParam := false

	for i := 0; i < len(patternSegs); i++ {
		patSeg := patternSegs[i]
		pathSeg := pathSegs[i]

		if strings.HasPrefix(patSeg, ":") {
			name := strings.TrimPrefix(patSeg, ":")
			if name == "" || pathSeg == "" {
				return nil, 0, false
			}
			params[name] = pathSeg
			hasParam = true
			continue
		}
		if patSeg != pathSeg {
			return nil, 0, false
		}
		staticCount++
	}

	if !hasParam {
		return nil, 0, false
	}
	return params, staticCount, true
}

func splitRouteSegments(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return []string{}
	}
	return strings.Split(trimmed, "/")
}

func (r *Runtime) invokeRouteHandler(server *HTTPServer, path string, method string, query map[string]string, form map[string]string) (*HTTPResponse, error) {
	r.handlerMu.Lock()
	defer r.handlerMu.Unlock()

	normalizedMethod := strings.ToUpper(method)

	r.httpMu.Lock()
	handlerHandle, routeParams, ok := resolveRouteByMethod(path, normalizedMethod, server.routes)
	r.httpMu.Unlock()
	if !ok {
		return nil, fmt.Errorf("route not found: %s", path)
	}

	mergedQuery := make(map[string]string, len(query)+len(routeParams))
	for key, value := range query {
		mergedQuery[key] = value
	}
	for key, value := range routeParams {
		mergedQuery[key] = value
	}

	reqObj, err := r.buildRequestObject(path, method, mergedQuery, form)
	if err != nil {
		return nil, err
	}
	if r.db == nil {
		return nil, errors.New("database not initialized")
	}
	if r.instance == nil || r.store == nil {
		return nil, errors.New("no instance")
	}

	tx, err := r.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("transaction begin error: %w", err)
	}
	r.currentTx = tx
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
		r.currentTx = nil
	}()

	handlerVal, err := r.getValue(handlerHandle)
	if err != nil {
		return nil, err
	}
	if handlerVal.Kind != KindString {
		return nil, fmt.Errorf("handler is not a string, kind=%d", handlerVal.Kind)
	}
	handlerFunc := r.instance.GetFunc(r.store, handlerVal.Str)
	if handlerFunc == nil {
		return nil, fmt.Errorf("handler function not found: %s", handlerVal.Str)
	}

	result, err := handlerFunc.Call(r.store, reqObj)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, errors.New("handler returned nil")
	}
	resHandle, ok := result.(*Value)
	if !ok {
		return nil, fmt.Errorf("handler result is not externref: %T", result)
	}
	resVal, err := r.getValue(resHandle)
	if err != nil {
		return nil, err
	}
	if msg, isErr, err := r.resultErrorMessage(resHandle); err != nil {
		return nil, err
	} else if isErr {
		return nil, errors.New(msg)
	}
	if resVal.Kind != KindObject {
		return nil, fmt.Errorf("handler result is not object, kind=%d", resVal.Kind)
	}

	bodyKey := r.newValue(Value{Kind: KindString, Str: "body"})
	bodyHandle, err := r.objGet(resHandle, bodyKey)
	if err != nil {
		return nil, err
	}
	bodyVal, err := r.getValue(bodyHandle)
	if err != nil {
		return nil, err
	}
	if bodyVal.Kind != KindString {
		return nil, fmt.Errorf("response body is not string, kind=%d", bodyVal.Kind)
	}

	contentType := "text/plain; charset=utf-8"
	contentTypeKey := r.newValue(Value{Kind: KindString, Str: "contentType"})
	if ctHandle, err := r.objGet(resHandle, contentTypeKey); err == nil {
		if ctVal, err := r.getValue(ctHandle); err == nil && ctVal.Kind == KindString {
			contentType = ctVal.Str
		}
	}

	redirectURL := ""
	if contentType == "redirect" {
		redirectURLKey := r.newValue(Value{Kind: KindString, Str: "redirectUrl"})
		if urlHandle, err := r.objGet(resHandle, redirectURLKey); err == nil {
			if urlVal, err := r.getValue(urlHandle); err == nil && urlVal.Kind == KindString {
				redirectURL = urlVal.Str
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("transaction commit error: %w", err)
	}
	committed = true

	// Request境界でGCポリシーを評価する。
	// 条件: リクエスト回数 / Goヒープ増分 / 経過時間のいずれか。
	r.maybeStoreGC(false)

	return &HTTPResponse{
		Body:        bodyVal.Str,
		ContentType: contentType,
		StatusCode:  http.StatusOK,
		RedirectURL: redirectURL,
	}, nil
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

	server.mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		query := make(map[string]string)
		for key, values := range req.URL.Query() {
			if len(values) > 0 {
				query[key] = values[0]
			}
		}

		form := make(map[string]string)
		if req.Method == "POST" {
			_ = req.ParseForm()
			for key, values := range req.PostForm {
				if len(values) > 0 {
					form[key] = values[0]
				}
			}
		}

		response, err := r.invokeRouteHandler(server, req.URL.Path, req.Method, query, form)
		if err != nil {
			fmt.Fprintf(os.Stderr, "HTTP handler error: %v\n", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		if response.ContentType == "redirect" {
			if response.RedirectURL == "" {
				http.Error(w, "Invalid redirect response", http.StatusInternalServerError)
				return
			}
			http.Redirect(w, req, response.RedirectURL, http.StatusFound)
			return
		}

		w.Header().Set("Content-Type", response.ContentType)
		w.WriteHeader(response.StatusCode)
		_, _ = w.Write([]byte(response.Body))
	})

	// Flush accumulated output to stdout before blocking on ListenAndServe
	fmt.Print(r.output.String())
	r.output.Reset()
	return http.ListenAndServe(port, server.mux)
}

// httpResponseText creates a text response (from raw memory)
func (r *Runtime) httpResponseText(caller *wasmtime.Caller, textPtr int32, textLen int32) (*Value, error) {
	return r.httpResponse(caller, textPtr, textLen, "text/plain; charset=utf-8")
}

// httpResponseHtml creates an HTML response (from raw memory)
func (r *Runtime) httpResponseHtml(caller *wasmtime.Caller, htmlPtr int32, htmlLen int32) (*Value, error) {
	return r.httpResponse(caller, htmlPtr, htmlLen, "text/html; charset=utf-8")
}

// httpResponseTextStr creates a text response from a string object handle
func (r *Runtime) httpResponseTextStr(strHandle *Value) (*Value, error) {
	return r.httpResponseStr(strHandle, "text/plain; charset=utf-8")
}

// httpResponseHtmlStr creates an HTML response from a string object handle
func (r *Runtime) httpResponseHtmlStr(strHandle *Value) (*Value, error) {
	return r.httpResponseStr(strHandle, "text/html; charset=utf-8")
}

// httpResponseStr creates a response with the specified content type from a string handle
func (r *Runtime) httpResponseStr(strHandle *Value, contentType string) (*Value, error) {
	strVal, err := r.getValue(strHandle)
	if err != nil {
		return nil, err
	}
	if strVal.Kind != KindString {
		return nil, errors.New("expected string")
	}
	text := strVal.Str

	// Create response object with body and contentType
	resObj := r.newValue(Value{Kind: KindObject, Obj: &Object{Order: []string{}, Props: map[string]*Value{}}})
	bodyKey := r.newValue(Value{Kind: KindString, Str: "body"})
	bodyValue := r.newValue(Value{Kind: KindString, Str: text})
	_ = r.objSet(resObj, bodyKey, bodyValue)
	contentTypeKey := r.newValue(Value{Kind: KindString, Str: "contentType"})
	contentTypeValue := r.newValue(Value{Kind: KindString, Str: contentType})
	_ = r.objSet(resObj, contentTypeKey, contentTypeValue)

	return resObj, nil
}

// httpResponse creates a response with the specified content type
func (r *Runtime) httpResponse(caller *wasmtime.Caller, textPtr int32, textLen int32, contentType string) (*Value, error) {
	// Get text from memory
	ext := caller.GetExport("memory")
	if ext == nil {
		return nil, errors.New("memory not found")
	}
	memory := ext.Memory()
	if memory == nil {
		return nil, errors.New("memory not found")
	}
	data := memory.UnsafeData(caller)
	start := int(textPtr)
	end := start + int(textLen)
	if start < 0 || end > len(data) {
		return nil, errors.New("text string out of bounds")
	}
	text := string(data[start:end])

	// Create response object with body and contentType
	resObj := r.newValue(Value{Kind: KindObject, Obj: &Object{Order: []string{}, Props: map[string]*Value{}}})
	bodyKey := r.newValue(Value{Kind: KindString, Str: "body"})
	bodyValue := r.newValue(Value{Kind: KindString, Str: text})
	_ = r.objSet(resObj, bodyKey, bodyValue)
	contentTypeKey := r.newValue(Value{Kind: KindString, Str: "contentType"})
	contentTypeValue := r.newValue(Value{Kind: KindString, Str: contentType})
	_ = r.objSet(resObj, contentTypeKey, contentTypeValue)

	return resObj, nil
}

// httpGetPath gets the path from a request object
func (r *Runtime) httpGetPath(reqHandle *Value) (*Value, error) {
	pathKey := r.newValue(Value{Kind: KindString, Str: "path"})
	return r.objGet(reqHandle, pathKey)
}

// httpGetMethod gets the method from a request object
func (r *Runtime) httpGetMethod(reqHandle *Value) (*Value, error) {
	methodKey := r.newValue(Value{Kind: KindString, Str: "method"})
	return r.objGet(reqHandle, methodKey)
}

// httpResponseJson creates a JSON response from a data handle
func (r *Runtime) httpResponseJson(dataHandle *Value) (*Value, error) {
	val, err := r.getValue(dataHandle)
	if err != nil {
		return nil, err
	}

	// Convert to JSON
	jsonStr := r.valueToJSON(val)

	// Create response object
	resObj := r.newValue(Value{Kind: KindObject, Obj: &Object{Order: []string{}, Props: map[string]*Value{}}})
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
func (r *Runtime) httpResponseRedirect(caller *wasmtime.Caller, urlPtr int32, urlLen int32) (*Value, error) {
	// Get URL from memory
	ext := caller.GetExport("memory")
	if ext == nil {
		return nil, errors.New("memory not found")
	}
	memory := ext.Memory()
	if memory == nil {
		return nil, errors.New("memory not found")
	}
	data := memory.UnsafeData(caller)
	start := int(urlPtr)
	end := start + int(urlLen)
	if start < 0 || end > len(data) {
		return nil, errors.New("url string out of bounds")
	}
	url := string(data[start:end])

	return r.createRedirectResponse(url)
}

// httpResponseRedirectStr creates a redirect response from a string handle
func (r *Runtime) httpResponseRedirectStr(strHandle *Value) (*Value, error) {
	strVal, err := r.getValue(strHandle)
	if err != nil {
		return nil, err
	}
	if strVal.Kind != KindString {
		return nil, errors.New("expected string")
	}
	return r.createRedirectResponse(strVal.Str)
}

// createRedirectResponse creates a response object for redirects
func (r *Runtime) createRedirectResponse(url string) (*Value, error) {
	resObj := r.newValue(Value{Kind: KindObject, Obj: &Object{Order: []string{}, Props: map[string]*Value{}}})
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
