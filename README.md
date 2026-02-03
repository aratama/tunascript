# TunaScript コンパイラ

## 概要

このリポジトリには、WASM をターゲットとする最小限の TypeScript サブセットの Go 製コンパイラが含まれています。コンパイラは WAT を出力し、wasmtime-go で WASM に変換します。ランタイムも wasmtime-go に依存しており、外部 wasmtime CLI は不要です。

言語の正式名称は **TunaScript** ですが、ドキュメントやツールでは _tuna_ と略して呼ばれることもあります。

詳細な言語仕様は `spec.md` を参照してください。

## 要件

TunaScript コンパイラのビルド・実行には、以下の環境が必要です。

- Go 1.21 以上
- CGO 有効
- C コンパイラ（wasmtime-go が依存）

## 実行

TunaScript プログラムを直接起動するには、以下のコマンドを実行してください。

```shell
go run ./cmd/tuna run <entry.tuna> [args...]
```

## TunaScript プログラムのビルドと実行

```shell
go run ./cmd/tuna build <entry.tuna>
```

`entry.tuna` と同じフォルダに `entry.wat` と `entry.wasm` が生成されます。生成された `*.wasm` は現在のところ TunaScript のランタイム関数に依存しており、`wasmtime` 等のランタイムでそのまま実行することはできません。

ビルド済みの `*.wasm` を TunaScript ランタイムで実行するには、以下のコマンドを使用してください。

```shell
go run ./cmd/tuna launch <entry.wasm> [args...]
```

## サンプルの実行

TODO リストの Web サービスのサンプルを起動するには、以下のコマンドを実行します。

```shell
go run ./cmd/tuna run example/server/server.tuna example/server/todo.sqlite3
```

## コンパイラオプション

### `tuna format --write --type <file.tuna>`

コードフォーマッタを使ってソースコードを整形できます。

- `--write` でフォーマット結果をソースファイルに書き戻します。
- `--type` でローカル変数に型推論で決定した型注釈を追加します。

## エディタサポート(vscode)

`editors` ディレクトリには TunaScript 用のシンタックスハイライト拡張機能が含まれています。`Tasks: Run Task` から `Install VSIX Extension` を選ぶとインストールできます（npm が必要です）。
