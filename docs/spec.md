# TunaScript 言語仕様

## 1. 概要

TunaScriptは以下のようなコンセプトを持ったプログラミング言語です。

- 静的型付けです。トップレベルの関数は型注釈が必須で、ローカル変数は型注釈を省略できます。
- 構文はごくシンプルで、大半はTypeScriptに似せてあります。
- SQLをソースコード中にそのまま記述でき、型も自動で推論されます。
- JSXのサブセットをサポートしており、ソースコード中にそのまま記述できます。
- WASMへとコンパイルされます。
- データ型がJSONフレンドリーです。データ型を定義すると、自動的にJSONデコーダーも生成されます
- ランタイムは`wasmtime-go`をベースにして独自インポート関数を追加したものです。

## 2. 型

### 2.1 プリミティブ

- `null`
- `undefined`
- `boolean`
- `integer`
- `number`
- `string`
- `json`
- `void`

`json` は任意のJSON値を表すプリミティブ型です。`json` はUnion型ではないため、`switch` 式の `case v as T` による型の絞り込み（値の取り出し）はできません。

`undefined` は「値が存在しない」ことを表すプリミティブ型です。`undefined` はリテラルとして `undefined` と書け、`==` / `!=` で比較できます。

`decode<T>` でオブジェクト型をデコードする際、プロパティ型に `undefined` を含めることでそのプロパティを optional として扱います（例: `{ name: string | undefined }`）。JSON側にそのキーが無い場合、デコードは成功し、そのプロパティ値は `undefined` になります。

`stringify` でオブジェクトをJSONに変換する際、値が `undefined` のプロパティは出力されません（例: `{ name: undefined, x: 1 }` は `{"x":1}` になります）。


### 2.2 複合型

- 配列: `T[]`（`Array<T>` のエイリアス。たとえば `string[]` は `Array<string>` と同じです）
- タプル: `[T1, T2, ...]`
- オブジェクト: `<T>{ key: T, ... }`
- 関数型: `<T>(a: T, b: U) => R`
- マップ: `Map<T>`（文字列キー → 値 `T` の動的オブジェクト。`Request` の `query` / `form` で使います）

オブジェクト型のプロパティ名は、識別子（例: `name`）または文字列リテラル（例: `"name"`）で書けます。識別子の場合は `"..."` を省略できます（例: `{ name: string }`）。

### 2.3 Union型

- `T | U` で **Union型** を表します。
- Union型の値は `T` または `U` のいずれかです。

例:

```typescript
const v: integer | string = 42;
```

Union型の値を取り出すには `switch` 式の `case v as T` を使います（5.7参照）。

### 2.4 型エイリアス

TypeScriptと同様の構文で型に別名を付けることができます。

```typescript
type MyType = { name: string, age: integer };
export type Response = { body: string, contentType: string };
```

- `type Name = TypeExpr;` で型エイリアスを定義できます。
- `export` を付ければモジュール外に公開できます。
- 他のモジュールから `import { type TypeName } from "module";` でインポートできます。
- 型をインポートするときは必ず `type` キーワードを付ける必要があります。
- 型エイリアスは型注釈で使用できます。
- 型エイリアスには型パラメータを `<T>` 形式で付けられ、型式の中でそのパラメータを参照することで汎用的な別名を定義できます。たとえば `Result<T>` は次のように書けます:

```typescript
type Error = { type: "Error", message: string }
type Result<T> = T | Error
```

このようなユニオンを型エイリアスにまとめておくと、`Result<string>` のように使い回せます。`prelude` でも `Error` / `Result` を用意しており、`import { type Result, type Error } from "prelude";` のように取り込みできます。

#### preludeの型エイリアス

preludeには以下の型エイリアスが定義されている:

| 型名        | 定義                                                                   |
| ----------- | ---------------------------------------------------------------------- |
| `JSX`       | `string`                                                               |
| `Error`     | `{ type: "Error", message: string }`                                   |
| `Result<T>` | `T \| Error`                                                           |

そのほか、`Map<T>` は **文字列キー → 値 `T`** の動的オブジェクトを表し、`req.query.foo` や `req.form.bar` のように自由にアクセスできます。`Map<T>` を使うことで汎用的なオブジェクトやプロパティ型を記述できます。

例:

```typescript
import { type JSX } from "prelude";
import { responseHtml } from "http";

function handleRoot(): JSX {
  return responseHtml("<h1>Hello</h1>");
}
```

`Result<T>` は `T | Error` のユニオンで、`switch` の `case ... as T` で分岐できます（`T` は型名でも型式でも構いません）。

```typescript
import { log, type Result, type Error } from "prelude";

const response: Result<string> = "ready";
const message = switch (response) {
  case value as string: value
  case { message } as Error: message
};
log(message);
```

#### リテラル型

リテラル型は特定の値だけを許す型で、文字列リテラル、整数、浮動小数点、真偽値のリテラルをそのまま型として書けます。たとえば `status` を `"error"` に限定することで、コードの意図が明示的になります:

```typescript
const status: "error" = "error";
```

リテラル型はその値そのものしか代入できないため、`string` や `integer` などの汎用的な型からの代入はエラーになります。逆に、リテラル型はより広い型（`string` / `integer` / `boolean` / `number`）には代入可能なので、タグ付きユニオンの `"type"` プロパティに使うと `switch` での絞り込みが強力になります。

`null` もリテラル型と考えられ、型名 `null` はただ1つの値 `null` を許します。`const missing: null = null;` のように書くことで、`null` 以外の値の代入はコンパイル時エラーになります。`null` は `RowType | null` のように Union と組み合わせてオプショナルな値を表現するのにも便利です。

### 2.5 型のルール

- 異なる型の比較・暗黙変換は行いません。
- `integer` と `number` の比較は **コンパイルエラー** になります。
- `parse` は `string` をJSONとしてパースして `json` を返します（組み込みライブラリ参照）。
- 配列とオブジェクトはすべてイミュータブルであり、生成後に要素を書き換える術は提供しません。

## 3. 変数

- すべて `const` です。
- トップレベル変数は **型注釈必須** です。
- ローカル変数は型推論により **型注釈省略可能** です。

例:

```typescript
// トップレベル（型注釈必須）
const x: integer = 1;
const s: string = "a";

function example(): void {
  // ローカル変数（型推論により省略可能）
  const y = 2; // integer と推論
  const t = "hello"; // string と推論
  const arr = [1, 2, 3]; // integer[] と推論

  // 型注釈を明示することも可能
  const z: integer = 3;
}
```

### 3.1 for-of文での型推論

for-of文でも型推論が使用可能です:

```typescript
const nums: integer[] = [1, 2, 3];
for (const n of nums) {
  // n は integer と推論
  log(n);
}
```

オブジェクトや配列の分割代入もループ変数として使えます。たとえば `fetch_all` でテーブル行の配列を取得した結果は `{ [column]: string }[]` なので、`for (const { post_id, post_title, author_name } of rows)` のように必要なプロパティを展開して直接使えます。配列／タプルを反復する場合は `for (const [first, second] of pairs)` と書いて複数の要素を同時に分解できます。

### 3.2 配列の分割代入（destructuring）

配列やタプルを分割して複数の変数に代入できます:

```typescript
const arr: string[] = ["a", "b", "c"];
const [first, second, third] = arr;  // first="a", second="b", third="c"

// タプルでも使用可能
const tuple: [integer, string] = [1, "hello"];
const [num, str] = tuple;  // num=1, str="hello"

// 型注釈も可能（通常は不要です）
const [x: string, y: string] = ["foo", "bar"];
```

### 3.3 オブジェクトの分割代入（destructuring）

オブジェクトのプロパティを分割して変数に代入できます:

```typescript
const obj: { name: string; age: integer } = { name: "Alice", age: 30 };
const { name, age } = obj; // name="Alice", age=30

// 型注釈も可能（通常は不要です）
const { name: string, age: integer } = obj;
```

変数名はオブジェクトのキー名と一致する必要があります。キーのリネームはサポートしません。

## 4. 関数

- 宣言構文: `function add(a: integer, b: integer): integer { return a + b; }`
- 関数宣言ではパラメータ型・戻り値型の注釈が必須です。関数リテラルは文脈（たとえば `map` / `filter` / `reduce` の期待型）から型を推論できる場合にのみ省略可能です。
- `export` を付ければ外部公開できます（`export function`）。

例:

```typescript
function double(n: integer): integer {
  return n * 2;
}

const nums: integer[] = [1, 2, 3];
const doubled: integer[] = nums.map(double);
```

### 4.1 関数リテラル

関数リテラルは `function(arg: T): R { ... }` という形式で式の中に書ける匿名の関数です。引数や戻り値の型は文脈から推論されるので、省略することもできます。文脈がない場合は型注釈を追加してください。

関数リテラルの本体はモジュールスコープで型チェックされるため、ローカル変数を捕捉するクロージャとしては振る舞いません。たとえば `map` や `filter`、`reduce` などの組み込み関数に渡す `function` リテラルは引数の型と戻り値の型が既知なので、以下のように書いて型注釈を省略できます:

```typescript
const nums: integer[] = [1, 2, 3, 4];
const doubled = map(nums, function (value) {
  return value * 2;
});
const evens = filter(nums, function (value) {
  return value % 2 == 0;
});
const total = reduce(
  nums,
  function (acc, value) {
    return acc + value;
  },
  0,
);
```

この例では `map` が `(value: integer) => U` を期待しているため、`value` は `integer` と推論され、戻り値も自動的に `integer` になります。`reduce` では累積値 `acc` の型として初期値 `0` の型 (`integer`) が知られているので、パラメータと戻り値の型がすべて推論されます。

### 4.2 メソッドスタイル呼び出し（ドット構文）

関数呼び出しの最初の引数をドットの前に置くシンタックスシュガーをサポート:

```typescript
// 通常の呼び出し
addRoute(server, "/", handler);
listen(server, ":8888");

// メソッドスタイル呼び出し（等価）
server.addRoute("/", handler);
server.listen(":8888");
```

これは純粋なシンタックスシュガーであり、`obj.func(a, b)` は `func(obj, a, b)` と同等に扱われます。

`const obj: { func: (a: integer) => void }`であっても、その関数を`obj.func(a)`という構文で呼ぶことはできません。括弧を追加して`(obj.func)(a)`のように書く必要があります。

### 4.3 型引数付き関数呼び出し

一部の関数は型引数を受け取ります。型引数は `<...>` を関数名の直後に書き、通常の呼び出しと同様に引数リスト `(...)` を続けます。

```typescript
const v = decode<Person>(jsonValue);
```

- `func<T>(...)` の `T` を **型引数** と呼びます。
- 現時点では、型引数付き呼び出しは `decode` など一部の組み込み関数でのみ使用できます。ユーザー定義の関数呼び出しに型引数を付けることはできません（コンパイルエラー）。

## 5. 式と演算子

### 5.1 算術

- `+ - * / %`
- `integer` は整数演算、`number` は浮動小数点演算です。
- `+` は **string + string のみ**連結になります。

### 5.2 比較

- `==` / `!=`
- `integer` / `number` / `boolean` / `string` / `array` / `object` / `null` / `undefined` に対応します。
- 参照比較ではなく **値の比較** になります。

### 5.3 論理

- `&` / `|` のみ（`boolean`）

### 5.4 単項

- `+` / `-`

### 5.5 if式

- `if (cond) { then } else { else }`
- `cond` は `boolean` 型でなければなりません。
- `if` は **値を返す式** です。
  - `else` を省略した場合、`cond` が `false` のときは `undefined` を返します。
  - 型は `then` と `else` の **Union型** になります（`else` 省略時は `then | undefined`）。
  - `{ }` ブロックに複数の文がある場合は **最後の文の値** がそのブロック全体の値になります（最後が式文でない場合は `undefined`）。

例:

```typescript
const status = if (completed == "1") { "[x]" } else { "[ ]" };
const abs = if (x < 0) { -x } else { x };

const v: boolean = true;
const a: integer | undefined = if (v) { 42 };
const b: integer | string = if (v) { 42 } else { "42" };

const c: integer = if (v) {
  const base: integer = 40;
  base + 2
} else {
  0
};
```

### 5.6 Result展開演算子 `?`

`expr?` は `expr` が `Result<T>`（=`T | Error`）のときに使える省略記法です。

- `expr` が成功値（`T`）なら、その値を返します。
- `expr` が `Error` なら、現在の関数から即座にその `Error` を返します。
- そのため `?` を使う関数の戻り値型は `Error` を含む必要があります（例: `Result<T>`）。

例:

```typescript
import { type Result } from "prelude";

function first(xs: integer[]): Result<integer> {
  const value: integer = xs[0]?;
  return value;
}
```

### 5.7 switch式

Rustの`match`に似たパターンマッチング式。`break`は不要です。

```typescript
switch (expr) {
  case pattern1:
    result1;
  case pattern2:
    result2;
  default:
    defaultResult;
}
```

- 各caseは値を返す式を持ちます。
- 複数の文を実行する場合は `{ }` でブロックを囲みます（ブロックの最後が式文ならその値、そうでなければ `void`）。
- `default` は省略可能です（値を返すswitch式では推奨します）。
- Union型の分岐は `case pattern as T:` を使います。
  - `pattern` は束縛パターンです（例: `name`, `{ prop }`, `[a, b]`）
  - `T` は型名（型エイリアス）でも型式でも構いません（例: `Error`, `string`, `{ type: "Error", message: string }`）
  - `case name as T` の中では `name` は `T` として扱われます（`name` は任意の識別子で構いません）
  - `case { prop } as T` は `T` に絞り込んだ後、オブジェクトのプロパティ `prop` を変数 `prop` に束縛します
  - `case [a, b] as T` は `T` に絞り込んだ後、配列/タプルの要素を変数 `a`, `b` に束縛します
- TunaScriptはTypeScriptのような制御フロー解析による型の絞り込みを行いません。Union型（`Result<T>` など）は必ず `switch` 式で絞り込みを行ってください。

例:

```typescript
// ❌ if による型の絞り込みはできません（コンパイルエラー）
const raw: Result<string> = { "type": "Error", "message": "oops" };
if (raw["type"] == "Error") {
  log(raw["message"]);
} else {
  log(raw);
}
```

```typescript
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
  case n as integer: "v is integer: " + toString(n)
  case s as string: "v is string: " + s
};
```

### 5.8 テンプレートリテラル

バッククオート `` `...` `` でテンプレートリテラルを書けます。テンプレートリテラルは複数行文字列をそのまま記述でき、`${expr}` で式を埋め込めます。

```typescript
const name = "Tuna";
const count = 3;
const msg = `Hello, ${name}! count=${count}`;

const multi = `line1
line2`;
```

- タグ付きテンプレート（`tag\`...\``）はサポートしません。
- `${expr}` に埋め込めるのは `string` / `integer` / `number` / `boolean`（およびそれらのUnion）です。
- 埋め込み式は `toString` 相当で文字列化されます。

## 6. 文字列

- UTF-8 です。
- 連結は `string + string` のみです。
- 数値は `toString` で明示的に変換します。
- テンプレートリテラル（`` `...${expr}...` ``）を使うと、複数行文字列と埋め込みが書けます。

## 7. 配列 / タプル

- 配列リテラル: `[a, b, c]` です。
- 配列リテラルでは `...expr` で別の配列を展開できます。スプレッド先は配列でなければならず、要素型は揃っている必要があります。
- タプル型: `[integer, string]` です。
- 配列リテラルは要素型が揃わない場合、タプル型として推論されます。
- インデックス: `arr[i]`（`i` は `integer`）です。戻り値は `Result<T>`（要素型 `T` と `Error` のUnion）になります。
- `arr[i]?` を使うと成功時は `T`、失敗時はその `Error` を関数から返せます。
- `for (const x: T of arr)` で反復（配列のみ）します。
- タプル型はインデックスアクセスで利用します。

## 8. オブジェクト

- キーは **文字列リテラルのみ** です。
- アクセスは `obj.foo` のみです。
- イミュータブルです。
- スプレッド: `{ ...obj, prop: value }` です。
- 比較はすべてのプロパティが `==` で等しいときに等しいです。`stringify` は保持順で出力します（`parse` で生成したオブジェクトはキー辞書順です）。

## 9. 文

- `const` 宣言です。
- `if / else` です。
- `for (const x: T of arr)` です。
- `for (const { prop } of arr)` / `for (const [first, second] of arr)`（オブジェクトやタプルの分割代入に対応）です。
- `return` です。
- 式文です。

## 10. モジュール

- `import { foo } from "./mod";` です。
- `import { log } from "prelude";` です。
- `export const name = ...;` です。
- 相対パスは `.ts` を省略可能です。

## 11. SQL

ソースコード内に直接SQLクエリを記述できます。Rustのsqlxライブラリに倣い、期待する結果に応じたキーワードを使用します。

### 11.1 クエリキーワード

| キーワード            | 用途                                           | 戻り値の型                           |
| --------------------- | ---------------------------------------------- | ------------------------------------ |
| `execute`             | 結果を返さないクエリ（INSERT, UPDATE, DELETE） | `void`                               |
| `fetch_one`           | 必ず1行を返すクエリ                            | `{ [column]: string }`               |
| `fetch_optional`      | 0または1行を返すクエリ                         | `{ [column]: string }` (または null) |
| `fetch` / `fetch_all` | 全行を返すクエリ                               | `{ [column]: string }[]`             |

### 11.2 構文

```typescript
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

```typescript
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

`execute` は戻り値を返さない（`void`）です。INSERT, UPDATE, DELETE などの変更クエリに使用します。

#### fetch_one

`fetch_one` は必ず1行を返します。行が存在しない場合はランタイムエラーになります。

```typescript
const { id, name } = fetch_one { SELECT id, name FROM users WHERE id = 1 };
```

#### fetch_optional

`fetch_optional` は0または1行を返します。行が存在しない場合はnullを返します。

```typescript
const row = fetch_optional { SELECT id, name FROM users WHERE id = {id} };
// rowがnullかどうかをチェックして使用
```

型システム上では戻り値が `{ [column]: string } | null` (行型と `null` の Union 型) になります。null が返るケースには明示的にチェックを入れてください。

#### fetch / fetch_all

`fetch` と `fetch_all` は同じ動作で、各行のデータ（カラム名をキーとしたオブジェクト）の配列を返します。

各行のオブジェクトはSELECT文で指定したカラム名をキーとして持ち、値はすべて文字列として返されます。

```typescript
const rows = fetch_all { SELECT id, name FROM users };
// rows[0] は { id: "1", name: "Alice" } のようなオブジェクト
const { id, name } = rows[0];
```

### 11.5 データベースの永続化

- SQLiteを内蔵しています。デフォルトではインメモリーデータベースが自動で開かれますが、`dbOpen`でデータベースファイルを開くこともできます。
- 単一のSQLiteファイル接続しか提供していないため、並列プロセスや複数の接続から同時に書き込むケースではSQLiteのロックに注意する必要があります。
- 複数のSQLデータベース接続は未対応

### 11.6 パラメータ埋め込み

SQL文内で `{式}` 構文を使用して変数をクエリに埋め込めます:

```typescript
const title: string = "買い物";
execute {
  INSERT INTO todos (title) VALUES ({title})
};
```

これは内部的にパラメータ化されたクエリに変換され、SQLインジェクションを防ぎます。

### 11.7 テーブル定義

`create_table` キーワードでテーブルのスキーマを定義できます:

```typescript
create_table todos {
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  title TEXT NOT NULL,
  completed INTEGER DEFAULT 0,
  created_at TEXT DEFAULT CURRENT_TIMESTAMP
}
```

テーブル定義には以下の効果があります:

1. **コンパイル時検証**: `execute`, `fetch_one`, `fetch_all` 等の SQL ブロック内で参照されるテーブル名とカラム名が `create_table` 定義と一致するか検証します
2. **自動テーブル作成**: `dbOpen` プログラム起動時に、テーブルが存在しない場合は自動的に作成します
3. **スキーマ検証**: プログラム起動時に、テーブルが存在する場合、カラム名と型が定義と一致するか検証します（不一致の場合はエラーになります）
4. **行型エイリアスの自動生成**: テーブル名が行のオブジェクト型のエイリアスとして自動的に定義されます。各カラムは `string` 型として扱われます

#### 行型エイリアスの使用例

```typescript
create_table todos {
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  title TEXT NOT NULL,
  completed INTEGER DEFAULT 0
}

// 上記のテーブル定義により、以下の型エイリアスが自動定義される:
// type todos = { id: string, title: string, completed: string }

function renderTodoRow(row: todos): JSX {
  return <li>{row.title}</li>;
}

function renderTodos(): JSX {
  const rows = fetch_all { SELECT id, title, completed FROM todos };
  // row の型として todos を使用可能
  return <ul>{rows.map(renderTodoRow)}</ul>;
}
```

### 12.3 JSX構文

サーバーサイドレンダリング用のJSX構文をサポートします。JSX要素は文字列に変換されます。

- JSX は `prelude` がエクスポートする `type JSX = string` のエイリアスです。JSX を返す関数やカスタムコンポーネントのプロパティでこの型を使う場合は `import { type JSX } from "prelude";` で明示的にインポートしてください。

#### 12.3.1 基本構文

```typescript
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

`{式}` 構文で文字列・数値・真偽値を埋め込めます:

```typescript
const title = "Hello";
const count = 42;

<div>
  <h1>{title}</h1>
  <p>Count: {count}</p>
</div>
```

#### 12.3.3 属性

文字列リテラルまたは式を属性値として使用できます:

```typescript
// 文字列リテラル
<input type="text" placeholder="Enter name" />

// 式
const className = "highlight";
<div class={className}>Content</div>
```

#### 12.3.4 フラグメント

`<>...</>` でフラグメント（タグなしコンテナ）を作成できます:

```typescript
<>
  <p>First paragraph</p>
  <p>Second paragraph</p>
</>
```

#### 12.3.5 responseHtmlとの使用

JSXは `responseHtml` 関数と組み合わせてHTMLレスポンスを返すのに最適です:

```typescript
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

- サーバーサイドレンダリング専用です（クライアントサイドJavaScriptは生成されません）。
- イベントハンドラー（onClick等）は未対応です。
- カスタムコンポーネント（大文字で始まるタグ）は 12.3.7 のルールに従って関数呼び出しに変換されます。
- JSX式内でオブジェクト・配列は埋め込み不可です（プリミティブ型のみ）。

#### 12.3.7 カスタムコンポーネント

大文字で始まるJSXタグはカスタムコンポーネントとして扱われ、同名のトップレベル関数への呼び出しに変換されます。この関数は単一のオブジェクト引数（プロパティオブジェクト）を受け取り、`JSX`（文字列の別名）を返す必要があります。変換ルールは以下のとおりです。

- JSXの属性はプロパティオブジェクトのフィールドになります。属性値には文字列リテラルまたはプリミティブ（文字列、整数、浮動小数点、真偽値）を返す式しか使えず、対応するプロパティ型と一致する必要があります。対応するプロパティが明示的に定義されていない場合、`Map<string>` のようなインデックスシグネチャを通じて許可されていない限りコンパイルエラーになります。
- `<CustomComponent>` のようにネストしたJSXがあると、それらは文字列として結合され、`children` プロパティに渡されます。コンポーネントのプロパティ型が `children`（またはインデックスで任意の名前）を受け取るように定義されていない場合は、ネストされたJSXはエラーになります。`children` プロパティがある場合、子要素がないときは空文字列が渡されるため常に安全に扱えます。
- カスタムコンポーネントの関数が返す文字列は通常のJSXと同様に埋め込まれるため、`responseHtml` などで結果を合成できます。
  なお、カスタムコンポーネントが返す `JSX` 文字列は周囲の JSX 子要素と同様にその場で連結されるため、通常のタグとまったく同じように出力されます。
- プロパティが全く必要ないコンポーネント関数は引数を省略して定義できます。この場合、JSX 側で属性や子要素を与えるとコンパイルエラーになります。

```typescript
function Layout(props: { title: string, children: JSX }): JSX {
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

- コンパイラは WAT を生成し、wasmtime-go の `Wat2Wasm` で WASM を生成します。
- 実行は同梱 CLI の `run` で行います。
- `run --sandbox` では通常の標準出力ではなく、`{ stdout: string, html: string, exitCode: integer, error: string }` 形式のJSON文字列1件を標準出力に返します。
- **CGO と C コンパイラが必要**です（wasmtime-go が C 依存）。
