# TunaScript 組み込みライブラリ

このドキュメントは組み込みモジュールの概要と依存関係をまとめたものです。

## 依存区分

- **Wasm内完結**: A1/A2（TunaScript + WAT）だけで動作します。
- **ホスト連携あり**: SQL・環境変数・フォーマッタなどホスト実装を呼び出します。
- **バックエンド依存**: `--backend=gc` と `--backend=host` で実装が切り替わります。

## prelude（Wasm内完結）

A1（純粋TunaScript）とA2（WAT実装）の基本APIです。

- A1: `fallback`, `then`, `error`
- A2: `log`, `to_string`, `string_length` ほか内部の低レベル関数

`log` は `wasi_snapshot_preview1.fd_write`（fd=1）を使用します。

## server（ホスト連携あり）

- `get_args`, `get_env`
- `gc`

## array（Wasm内完結）

- `range`, `length`, `map`, `filter`, `reduce`

## json（バックエンド依存）

- `stringify`, `parse`, `decode`
- `--backend=gc`: `stringify` / `parse` / `decode` はWAT実装で Wasm 内完結。
- `--backend=host`: 既存のホスト実装を利用します。

## http（バックエンド依存）

- `create_server`, `add_route`, `listen`
- `response_text`, `response_html`, `response_json`, `response_redirect`
- `get_path`, `get_method`
- `--backend=gc`: `listen` はソケットサーバーを起動せず、`GET /` を1回実行し、`Response.body` を `fd_write` の fd=3 に書き込みます。
- `--backend=host`: 実際のソケットサーバーを起動し、HTTPリクエストを処理します。

## sqlite（ホスト連携あり）

- `db_open`, `gc_open`, `sqlQuery`（`gc_open` は `db_open` の別名）
- SQL構文 (`execute`, `fetch_one`, `fetch_optional`, `fetch_all`) は内部的に `sqlite` を利用します。
- `--backend=gc`: `db_open` は no-op で `undefined` を返し、デフォルトのインメモリDB（`:memory:`）を継続します。
- `--backend=host`: `db_open` / `gc_open` が実際のSQLiteファイルを開きます。

## file（バックエンド依存）

- `read_text`, `write_text`, `append_text`, `read_dir`, `exists`
- `--backend=gc`: `read_text` / `write_text` / `append_text` / `read_dir` は常に `error`、`exists` は常に `false`。
- `--backend=host`: 実際のファイルシステムに対して読み書きを行います。

## runtime（ホスト連携あり）

- `run_formatter`
- `run_sandbox`
- `run_sandbox` は現在のバックエンド設定に関わらず、常に `gc` バックエンドで `source` を実行します。
- 戻り値は `{ stdout: string, html: string } | error` です。

## interop（内部）

GCブリッジ用の内部モジュールです。公開APIとしての利用は想定していません。
