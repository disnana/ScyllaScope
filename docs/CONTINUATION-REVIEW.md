# 継続レビューで修正した不具合

初回提出後、通知とライフサイクルを別のレビュー担当が再点検し、下記を修正しました。変更は同じ開発ブランチとDraft PR #1で追跡します。既存LICENSEは変更していません。

| ID | 再現する条件・影響 | 修正と回帰 |
|---|---|---|
| N01 | 旧版由来/通知disabled中のactiveを後から通知enableし、送信失敗後に復旧するとpendingが保存されず再送欠落 | 既存activeも配送前にpendingを永続化。失敗→復旧→再起動を回帰 |
| N02 | 未復旧がUnknownへ変わるとAlertmanager heartbeatが止まり期限切れ | 正常証拠がない未復旧は永続状態を維持しheartbeat継続 |
| N03 | 同一対象の再発と前回の遅延復旧が同じalert identity | incident_idでepisodeを分離。実Alertmanagerでも遅延復旧が新episodeを上書きしないことを確認 |
| N04 | 退役activeが蓄積すると全件をexisting mapへ集積しheapピークが増加 | 256件ずつ処理し、現在のsignalに一致するseenだけ保持。未知を復旧にはしない |
| N05 | 旧版の重複active行が同一sampleの復旧確認回数を水増し | 各signalのsample時刻で重複カウントを抑止 |
| N06 | HAの一受信口がハングすると逐次送信・単一workerで大量通知が滞留 | 32有限worker、最大8宛先並行。520件、ハング混在、inflight<=160/queue<=128/retry<=2048、shutdown待機を回帰 |
| L01 | 管理ファイルの親がsymlinkだと権限変更や削除が別の場所へ向く | 親ディレクトリを変更前に検証。外部sentinelを使った回帰 |
| L02 | managed unitがない既存active/enabledサービスを停止せず監視DBをバックアップし得る | 配置前に管理外unitを拒否し手動移行へ。稼働中DBをコピーしない |
| L03 | systemd manager不在とinactive unitを区別できず配置へ進む | manager接続をファイル配置/ユーザー作成前に検証 |

`Engine.Stats()`はIDを含まない資源件数だけを返し、長時間測定で上限を観測します。HTTP 2xxは通知サービスによる受理であり、最終receiverへの実配送と原子的に結合していません。特にAlertmanagerのactive alertsは再起動時の再送が必要です。HAは単一process停止への対策であり、全AMの同時喪失、分断中のexactly-once、group_wait内に復旧した短い障害の必達を保証しません。

試験の追加: 実ScyllaDB 6.2.3の隔離fixture、実Alertmanagerのepisode/2-process HA、60分の統合collector負荷。実施結果・未検証事項はVALIDATION.mdを参照してください。
