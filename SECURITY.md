> 0.6.3-rc.2: この文書は元版の設計説明を含みます。今回の変更・検証範囲・残課題は `docs/RC-REVIEW.md` と `VALIDATION.md` を優先してください。実機での安全性検証が完了したという意味ではありません。

# Security scope / 0.6.0

## 想定境界

ScyllaScopeは信頼する管理者が、Tailscaleなどの私設ネットワーク内で使用する監視UIです。公開インターネット向けマルチテナント製品ではありません。UIログインは1つの管理トークンであり、個人アカウント、RBAC、SSO、MFA、改ざん耐性を持つ外部監査ログ、組織別権限分離は未実装です。企業導入時は組織の認証基盤/ネットワーク境界、依存脆弱性検査、運用手順のレビューが別途必要です。

## 実装済み

- 私設/loopback/Tailscale IP以外へのbindは明示設定がないと拒否。
- UIトークンとpeer共有トークンを分離。定数時間比較。
- ログインごとに別の乱数セッション。HttpOnly/SameSite=Strict、24h、最大256。logoutでそのセッションを即失効しSSEも切断。
- HTTPS直接終端時はSecure cookie。HTTPはVPN内でもブラウザ上はHTTP表示になる。
- Hostチェック、書込APIのsame-origin/X-ScyllaScope-Requestチェック、JSONサイズ/未知フィールド/多重JSON制限。
- ログイン試行制限、CQL同時実行2、SSE32、各種入出力サイズ・時間制限。
- CQLコンソール既定OFF。限定SELECT、関数/alias/コメント/ALLOW FILTERING/複文/書込を拒否。最大200行既定、500行上限、DB側LIMITも適用。
- 設定/診断exportからトークン・CQLパスワード・Webhook URLを除去。
- Svelteによる文字列escape。任意ノード名/エラーをHTMLとして挿入しない。CSVの式先頭を無害化。
- 外部CDN依存なし。APIをservice workerにキャッシュしない。
- systemd専用ユーザー、能力集合なし、NoNewPrivileges、ProtectSystem/ProtectHome、限定したstate書込。
- インストール/アンインストール/復元の排他ロック、バイナリの原子的置換、失敗時の旧ファイル復元。

## 注意

読み取りCQLでも情報取得・DB負荷は発生します。DB側に読み取り専用資格情報と最小権限を設定してください。`console.allow_user_tables: true` はユーザーテーブル全般への限定SELECTを許可するため、必要な場合だけ有効化してください。

通知・診断にはホスト名、IP、エラー文が含まれます。exportの秘密値は除去しますが、内部インフラ情報まで匿名化されるわけではありません。Webhooks/ログ/backupを外部共有する前に確認してください。UIトークンをGET URL、チケット、スクリーンショットに載せないでください。

共有peer tokenを持つ全ノードは同じ信頼境界です。peer署名鍵ごとのID認証・合意形成・悪意あるpeerの排除機構はありません。別ノードの観測を真偽の証明として扱わず、観測元と時刻を表示します。

HTTPの自己申告ヘッダだけでHTTPSと判断しません。TLS終端proxyを使う場合はHost許可、SSE buffering無効、セッション保護を環境に合わせて設定してください。Scylla管理REST APIの公開bind変更は不要です。

インストール先/管理ディレクトリはrootが管理してください。backupsは設定とトークンを含み得ます。旧版データを戻す操作には監視履歴の巻き戻りが伴います。

## 検証・監査の限界

APIの認証/CSRF/secret redaction、peer分離、logout/SSE失効、XSS表示、ファイル復元などの回帰試験を同梱しています。侵入テストや外部監査の代替ではありません。go.sumは初回 `go mod tidy` で作り、依存を固定したうえでCIのgovulncheckを実行してください。この配布環境で最新脆弱性DBへのスキャンは完了していません。
