# Negitoro Language Support for VS Code

Negitoro言語のシンタックスハイライトをVS Codeに追加する拡張機能です。

## 機能

- `.ngtr` ファイルのシンタックスハイライト
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

- プリミティブ: `integer`, `number`, `boolean`, `string`, `void`
- SQLテーブル定義用: `INTEGER`, `TEXT`, `REAL`, `BLOB` など

### その他

- ブーリアンリテラル: `true`, `false`
- 文字列リテラル: `"..."`
- 数値リテラル: `123`, `3.14`
- コメント: `// ...`, `/* ... */`

## インストール方法（VSIX パッケージのみ）

1. VSIX パッケージを作成してインストール:

```bash
cd /path/to/negitoro/editors/negitoro.negitoro-0.1.0
rm -f negitoro.vsix
npx @vscode/vsce package --allow-missing-repository -o negitoro.vsix
code --install-extension negitoro.vsix --force
```

2. VS Codeを再読み込み（`Ctrl+Shift+P` → "Developer: Reload Window"）

## アンインストール

コマンドラインから:

```bash
code --uninstall-extension negitoro.negitoro
```

または手動で削除:

```bash
# Linux / macOS
rm -rf ~/.vscode/extensions/negitoro.negitoro-*

# WSL (VS Code Remote)
rm -rf ~/.vscode-server/extensions/negitoro.negitoro-*

# Windows (PowerShell)
Remove-Item -Recurse "$env:USERPROFILE\.vscode\extensions\negitoro.negitoro-*"
```

## ライセンス

MITライセンス
