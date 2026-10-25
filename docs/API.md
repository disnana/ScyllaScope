# HTTP API / v0.6.0

既定ポート8091。Hostはlistenのホストかsecurity.allowed_hostsが必要です。UI APIはcookieかUIトークンのBearer認証。peer APIは別のcluster_tokenのBearer認証です。

POSTは `Content-Type: application/json` と `X-ScyllaScope-Request: 1`、同一originを要求します。CLIのBearerリクエストはOriginなしでも許可します。JSONは32KiBまで、未知キー/複数JSONを拒否します。

| Method | Path | 用途 |
|---|---|---|
| GET | /healthz | process liveness / version。DB正常を意味しない |
| GET | /readyz | 新鮮なクラスタ一覧取得の状態 |
| GET | /api/v1/auth | ログイン要否・セッション状態 |
| POST | /api/v1/login | `{"token":"UI token"}`。cookie発行 |
| POST | /api/v1/logout | 現セッションを失効 |
| GET | /api/v1/snapshot | 最新の集計snapshot |
| GET | /api/v1/stream | SSE `snapshot` / `auth_expired` |
| POST | /api/v1/refresh | 監視更新。実行中ならbusy |
| GET | /api/v1/events?limit=100 | 上限500 |
| GET | /api/v1/incidents?all=1 | active/復旧履歴（表示用の最新500件。通知処理は独立したページ取得） |
| POST | /api/v1/incidents/action | `{"id":"...","action":"ack"}`、snooze/resume。snoozeはminutes:15/60/1440 |
| GET | /api/v1/history?hours=6 | 監視サンプル。時間範囲は保存期間に制限 |
| POST | /api/v1/query | `{"cql":"SELECT * FROM system.cluster_status;"}`。既定無効 |
| GET | /api/v1/config | 有効設定のredacted view |
| POST | /api/v1/doctor | 診断実行 |
| POST | /api/v1/notifications/test | 設定済み通知先へのテスト。呼出間隔制限あり |
| GET | /api/v1/export | 診断JSON。内部IP等を含む |
| GET | /api/v1/peer/state | このノード自身の元観測。cluster_token認証 |

Snapshotのschema_versionは互換性用です。peer payloadは対応version、cluster_name、送信元address、時刻、サイズを検証します。health statusは観測の集約であり、複製一致の証明ではありません。

SSEはブラウザ/サーバーで同時接続数を制限し、遅い購読者へ古いsnapshotを無制限に溜めません。再接続時は最新snapshotを送ります。全イベントの確実な配信が必要な場合、SSEだけでなくイベント履歴を参照してください。


## v0.6.3-rc.1 runtime diagnostics

`GET /api/v1/runtime` requires UI authentication even when anonymous read-only mode is enabled. It returns Go version, heap/GC counters, goroutine count, and available Linux RSS/FD counters. It never returns environment variables, credentials, or profiles.

Normal API work has a 16-request admission limit (503 + Retry-After), SSE has 32 independent slots, and CQL queries have two slots. SSE updates now reuse the collector's event/incident view. An incident action may be reflected in the stream on the next collection; the action endpoint and incident-history endpoint remain available for immediate reads.
