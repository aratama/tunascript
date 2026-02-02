# 言語仕様（Negitoro → WASM）

## 1. 概要

- TypeScript の最小サブセット。
- クラス / インターフェイス / 名前空間 / this は不使用。
- 代入はなく、`const` のみ。
- 関数は `function` 宣言のみ（トップレベルのみ）。
- 繰り返しは `for (const x: T of arr)` のみ。
- ESModule 形式の `import` / `export`。
- バックエンドは WAT を生成し、wasmtime-go の `Wat2Wasm` で WASM へ変換。

## 2. 型

### 2.1 プリミティブ

- `integer`（整数）
- `float`（浮動小数点）
- `boolean`（`true` / `false`）
- `string`（UTF-8）
- `void`

### 2.2 複合型

- 配列: `T[]`
- タプル: `[T1, T2, ...]`
- オブジェクト: `{ "key": T, ... }`
- 関数型: `(a: T, b: U) => R`

### 2.3 型エイリアス

TypeScriptと同様の構文で型に別名を付けることができる。

```
type MyType = { "name": string, "age": integer };
export type Response = { "body": string, "contentType": string };
```

- `type Name = TypeExpr;` で型エイリアスを定義。
- `export` を付ければモジュール外に公開できる。
- 他のモジュールから `import { type TypeName } from "module";` でインポートできる。
- 型をインポートするときは必ず `type` キーワードを付ける必要がある。
- 型エイリアスは型注釈で使用できる。

#### preludeの型エイリアス

preludeには以下の型エイリアスが定義されている:

| 型名       | 定義                                        |
| ---------- | ------------------------------------------- |
| `Request`  | `{ "path": string, "method": string, "query": stringMap, "form": stringMap }` |
| `Response` | `{ "body": string, "contentType": string }` |

`stringMap` は **文字列キー → 文字列値** の動的オブジェクトを表す（`req.query.foo` の型は `string`）。この型は `Request` の `query` / `form` 専用で、型注釈で直接表現する構文はない。

例:

```
import { type Request, type Response, responseHtml } from "prelude";

function handleRoot(req: Request): Response {
  return responseHtml("<h1>Hello</h1>");
}
```

### 2.4 型のルール

- 異なる型の比較・暗黙変換はしない。
- `integer` と `float` の比較は **コンパイルエラー**。
- `parse` は文脈から戻り型を決定する（後述）。
- 配列とオブジェクトはすべてイミュータブルであり、生成後に要素を書き換える術は提供しない。

## 3. 変数

- すべて `const`。
- トップレベル変数は **型注釈必須**。
- ローカル変数は型推論により **型注釈省略可能**。

例:

```
// トップレベル（型注釈必須）
const x: integer = 1;
const s: string = "a";

function example(): void {
  // ローカル変数（型推論により省略可能）
  const y = 2;           // integer と推論
  const t = "hello";     // string と推論
  const arr = [1, 2, 3]; // integer[] と推論

  // 型注釈を明示することも可能
  const z: integer = 3;
}
```

### 3.1 for-of文での型推論

for-of文でも型推論が使用可能:

```
const nums: integer[] = [1, 2, 3];
for (const n of nums) {  // n は integer と推論
  print(n);
}
```

### 3.2 配列の分割代入（destructuring）

配列やタプルを分割して複数の変数に代入可能:

```
const arr: string[] = ["a", "b", "c"];
const [first, second, third] = arr;  // first="a", second="b", third="c"

// タプルでも使用可能
const tuple: [integer, string] = [1, "hello"];
const [num, str] = tuple;  // num=1, str="hello"

// 型注釈も可能（通常は不要）
const [x: string, y: string] = ["foo", "bar"];
```

### 3.3 オブジェクトの分割代入（destructuring）

オブジェクトのプロパティを分割して変数に代入可能:

```
const obj: { "name": string, "age": integer } = { "name": "Alice", "age": 30 };
const { name, age } = obj;  // name="Alice", age=30

// 型注釈も可能（通常は不要）
const { name: string, age: integer } = obj;
```

変数名はオブジェクトのキー名と一致する必要がある。キーのリネームはサポートしない。

## 4. 関数

- トップレベルでのみ宣言可能（高階関数やクロージャは不可）。
- 宣言構文: `function add(a: integer, b: integer): integer { return a + b; }`
- パラメータ型・戻り値型が必須。
- `export` を付ければ外部公開できる（`export function`）。
- 匿名関数 / ラムダ / アロー式は使用できない。コールバックにはトップレベルの関数を渡す。

例:

```
function double(n: integer): integer {
  return n * 2;
}

const nums: integer[] = [1, 2, 3];
const doubled: integer[] = nums.map(double);
```

### 4.1 メソッドスタイル呼び出し（ドット構文）

関数呼び出しの最初の引数をドットの前に置くシンタックスシュガーをサポート:

```
// 通常の呼び出し
addRoute(server, "/", handler);
listen(server, ":8888");

// メソッドスタイル呼び出し（等価）
server.addRoute("/", handler);
server.listen(":8888");
```

これは純粋なシンタックスシュガーであり、`obj.func(a, b)` は `func(obj, a, b)` と同等に扱われる。

`obj.func(a, b)`で`obj`オブジェクトの`func`プロパティが関数であっても、その関数を`obj.func(a, b)`という構文で呼ぶことはできない。括弧を追加して`(obj.func)(a, b)`のように書く必要がある。

## 5. 式と演算子

### 5.1 算術

- `+ - * / %`
- `integer` は整数演算、`float` は浮動小数点演算。
- `+` は **string + string のみ**連結。

### 5.2 比較

- `==` / `!=`
- `integer` / `float` / `boolean` / `string` / `array` / `object` に対応。
- 参照比較ではなく **値の比較**。

### 5.3 論理

- `&` / `|` のみ（`boolean`）

### 5.4 単項

- `+` / `-`

### 5.5 三項演算子

- `cond ? then : else`
- `cond` は `boolean` 型でなければならない。
- `then` と `else` は同じ型でなければならない。

例:

```
const status = completed == "1" ? "[x]" : "[ ]";
const abs = x < 0 ? -x : x;
```

### 5.6 switch式

Rustの`match`に似たパターンマッチング式。breakは不要。

```
switch (expr) {
  case pattern1:
    result1
  case pattern2:
    result2
  default:
    defaultResult
}
```

- 各caseは値を返す式を持つ
- 複数の文を実行する場合は `{ }` でブロックを囲む（void型）
- `default` は省略可能（値を返すswitch式では推奨）

例:

```
// 値を返すswitch式
const message = switch (status) {
  case 0: "pending"
  case 1: "completed"
  default: "unknown"
};

// void型（文を実行）
switch (cmd) {
  case "help":
    showUsage()
  case "version":
    showVersion()
  case "open": {
    if (argc >= 2) {
      openTodo(args[1]);
    } else {
      print("エラー");
    }
  }
  default:
    showUsage()
};
```

## 6. 文字列

- UTF-8。
- 連結は `string + string` のみ。
- 数値は `toString` で明示的に変換。

## 7. 配列 / タプル

- 配列リテラル: `[a, b, c]`
- 配列リテラルでは `...expr` で別の配列を展開できる。スプレッド先は配列でなければならず、要素型は揃っている必要がある。
- タプル型: `[integer, string]`
- 配列リテラルは要素型が揃わない場合、タプル型として推論される。
- インデックス: `arr[i]`（`i` は `integer`）
- `for (const x: T of arr)` で反復（配列のみ）。
- タプル型はインデックスアクセスで利用する。

## 8. オブジェクト

- キーは **文字列リテラルのみ**。
- アクセスは `obj.foo` のみ。
- イミュータブル。
- スプレッド: `{ ...obj, prop: value }`
- 比較はすべてのプロパティが `==` で等しいときに等しい。`stringify` は保持順で出力（`parse` で生成したオブジェクトはキー辞書順）。

## 9. 文

- `const` 宣言
- `if / else`
- `for (const x: T of arr)`
- `return`
- 式文

## 10. モジュール

- `import { foo } from "./mod";`
- `import { print } from "prelude";`
- `export const name = ...;`
- 相対パスは `.ts` を省略可能。

## 11. SQL

ソースコード内に直接SQLクエリを記述できる。Rustのsqlxライブラリに倣い、期待する結果に応じたキーワードを使用する。

### 11.1 クエリキーワード

| キーワード            | 用途                                           | 戻り値の型                           |
| --------------------- | ---------------------------------------------- | ------------------------------------ |
| `execute`             | 結果を返さないクエリ（INSERT, UPDATE, DELETE） | `void`                               |
| `fetch_one`           | 必ず1行を返すクエリ                            | `{ [column]: string }`               |
| `fetch_optional`      | 0または1行を返すクエリ                         | `{ [column]: string }` (または null) |
| `fetch` / `fetch_all` | 全行を返すクエリ                               | `{ [column]: string }[]`             |

### 11.2 構文

```
// 結果を返さないクエリ
execute {
  INSERT INTO users (name) VALUES ({name})
};

// 必ず1行を返すクエリ
const row = fetch_one {
  SELECT id, name FROM users WHERE id = 1
};

// 0または1行を返すクエリ
const maybeRow = fetch_optional {
  SELECT id, name FROM users WHERE id = {id}
};

// 全行を返すクエリ
const result = fetch_all {
  SELECT id, name FROM users ORDER BY id
};
```

### 11.3 例

```
// INSERTとlast_insert_rowid()の取得
execute {
  INSERT INTO users (name) VALUES ({name})
};
const { id } = fetch_one {
  SELECT last_insert_rowid() AS id
};
print("Created user #" + id);

// 全レコードの取得
const rows = fetch_all {
  SELECT id, name FROM users ORDER BY id
};
for (const row of rows) {
  const { id, name } = row;
  print(id + ": " + name);
}
```

### 11.4 戻り値の型

#### execute

`execute` は戻り値を返さない（`void`）。INSERT, UPDATE, DELETE などの変更クエリに使用する。

#### fetch_one

`fetch_one` は必ず1行を返す。行が存在しない場合はランタイムエラーになる。

```
const { id, name } = fetch_one { SELECT id, name FROM users WHERE id = 1 };
```

#### fetch_optional

`fetch_optional` は0または1行を返す。行が存在しない場合はnullを返す。

```
const row = fetch_optional { SELECT id, name FROM users WHERE id = {id} };
// rowがnullかどうかをチェックして使用
```

#### fetch / fetch_all

`fetch` と `fetch_all` は同じ動作で、各行のデータ（カラム名をキーとしたオブジェクト）の配列を返す。

各行のオブジェクトはSELECT文で指定したカラム名をキーとして持ち、値はすべて文字列として返される。

```
const rows = fetch_all { SELECT id, name FROM users };
// rows[0] は { "id": "1", "name": "Alice" } のようなオブジェクト
const { id, name } = rows[0];
```

### 11.5 データベースの永続化

`dbSave`関数でインメモリデータベースをファイルに保存できる:

```
import { dbSave } from "prelude";

dbSave("database.sqlite3");
```

### 11.6 パラメータ埋め込み

SQL文内で `{式}` 構文を使用して変数をクエリに埋め込める:

```
const title: string = "買い物";
execute {
  INSERT INTO todos (title) VALUES ({title})
};
```

これは内部的にパラメータ化されたクエリに変換され、SQLインジェクションを防ぐ。

### 11.7 テーブル定義

`create_table` キーワードでテーブルのスキーマを定義できる:

```
create_table todos {
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  title TEXT NOT NULL,
  completed INTEGER DEFAULT 0,
  created_at TEXT DEFAULT CURRENT_TIMESTAMP
}
```

テーブル定義には以下の効果がある:

1. **コンパイル時検証**: `execute`, `fetch_one`, `fetch_all` 等の SQL ブロック内で参照されるテーブル名とカラム名が `create_table` 定義と一致するか検証
2. **自動テーブル作成**: `dbOpen` 実行時に、テーブルが存在しない場合は自動的に作成
3. **スキーマ検証**: テーブルが存在する場合、カラム名と型が定義と一致するか検証（不一致の場合はエラー）
4. **行型エイリアスの自動生成**: テーブル名が行のオブジェクト型のエイリアスとして自動的に定義される。各カラムは `string` 型として扱われる

#### 行型エイリアスの使用例

```
create_table todos {
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  title TEXT NOT NULL,
  completed INTEGER DEFAULT 0
}

// 上記のテーブル定義により、以下の型エイリアスが自動定義される:
// type todos = { "id": string, "title": string, "completed": string }

function renderTodoRow(row: todos): JSX {
  return <li>{row.title}</li>;
}

function renderTodos(): JSX {
  const rows = fetch_all { SELECT id, title, completed FROM todos };
  // row の型として todos を使用可能
  return <ul>{rows.map(renderTodoRow)}</ul>;
}
```

### 11.8 制限事項

- インメモリSQLiteデータベースを使用
- 複数のSQLデータベース接続は未対応

## 12. prelude

組み込みパッケージ `prelude` を利用する。

### 12.1 関数

- `print(value: T): void`
  - 文字列はそのまま出力。
  - それ以外は `stringify` 相当で出力。
- `stringify(value: T): string`
- `parse(s: string): T`
  - 戻り型は **文脈から推論**。文脈がない場合はエラー。
  - JSON 数値は `1` なら `integer`、`1.0` や `1e3` は `float`。
- `toString(value: integer | float | boolean | string): string`
- `range(start: integer, end: integer): integer[]`
  - `start` 以上 `end` **以下**の連続した `integer` を格納した配列を返す（`range(2, 5)` は `[2, 3, 4, 5]`）。
- `length(array: T[]): integer`
  - 配列の長さを返す。
- `map(array: T[], fn: (value: T) => U): U[]`
  - 各要素に `fn` を適用した新しい配列を返す。
- `filter(array: T[], fn: (value: T) => boolean): T[]`
  - `fn` が `true` を返した要素のみを含む配列を返す。
- `reduce(array: T[], fn: (acc: R, value: T) => R, initial: R): R`
  - 配列を畳み込み、`fn` の戻り値を結果とする。
- `dbSave(filename: string): void`
  - インメモリSQLiteデータベースを指定したファイルに保存する。
- `dbOpen(filename: string): void`
  - 指定したSQLiteファイルをインメモリに読み込む。ファイルが存在しない場合は新規作成。`create_table` 定義がある場合、テーブルの自動作成と検証を行う。
- `getArgs(): string[]`
  - コマンドライン引数を配列として返す。

### 12.2 HTTPサーバー

組み込みHTTPサーバー機能を提供する。

#### 12.2.1 HTTPサーバー関数

- `createServer(): Server`
  - 新しいHTTPサーバーインスタンスを作成する。
- `addRoute(server: Server, path: string, handler: (req: Request) => Response): void`
  - サーバーに指定したパスのルートを追加する。ハンドラーはリクエストを受け取り、レスポンスを返す関数。
- `listen(server: Server, port: string): void`
  - サーバーを指定したポートで起動する。この関数はブロッキングで、サーバーが終了するまで戻らない。
- `responseText(text: string): Response`
  - テキストレスポンスを作成する。
- `getPath(req: Request): string`
  - リクエストのパスを取得する。
- `getMethod(req: Request): string`
  - リクエストのHTTPメソッド（GET, POSTなど）を取得する。

#### 12.2.2 型

- `Server`: HTTPサーバーインスタンス
- `Request`: HTTPリクエスト（`{ "path": string, "method": string, "query": stringMap, "form": stringMap }` オブジェクト）
- `Response`: HTTPレスポンス（`{ "body": string }` オブジェクト）

#### 12.2.3 使用例

```
import { print, createServer, addRoute, listen, responseText } from "prelude";

function handleHello(req: { "path": string, "method": string }): { "body": string } {
  return responseText("hello");
}

export function main(): void {
  const server = createServer();
  addRoute(server, "/hello", handleHello);
  print("Starting server on :8080");
  listen(server, ":8080");
}
```

`createServer` / `addRoute` / `listen` / `responseText` / `getPath` / `getMethod` を含むすべての関数は `import { ... } from "prelude";` で明示的にインポートする。

### 12.3 JSX構文

サーバーサイドレンダリング用のJSX構文をサポート。JSX要素は文字列に変換される。

#### 12.3.1 基本構文

```
// 単純な要素
<div>Hello World</div>

// セルフクロージングタグ
<br />
<img src="image.png" />

// ネスト
<html>
  <head>
    <title>My Page</title>
  </head>
  <body>
    <h1>Welcome</h1>
  </body>
</html>
```

#### 12.3.2 式の埋め込み

`{式}` 構文で文字列・数値・真偽値を埋め込める:

```
const title = "Hello";
const count = 42;

<div>
  <h1>{title}</h1>
  <p>Count: {count}</p>
</div>
```

#### 12.3.3 属性

文字列リテラルまたは式を属性値として使用できる:

```
// 文字列リテラル
<input type="text" placeholder="Enter name" />

// 式
const className = "highlight";
<div class={className}>Content</div>
```

#### 12.3.4 フラグメント

`<>...</>` でフラグメント（タグなしコンテナ）を作成できる:

```
<>
  <p>First paragraph</p>
  <p>Second paragraph</p>
</>
```

#### 12.3.5 responseHtmlとの使用

JSXは `responseHtml` 関数と組み合わせてHTMLレスポンスを返すのに最適:

```
import { createServer, addRoute, listen, responseHtml } from "prelude";

function handleRoot(req: { "path": string, "method": string }): { "body": string, "contentType": string } {
  const title = "Hello from Negitoro";
  return responseHtml(
    <html>
      <head>
        <meta charset="utf-8" />
        <title>{title}</title>
      </head>
      <body>
        <h1>{title}</h1>
      </body>
    </html>
  );
}

export function main(): void {
  const server = createServer();
  addRoute(server, "/", handleRoot);
  listen(server, ":8888");
}
```

#### 12.3.6 制限事項

- サーバーサイドレンダリング専用（クライアントサイドJavaScriptは生成されない）
- イベントハンドラー（onClick等）は未対応
- コンポーネント（大文字で始まるタグ）は未対応
- JSX式内でオブジェクト・配列は埋め込み不可（プリミティブ型のみ）

## 13. 実行

- コンパイラは WAT を生成し、wasmtime-go の `Wat2Wasm` で WASM を生成。
- 実行は同梱 CLI の `run` で行う。
- **CGO と C コンパイラが必要**（wasmtime-go が C 依存）。

## 14. 制限事項

- 代入・再代入は不可。
- 高階関数・クロージャは不可。
- `null` / `undefined` は未対応。
- JSON の `null` は `parse` でエラー。
