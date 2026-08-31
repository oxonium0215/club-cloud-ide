# 将来のリファクタ項目メモ

検証・開発中に見つかった、今は対応しないが将来検討すべき項目の記録。

## coder ユーザー名のリネーム

システム全体で `coder` というユーザー名・`/home/coder/` パスが使われているが、
これは Coder プラットフォーム時代の名残。Coder 廃止後も以下に埋め込まれている。

- `scripts/setup-base-image.sh` の `useradd ... coder`
- `templates/lxd-siv3d/files/apply.sh` の配布先 (`/home/coder/...`)
- `templates/lxd-siv3d/files/entrypoint.sh` の `sudo -u coder`
- `portal/ui.go` の cloud-init (`users: - name: coder`)
- 各設定ファイルの `/home/coder/...` パス

**対応する場合**: `osgsuken` や `student` などへ統一リネーム + ゴールデンイメージ再ビルドが必要。
機能上は問題ないため、優先度は低い。

## その他 (追記予定)
