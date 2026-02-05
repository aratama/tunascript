# 組み込みライブラリ

TunaScriptには以下の組み込みライブラリがあります。

- `prelude`
- `json`
- `array`
- `runtime`
- `http`
- `sqlite`

## prelude

### 12.0 型エイリアス

- `type Error = { type: "Error", message: string }`

### 12.1 関数

- `log(value: T): void`
  - 文字列はそのまま出力します。
  - それ以外は `stringify` 相当で出力します。
  - `tuna run --sandbox` では標準出力へ直接は出さず、内部バッファに蓄積されて最終JSONの `stdout` フィールドに入ります。
- `Error(message: string): Error`
  - `T | Error` の失敗値を作成します（`{ "type": "Error", "message": message }` 相当）。

例:

```typescript
import { log, type Error } from "prelude";
import { parse, decode } from "json";

type Person = { name: string; age: number };

const parsed: json | Error = parse("{\"name\": \"Alice\", \"age\": 30}");

switch (parsed) {
  case json as json: {
    const decoded: Person | Error = decode<Person>(json);
    switch (decoded) {
      case decoded as Person:
        log(decoded.name);
      case decoded as Error:
        log(decoded.message);
    }
  }
  case parsedError as Error:
    log(parsedError.message);
}
```
- `toString(value: integer | number | boolean | string): string`
- `getArgs(): string[]`
  - コマンドライン引数を配列として返します。
- `getEnv(name: string): string`
  - 指定した名前の環境変数の値を返します。存在しない場合は空文字列になります。
  - `tuna run --sandbox` では常に空文字列を返します。

### 12.1.1 jsonモジュール

`json` モジュールは JSON 関連の関数を提供します。

- `stringify(value: T): string`
  - JSON文字列に変換します。
  - オブジェクトのプロパティ値が `undefined` の場合、そのプロパティは出力されません。
- `parse(s: string): json | Error`
  - `s` をJavaScriptの `JSON.parse` と同様にJSONとしてパースします。
  - 成功時は `json`、失敗時は `Error` を返します。
  - JSON 数値は `1` なら `integer`、`1.0` や `1e3` は `number` になります。
- `decode<T>(json: json): T | Error`
  - `json` を型 `T` としてデコードし、成功時は `T`、失敗時は `Error` を返します。
  - `T` はJSONとして表現可能な型（`integer` / `number` / `boolean` / `string` / `null` / `json` / 配列 / タプル / オブジェクト / Union など）である必要があります。
  - `T` のオブジェクト型で `undefined` を含むプロパティは optional として扱われ、JSON側にキーが無い場合は `undefined` になります。
  - 型引数は省略できません（例: `decode<Person>(json)`）。

### 12.1.2 arrayモジュール

`array` モジュールは配列処理関数を提供します。

- `range(start: integer, end: integer): integer[]`
  - `start` 以上 `end` **以下**の連続した `integer` を格納した配列を返します（`range(2, 5)` は `[2, 3, 4, 5]`）。
  - `end < start` などで範囲が不正な場合は空配列 `[]` を返します。
- `length(array: T[]): integer`
  - 配列の長さを返します。
- `map<T, S>(fn: (value: T) => S, xs: Array<T>): Array<S>`
  - 型変数 `T` / `S` を使って `xs` の各要素を `fn` で変換します。
- `filter<T>(fn: (value: T) => boolean, xs: Array<T>): Array<T>`
  - `fn` が `true` を返した要素のみを残します。
- `reduce<T, R>(fn: (acc: R, value: T) => R, xs: Array<T>, initial: R): R`
  - `fn` による畳み込みの結果（型 `R`）を返します。`initial` は初期累積値です。

### 12.1.3 runtimeモジュール

`runtime` モジュールは実行環境に関する関数を提供します。

- `runSandbox(source: string): string`
  - `source`（TunaScriptコード文字列）をサンドボックス実行し、`{ stdout, html, exitCode, error }` 形式のJSON文字列を返します。
  - この関数はサンドボックスモード内（`tuna run --sandbox`）では使用できません。
- `runFormatter(source: string): string | Error`
  - `source`（TunaScriptコード文字列）をフォーマットします。
  - フォーマット成功時は整形済みコード文字列、失敗時は `Error` を返します。

### 12.1.4 sqliteモジュール

`sqlite` モジュールは `dbOpen` を含む SQLite 固有の関数を提供します。

- `dbOpen(filename: string): undefined | Error`
  - 指定したSQLiteファイルを直接開きます。ファイルが存在しない場合は新規作成され、書き込みはそのままファイルに反映されます。`create_table` 定義がある場合、テーブルの自動作成と検証が行われます。
  - この関数は `import { dbOpen } from "sqlite";` でインポートしてください。
  - `tuna run --sandbox` では no-op になり、常にインメモリDB (`:memory:`) が使われます。

### 12.2 HTTPサーバー

組み込みHTTPサーバー機能を提供します。

#### 12.2.1 HTTPサーバー関数

これらの関数（`responseHtml`, `responseJson`, `responseRedirect` などを含む）は `http` モジュールから `import { ... } from "http";` で明示的にインポートしてください。

- `createServer(): Server`
  - 新しいHTTPサーバーインスタンスを作成します。
- `addRoute(server: Server, path: string, handler: (req: Request) => Response | Error): void`
- `addRoute(server: Server, method: string, path: string, handler: (req: Request) => Response | Error): void`
  - サーバーに指定したパスのルートを追加します。ハンドラーはリクエストを受け取り、成功時は `Response`、失敗時は `Error` を返す関数です。
  - `method` 付きの形式では、`"get"` または `"post"` を指定できます。指定したメソッドのときだけハンドラーが実行されます。
  - 3引数形式（`method` 省略）はすべてのメソッドにマッチします。
  - `path` は `/:id` や `/run/:id` のようなパスパラメータにも対応しています。マッチした値は `req.query.id` のように `query` に展開されます。
  - 同じメソッドの中では完全一致ルートが優先され、完全一致が無い場合にパスパラメータルートが解決されます。
  - メソッド指定ルートが優先され、見つからない場合は `method` 省略（全メソッド）ルートにフォールバックします。
  - `tuna run --sandbox` では `addRoute(server, "/", handler)` の重複登録はエラーになります。
- `listen(server: Server, port: string): void`
  - サーバーを指定したポートで起動します。この関数はブロッキングで、サーバーが終了するまで戻りません。
  - `tuna run --sandbox` ではソケットは開かず、`GET /` を1回だけ仮想実行して直ちに終了します。`Response.body` は最終JSONの `html` フィールドに入ります。
- `responseText(text: string): Response`
  - テキストレスポンスを作成します。
- `responseHtml(html: string): Response`
  - HTML を直接返す `contentType: "text/html; charset=utf-8"` のレスポンスを作成します。
- `responseJson(data: string): Response`
  - JSON ボディを返すレスポンス（`contentType` は `application/json`）を作成します。
- `responseRedirect(url: string): Response`
  - `Location` ヘッダーと `302 Found` をセットしたリダイレクトを作成します。
- `getPath(req: Request): string`
  - リクエストのパスを取得します。
- `getMethod(req: Request): string`
  - リクエストのHTTPメソッド（GET, POSTなど）を取得します。

- HTTPハンドラーは `addRoute` で登録されると、`listen` から実行されるたびに自動的にSQLiteトランザクション (`BEGIN ... COMMIT/ROLLBACK`) 内で動作します。ハンドラーが正常に戻るとコミットされ、ハンドラーの呼び出し中にエラーが発生した場合はロールバックされて変更は破棄されます。
- このトランザクションの性質上、HTTPリクエストは1つずつ順番に処理され、同時リクエストは順番待ちになります。

#### 12.2.2 型

- `JSX`: サーバーサイドレンダリング済みHTML断片（`string` のエイリアス）
- `Server`: HTTPサーバーインスタンス
- `Request`: HTTPリクエスト（`{ path: string, method: string, query: Map<string>, form: Map<string> }` オブジェクト）
- `Response`: HTTPレスポンス（`{ body: string }` オブジェクト）

これらの型は `http` モジュールからエクスポートされる型エイリアスなので、必要に応じて `import { type JSX, type Request, type Response } from "http";` で明示的にインポートしてください。

## サンドボックス実行結果

`tuna run --sandbox <entry.tuna>` の標準出力は、必ず次のJSON 1件です。

```json
{"stdout":"...","html":"...","exitCode":0,"error":""}
```

- `stdout`: `log` の出力（改行込み）
- `html`: `/` ハンドラーの `Response.body`
- `exitCode`: 成功時 `0`、失敗時 `1`
- `error`: 成功時は空文字、失敗時はエラーメッセージ
