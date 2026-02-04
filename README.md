# TunaScript コンパイラ

## 概要

このリポジトリには、WASM をターゲットとする最小限の TypeScript サブセットの Go 製コンパイラが含まれています。コンパイラは WAT を出力し、wasmtime-go で WASM に変換します。ランタイムも wasmtime-go に依存しており、外部 wasmtime CLI は不要です。

言語の正式名称は **TunaScript** ですが、ドキュメントやツールでは _tuna_ と略して呼ばれることもあります。

詳細な言語仕様は `docs/spec.md` を参照してください。

- SQLフレンドリー。SQLを直接コード内に記述できます。
- JSONフレンドリー。データ型はすべてJSONライクです。また、データ型を定義すると同時にJSONデコーダーも定義されます。
- シンプル。TypeScriptに似た構文を持ちますが、機能は最小限です。

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

## Cloud Run デプロイ

### コンテナ画像構成

- `Dockerfile` はマルチステージ構成で Go 1.24.0 のビルド環境から `tuna` バイナリを生成し、distroless イメージへコピーします。
- `example/server` フォルダと Web UI に必要なファイルもコンテナに含めるので、`docker build -t tuna-server .` でローカルでビルド／`docker run --rm -p 8888:8888 tuna-server` でサービスを直接確認できます（サーバーは `:8888` で待ち受けます）。

### Cloud Build + Cloud Run 自動デプロイ

- `cloudbuild.yaml` を使えば `gcloud builds submit --config cloudbuild.yaml --substitutions=_SERVICE_NAME=tuna-server,_REGION=asia-northeast1` のようにコマンドを打つだけで、イメージのビルド・コンテナレジストリへのプッシュ・Cloud Run へのデプロイが順番に実施されます。
- 同構成では `--port 8888` を指定しており、Cloud Run サービスは `tuna run example/server/server.tuna example/server/todo.sqlite3` を自動で起動します。`gcloud config set project <YOUR_PROJECT>` を済ませてから上記コマンドを実行し、必要に応じて `_SERVICE_NAME`／`_REGION` を上書きしてください。

## TODO

- Result型
- エラーハンドリング
- 自動JSONデコーダー。zodなどのように型とデコーダーの二重定義になりません
