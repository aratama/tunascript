# Negitoro Compiler

## Overview

This repo contains a Go compiler for a minimal TypeScript subset that targets WASM. The compiler emits WAT and converts it to WASM via wasmtime-go. The runtime uses wasmtime-go (no external wasmtime CLI needed).

Language details are in `spec.md`.

## Layout

- `cmd/negitoro` CLI entrypoint
- `internal/ast` AST definitions
- `internal/lexer` / `internal/parser` lexer + parser
- `internal/types` type checker
- `internal/compiler` WAT generator and WAT->WASM conversion
- `internal/runtime` runtime (wasmtime-go)

## Requirements

- Go 1.21+
- CGO enabled
- A C compiler available (wasmtime-go depends on C)

## Build

```
go run ./cmd/negitoro build <entry.ngtr> [-o name]
```

- Produces `<entry-dir>/<name-or-entry-base>.wat` and `.wasm` in the same folder as the `.ngtr`.

## Run

```
go run ./cmd/negitoro run <entry.ngtr> [args...]
```

## Format

コードフォーマッタを使ってソースコードを整形できます。

```
# フォーマット結果を標準出力に表示
go run ./cmd/negitoro fmt <file.ngtr>

# ファイルを上書き保存
go run ./cmd/negitoro fmt -w <file.ngtr>

# 複数ファイルを一括フォーマット
go run ./cmd/negitoro fmt -w *.ngtr
```

## Tests (detailed commands)

### Run all tests (PowerShell)

```
$env:CGO_ENABLED=1
# Ensure a C compiler is installed and on PATH
go test ./...
```

### Run one package

```
go test ./internal/compiler -v
```

### Run a single test

```
go test ./internal/compiler -run TestArrayForOf -v
```

### Common failures

- `undefined: wasmtime.*`
  - CGO is disabled or a C compiler is missing.
  - Enable CGO and install a C toolchain, then re-run tests.

## エディタサポート

### VS Code

Negitoro言語のシンタックスハイライト拡張機能が `editors/vscode` に含まれています。

#### インストール方法

1. シンボリックリンクを作成（開発用に推奨）:

```bash
# Linux / macOS
ln -s $(pwd)/editors/vscode ~/.vscode/extensions/negitoro.negitoro-0.1.0

# Windows (PowerShell を管理者として実行)
cmd /c mklink /D "$env:USERPROFILE\.vscode\extensions\negitoro.negitoro-0.1.0" "$(Get-Location)\editors\vscode"
```

2. VS Codeを再起動

詳細は [editors/vscode/README.md](editors/vscode/README.md) を参照してください。
