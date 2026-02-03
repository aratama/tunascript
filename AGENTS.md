- Gitの操作は禁じられています。決してコミットしたりプッシュしたりしてはいけません。
- すべてのファイルはUTF-8(BOMなし)で保存してください。BOM付きUTF-8は禁止されています。
- タスクが完了したらテストを実行し、もしテストが失敗していたら修正してください。
- TunaScriptの構文を変更したら、必ずspec.mdに反映させてください。
- TunaScriptの構文を変更したり、拡張の再インストールを求められたら、以下のようなコマンドで拡張を再インストールしてください

```shell
cd ./editors/tuna.tuna-0.1.0 && rm -f tuna.vsix && npx @vscode/vsce package --allow-missing-repository -o tuna.vsix && code --install-extension tuna.vsix --force
```
