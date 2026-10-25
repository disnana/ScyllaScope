# ScyllaScope v0.6.3

Linux AMD64/ARM64向けのビルド済み配布です。利用者側のGo/Node.jsやソースビルドは不要です。[READMEのinstall/update/uninstall](../README.md#install)を使用してください。CPUを自動判定し、取得したarchiveのSHA-256・内容・バージョンを確認してから導入します。

更新は既存設定・認証情報・監視DBを保持し、起動失敗時はバックアップへ復元します。アンインストールも設定・データ・バックアップの保持が既定です。ScyllaDB本体のservice/config/dataは変更しません。既存LICENSEとRCのライセンス表示を保持し、依存ライブラリのlicense本文を同梱しています。

通知の永続化・再試行、500件を超える未配送通知の処理、ノード識別子に伴う保持、Topology保存とJSON/SSEの不要allocationを修正しました。詳細は[変更履歴](../CHANGELOG.md)と[検証報告](../VALIDATION.md)を参照してください。

## 検証範囲と残る制約

- 全体test/vet/race、frontend/ブラウザ回帰、取得スクリプトの隔離失敗系、Linux AMD64/ARM64のクロスビルドと実archive/ELF/checksum/license/version検証をCIで実施します。CIと配布元commitは当ReleaseのSource commitおよびリポジトリのActionsで確認できます。
- 数週間のRSS微増の解消は未確認です。既存の短時間・60分・要素別測定は長期受入の代替になりません。元のcollector試験で観測した2件のDial遅延の低層原因も未確定です。[RSS切り分け](RSS-ISOLATION-RESULTS.md)、[collector追跡と追加環境条件](COLLECTOR-FOLLOWUP.md)を引き継ぎます。
- 開発containerはPID1がsystemdでなくmanager busがありません。実systemdによる新規導入→設定保持更新→起動失敗rollback→保持アンインストール→再導入は未検証です。隔離fixtureとbackendのファイルtransaction回帰に成功した範囲を、実サービス操作の成功とは扱いません。
- ARM64はクロスビルド・ELF検証までで、ARM64実機起動は未検証です。実gocql sessionを含むクラスタ環境・TLS/認証・Alertmanager HA停止/分断の残る条件も[検証報告](../VALIDATION.md)を参照してください。
- 本番接続・導入やsystemd/security設定の変更は本タスクでは行っていません。導入前に使い捨てsystemd VMで上記ライフサイクルとScyllaDB不変を確認し、対象環境で2〜4週間のGo heap/RSS・キュー・ノード入れ替えを観測してください。[更新・切り戻し手順](../UPGRADE.md)

配布版名の正式版への変更は、これらの未検証事項が解消したという表明ではありません。
