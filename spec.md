# 言語仕様（TunaScript → WASM）

## 1. 概要

TunaScript（通称:tuna）はTypeScriptの最小サブセットです。以下の制約のもとでコンパイルと実行を行います。

- クラス / インターフェイス / 名前空間 / this は不使用。
- 代入はなく、`const` のみ。
- 関数は `function` 宣言のみ（トップレベルのみ）。
- 繰り返しは `for (const x: T of arr)` のみ。
- ESModule 形式の `import` / `export`。
- バックエンドは WAT を生成し、wasmtime-go の `Wat2Wasm` で WASM へ変換。

## 2. 型

### 2.1 プリミティブ

- `integer`（TunaScript が TypeScript に追加した整数型）
- `number`（浮動小数点。TypeScript の標準 `number` 型）
- `boolean`（`true` / `false`）
- `string`（UTF-8）
- `void`

TunaScript では浮動小数点を扱う型の名前は `number` であり、`float` という名前は使いません。`integer` はこれに対する整数専用の型として追加されています。

### 2.2 複合型

- 配列: `T[]`（`Array<T>` のエイリアス。たとえば `string[]` は `Array<string>` と同じです）
- タプル: `[T1, T2, ...]`
- オブジェクト: `{ "key": T, ... }`
- 関数型: `(a: T, b: U) => R`
- マップ: `Map<T>`（文字列キー → 値 `T` の動的オブジェクト。`Request` の `query` / `form` で使います）
- オブジェクトリテラルでは `foo: value` のような識別子キーも使え、識別子は暗黙的に文字列キーとして扱われるため `Map<T>` を構築する際にも便利です。各プロパティの値は `T` に代入可能である必要があります。

型変数（`T`, `U`, `S` など）を使うと汎用的な型を表現できます。たとえば `Array<T>` や `Map<T>`、`(x: T) => S` のように、関数や型エイリアスで個々の型を後から埋められる柔軟な型を記述できます。関数型式ではパラメータ一覧の手前に `<T, U>` のような型パラメータリストを置いて、その式の中で型変数を参照できます。たとえば `const foo: <T>(arr: T[], fn: (n: T) => T) => T[] = map;` のように書いて、`map` などの汎用関数を値として扱うときの型を明示できます。型パラメータはその式のスコープ内でのみ有効です。

### 2.3 Union型

- `T | U` で **Union型** を表す。
- Union型の値は `T` または `U` のいずれか。

例:

```
const v: integer | string = 42;
```

Union型の値を取り出すには `switch` 式の `case v as T` を使う（5.6参照）。

### 2.4 型エイリアス

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

| 型名   | 定義       |
| ------ | ---------- |
| `JSX`  | `string`   |

そのほか、`Map<T>` は **文字列キー → 値 `T`** の動的オブジェクトを表し、`req.query.foo` や `req.form.bar` のように自由にアクセスできます。`Map<T>` を使うことで汎用的なオブジェクトやプロパティ型を記述できます。

例:

```
import { type JSX } from "prelude";
import { responseHtml } from "http";

function handleRoot(): JSX {
  return responseHtml("<h1>Hello</h1>");
}
```

### 2.5 型のルール

- 異なる型の比較・暗黙変換はしない。
- `integer` と `number` の比較は **コンパイルエラー**。
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
  log(n);
}
```

オブジェクトや配列の分割代入もループ変数として使えます。たとえば `fetch_all` でテーブル行の配列を取得した結果は `{ [column]: string }[]` なので、`for (const { post_id, post_title, author_name } of rows)` のように必要なプロパティを展開して直接使えます。配列／タプルを反復する場合は `for (const [first, second] of pairs)` と書いて複数の要素を同時に分解できます。

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

- 関数はトップレベルでのみ宣言可能。高階関数やクロージャの構文は用意していませんが、コールバックには他の式の中で定義する関数リテラルを渡せます（後述）。
- 宣言構文: `function add(a: integer, b: integer): integer { return a + b; }`
- 関数宣言ではパラメータ型・戻り値型の注釈が必須です。関数リテラルは文脈（たとえば `map` / `filter` / `reduce` の期待型）から型を推論できる場合にのみ省略可能です。
- `export` を付ければ外部公開できる（`export function`）。

例:

```
function double(n: integer): integer {
  return n * 2;
}

const nums: integer[] = [1, 2, 3];
const doubled: integer[] = nums.map(double);
```

### 4.1 関数リテラル

関数リテラルは `function(arg: T): R { ... }` という形式で式の中に書ける匿名の関数です。戻り型注釈は `: R` で、本文はブロックまたは `=>` を使った1行の式から構成できます。たとえば `function (value) { return value * 2; }` や `function (value) => value * 2` のように呼び出しの場で手軽にコールバックを定義できます。引数や戻り値の型は文脈から推論されるので、省略することもできます。文脈がない場合は型注釈を追加してください。

関数リテラルの本体はモジュールスコープで型チェックされるため、ローカル変数を捕捉するクロージャとしては振る舞いません。たとえば `map` や `filter`、`reduce` などの組み込み関数に渡す `function` リテラルは引数の型と戻り値の型が既知なので、以下のように書いて型注釈を省略できます:

```
const nums: integer[] = [1, 2, 3, 4];
const doubled = map(nums, function (value) {
  return value * 2;
});
const evens = filter(nums, function (value) {
  return value % 2 == 0;
});
const total = reduce(nums, function (acc, value) {
  return acc + value;
}, 0);
```

この例では `map` が `(value: integer) => U` を期待しているため、`value` は `integer` と推論され、戻り値も自動的に `integer` になります。`reduce` では累積値 `acc` の型として初期値 `0` の型 (`integer`) が知られているので、パラメータと戻り値の型がすべて推論されます。

### 4.2 メソッドスタイル呼び出し（ドット構文）

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
- `integer` は整数演算、`number` は浮動小数点演算。
- `+` は **string + string のみ**連結。

### 5.2 比較

- `==` / `!=`
- `integer` / `number` / `boolean` / `string` / `array` / `object` に対応。
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
- Union型の分岐は `case v as T:` を使う
  - `v` は `switch` の値と同じ識別子でなければならない
  - `case v as T` の中では `v` は `T` として扱われる

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
      log("エラー");
    }
  }
  default:
    showUsage()
};

// Union型の分岐
const v: integer | string = 42;
const message = switch (v) {
  case v as integer: "v is integer"
  case v as string: "v is string"
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
- `for (const { prop } of arr)` / `for (const [first, second] of arr)`（オブジェクトやタプルの分割代入に対応）
- `return`
- 式文

## 10. モジュール

- `import { foo } from "./mod";`
- `import { log } from "prelude";`
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
log("Created user #" + id);

// 全レコードの取得
const rows = fetch_all {
  SELECT id, name FROM users ORDER BY id
};
for (const row of rows) {
  const { id, name } = row;
  log(id + ": " + name);
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

`sqlite` モジュールの `dbOpen` は指定したファイルを直接開き、すべての変更（`execute` や `INSERT` / `UPDATE` など）はそのままファイル上に反映されるため、`dbSave` のような追加の保存処理は不要です。ファイルが存在しない場合は自動作成され、`create_table` 定義に従ってテーブルの生成・検証が行われます。プログラム開始後（`main` の呼び出し直前）には `":memory:"` が自動的に開かれ、テーブル定義に従って内蔵のメモリデータベースが初期化されるため、テストコードで明示的に `dbOpen(":memory:")` を呼ぶ必要はありません。任意のファイルを使いたい場合やデータベースを切り替えたい場合は `import { dbOpen } from "sqlite";` して新しいパスを指定すれば、既存の接続は閉じられてから新しい接続が開始されます。

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

- 単一のSQLiteファイル接続しか提供していないため、並列プロセスや複数の接続から同時に書き込むケースではSQLiteのロックに注意する必要がある。
- 複数のSQLデータベース接続は未対応

## 12. prelude

組み込みパッケージ `prelude` を利用する。

### 12.1 関数

- `log(value: T): void`
  - 文字列はそのまま出力。
  - それ以外は `stringify` 相当で出力。
- `stringify(value: T): string`
- `parse(s: string): T`
  - 戻り型は **文脈から推論**。文脈がない場合はエラー。
  - JSON 数値は `1` なら `integer`、`1.0` や `1e3` は `number`。
- `toString(value: integer | number | boolean | string): string`
- `range(start: integer, end: integer): integer[]`
  - `start` 以上 `end` **以下**の連続した `integer` を格納した配列を返す（`range(2, 5)` は `[2, 3, 4, 5]`）。
- `length(array: T[]): integer`
  - 配列の長さを返す。
- `map<T, S>(fn: (value: T) => S, xs: Array<T>): Array<S>`
-  - 型変数 `T` / `S` を使って `xs` の各要素を `fn` で変換します。
- `filter<T>(fn: (value: T) => boolean, xs: Array<T>): Array<T>`
-  - `fn` が `true` を返した要素のみを残します。
- `reduce<T, R>(fn: (acc: R, value: T) => R, xs: Array<T>, initial: R): R`
-  - `fn` による畳み込みの結果（型 `R`）を返します。`initial` は初期累積値です。
- `getArgs(): string[]`
  - コマンドライン引数を配列として返す。

### 12.1.1 sqliteモジュール

`sqlite` モジュールは `dbOpen` を含む SQLite 固有の関数を提供します。

- `dbOpen(filename: string): void`
  - 指定したSQLiteファイルを直接開く。ファイルが存在しない場合は新規作成され、書き込みはそのままファイルに反映される。`create_table` 定義がある場合、テーブルの自動作成と検証が行われます。
  - この関数は `import { dbOpen } from "sqlite";` でインポートしてください。

### 12.2 HTTPサーバー

組み込みHTTPサーバー機能を提供する。

#### 12.2.1 HTTPサーバー関数

これらの関数（`responseHtml`, `responseJson`, `responseRedirect` などを含む）は `http` モジュールから `import { ... } from "http";` で明示的にインポートしてください。

- `createServer(): Server`
  - 新しいHTTPサーバーインスタンスを作成する。
- `addRoute(server: Server, path: string, handler: (req: Request) => Response): void`
  - サーバーに指定したパスのルートを追加する。ハンドラーはリクエストを受け取り、レスポンスを返す関数。
- `listen(server: Server, port: string): void`
  - サーバーを指定したポートで起動する。この関数はブロッキングで、サーバーが終了するまで戻らない。
- `responseText(text: string): Response`
  - テキストレスポンスを作成する。
- `responseHtml(html: string): Response`
  - HTML を直接返す `contentType: "text/html; charset=utf-8"` のレスポンスを作成。
- `responseJson(data: string): Response`
  - JSON ボディを返すレスポンス（`contentType` は `application/json`）。
- `responseRedirect(url: string): Response`
  - `Location` ヘッダーと `302 Found` をセットしたリダイレクトを作成。
- `getPath(req: Request): string`
  - リクエストのパスを取得する。
- `getMethod(req: Request): string`
  - リクエストのHTTPメソッド（GET, POSTなど）を取得する。

- HTTPハンドラーは `addRoute` で登録されると、`listen` から実行されるたびに自動的にSQLiteトランザクション (`BEGIN ... COMMIT/ROLLBACK`) 内で動作します。ハンドラーが正常に戻るとコミットされ、ハンドラーの呼び出し中にエラーが発生した場合はロールバックされて変更は破棄されます。
- このトランザクションの性質上、HTTPリクエストは1つずつ順番に処理され、同時リクエストは順番待ちになります。

#### 12.2.2 型

- `Server`: HTTPサーバーインスタンス
- `Request`: HTTPリクエスト（`{ "path": string, "method": string, "query": Map<string>, "form": Map<string> }` オブジェクト）
- `Response`: HTTPレスポンス（`{ "body": string }` オブジェクト）

これらの型は `http` モジュールからエクスポートされる型エイリアスなので、必要に応じて `import { type Request, type Response } from "http";` で明示的にインポートしてください。

#### 12.2.3 使用例

```
import { log, responseText } from "prelude";
import { createServer, addRoute, listen, type Request, type Response } from "http";

function handleHello(req: Request): Response {
  return responseText("hello");
}

export function main(): void {
  const server = createServer();
  addRoute(server, "/hello", handleHello);
  log("Starting server on :8080");
  listen(server, ":8080");
}
```

`createServer` / `addRoute` / `listen` は `import { ... } from "http";` で明示的にインポートし、`responseText` / `getPath` / `getMethod` は `prelude` からインポートしてください。

### 12.3 JSX構文

サーバーサイドレンダリング用のJSX構文をサポート。JSX要素は文字列に変換される。

- JSX は `prelude` がエクスポートする `type JSX = string` のエイリアスです。JSX を返す関数やカスタムコンポーネントのプロパティでこの型を使う場合は `import { type JSX } from "prelude";` で明示的にインポートしてください。

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
import { responseHtml } from "http";
import { createServer, addRoute, listen, type Request, type Response } from "http";

function handleRoot(req: Request): Response {
  const title = "Hello from TunaScript";
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
- カスタムコンポーネント（大文字で始まるタグ）は 12.3.7 のルールに従って関数呼び出しに変換される
- JSX式内でオブジェクト・配列は埋め込み不可（プリミティブ型のみ）

#### 12.3.7 カスタムコンポーネント

大文字で始まるJSXタグはカスタムコンポーネントとして扱われ、同名のトップレベル関数への呼び出しに変換されます。この関数は単一のオブジェクト引数（プロパティオブジェクト）を受け取り、`JSX`（文字列の別名）を返す必要があります。変換ルールは以下のとおりです。

- JSXの属性はプロパティオブジェクトのフィールドになります。属性値には文字列リテラルまたはプリミティブ（文字列、整数、浮動小数点、真偽値）を返す式しか使えず、対応するプロパティ型と一致する必要があります。対応するプロパティが明示的に定義されていない場合、`Map<string>` のようなインデックスシグネチャを通じて許可されていない限りコンパイルエラーになります。
- `<CustomComponent>` のようにネストしたJSXがあると、それらは文字列として結合され、`children` プロパティに渡されます。コンポーネントのプロパティ型が `children`（またはインデックスで任意の名前）を受け取るように定義されていない場合は、ネストされたJSXはエラーになります。`children` プロパティがある場合、子要素がないときは空文字列が渡されるため常に安全に扱えます。
- カスタムコンポーネントの関数が返す文字列は通常のJSXと同様に埋め込まれるため、`responseHtml` などで結果を合成できます。
なお、カスタムコンポーネントが返す `JSX` 文字列は周囲の JSX 子要素と同様にその場で連結されるため、通常のタグとまったく同じように出力されます。
- プロパティが全く必要ないコンポーネント関数は引数を省略して定義できます。この場合、JSX 側で属性や子要素を与えるとコンパイルエラーになります。

```
function Layout(props: { "title": string, "children": JSX }): JSX {
  return <section><h1>{props.title}</h1>{props.children}</section>;
}

function Page(): JSX {
  return (
    <div>
      <Layout title="Hello from TunaScript">
        <p>Welcome!</p>
      </Layout>
    </div>
  );
}
```

## 13. 実行

- コンパイラは WAT を生成し、wasmtime-go の `Wat2Wasm` で WASM を生成。
- 実行は同梱 CLI の `run` で行う。
- **CGO と C コンパイラが必要**（wasmtime-go が C 依存）。

## 14. 制限事項

- 代入・再代入は不可。
- 高階関数・クロージャは不可。
- `null` / `undefined` は未対応。
- JSON の `null` は `parse` でエラー。
