# 事前ビルド配布の準備と公開

利用者はGo/Node.jsを導入せず、Releaseのinstall.shとLinux用バイナリを使用します。Go/Node.jsによる検証・ビルドは開発側の環境で実施します。配布準備、公開、対象ホストでのインストールは別の操作です。

この変更は配布の仕組みを用意するもので、Release・tagを作成していません。公開の独立承認は保留です。既存のメモリ・systemd受入条件も、この仕組みやビルド成功だけで満たしたとは扱いません。

[配布検証報告](PREBUILT-VALIDATION.md)に実施済みと未検証を記録しています。

## asset契約

`VERSION`はvなし、tag/アーカイブ名はv付きです。例えばソース定数が`0.6.3-rc.2`なら、許可されるtagは`v0.6.3-rc.2`だけです。入力はv付き数値3成分と、必要なら英数字をdot/hyphenで区切ったprerelease（最大81文字）に限定し、+build metadataは受け付けません。packagerはソース定数を読んで照合し、バージョンを上書きするldflagsやsource書換えを使用しません。正式版を名乗るためのRC番号の偽装も行いません。

Releaseへ渡すassetは次の5個です。

- `scyllascope-vVERSION-linux-amd64.tar.gz`
- `scyllascope-vVERSION-linux-arm64.tar.gz`
- `VERSION`
- `SHA256SUMS`
- `install.sh`

各アーカイブには、次の5ファイルだけが含まれます。

- `scyllascope`（Linux・CGO_ENABLED=0の本番CLI）
- `VERSION`（vなし定数と改行）
- `LICENSE`（リポジトリ既存のまま）
- `THIRD_PARTY.md`（生成したGo SDK・production Go modules・Svelte等のexact license/noticeを集約）
- `LICENSES/RC-MIT.txt`

実config、token、secrets.env、DB/WAL/SHM、ログ、試験結果、source treeをアーカイブしません。固定の許可リストで作り、tar member一覧を照合します。`SHA256SUMS`は2アーカイブ・VERSION・install.shを対象とします。チェックサムは破損検出と取得assetの照合であり、同じ配布元から得たchecksumだけで独立した電子署名になるものではありません。

## ローカル/CIで候補を作る

Linux build hostに保守対象のGo（最低1.25）、race検証用C compiler、Node.js、Python 3、Git、GNU tar/gzip/coreutilsが必要です。利用者側の依存ではありません。source変更をレビュー・commitし、既存CIと検証報告を確認します。

```bash
tools/package-release.sh --tag v0.6.3-rc.2 --output /tmp/scyllascope-candidate-unique
```

出力先は新しいディレクトリにします。既存assetを上書き・混在させません。現在commitのgit archiveだけを一時build元にし、ignored/untrackedな実.env・logがgo:embed経由で混入することを防ぎます。GOWORK=off/GOFLAGS空を明示し、親のgo.workや任意build flagsを引き継ぎません。packagerはfrontend manifest照合、実Node再生成とmanifest再現性、frontend回帰、`./build.sh`による正規依存の全体test/vet/raceと本番buildを実行します。今回成功したBUILD-VERIFIED.sha256とCLI versionを確認後、Linux amd64/arm64をCGOなしでビルドします。古い成功印を置くだけで検証を省略できません。exportしたcommitted source/go.mod/go.sumの内容がvalidationで変わった場合も停止し、レビュー・commit後の再実行を求めます。ビルド成功後、tools/collect-notices.pyを1回実行し、使用したGo SDKとproduction Go module rootのLICENSE/NOTICE、bundled Svelte/関連licenseのexact textを生成THIRD_PARTY.mdへ集約して両アーカイブへ入れます。リポジトリの元noticeや既存LICENSEは書き換えません。

同じGo/依存/source条件で、mtime/ownerを固定した許可ファイルだけをtarへ入れます。ただし異なるtoolchain全体のbit再現性まで保証するものではありません。ARM64はクロスビルドであり、ARM実機での起動を代替しません。長期メモリ試験は追加実行しません。

## GitHub Actionsでの分離

`.github/workflows/release.yml`は`workflow_dispatch`だけで実行します。push/tag push/通常PRでは公開しません。

1. `publish=false`（既定）では、Actionsで選択したrefをcheckoutし、tag inputとソース定数を照合して候補をbuild/check/artifact化します。GitHub Release/tagは作りません。artifactの保持期間は14日です。
2. 公開を承認する前に、source commit、バージョン、license、archive member一覧、checksum、既存CI、実施済み/未検証事項、install/update/rollbackの手順をレビューします。root install.shは利用者側でのみ実行し、build jobでサービス操作をしません。
3. 公開用の既存tagは、人が別途承認した手順で用意します。このworkflowはtagを生成・pushしません。`publish=true`の場合は選択branchの内容を配布せず、inputに一致する既存tagそのものをcheckoutして検証・再ビルドします。
4. 公開は別途明示承認を得た後だけpublish=trueで手動dispatchします。この準備では新しいenvironment・required reviewers等のsecurity/永続アクセス設定を作成・変更しません。publish=falseが既定であり、現時点で公開実行・公開承認は行っていません。
5. build jobはcontents readのみ、publish jobだけcontents writeです。publish jobはbuildのimmutable artifactを受け取り、checksum/外部VERSIONを確認し、既存Releaseがあれば拒否します。API認証/通信エラーを「Releaseなし」と解釈しません。
6. `gh release create --verify-tag`で新しいReleaseを作ります。tagに`-`を含むRC等は`--prerelease`です。既存Releaseの上書きや`--clobber`は行いません。

Release作成後のasset upload等が通信途中で失敗した場合、結果を推測して再実行しないでください。まずReleaseの有無・draft状態・asset一覧を確認し、人が復旧方針をレビューします。workflowは自動削除や重複uploadで取り繕いません。

## 承認後に担当者が行う公開操作

このPRをmainへ取り込み、配布元commitと候補assetsをレビューします。`v0.6.3-rc.2`が未作成の場合に限り、タグ作成/pushの別承認を得た担当者が、そのレビュー済みmain commitへタグを作成します。既存タグが違うcommitを指す場合は書き換えず停止します。Releaseが既に存在すれば、再公開せず状態とassetを確認します。

既存タグがあり、Release公開を明示承認した後だけ、GitHub Actionsの `prepare-or-publish-prebuilt` をtag `v0.6.3-rc.2`・publish `true`で手動実行します。CLIからの同等操作は次です（このタスクでは実行していません）。

```bash
gh workflow run release.yml --ref main -f tag=v0.6.3-rc.2 -f publish=true
```

RCはprereleaseとして公開します。READMEの3コマンドはこの候補を固定しているためprereleaseでも取得できます。安定版のlatestを使う場合だけversion引数を外します。公開後、VERSION/SHA256SUMS/2CPU archives/install.shの存在とhashを確認し、3コマンドを使い捨てsystemd VMで受け入れます。公開前のURLを「公開済み/本番で動作済み」と報告しません。

## 配布後に残る検証

配布フローの成功は、実ホストでの導入成功や数週間のRSS問題の解消を意味しません。[collector追跡](COLLECTOR-FOLLOWUP.md)、[RSS切り分け](RSS-ISOLATION-RESULTS.md)、[導入・更新・切り戻し](../UPGRADE.md)の受入条件を維持します。ScyllaDB本体のservice/config/dataを変更する操作は配布・インストーラーの対象外です。
