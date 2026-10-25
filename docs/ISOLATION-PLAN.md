# RSS原因切り分け: 仮説・停止条件（実行前）

同じタスク・Draft PR #1で継続する。前回の60分×2、heap profile、native allocator診断を再利用し、同じ負荷の単純な延長はしない。本番接続、権限/セキュリティ設定変更、マージ・タグ・リリースは禁止する。

## 前回測定の訂正

前回のHTTP/SSE clientとsnapshot GET decoderは測定対象と同じGoプロセスにあった。したがって約+5.9MiB/時はアプリ本体だけのRSS増加率ではない。profileのbufio.Scannerも試験client由来である。さらにpayload/DBが増え、二つのrunが重複していた。前回の実測値は残すが、この交絡を除いた試験で判断し直す。

## 判別仮説

- **Go保持**: 固定state/固定payloadでも自然GC後のheap/objectsと特定rootのprofileが増えるなら、map・goroutine・cache等の保持元を追う。HeapAllocだけでは生存heapと呼ばず、NumGC/LastGC/NextGCと照合する。
- **native live増加**: SQLiteだけでlive allocationがoperation数に伴い増えるなら、解放漏れやstatement/connectionを調べる。
- **native高水位/断片化**: liveが安定し、mappedがsize classまで増えてから固定ならpool保持の候補。mappedとRSSは異なり、allocator実装のpool上限・decommitと合わせる。
- **JSON/transport**: 同じPublish4Hz・概ね9encode/秒のjson_onlyとjson_sseを比較。clientは別PIDで、client RSSを本体へ加算しない。実送受信件数・byte数で負荷成立とレート差を記録する。
- **collector/通知**: SQLiteと配信を外した実collector、およびbounded repositoryを用いた実notification engineで、ループ/キュー等の単独保持を確認する。fixture受信口も別processへ分ける。

## 逐次実行と時間予算

全caseは同じGo 1.27.3・memory.counters付きbinary/hash、同じGOMEMLIMIT=128MiB・GOGC=100・GOMAXPROCS=2で、新しい測定processを起動する。case同士は重ねず、準備・compileをすべて終えてから実行する。外部client/receiverはそのcaseの制御された構成要素であり、別にRSSを記録する。

| case | 時間上限（warmup込み） | 分離するもの |
|---|---:|---|
| idle | 5分 | sampler・runtime基準（warmup2分） |
| sqlite_fixed | 15分 | 固定snapshotの同一Topology1行Save/Load、行数増加なし |
| collector_fixed | 15分 | 実collector、固定3nodes、bounded repository、SQL/通知/配信なし |
| json_only | 10分 | Publish4Hz、Snapshot clone/JSON→discard9Hz |
| json_sse | 15分 | 同じPublish、外部persistent SSE2本×4Hz＋外部GET1Hz |
| notification | 15分 | 実engine、固定500open、外部AM型503/204 receiver、SQL/collectorなし |

計75分以内。idle以外のwarmupは原則3分。追加確認は、判別した一仮説に対する1case・最大20分のみとし、計95分を上限とする。候補修正のTopology compact保存の確認に割り当てる場合も、元のfull保存と入力・レート・byte数・対象semanticを記録する。新たな疑問が残っても無期限に延長せず、必要な環境/計測と未解決点を報告する。

## 入力・測定・停止

- 固定3nodes・500incidentsと固定長message。retained state数と実payload bytesの範囲を記録し、ノード入替/履歴蓄積による正当な成長を混ぜない。
- 5〜10秒ごとのRSS、GoSys、HeapAlloc/Inuse/Idle/Released、HeapObjects、StackInuse、NumGC/LastGC/NextGC、native live/mapped/mappings、FD/goroutine、operationsとresource counts。
- 測定中はruntime.GC、FreeOSMemory、cache削除、SQLite shrink、強制再起動を行わない。自然GCのprofileをwarmup/endに採取する。停止後のGCは1回の診断phaseだけにし、傾き/改善判定に入れず、core stateへの参照を保つ。
- RSS256MiB、native mapped128MiB、resource上限超過、HTTP/SQL等の検証失敗、child死活不良・operation進行停止、case時間超過で停止する。負荷停止で平坦になったものを合格扱いしない。
- startupの差、自然GCの位相、周期負荷の相関を記録する。傾きは記述統計で、ゼロ近傍やPASSだけを数週間の解消証明としない。

## 根拠のある修正候補

monitorは再起動用Topologyに全snapshot（incidents/events/metrics等）を保存するが、唯一の読み出しconsumerは起動時にClusterNameとNodes.metadataを使う。この重複JSON/BLOBは契約上不要であり、callerだけをmetadata保存へ縮小する案を独立確認した。Storeの汎用round-tripは維持し、旧full/newcompactの起動・fallback同等性、metadata鮮度、cluster mismatch、配信snapshot不変を回帰する。これは不要な保持と書込の削減であり、RSS主因を証明したという主張はしない。

collector_fixedには同一processの小さなCQL/HTTP fixture serverも含まれ、実gocql driverではない。collector単体の厳密なメモリ分解とは扱わない。外部runnerは独立にwall-clock・RSS・sample更新を監視し、上限/30秒のsample停止時は当該case専用process groupを停止して後続caseを実行しない。

## 10:10 UTC: 停止後の有限な再開計画

最初のcollector_fixedは15.52秒でloopback probe検証に失敗し、後続は未実行で停止した。失敗データは残す。200ms probe/1秒fixture deadlineを変えず、詳細エラーとfixture統計だけを加えた60秒の診断は成功したが、元の原因は再現できず未特定である。

collectorのみ診断binaryで予定の900秒を1回実行し、再度失敗すればそのcaseを未検証として止める。その他のJSON/SSE/通知/compactは元binaryで実行する。したがって全case同binaryという当初条件からcollectorだけ逸脱し、source/hashを別に記録する。productionコードは同一で、差分はテスト診断のみ。特にfull/compact SQLite比較は元の同一binaryを維持する。

追加の失敗15.52秒＋診断60秒を吸収するため、compactを最大20分から18分（1080秒）へ短縮する。各caseの通常測定＋失敗＋追加診断は計95分以内に収める。停止中の調査時間を含むwall-clock完了見込みは当初11:19から約11:24 UTCへ更新する。再試験の成功を元の失敗原因の解消とは扱わず、両方を報告する。
