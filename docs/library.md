# TunaScript 組み込みライブラリ

このドキュメントは組み込みモジュールの概要と依存関係をまとめたものです。

## 依存区分

- **ホスト非依存**: A1/A2（TunaScript + WAT）だけで動作します。WASIが不要な範囲でブラウザ実行も可能です。
- **ホスト依存**: ファイルI/O、DB、HTTP、環境変数などホスト機能が必要です。

## prelude（ホスト非依存）

A1（純粋TunaScript）とA2（WAT実装）の基本APIです。

- A1: `fallback`, `then`, `error`
- A2: `log`, `to_string`, `string_length` ほか内部の低レベル関数

`log` はWASIの標準出力に依存します。

## server（ホスト依存）

ホスト依存APIを集約したモジュールです。

- `get_args`, `get_env`, `getArgs`
- `gc`
- `sqlQuery`
- SQL構文 (`execute`, `fetch_one`, `fetch_optional`, `fetch_all`) は内部的に `server` を利用します。

## array（ホスト非依存）

- `range`, `length`, `map`, `filter`, `reduce`

## json（ホスト依存）

- `stringify`, `parse`, `decode`

## http（ホスト依存）

- `create_server`, `add_route`, `listen`
- `responseText`, `response_html`, `responseJson`, `response_redirect`
- `getPath`, `getMethod`

## sqlite（ホスト依存）

- `db_open`

## file（ホスト依存）

- `read_text`, `write_text`, `append_text`, `read_dir`, `exists`

## runtime（ホスト依存）

- `run_sandbox`, `run_formatter`

## host（内部）

GCブリッジ用の内部モジュールです。公開APIとしての利用は想定していません。
