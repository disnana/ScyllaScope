# 導入・更新・アンインストール・切り戻し

正式版は承認待ちです。最初にステージングのsystemdホストで受入試験を行い、1ノードずつ適用します。ScyllaDBのunit・設定・データには変更を加えません。

## ビルド済み配布の3コマンド

利用者のGo/Node/ソースビルドは不要です。公式の3コマンドは[README](README.md#install)を使ってください。初回Releaseは公開待ちで、RCは`--version v0.6.3-rc.2`で固定します。配布元/CPU/checksum/archive/versionの検証をすべて終えてから、既存のCLIによるファイルトランザクションを呼びます。取得/検証失敗時はサービスや導入ファイルを変更しません。

開発者がローカルソースから導入する場合だけ、`./build.sh`成功後に`./tools/install-local.sh`を使います。Goは自動更新しません。ビルド失敗時は検証印を取り消し、既存バイナリを残します。

## 新規導入

```bash
# 初回Release公開後、ビルド済みRCを検証して導入
bash -o pipefail -c 'curl -fsSL https://raw.githubusercontent.com/disnana/ScyllaScope/main/install.sh | bash -s -- install --version v0.6.3-rc.2'
```

配布インストーラーは検証済みRelease binaryを使用します。専用ユーザー `scyllascope` を作成し、取得・検証済みのバイナリを配置します。UIトークンは初回のみ作り、既存の `/etc/scyllascope/config.yaml`、ui-token、secrets.envを保持します。Scylla設定ファイルの読取権限が不足すれば停止し、DBの権限を自動変更しません。実際のトークンは保護された端末で `sudo cat /etc/scyllascope/ui-token` を確認します。

## 更新

現行バイナリ、config.yaml、ui-token、secrets.env、ScyllaScope DB/WAL/SHM、unitをバックアップします。独自drop-inは変更せず残しますが、自動バックアップには含めず別途保全してください。稼働中のSQLiteファイルを単純コピーしないでください。組み込み更新処理はScyllaScopeを停止後にバックアップし、更新起動に失敗した場合に復元します。

```bash
# ビルド済みRCを取得して更新（初回Release公開後）
bash -o pipefail -c 'curl -fsSL https://raw.githubusercontent.com/disnana/ScyllaScope/main/install.sh | bash -s -- update --version v0.6.3-rc.2'
```

既存設定が通常のパスなら引数は不要です。ローカルの別ファイルを明示的に適用する時だけ `./tools/install-local.sh --config /private/path/config.yaml` を使います。既存設定の初期化やサンプルへの置換は行いません。

バックアップは `/var/backups/scyllascope/<timestamp>` に0600の実体とmanifestを保存し、全ファイルのsize/SHA-256を復元前に検証します。新しいバックアップはsecrets.envも対象に含みます。RC以前の7エントリのバックアップを読む場合は、secrets.envを変更せず残します。チェックサムのない旧形式は警告を出します。ハッシュは偶発的破損の検出であり、root権限の攻撃者に対する署名ではありません。

## 切り戻し

```bash
# 新版CLIで最新バックアップを復元
sudo /usr/local/bin/scyllascope service rollback
sudo systemctl status scyllascope --no-pager
```

切り戻しでは保存時点の旧バイナリ、unit、設定と監視DBを復元します。バックアップ後のScyllaScope履歴・未送信通知が失われる可能性があります。ScyllaDBデータは対象外です。サービス停止に失敗した場合は稼働DBへ上書きしません。自動復元が失敗した場合は表示されたバックアップを保全し、停止状態とファイル内容を確認します。

独自unit/drop-in、独自ユーザー、SQLiteパス・トークンパスの変更は自動管理の対象外です。自動インストーラーは標準パスだけに対応します。drop-inと独自証明書は別にバックアップしてください。TLS設定ではインストーラーのプロセス生存判定と別に証明書・ホスト名検証を行います。

## アンインストール

```bash
bash -o pipefail -c 'curl -fsSL https://raw.githubusercontent.com/disnana/ScyllaScope/main/install.sh | bash -s -- uninstall'
```

ScyllaScopeのみ停止・無効化し、配置したバイナリとunitを削除します。設定・資格情報・監視DB・バックアップは既定で保持します。再インストールで再利用できます。

```bash
# 設定と監視DBも削除する場合だけ指定
sudo /usr/local/bin/scyllascope service uninstall --purge
```

`--purge` は `/etc/scyllascope` と `/var/lib/scyllascope` を削除します。切り戻し用バックアップには資格情報が含まれ得るため、公開せず、不要になった後に別途安全に管理してください。

## リリース前の実機受入

今回のファイルトランザクション試験と実ホストでのsystemd操作は別です。実機では新規導入→既存設定保持更新→起動失敗を注入→自動復元→明示的切り戻し→保持アンインストール→再導入を試し、各段階でScyllaDBのunit・設定・データが不変であることを確認します。認証/TLSと実クラスタ接続、2〜4週間のメモリ観測、Alertmanager HAメンバー停止・分断を受入条件に含めます。[検証結果](VALIDATION.md)

## 継続レビューで強化した拒否条件

標準の管理パスの親ディレクトリがsymlink/非ディレクトリの場合、権限変更・バックアップ・削除前に拒否します。管理対象の `/etc/systemd/system/scyllascope.service` がないのに既存サービスがactive/enabledなら、稼働中SQLiteをコピーしないよう手動移行を要求します。systemd managerへ接続できない環境でも、ファイル配置やユーザー作成前に停止します。対象ホストの実際の更新・切り戻しの受入試験を省略するものではありません。
