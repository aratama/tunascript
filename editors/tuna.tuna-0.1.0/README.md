# TunaScript Language Support for VS Code

TunaScriptのソースコードに対するシンタックスハイライトを提供する拡張機能です。

## 機能

- `.tuna` ファイルのシンタックスハイライト
- キーワード、型、関数、文字列、数値、コメントの色分け
- `sql { ... }` ブロック内のSQLハイライト
- `table` 定義のハイライト
- SQLパラメータ `{expr}` のハイライト
- 括弧の自動補完
- コメントの折りたたみ

## 対応するハイライト

### キーワード

- 制御構文: `if`, `else`, `for`, `of`, `return`, `switch`, `case`, `default`
- 宣言: `const`, `function`, `export`, `import`, `from`, `table`
- SQL: `sql`

### 型

- プリミティブ: `i64`, `i32`, `number`, `boolean`, `string`, `void`
- SQLテーブル定義用: `INTEGER`, `TEXT`, `REAL`, `BLOB` など

### その他

- ブーリアンリテラル: `true`, `false`
- 文字列リテラル: `"..."`
- 数値リテラル: `123`, `3.14`
- コメント: `// ...`, `/* ... */`

## インストール方法（VSIX パッケージのみ）

1. VSIX パッケージを作成してインストール:

```bash
cd /path/to/tuna/editors/tuna.tuna-0.1.0
rm -f tuna.vsix
npx @vscode/vsce package --allow-missing-repository -o tuna.vsix
code --install-extension tuna.vsix --force
```

2. VS Codeを再読み込み（`Ctrl+Shift+P` → "Developer: Reload Window"）

## アンインストール

コマンドラインから:

```bash
code --uninstall-extension tuna.tuna
```

または手動で削除:

```bash
# Linux / macOS
rm -rf ~/.vscode/extensions/tuna.tuna-*

# WSL (VS Code Remote)
rm -rf ~/.vscode-server/extensions/tuna.tuna-*

# Windows (PowerShell)
Remove-Item -Recurse "$env:USERPROFILE\.vscode\extensions\tuna.tuna-*"
```

## ライセンス

MITライセンス
