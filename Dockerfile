# -----------------------------------------------------------------------------
# Stage 1: builder
# -----------------------------------------------------------------------------
# Go + CGO 環境で tuna バイナリとサンプル用 wasm を生成する。
# 最終イメージにはビルドツールチェーンを含めない（マルチステージビルド）。
FROM golang:1.24.0-bullseye AS builder

# 以降の作業ディレクトリ。
WORKDIR /workspace

# 依存モジュールを先に解決して、ソース変更時のキャッシュ効率を上げる。
COPY go.mod go.sum ./
RUN go mod download

# 全ソースをコピーしてビルド。
COPY . .
# tuna CLI 本体をビルド（Cloud Run 実行時のエントリポイントになる）。
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o /workspace/tuna ./cmd/tuna
# サンプルを事前コンパイルして同梱する。
RUN /workspace/tuna build example/server/server.tuna -o example/server/server
RUN /workspace/tuna build example/playground/playground.tuna -o example/playground/playground

# -----------------------------------------------------------------------------
# Stage 2: runtime
# -----------------------------------------------------------------------------
# distroless で最小構成の実行専用イメージを作る。
FROM gcr.io/distroless/cc

WORKDIR /app
# builder で作った実行物のみをコピーする。
COPY --from=builder /workspace/tuna /app/tuna
COPY --from=builder /workspace/example /app/example
COPY --from=builder /workspace/lib /app/lib

# コンテナが待ち受ける想定ポート（Cloud Run の --port と合わせる）。
EXPOSE 8888

# ENTRYPOINT は固定コマンド。コンテナ起動時は常に /app/tuna が実行される。
ENTRYPOINT ["/app/tuna"]

# CMD は「デフォルト引数」。
# ここでは TODO サンプル起動を既定値としているだけで、Cloud Run デプロイ時に
# `gcloud run deploy ... --args "..."` を指定するとこの CMD は上書きされる。
#
# 例:
# - このまま `docker run` すると:
#     /app/tuna launch example/server/server.wasm example/server/todo.sqlite3
# - cloudbuild.yaml の TODO サービスでは `--args` により:
#     /app/tuna launch example/server/server.wasm /tmp/todo.sqlite3
# - cloudbuild.yaml の Playground サービスでは `--args` により:
#     /app/tuna launch example/playground/playground.wasm /tmp/playground.sqlite3
# というように、同一イメージでも起動対象を切り替えている。
CMD ["launch", "example/playground/playground.wasm", "example/playground/playground.sqlite3"]
