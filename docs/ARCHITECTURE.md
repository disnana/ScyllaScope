> 0.6.3-rc.2: この文書は元版の設計説明を含みます。今回の変更・検証範囲・残課題は `docs/RC-REVIEW.md` と `VALIDATION.md` を優先してください。実機での安全性検証が完了したという意味ではありません。

# Architecture

```
Browser (Svelte SPA, local assets)
    │ cookie + same-origin API / one SSE stream
    ▼
Go HTTP API ── immutable snapshots ── Monitor
    │                                 ├─ CQL metadata (local-host pinned)
    │                                 ├─ CQL OPTIONS / TCP 7000 probes
    │                                 ├─ local REST / metrics / systemd
    │                                 └─ authenticated peer observations
    ├─ SQLite history / events / incidents
    └─ Alert engine → Alertmanager (grouped / HA) or direct webhook
```

## Frontend

`web/src/` がソース。Svelte 5.48.0の実コンパイラを `web/build.mjs` から実行し、`internal/api/static/ui` へコンパイルします。SPAは1個のEventSourceを保持し、受信snapshotをstoreへ反映します。描画更新で `#page.innerHTML` を置換しません。

ノード・エッジ・表のキーを固定し、ルートはhashで遷移します。訪問済みページはhiddenで保持され、ルート単位にメインスクロールを保存します。詳細dialogの選択タブ・検索、native details、クエリ入力・結果、topology viewportをデータ更新から分離しています。logout時は認証状態と取得済みデータを消し、SSE/取得中リクエストを終了します。

履歴は時間範囲ごとのキャッシュを持ち、表示中のmetricsのみ60秒間隔で再取得します。ライブsnapshotと履歴取得は別です。チャートの欠測はnullとして線を分断します。

## Backend

`monitor.Source` と `repository.Repository` は、プロダクションのCQL/SQLiteと試験用fixtureを分離する境界です。本番コマンドがtestkitを呼ぶことはありません。gocqlのimport pathはgithub.com/gocql/gocqlですが、go.mod replaceでScyllaDB forkを使用します。

メタデータの自動failoverを抑制し、この観測ノードから見た状態を他ノードの結果と混同しません。共有するのは各peerが直接観測した元の方向と時刻です。転送された観測を再転送して鮮度を延長することはしません。

失敗/未確認/期限切れ/対象外を別状態とし、直近snapshotが古い時はフロント側の時計でも正常表示を取り下げます。clock skewに許容上限を設けています。wall clockを使うためNTP同期を推奨します。

## 実装の上限

監視対象既定64ノード、設定上限128。監視concurrency既定4。SSE最大32、クエリ同時2。3ノードfixtureの回帰試験が主で、大規模クラスタ・数千同時クライアントの負荷試験は未実施です。接続図/全方向観測はノード数の二乗で増え得るため、大規模構成の無制限利用は想定していません。

## 意図的に実装しないもの

DBの再起動、データ削除、automatic repair、backup/restore実行、CQLの任意書込、分散ロック/合意による自動復旧はありません。監視不達を根拠にDBを再起動すると、ネットワーク分断時に障害を増幅させるためです。
