# ScyllaScope

ScyllaDBの各ノードから接続・クラスタ状態を観測する監視UIです。Linux AMD64/ARM64向けのビルド済み配布を使うため、利用者のホストにGo・Node.js・ソースビルドは不要です。

公式Releaseの`v0.6.3`を取得します。Release assetsが取得できない場合や検証に失敗した場合、導入前に停止します。 [公開手順と準備済み配布物](docs/RELEASING.md)

## install

```bash
bash -o pipefail -c 'script=$(curl -fsSL https://raw.githubusercontent.com/disnana/ScyllaScope/main/install.sh) && printf "%s\n" "$script" | bash -s -- install --version v0.6.3'
```

## update

```bash
bash -o pipefail -c 'script=$(curl -fsSL https://raw.githubusercontent.com/disnana/ScyllaScope/main/install.sh) && printf "%s\n" "$script" | bash -s -- update --version v0.6.3'
```

## uninstall

```bash
bash -o pipefail -c 'script=$(curl -fsSL https://raw.githubusercontent.com/disnana/ScyllaScope/main/install.sh) && printf "%s\n" "$script" | bash -s -- uninstall'
```

それぞれ1回のコマンドで完結します。必要時のみsudoがパスワードを求めます。Bash、curl、Linuxの標準的なtar/sha256sum/coreutils、動作中のsystemdが必要です。非対応OS/CPU・取得失敗・検証失敗は導入前に停止します。上のコマンドはscriptの取得が完全に成功してからbashへ渡し、取得失敗時は導入を開始しません。パイプの失敗も終了コードへ反映します。

`--version`はバージョン固定です。公開された安定版の最新版を使う場合、install/updateから`--version v0.6.3`を外します。GitHubのlatestはprereleaseを含まないため、RCは明示して固定します。Scriptは公式`disnana/ScyllaScope`のReleaseだけを取得し、CPUを自動判定、SHA-256・archive member・CLI versionを検証します。checksumは同じ配布元の整合性確認で、独立署名の代替ではありません。

installは専用ユーザーでScyllaScopeだけを起動し、通常の`/etc/scylla/scylla.yaml`から設定を検出します。既存の`/etc/scyllascope/config.yaml`、ui-token、secrets.env、監視DBを保持します。updateは停止後バックアップし、起動失敗時は旧バイナリ・設定・監視DBへ自動復元します。uninstallはScyllaScopeのサービス・導入ファイルを削除し、設定・データ・バックアップ・専用ユーザーを保持します。ScyllaDBのサービス・設定・データは変更しません。[更新・切り戻しの詳細](UPGRADE.md)

UIの接続先は導入完了時に表示されます。ログイントークンは保護された端末で`sudo cat /etc/scyllascope/ui-token`を確認してください。Scylla設定の読取権限が不足する環境は失敗し、その権限を自動変更しません。監視UIは閉域で使用し、複数ノードのpeer共有には共通cluster_tokenを設定します。[通知集約とHAの条件](docs/NOTIFICATIONS.md)

## 検証状態

ビルド済み配布と3コマンドの検証範囲は[配布検証報告](docs/PREBUILT-VALIDATION.md)を参照してください。

既存LICENSEを保持してv0.6.3-rc.2を取り込み、メモリ保持・不要allocation、永続通知/500件超処理、設定保持/切り戻しを修正しています。[実施済み・失敗・未検証](VALIDATION.md)を区別して報告しています。数週間のRSS問題の解消は未判定です。実systemd hostでの導入/更新/切り戻し受入も、現在の開発containerでは実施できていません。

## 開発者向けソースビルド

配布バイナリの利用者はこの手順を実行する必要はありません。開発・配布作成にはGo 1.25以上（保守対象の最新パッチ）、Node.js、Python 3が必要です。

```bash
node web/build.mjs
node --test web/test/*.test.mjs
python3 tools/test_build_gate.py
./build.sh
./tools/install-local.sh
```

`build.sh`は正規依存・全体test/vet/race・production buildを必須とし、Goのインストールやサービス操作は行いません。`tools/install-local.sh`だけが検証済みのローカルビルドを導入します。配布作成と公開は[RELEASING](docs/RELEASING.md)へ分離しています。長期メモリ試験を通常導入や更新の前提にはしません。[長期測定計画](docs/MEMORY.md)

## ライセンス

既存の[MIT LICENSE](LICENSE)を変更せず保持しています。[第三者表示](THIRD_PARTY.md)とRCのMIT表示も配布archiveへ含めます。実環境設定・認証情報・内部アドレス・元ログは配布しません。
