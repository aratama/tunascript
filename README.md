# Negitoro Compiler

## Overview

This repo contains a Go compiler for a minimal TypeScript subset that targets WASM. The compiler emits WAT and converts it to WASM via wasmtime-go. The runtime uses wasmtime-go (no external wasmtime CLI needed).

Language details are in `spec.md`.

## Requirements

Negitoroコンパイラをビルドや実行するには、以下の開発環境が必要です

- Go 1.21+
- CGO enabled
- A C compiler available (wasmtime-go depends on C)

## Run

Negitoroコンパイラを直接実行しNegitoroプログラムを起動するには、以下のコマンドを実行してください。

```shell
go run ./cmd/negitoro run <entry.ngtr> [args...]
```

## Negitoroプログラムのビルドと実行

```shell
go run ./cmd/negitoro build <entry.ngtr>
```

`entry.ngtr`と同じフォルダに、`entry.wat` と `entry.wasm`が生成されます。
ただし現状ではビルドした`*.wasm`はNegitoroのランタイム関数に依存しており、`wasmtime`などのランタイム環境では実行できません。
コンパイル済みの`*.wasm`を実行するには、以下のコマンドを使います。

```shell
go run ./cmd/negitoro launch <entry.ngtr> [args...]
```

将来的には、WASI Preview3に対応したwasmtimeなどのランタイムで直接実行できるようになるかもしれません。

## サンプルの実行

TODOリストのウェブサービスのサンプルを起動するには、以下のコマンドを実行します。

```shell
go run ./cmd/negitoro run example/server/server.ngtr example/server/todo.sqlite3
```

## Compiler Options

### `negitoro format --write --type <file.ngtr>`

コードフォーマッタを使ってソースコードを整形できます。

- `--write` フォーマットした結果で上書きします。
- `--type` ローカル変数に型推論で決定した型注釈を追加します。

## エディタサポート(vscode)

Negitoro言語のシンタックスハイライト拡張機能が `editors` に含まれています。
`Tasks: Run Task`から`Install VSIX Extension`を選択するとインストールできます(要NPM)。
