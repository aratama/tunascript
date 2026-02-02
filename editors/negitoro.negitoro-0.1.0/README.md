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

- プリミティブ: `integer`, `float`, `boolean`, `string`, `void`
- SQLテーブル定義用: `INTEGER`, `TEXT`, `REAL`, `BLOB` など

### その他

- ブーリアンリテラル: `true`, `false`
- 文字列リテラル: `"..."`
- 数値リテラル: `123`, `3.14`
- コメント: `// ...`, `/* ... */`

## インストール方法

### 拡張機能ディレクトリの場所

| 環境             | パス                                |
| ---------------- | ----------------------------------- |
| Linux            | `~/.vscode/extensions/`             |
| macOS            | `~/.vscode/extensions/`             |
| Windows          | `%USERPROFILE%\.vscode\extensions\` |
| **WSL (Remote)** | `~/.vscode-server/extensions/`      |

> **注意**: WSL上でVS Code Remoteを使用している場合は `~/.vscode-server/extensions/` を使用してください。

### 方法1: シンボリックリンク（開発用）

1. VS Codeの拡張機能ディレクトリにシンボリックリンクを作成:

```bash
# Linux
ln -s /path/to/negitoro/editors/negitoro.negitoro-0.1.0 ~/.vscode/extensions/negitoro.negitoro-0.1.0

# macOS
ln -s /path/to/negitoro/editors/negitoro.negitoro-0.1.0 ~/.vscode/extensions/negitoro.negitoro-0.1.0

# WSL (VS Code Remote)
ln -s /path/to/negitoro/editors/negitoro.negitoro-0.1.0 ~/.vscode-server/extensions/negitoro.negitoro-0.1.0

# Windows (PowerShell を管理者として実行)
cmd /c mklink /D "$env:USERPROFILE\.vscode\extensions\negitoro.negitoro-0.1.0" "C:\path\to\negitoro\editors\negitoro.negitoro-0.1.0"
```

2. VS Codeを再起動

### 方法2: 拡張機能フォルダにコピー

1. `editors/negitoro.negitoro-0.1.0` フォルダを VS Code の拡張機能ディレクトリにコピー:

```bash
# Linux
cp -r /path/to/negitoro/editors/negitoro.negitoro-0.1.0 ~/.vscode/extensions/negitoro.negitoro-0.1.0

# WSL (VS Code Remote)
cp -r /path/to/negitoro/editors/negitoro.negitoro-0.1.0 ~/.vscode-server/extensions/negitoro.negitoro-0.1.0

# macOS
cp -r /path/to/negitoro/editors/negitoro.negitoro-0.1.0 ~/.vscode/extensions/negitoro.negitoro-0.1.0

# Windows (PowerShell)
Copy-Item -Recurse "C:\path\to\negitoro\editors\negitoro.negitoro-0.1.0" "$env:USERPROFILE\.vscode\extensions\negitoro.negitoro-0.1.0"
```

2. VS Codeを再起動

### 方法3: VSIX パッケージとしてインストール（推奨）

1. パッケージを作成:

```bash
cd /path/to/negitoro/editors/negitoro.negitoro-0.1.0
npx @vscode/vsce package --allow-missing-repository -o negitoro.vsix
```

2. コマンドラインからインストール:

```bash
code --install-extension negitoro.vsix
```

または、VS Code GUIからインストール:

- VS Code を開く
- `Ctrl+Shift+P` (macOS: `Cmd+Shift+P`) でコマンドパレットを開く
- "Extensions: Install from VSIX..." を選択
- 生成された `negitoro.vsix` ファイルを選択

3. VS Codeを再読み込み（`Ctrl+Shift+P` → "Developer: Reload Window"）

### 拡張機能の更新

拡張機能を更新する場合は、VSIXを再ビルドして `--force` オプション付きでインストール:

```bash
cd /path/to/negitoro/editors/negitoro.negitoro-0.1.0
npx @vscode/vsce package --allow-missing-repository -o negitoro.vsix
code --install-extension negitoro.vsix --force
```

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
