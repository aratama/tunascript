# TunaScript 組み込みライブラリ

このドキュメントは組み込みモジュールの概要と依存関係をまとめたものです。

## 依存区分

- **Wasm内完結**: A1/A2（TunaScript + WAT）だけで動作します。
- **ホスト連携あり**: SQL・環境変数・フォーマッタなどホスト実装を呼び出します。

## prelude（Wasm内完結）

A1（純粋TunaScript）とA2（WAT実装）の基本APIです。

- A1: `fallback`, `then`, `error`
- A2: `log`, `to_string`, `string_length` ほか内部の低レベル関数

`log` は `wasi_snapshot_preview1.fd_write`（fd=1）を使用します。

## server（ホスト連携あり）

- `get_args`, `get_env`, `getArgs`
- `gc`
- `sqlQuery`
- SQL構文 (`execute`, `fetch_one`, `fetch_optional`, `fetch_all`) は内部的に `server` を利用します。

## array（Wasm内完結）

- `range`, `length`, `map`, `filter`, `reduce`

## json（ホスト連携あり）

- `stringify`, `parse`, `decode`

## http（Wasm内完結）

- `create_server`, `add_route`, `listen`
- `responseText`, `response_html`, `responseJson`, `response_redirect`
- `getPath`, `getMethod`
- `listen` はソケットサーバーを起動せず、`GET /` を1回実行し、`Response.body` を `fd_write` の fd=3 に書き込みます。

## sqlite（ホスト連携あり）

- `db_open`
- `db_open` は通常モード（GCバックエンド）では no-op で、`undefined` を返します。
- SQL実行はデフォルトのインメモリDB（`:memory:`）で継続します。

## file（Wasm内完結）

- `read_text`, `write_text`, `append_text`, `read_dir`, `exists`
- `read_text` / `write_text` / `append_text` / `read_dir` は常に `error` を返します。
- `exists` は常に `false` を返します。

## runtime（ホスト連携あり）

- `run_formatter`

## host（内部）

GCブリッジ用の内部モジュールです。公開APIとしての利用は想定していません。
