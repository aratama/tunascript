FROM golang:1.24.0-bullseye AS builder

WORKDIR /workspace

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o /workspace/tuna ./cmd/tuna
RUN /workspace/tuna build example/server/server.tuna -o example/server/server

FROM gcr.io/distroless/cc

WORKDIR /app
COPY --from=builder /workspace/tuna /app/tuna
COPY --from=builder /workspace/example /app/example

EXPOSE 8888
ENTRYPOINT ["/app/tuna"]
CMD ["launch", "example/server/server.wasm", "example/server/todo.sqlite3"]
