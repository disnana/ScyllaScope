## 追加継続: collectorの停止判定修正とSave-only比較

既存20秒syscall追跡は80refresh/480接続成功。自身のtest childのgetsockoptに250ms遅延を注入すると、SO_ERROR=0/accept済みでもCQL200ms deadline NGとなる機構と旧harness即fatalを短く再現。元2件の実際のscheduler/kernel/quota原因を証明したものではない。

f6f941fでmemory resourceとCQL/TCP健康判定を分離。固定6slot/初回最大6log、累積NGは回復後も保持。実注入smokeはcollector資源完走/health degraded→後続json_only pass→controller exit2。runner4回帰、tracker2回帰/race成功。追加短caseを含め93.82分、95分以内。

SQLiteはLoad startup1回/以後Saveのみを実SQLiteで追加比較。full393216B保存/1,014,019Go B per Save、compact2970B保存/10,488Go B per Save（caller metadata抽出込み）。RSS推定に換算しない。新資料docs/COLLECTOR-FOLLOWUP.mdに保持owner/上限/具体的追加VM・長期・実systemd条件を明記。production runtimeは58b5dbbから不変。final CIを確認してPR更新する。

## 11:28 UTC: 全予定case終了、最終資料/CIへ

修正後json_sse360秒は11:20:55–11:26:57 UTCに成功。case process合計5586.281秒（93.10分）、失敗/60秒診断/確認を含め95分以内。各case別processで最長18分、単一93分の連続稼働ではない。旧60分×2も根拠として再利用した。

修正runtime58b5dbbで全体build/test/vet/race/modules verify・AMD64/ARM64・frontend再現性・Node18・build gate5・Chromium21・govulncheck成功、同commitのPR CIも成功。SSE framing802955→32B/op、sampled allocation profileは11.23→5.03MiB/s（異なる有限区間の推定）。長期RSS解消とは呼ばない。

docs/RSS-ISOLATION-RESULTS.mdとpublic-safe summary/figure/profile要約を追加。collector2失敗、実gocql長期・systemd実機・2〜4週間受入未検証を明記。独立資料確認後、docs-only最終HEADをpushしCI結果を確認する。マージ/タグ/releaseは行わない。

## 11:18 UTC: 基準測定終了、SSE修正・API回帰成功

compact1080秒は10:59:07–11:17:08 UTCに完走。原binaryによる基準測定をすべて終了し、データを保持した。その後、SSEのheader/JSON bytes/終端を単一deadline/flush下で分けて書く修正を適用。API全体のrace回帰が4.143秒で成功。large Unicode JSON wire、各部分のshortwrite/error、flush error、slot/購読解放を追加確認。

393KB framing-only benchmark（JSON生成・socketなし、Go1.27.3/GOGC100/GOMAXPROCS2/GOMEMLIMIT128MiB）はformatted802955B/op・5allocs/op、parts32B/op・1alloc/op。これは不要allocation削減の実証であり、RSS微増や数週間問題の解消判定ではない。全体build/test/race後、新binaryでjson_sse360秒の確認を1本行う。測定とbuildは重ねない。

## 11:00 UTC: 通知完走、compact開始

通知900秒は10:44:07–10:59:07 UTCに完走。受信376390件、204受理188037件、意図した503が188353件、receiver検証エラー0。観測queue最大128/inflight160/retry500、固定インシデント500、両worker/queueの上限内。RSSのwarmup後記述傾き+1.10MiB/h、末尾+0.42MiB/hであり、短時間の値を長期安定の証明にはしない。

10:59:07 UTCにSQLite compact1080秒を開始。原binary・固定full inputのままmetadata保存を比較する。基準測定完了見込み11:18 UTC、その後profileに根拠のあるSSE分割write修正・回帰/benchmark・6分確認・最終検証を行う。測定中のproduction変更/buildなし。collector失敗2件は未解明のまま保持。最終見込み約11:30 UTC。

## 10:46 UTC: SSE完走とprofileの根拠

JSONのみ600秒、JSON/SSE900秒が完走。SSEは10:29:04–10:44:07 UTC、受信検証エラー0、約4Hz×2 SSE＋GET1Hzが成立。warmup→end alloc_space profileの推定約8.48GB中、fmt.Sprintf経路約4.62GB（54.44%）。これは累積の一時allocationで、live/RSSや漏れbytesではない。現在の通知900秒→compact1080秒の測定は中断しない。

全case完了後、payload全体のSprintfを分割writeへ変える最小案を回帰/benchmarkで確認し、新binaryで6分のjson_sse比較を1本追加する。未完了collectorの未消費時間を使って全測定95分以内。初回計画からの変更理由をISOLATION-PLAN.mdに明記。数週間問題の解消は未判定。

## 10:26 UTC: 現在の継続状況

60秒診断は約10:07:28–10:08:28 UTC成功（時刻はsampleから復元）。collector再確認は10:09:45–10:17:27、461.53秒でCQL Dial 200ms timeoutにより停止。直後のTCP接続成功、元の原因は未特定。両失敗と診断成功を残し、collectorの3回目は行っていない。production処理やtimeoutを変更していない。

JSONのみは10:19:03 UTC開始、10:26時点で約430秒・JSON処理3849回・RSS15.79MiB、処理成立。測定は中断せず継続中。続くJSON/SSE15分・通知15分・compact18分を含め、残り約51分、終了見込み11:18 UTC。初回collectorの未完了を成功へ置換せず、最終のbuild/test/race/browser/CIとPR更新は測定後に行う。

## 10:17:27 UTC: collector再確認も停止、独立caseへ進む

collector再確認は461.53秒で停止。CQL OPTIONS用Dial(127.0.0.3のfixture)が200msでi/o timeout、直後のInternode TCP接続は0.53msで成功。fixtureの非EOF読込/書込失敗は0、最大handler開始遅延14.11ms、最大header読込182.59ms。メモリ閾値超過ではなく、接続の失敗原因は未特定。2件の失敗と60秒成功の全証拠を保持し、collectorの追加実行はしない。

JSONのみ10分→JSON/SSE15分→通知15分→SQLite compact18分を原binaryで順次実行する。collectorは未完了のまま報告し、他caseの成立条件を確認する。全体wall-clock見込みは約11:16 UTCへ更新、測定時間上限95分は維持。

## 10:04:40 UTC: collectorケース停止

collector_fixedが15.52秒でループバックprobeの成立検証に失敗。停止条件に従いrunnerは後続JSON/SSE/通知/compactを実行せず終了した。idle/SQLiteの完走データは保持し、失敗caseも削除しない。fixtureと本体のどちらの不具合か、probe詳細を追加して最大60秒の診断を1回行う。完了見込み11:19 UTCは停止により更新が必要。測定中の強制GC・cache削除・本番変更は行っていない。

## 10:04 UTC: 逐次測定2件完了

idle 09:44:23–09:49:23、SQLite fixed 09:49:23–10:04:24を完了。SQL/負荷進行検証成功。SQLite warmup後のsampleではnative live356件・mapped3MiB・FD10で一定。collector fixed開始、見込み10:19 UTC。全体見込み11:19 UTCは変わらず、他caseは未検証。ソース/測定binaryの変更・再起動は行っていない。RSS改善の最終判定は未実施。

# 2026-10-25 09:44 UTC: RSS切り分け継続

同じDraft PR #1の90ae00dでTopologyをmetadata保存へ縮小。全体test/race/vet成功。旧full/newcompact再起動と配信データ保持の回帰成功。

測定開始09:44:23 UTC。7caseを新しいprocessで逐次実行、合計上限95分、完了見込み11:19 UTC頃。binary SHA256 40aec0060adba9d55cabcc0ef354db3ab42a5870313d5e0e9a4b15bc7859dcda。詳細はISOLATION-PLAN.md。測定中のbuild・他負荷試験なし。前回RSSは試験clientを含むことを訂正し、今回は外部client/receiverを分離。現時点ではRSS改善も数週間の解消も未判定。マージ・タグ・正式公開は保留。

---

# 継続タスクの進捗時点記録 — 2026-10-25 09:04 UTC

既存Draft PR #1で継続中。二つの60分測定は中断・再起動せず完了しました。

| run | 開始UTC（概算） | 完了UTC（概算） | 結果 |
|---|---|---|---|
| retry期限回収前（他レビュー修正済み） | 07:41:23 | 08:41:23 | PASS。HTTP/SSE失敗0、初期pending0、retry記録622 |
| retry期限回収後 | 08:01:34 | 09:01:34 | PASS。HTTP/SSE失敗0、初期pending0、retry記録1 |

更新回数14,302/14,274、SSE受信30,110/30,052、再接続1,506/1,504、snapshot GETは各3,599。意図した通知先503は別に集計しています。

不要なretry保持は改善しましたが、RSSのwarmup後OLSは前後とも約+5.9MiB/時でした。**長期メモリの解消判定は未達**です。payload/DBの増加、JSON/SSE作業領域、GC外allocator、idle/released等を分けた根拠と限定を継続検証報告へ記録しました。

現在は最終HEADのbuild/test/race/browser/CIを確認中です。追加の自動承認拒否・結果不明の書込はありません。実systemdはPID1=tail、manager/bus不在のため未実施です。daemonバイナリ自体は存在します。PID1やホストcgroupの変更、本番接続・変更、マージ、タグ、正式リリースは行っていません。

最終報告: [CONTINUATION-VALIDATION.md](CONTINUATION-VALIDATION.md)。通常の実機受入と2〜4週間の測定は正式版の残条件です。

このページは09:04時点の記録です。確定HEADのCI結果・完了状態はPR本文を参照してください。最終報告には同一ホストでの前後run約40分の重なりも明記し、尾部傾きの差の因果帰属をしていません。
