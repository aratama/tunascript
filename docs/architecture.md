# TunaScript コンパイラ アーキテクチャ

このドキュメントは現在の TunaScript コンパイラ（`gc` / `host` バックエンド）の実装概要をまとめたものです。実装は主に `cmd/tuna` と `internal/` 以下にあります。

## 全体像

- 入力: `.tuna` / `.ts` ソース
- 出力: WAT（テキスト）と WASM（バイナリ）
- 実行: `wasmtime-go` ベースのランタイム（`internal/runtime`）

## コンポーネント構成

- `cmd/tuna`: CLI エントリ。`build` / `run` / `launch` / `format` を提供（`build`/`run` は `--backend=gc|host` を受理）。
- `internal/compiler`: 解析・型検査・コード生成のオーケストレーション。
- `internal/parser` / `internal/ast`: パーサと AST 定義。
- `internal/types`: 型チェックとシンボル解決。
- `internal/runtime`: 実行環境とホスト関数実装。
- `lib/`: 組み込みライブラリ（`.tuna` 宣言と `.wat` 実装）。

## コンパイルパイプライン

1. `lib` ディレクトリ探索（`TUNASCRIPT_LIB_DIR` があれば優先）
2. エントリファイルから `import` を辿って AST 構築
3. SQL/テーブル定義がある場合は `server` を自動ロード
4. 型検査（`internal/types`）
5. WAT 生成（imports / memory / globals / functions / init / start）
6. `Wat2Wasm` による WASM 変換

## バックエンド

### `gc` バックエンド

Wasm GC 前提で動作し、参照型は `anyref` を使います。

- `lib/prelude.wat`: 文字列・配列・オブジェクト・値操作の基盤実装
- `lib/array.wat`: `range` / `map` / `filter` / `reduce`
- `lib/http.wat`: `listen` は実サーバーを起動せず、`GET /` を1回実行して fd=3 へ出力
- `lib/file.wat`: `read_text` などは常に `error`、`exists` は常に `false`
- `lib/sqlite.wat`: `db_open` は no-op で `undefined` を返し、`:memory:` を継続
- `lib/json.wat` / `lib/runtime.wat`: `host` ブリッジ経由でホスト実装へ委譲
- `lib/host.wat`: `anyref` ⇔ `externref` の相互変換
- `lib/server.wat`: SQL/環境変数などのホスト連携 API

### `host` バックエンド

- 参照型は `gc` バックエンドと同様に `anyref` を使います。
- `lib/*.host.wat` が選択されるモジュールでは、Wasm内は `anyref` のまま保持し、Goホスト関数呼び出し直前に `host.to_host` / `host.to_gc` で相互変換します。
- `http` は実ソケットサーバーを起動します。
- `file` は実ファイルシステムを操作します。
- `sqlite.db_open` / `sqlite.gc_open` は実SQLiteファイルを開きます。
- `runtime.run_sandbox` はこのモードでも内部的には `gc` バックエンド固定で実行します。

## 関数値ディスパッチ

- `Generator` が `__call_fn_dispatch` を生成
- 関数値ラッパー（`*_fnvalue`）をエクスポート
- `prelude.call_fn` が `__call_fn_dispatch` を呼んで動的呼び出しを実現

## 関連ファイル

- `internal/compiler/compiler.go`
- `internal/compiler/generator.go`
- `internal/runtime/runtime.go`
- `internal/runtime/runner.go`
- `lib/prelude.wat`
- `lib/array.wat`
- `lib/http.wat`
- `lib/http.host.wat`
- `lib/file.wat`
- `lib/file.host.wat`
- `lib/sqlite.wat`
- `lib/sqlite.host.wat`
- `lib/json.wat`
- `lib/runtime.wat`
- `lib/host.wat`
- `lib/server.wat`
