- Gitの操作は禁じられています。決してコミットしたりプッシュしたりしてはいけません。
- すべてのファイルはUTF-8(BOMなし)で保存してください。BOM付きUTF-8は禁止されています。
- タスクが完了したらテストを実行し、もしテストが失敗していたら修正してください。
- Negitoroの構文を変更したら、必ずspec.mdに反映させてください。
- Negitoroの構文を変更したり、拡張の再インストールを求められたら、以下のコマンドで拡張を再インストールしてください

```
/home/cubbit/negitoro/editors/negitoro.negitoro-0.1.0 && rm -f negitoro.vsix && npx @vscode/vsce package --allow-missing-repository -o negitoro.vsix && code --install-extension negitoro.vsix --force
```
