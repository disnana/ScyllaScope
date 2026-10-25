# ビルド済み配布の検証

以下はPR #2の公開準備時点の検証記録です。公開後の状態は[Releases](https://github.com/disnana/ScyllaScope/releases)と[CI](https://github.com/disnana/ScyllaScope/actions)を参照してください。

PR #1のmainマージ（62aa061）を確認し、main起点のfix/prebuilt-distributionと[PR #2](https://github.com/disnana/ScyllaScope/pull/2)で対応した。確認時点でReleaseとtagは0件、自動Releaseも起動していなかった。既存のマージ済みbranchへ追記していない。公開・tag・追加mergeは行っていない。

## 実施済み

- bootstrapの43 unittestが成功（約11.4秒）。ネットワーク/sudo/id/unameをprivate fixtureへ置換し、公式URL・CPU・pin/latest・version・checksum・archive・失敗系を確認した。
- READMEのinstall/update/uninstallの実3コマンドを抽出し、そのまま隔離fixtureで実行した。scriptを全部受け取った後のcurl失敗と途中切断のどちらでも、3操作すべてsudo/service/payload実行0を確認した。script単体のstdin/pipe実行、途中footer欠落の無操作も確認した。
- 取得/manifest欠落・重複・不正hash/checksum不一致/archive欠落・重複・traversal・link・途中切断/VERSIONとCLI version不一致/未導入update/unsupported OS・CPU/backend非0を確認。失敗を成功へ変換しない。
- 実packagerを実行し、frontendの生成hash再現性、Go1.27.3の正規依存/mod verify、全体test/vet/race、production CLI、CGOなしLinux AMD64/ARM64ビルドが成功した。新しい長期メモリ試験は実行していない。
- 作業treeのignoredなstatic/.envへ合成sentinelを置いた状態でpackagingし、両archiveの実ELFに混入しないことを確認した。ビルド元はcommitted git archiveのみ。exported全tracked sourceのhashも検証後に不変だった。
- 両archiveの5 regular members、CPU/ELF64/動的interpreter不要、全assetのSHA256、exact VERSION、既存LICENSE/RC notice、Go SDK・13 production modules（両CPUのunion）・Svelte等のlicense本文を確認。AMD64実binaryはversionを実行して確認した。
- 既存serviceの実ファイルbackup/restore・破損拒否・設定/secret保持・失敗復元の回帰も全体suiteで成功。新取得scriptはそのbackendを再利用し、別の更新処理を実装していない。
- 7c02743の[CI](https://github.com/disnana/ScyllaScope/actions/runs/40197965147)で全体test/vet/race、両CPU packaging/実asset検証、ブラウザ回帰、govulncheckが成功した。最終HEADは[PR #2のchecks](https://github.com/disnana/ScyllaScope/pull/2/checks)で再確認する。CIはprebuilt-release artifactを保存し、Release/tagを公開しない。

ローカル候補はdist/v0.6.3-rc.2（Git対象外）へ準備した。最初の実packagingのsourceは4748e62で、以後はREADME取得gate/同回帰/両CPU notice収集等を更新した。最終CIは最終HEADから配布を再生成するため、公開にはそのレビュー済みsource/tagから再生成したassetを使う。

旧RC候補の結果を履歴として保持する。正式版候補はソース定数・web metadata・README・workflowをv0.6.3へ更新し、最終PR CIから再packagingする。[正式版リリースノート](RELEASE-NOTES.md)に未検証事項を引き継ぐ。

## この報告作成時点で実施していないこと

- 実systemdのinstall→update→起動失敗rollback→uninstall→再導入。このcontainerはPID1がtailでmanager busがなく、systemd/PID1/cgroup/securityを変更して代用していない。shell層のfixture dispatch、実backendファイルtransaction回帰、実systemd lifecycleを混同しない。
- ARM64実機での起動。クロスビルド/ELF/metadata検証と、ネイティブsystemd上の実行は別である。
- 公開Release assetの実URLを使った導入。この報告作成時点のmain/install.shは旧ローカルbuild用であり、PR反映と初回Release公開まではREADMEの3コマンドを実運用しない。Release workflowのpublish=true経路も、権限外の公開をテスト目的で実行していない。
- 本番への接続/インストール、ScyllaDBのunit/config/data変更、権限/security設定・永続アクセスの追加、tag/Release/追加PR merge。

## 公開する際の最終段階

公開の明示承認を受けた後、v0.6.3の最終CI成功後にPRをmainへ取り込み、レビュー済みcommitのv0.6.3 tagを用意し、manual release workflowでpublish=trueとする。既存Releaseがある/存在確認が失敗した場合は停止する。v0.6.3はprerelease=false/latestで、READMEは固定versionを使う。公開後に使い捨てLinux systemd VMで3操作と失敗復元・ScyllaDB無変更を受け入れる。[具体的公開手順](RELEASING.md)

旧メモリ/実クラスタの結果と残件は[VALIDATION](../VALIDATION.md)を維持する。配布完成やCI成功を、数週間のRSS解消・実systemd受入の代替にしない。
