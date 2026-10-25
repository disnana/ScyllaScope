# 現在の検証状況

最新のcollector停止の分類修正、注入回帰、本番どおりのSave-only比較、追加環境の具体条件は[追加切り分け](docs/COLLECTOR-FOLLOWUP.md)を参照してください。元の2件の低層遅延原因は未確定、health NGと資源判定を分離しました。

最新のTopology保存縮小・SSE allocation削減・要素を分けた逐次測定・未完了case・最終検証は[RSS切り分け結果](docs/RSS-ISOLATION-RESULTS.md)を参照してください。前回の60分×2と実Scylla/HA/旧DBは[継続検証結果](docs/CONTINUATION-VALIDATION.md)に履歴として残しています。前回RSS値には同processの試験clientも含まれていました。

**通知・保持・ライフサイクル・不要allocationを修正しましたが、数週間のRSS問題の受け入れは未達です。collectorのloopback Dial失敗2件も未解明として残します。Draftと正式版昇格保留を維持します。**

以下は初回提出時点の履歴です。10分試験や古い件数を現在の最終検証と混同しないでください。

---

# 初回提出の検証記録（2026-10-25）

正式版への昇格・マージは承認待ちです。バージョンは0.6.3-rc.2を保持しています。公開対象から実環境設定・認証情報・内部アドレス・元の生ログを除外しました。[取り込み元と公開範囲](docs/IMPORT.md)

## 実施済み

| 検証 | 結果・範囲 |
|---|---|
| 本番CLI | Go 1.27.3で `./build.sh` 成功。正規依存・go.sum・mod verify、全体test/vet/race、CLI実ビルド |
| Go回帰 | 最終の全体通常試験は104トップレベル成功（subtest込み119）。有限soak用2件のみ意図したskip、別実行で両方成功 |
| Node/Svelte | 18件成功。16 component、60 local assetの生成とmanifest再現性を確認 |
| ビルド保護 | 失敗分岐5件成功。失敗時の古い検証印削除・既存バイナリ保持 |
| 実Chromium | 21件成功。非表示ページの幅0でズーム中心が変わる不具合を発見・修正。ノード検索も実データ一致まで確認 |
| 実SQLite | moderncドライバで1201件のpending通知を保存・prune・再オープン・113件ずつ取得。表示は500件でも作業は1201件を取得。ID指定配送も検証 |
| キュー | 700件が128枠のキューを巡回し、古い作業に処理機会があることを確認 |
| 通知停止/再起動 | 全受信口503→プロセス/SQLite再オープン→受信口復帰で障害→復旧を再送。追加試験はalert package raceでも成功 |
| 実Alertmanager 0.28.1 | 2観測元の集約、片側だけの復旧、別プロトコル障害の分離、全体復旧、永続通知ログを使用した再起動後の重複抑制を検証 |
| インストール処理 | 実Goでファイル保存/破損検出/復元とsystemctlコマンド列を単体試験。新manifestのsecrets.env保持と旧manifest読込を確認。実systemd操作とは区別 |
| CPU | linux/amd64・linux/arm64へクロスビルド。ARM実機実行ではない |
| 依存検査 | Go 1.27.3のgovulncheckは `No vulnerabilities found`。最初のGo 1.26.1は標準ライブラリの到達可能な既知脆弱性25件を検出したため、公式チェックサム照合済み最新安定版へ変更 |
| リモートCI | [GitHub Actions](https://github.com/disnana/ScyllaScope/actions/runs/40171677791)成功。対象は187a4f8。本報告と追加の通知停止回帰は後続コミットで、ローカル全体試験・alert race成功 |
| 公開前確認 | 既存LICENSEが元mainとバイト一致。RC MIT表示・vendor manifestを保持。Git indexの禁止パス/資格情報パターン/内部CGN値検査と手動レビュー |

メモリカウンターの修正を外した比較コピーでは、1000退役identityの回帰が「1000件保持」で失敗し、修正後は成功しました。これはコード上の保持経路の再現であり、実環境の数週間の微増の主因と確定した証拠ではありません。

## 10分間の有限負荷（最終ランタイムコード、Go 1.27.3）

両suiteを各600秒、`GOMEMLIMIT=128MiB`で実行しました。Goのソフト目標でありRSS上限ではありません。ウォームアップ中には別のコンパイル/試験も実行しており、旧版との厳密な性能比較ではありません。

| 指標 | HTTP/SSE | 実SQLite＋incident churn |
|---|---:|---:|
| 負荷 | Publish 57,340回、SSE受信462,668件 | 51,021サイクルのincident評価・履歴書込 |
| その他 | 再接続4,632回、snapshot GET 29,066回 | 20サイクルごとprune、200ごとDB再オープン |
| 記録された失敗 | 0、終了後active SSE 0 | 0 |
| GC後heap開始→終了 | 1,974,712→2,393,632 B | 311,736→492,568 B |
| RSS開始→終了 | 14,032,896→18,702,336 B | 12,570,624→23,134,208 B |
| goroutine開始→終了 | 2→2 | 3→3（終了時もSQLiteはopen） |
| FD開始→終了 | 6→6 | 9→9（終了時もSQLiteはopen） |

HTTP/SSEは本物のHTTP・認証付きSSE・切断/再接続を使用し、保存先/データはfixtureです。SQLite suiteは実moderncドライバを使用しますが、入力は合成データです。両方とも実ScyllaDB・本番collectorループ・systemdを含みません。開始/終了のHTTP値は明示GC後、途中のHTTPサンプルは通常GC周期の値、SQLiteサンプルは明示GC後です。

heap/RSSは開始値へ戻っていません。短時間の結果から、数週間のメモリ増加が解消したとは断定しません。別のGo 1.26.1/中間コードの先行10分試験も成功しましたが、上の測定値と混ぜていません。全体の時系列・条件・集計は[results.json](docs/validation/results.json)に記録し、元の生ログはコミットしていません。

## 未検証・正式版の受入条件

- 実ScyllaDBの認証、CA/hostname、接続初期化・再接続・停止時の挙動、driver内部frame/pageの割り当て。
- 運用中の巨大な旧SQLite DB複製を使った移行・index作成時間・ディスク使用量。
- 対象systemdホストでの新規導入、設定保持更新、意図的起動失敗、自動/明示復元、アンインストールと再導入。独自unit/drop-inとTLSの受入。
- 実時間2〜4週間の同条件メモリ観測、実ノード入替・分断・通知停止を含む傾向比較。
- ARM実機、Dockerイメージビルド、Alertmanager HAの実メンバー停止とネットワーク分断。

通知のexactly-onceは保証しません。集約には独立したAlertmanagerが必要であり、全観測元停止時の期限切れは正常復旧の証明ではありません。別系統の死活監視を併用します。[通知構成と限界](docs/NOTIFICATIONS.md)

## 再現

```bash
node web/build.mjs
node --test web/test/*.test.mjs
python3 tools/test_build_gate.py
./build.sh
go test -count=1 -json ./...
SCYLLASCOPE_SOAK_SECONDS=600 GOMEMLIMIT=128MiB go test -timeout=15m -count=1 -v ./internal/api ./tests/runtime -run 'TestSoakRuntime|TestSQLiteIncidentSoak'
python3 tools/test_alertmanager.py /path/to/alertmanager
```

ブラウザは別端末で `go run ./cmd/devserver` を起動後、`CHROMIUM_BIN=/path/to/chromium python3 tests/e2e/test_browser.py`。手順と残条件は[RCレビュー追跡](docs/RC-REVIEW.md)、[長期測定](docs/MEMORY.md)、[導入・切り戻し](UPGRADE.md)にあります。
