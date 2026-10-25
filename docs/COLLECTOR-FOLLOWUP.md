# collector停止の切り分けと修正

前回の[93.10分の逐次測定](RSS-ISOLATION-RESULTS.md)で残った2件を再点検した。**修正したのは、プローブ健全性NGで独立した資源計測まで失うハーネスの判定である。元の2件の低層の接続遅延原因を特定・修正したとするものではない。** CQLの200msは変更せず、過去の失敗も成功に書き換えていない。本番・systemd/PID1/cgroup/security設定は変更していない。

## 判明したこと

詳細な失敗は `DialContext` 内で、CQL OPTIONS送信・SUPPORTED解析より前に起きていた。続くTCPは独立した200ms contextで成功し、親15秒の収集予算枯渇ではない。fixtureの1秒read deadline・レスポンス内容・同じportの共有・closure・cleanupに、このDial失敗を直接説明する欠陥は見つからなかった。

受付数11070、SUPPORTED5534という集計は「接続済みだがOPTIONS未送信」の可能性と整合する。ただし元の記録には接続tupleごとの対応がなく、最大header読込182.6msを失敗したDialと同一と断定できない。失敗時刻付近のcgroup throttling増加も粗い時間区間の観測であり、因果は未確定である。

Go1.27.3 `net/fd_unix.go` のconnect処理は、SO_ERRORが0になってもcontext deadlineの競合があればエラーへ変換する。これは期限切れの接続を成功として返さないための動作である。probe.CQLの失敗通知と、接続後のClose/cancel時Closeも正しい処理で、これらのログは保持漏れの証拠ではない。

## 有限な機構確認

同じ既存binaryを20秒だけstraceで追跡した。80 refresh、480接続、SUPPORTED240、fixture read/writeエラー0。接続要求→SO_ERROR確認481件の中央値約0.57ms、最大約20.48msだった。追跡下の有限結果であり、元の遅延が消滅した証明ではない。

次に自身のloopback test childの最初の該当getsockoptへ250msの遅延を注入した。straceの指定はtracee/threadごとに適用され、全processで1回だけではない。元binaryのtraceで4件のSO_ERROR=0が遅延し、fixtureは6接続を受付、SUPPORTED0。CQL/TCPは約250msでDial timeoutになり、旧ハーネスは約0.88秒でfatal終了した。**kernel側の受付とclient側deadline NGが両立し、その健全性NGだけでメモリtrialが止まる機構を再現した。元の2件が同じ遅延原因だったという証明ではない。**

## 修正・回帰

- Refresh進行、固定3 nodes/500 incidents/discovery、RSS/native/FD/goroutine/queue等の資源条件は従来どおり失敗時に停止する。
- CQL/TCPのNGは累積件数と時刻・遅延・判別できるphase・対象indexへ記録し、後続成功でも消さない。判別不能なphaseはunknownとする。
- 保持はnode×protocolの6固定slot。privateログも各slotの初回だけ、最大6件とし、新しいログ蓄積による見かけのメモリ増加を入れない。公開要約には元のerror文字列・URL/PID/pathを入れない。
- runnerは資源完走と健全性を別に扱う。健全性NGは`degraded`で残して後続の独立caseを実行し、全体終了コードを2にする。資源失敗やend telemetry欠落は成功扱いしない。

同種の注入下で修正binaryのcollectorを10秒、後続json_onlyを10秒実行した。collectorは39 refresh/234接続、CQL NG3件・TCP NG1件を保持して資源計測を完走し、後続caseも実行した。manifestは`degraded, pass`、controllerは終了コード2。短い診断に傾きの有効sample数はなく、RSS比較には使わない。先行93.10分に今回の20秒追跡・旧注入失敗・新2caseを加えたcase時間も約93.82分で、95分以内である（通常のunit/benchmark/CI時間とは区別）。

11連続NGでもログ1件・失敗11件となり、回復後もNGを保持するtracker回帰2件（race含む）、runnerの終了状態/回復/欠落telemetry/資源停止の回帰4件が成功した。CIにもこれらの回帰を追加した。[数値・source/hash・分類結果](validation/collector-followup.json)。生のsyscall/test logは公開しない。

## 本番と同じ保存の比較

前回のSave+Load4Hzは、本番のstartup Loadを増幅するため、そこでのRSS差を本番の定常効果へ換算できない。追加benchmarkは実SQLiteで起動時Loadを1回だけ実行し、以後の測定loopをSaveだけにした。compactは毎回metadata抽出のcaller allocationも含む。

| 固定input 393216 bytes | 保存bytes | Go B/Save | Go allocs/Save | startup Load |
|---|---:|---:|---:|---:|
| full | 393216 | 1,014,019 | 48 | 1 |
| compact | 2,970 | 10,488 | 21 | 1 |

これは不要な保存payloadとGo allocation削減の確実な効果である。native要求bytes・本番RSS削減量・数週間の傾きではない。標準Go benchmarkのcalibration/GCは別であり、RSS受入試験として扱わない。フレーム化802955→32B/opのSSE比較も同じく局所allocationの評価である。

## 現資料で確認できた保持と上限

`m.snapshot`は現snapshot、`m.known`はmetadataのみで2×max_nodes。通知はqueue128、worker32、inflight160、retry2048、AM宛先8。SSE32枠・通知channel容量1、API16/query2、HTTP transportのidle等に制限がある。**これらは件数/経路の上限で、全接続・payload bytes・RSS全体の上限ではない。tracks512は診断の停止閾値であり、productionの明示hard capではない。**

compactではnative mapped1MiB/live260件とGoSys17,668,360 bytesが一定でもRSSは小さく増えた。json_only/通知にもmapping一定時のRSS変動がある。既存領域のresident化、HeapReleased、stack/code/fileを分ける必要があり、RSS−GoSysをnativeの使用量にはできない。自然GCのHeapObjectsは未回収garbageも含む。終了GCはDB/HTTP停止後であり、生存rootの証明にはならない。

moderncの空きpoolはF≤4MiB+H（Hは動的な過去live高水位）。管理mappingについてM=live+F≤4MiB+2Hという形の上限はあるが、H自体は将来のpayload/並行量で増える。空carve-target・size classごとのhot領域、要求サイズも考慮が必要で、固定4MiBやRSS上限とは呼べない。SSE修正後6分の負のRSS傾きも、週単位の合格根拠にしない。

## 追加環境で必要なことと安全な次の手順

1. **元のloopback遅延の低層原因**：専用/予約vCPUの非本番Linux VMで同じGo・200ms条件を使い、自身のfixtureだけのconnect/accept/SO_ERROR/close、runtime scheduler trace、cpu.stat/PSI/stealを共通時刻で対応させる。最初のNGで該当接続前後を保存し、非trace controlと区別する。quotaやsecurity設定を変えて成功へ持ち込まない。現在の粗い時系列とaggregateではkernel・scheduler・quota等のどれかを確定できない。
2. **長期のRSS原因/収束**：固定Go/kernel/cgroup、外部client/receiver、実Scylla6.2.3の隔離fixture3nodesを用意。固定state/DB量controlと、node入替/CQL再接続/通知遅延/履歴retention運用を分ける。保持期間H時間の一巡後7日間を含め、例えばH=168hなら14日以上観測する。GC周期ごとのheap/objects低位包絡・private profile、native live/mapped、HeapIdle/Released、RSS/PSS/anon/file、DB/WAL bytes、保持件数/処理率を併記する。named Go VMAがない場合、匿名RSS全体をnativeへ帰属させない。
3. **停止/受け入れ**：sampler/負荷停止、健全性NG、事前の資源予算・disk空き下限で証拠を採取する。健康NGはNGとして保持し、資源結果と分けて止めるcaseを判断する。閾値未到達だけで合格にせず、固定stateでGC低位包絡/profile支配root/native高水位が持続上昇せず、retention一巡後の件数・DBが予算内、通知復帰時にpendingがdrainすることを確認する。
4. **systemd lifecycle**：managerが起動した使い捨てVMで導入→設定保持更新→起動失敗復元→旧版rollback→保持アンインストール→再導入を行い、ScyllaDB unit/config/dataが不変と確認する。現在はPID1がtailでmanager busがなく、このcontainerのPID1/cgroup/security変更では代用しない。

実gocqlの認証/TLS/停止・再接続・Closeの機能試験は既に隔離実fixtureで成功している。追加環境が必要なのは長期driver allocation、複数node/分断・retention定常化と実systemd受入であり、既済の機能検証を未実施へ戻すものではない。正式版への昇格は引き続き保留する。
