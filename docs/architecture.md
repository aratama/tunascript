# TunaScript コンパイラ アーキテクチャ

このドキュメントは、現在の TunaScript コンパイラの構成と処理フロー、そして `hostref` / `gc` の 2 つのバックエンドの違いを整理したものです。実装は主に `cmd/tuna` と `internal/` 以下にあります。

**全体像**

- 入力: `.tuna` / `.ts` ソース
- 出力: WAT（テキスト）と WASM（バイナリ）
- 実行: wasmtime-go ベースのランタイム（`internal/runtime`）

## コンポーネント構成

- `cmd/tuna`: CLI エントリ。`build` / `run` / `launch` / `format` を提供し、`--backend hostref|gc` でコード生成バックエンドを切り替える（デフォルトは `hostref`）。
- `internal/compiler`: 解析・型検査・コード生成のオーケストレーション。`Compiler.Compile()` が処理の中心。
- `internal/parser` / `internal/ast`: パーサと AST 定義。
- `internal/types`: 型チェックとシンボル解決。
- `internal/runtime`: wasmtime-go を使った実行環境。ホスト関数の実装もここに集約。
- `lib/`: 組み込みライブラリ（TunaScript 実装と、GC バックエンド用 WAT 実装）。

## コンパイルパイプライン

1. **ライブラリ探索と組み込みモジュールの登録**: `lib` ディレクトリを探索して組み込みモジュールの一覧を構築し、`TUNASCRIPT_LIB_DIR` があればそれを優先する。
2. **モジュール読み込み**: 入口ファイルから `import` を辿り、再帰的に AST を構築する。組み込みモジュールは `lib/*.tuna` から読み込み、`.tuna` / `.ts` 以外の拡張子は「テキストモジュール」として文字列 `default` を生成する。
3. **サーバーモジュールの自動ロード**: SQL やテーブル定義など、サーバー機能が必要な AST を検出すると `server` モジュールを追加で読み込む。
4. **型検査**: `internal/types` のチェッカーで全モジュールを型検査する。
5. **コード生成（WAT）**: 文字列リテラルの収集とインターン、関数・グローバルの識別子割り当て、WAT モジュールの出力（imports / memory / globals / functions / init / start）を行う。テーブル定義がある場合は JSON を生成し、`__init` で `server.register_tables` を呼び出す。
6. **WAT → WASM 変換**: `wasmtime-go` の `Wat2Wasm` を使用する。

## バックエンド共通の重要点

- 参照型（文字列・配列・オブジェクト・JSON・関数値など）はバックエンドにより `externref` または `anyref` に割り当てられる。
- 文字列リテラルは線形メモリに埋め込み、`__init` で `prelude.str_from_utf8` を通じてインターンする。
- 関数値は「関数名を表す文字列」として扱われ、関数値用のラッパー関数を WASM 側でエクスポートする。

## hostref バックエンド

`hostref` はデフォルトのバックエンドで、参照型を `externref` として扱います。値の実体は Go 側の `*runtime.Value` で管理され、WASM とホストが `externref` を介してやり取りします。

**特徴**

- 参照型は `externref`。
- 組み込み API は主にホスト（Go）側で実装される。
- `__main_result` をエクスポートし、ランタイムが `main` の戻り値（`void | error`）を検査できる。

**実装の要点**

- `lib/*.tuna` の `extern` 宣言は、`internal/runtime` のホスト関数として実装される。
- `Generator` は `extern` 関数を `import` として出力する。
- 代表的なホスト関数群は `prelude` 系（値の生成・変換・比較・配列/オブジェクト操作など）と、`json` / `array` / `http` / `file` / `sqlite` / `server` などの組み込み API。
- `call_fn` は Go 側で実装され、関数名文字列を使って `instance.GetFunc` からラッパー関数を取得し、引数配列を渡して呼び出す。
- `server.gc` は `store.GC()` を呼び出し、ランタイム内でヒューリスティックな GC 閾値も管理している。

## gc バックエンド

`gc` は Wasm GC を前提としたバックエンドです。参照型は `anyref` として扱われ、値や配列・オブジェクトの実体は WAT で定義された GC 構造体で表現されます。

**特徴**

- 参照型は `anyref`。
- 主要なランタイム機能は WAT 側に実装され、WASM 内で完結する。
- ホストとの境界は `host.wat` による型変換を通す。

**WAT 実装の構成**

`lib/prelude.wat` は GC 向けの値表現を定義し（`struct` / `array` など）、文字列・配列・オブジェクト・数値/真偽値/Null/Undefined の基本操作を実装する。`prelude.call_fn` は後述のディスパッチに接続される。

`lib/array.wat` は `array.map` / `filter` / `reduce` / `range` などの高階関数を WAT で実装する。

`lib/host.wat` は `anyref` ⇔ `externref` の相互変換を行うブリッジで、`host.to_gc` / `host.to_host` を提供する。

`lib/server.wat` は SQL/環境変数などのサーバー API を `host` へ委譲し、結果を `anyref` に変換して返す。

**コンパイル時の特別処理**

- 組み込みモジュールについて `.wat` が存在する場合に限り、その WAT を取り込む。
- `moduleDefinedInWAT` を使って、WAT 内で定義済みの `extern` を `import` から除外する。
- `filterDeclsForGC` により、WAT 実装のない `extern` 宣言は除去され、それを呼び出す関数も削除される。これにより GC バックエンドで未対応の API を利用した場合はコンパイル時に落ちる設計になっている。

**関数値のディスパッチ**

- `Generator` が `__call_fn_dispatch` を生成する。
- 関数値ラッパー（`*_fnvalue`）を一括エクスポートする。
- `prelude.call_fn` は `__call_fn_dispatch` を呼び、関数名文字列に基づいてディスパッチする。
- これにより、WASM 内で完結した高階関数呼び出しが可能になる。

## hostref / gc の主な違い（要約）

| 観点 | hostref | gc |
| --- | --- | --- |
| 参照型 | `externref` | `anyref` |
| ランタイムの所在 | Go 側（`internal/runtime`）が中心 | WAT 側（`lib/*.wat`）が中心 |
| ホスト境界 | ほぼ全 API がホスト実装 | `host.wat` を介した最小限の API 呼び出し |
| 関数値呼び出し | Go の `call_fn` が `instance.GetFunc` で解決 | `__call_fn_dispatch` による WASM 内ディスパッチ |
| `__main_result` エクスポート | あり | なし |

## 関連ファイル

- `internal/compiler/compiler.go`
- `internal/compiler/generator.go`
- `internal/compiler/wat2wasm_*.go`
- `internal/runtime/runtime.go`
- `lib/prelude.tuna`, `lib/prelude.wat`
- `lib/array.tuna`, `lib/array.wat`
- `lib/host.tuna`, `lib/host.wat`
- `lib/server.tuna`, `lib/server.wat`
