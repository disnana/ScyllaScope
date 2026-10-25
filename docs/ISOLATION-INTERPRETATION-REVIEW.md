# RSS切り分けの解釈に対する独立レビュー

状態: 2026-10-25、09:44 UTC開始の逐次測定が進行中の時点で行うread-onlyレビューです。新しい測定結果、原因確定、改善量、メモリ問題の解消判定はここに記載しません。測定の上限は計画どおり最長95分です。本レビューではこの文書だけを追加し、追加のbuild・test・測定を起動せず、ランタイム・harness・実行中processを変更していません。

参照した証拠は[前回の継続検証](CONTINUATION-VALIDATION.md)、[前回before/after要約](validation/extended-before.json)、[after要約](validation/extended-after.json)、[今回の計画](ISOLATION-PLAN.md)、[計画レビュー](ISOLATION-PLAN-REVIEW.md)、[native診断](NATIVE-MEMORY.md)、現在のTopology保存差分、ローカルに取得済みのmodernc依存ソースです。前回profileの差分は既存報告の内容を再点検したもので、測定と並行してprofile解析コマンドを新規実行したわけではありません。

## 「bounded」と呼べるものの範囲

| 対象 | 言えること | そのまま言えないこと |
|---|---|---|
| 通知channel・worker | 実装上queue128、delivery worker32、したがって配送中＋queueのinflight上限160。AM宛先は最大8。retry記録は最大2048で、期限切れ非inflightを回収する | 各job/incident/bodyのbytesや、HTTP transport・Go arenaを含むプロセスRSS全体の上限 |
| discovery cache | `2 × max_nodes`の件数制限と退役処理がある | 実gocqlのsession、内部frame/page、全てのノード関連オブジェクトも同じ上限になること |
| 作業取得 | 通知用の読込は256件ずつで、全active履歴を一時mapへ集積しない | ディスク上の未解決インシデント数が固定であること、件数制限だけで全payload bytesも固定になること |
| 今回のrepository/fixture | synthetic inputは3 nodes・500 incidents、履歴/eventを蓄積せず、Topology1行。件数とbyte範囲を記録する | ノード入替・長期履歴・実運用でのincident増加・大きな返却値も同じ挙動になること |
| RSS/native停止閾値 | 閾値超過、進行停止、時間超過を検出して実験を止める | 閾値未到達をリーク不在の証明とすること、sample間の瞬間ピークを捕捉したこと |

件数が減っても、Go mapのbacking storageやallocatorの空きpageが即座に縮むとは限りません。件数・保持bytes・OS mapping・RSSは別の量です。逆に、単にGC前の未回収garbageが増えただけのHeapAllocを「保持オブジェクトの増加」と呼ぶのも不十分です。

## modernc allocatorのpool上限は固定のRSS上限ではない

確認した実装は`modernc.org/memory v1.12.1`です。

- `memory.go:126–143`の`maxFreedSize`は **4MiB＋hiLive** であり、hiLiveは「現在のlive bytes」ではなく、そのallocatorで過去最大となったlive mapping量です。`addLive`は新しい高水位で更新し、通常のFreeでは元へ戻しません。したがって「free poolには規則上の上限がある」は正しい一方、「プロセス全期間で固定の4MiBに収まる」は誤りです。将来の大きな一時allocationや並行量がhiLiveを上げれば、その後に許されるpool量も増えます。
- `memory.go:283–340`の`release/retain`は空regionを保持し、poolが上限を超える時にunmapします。`UintptrFree`はサイズclassごとの空carve-target pageも残します。pool上限はfreed領域に対するもので、live/部分live pageやその他のメモリまで含めた全体上限ではありません。
- `hotBytes=4MiB`は**サイズclassごと**のhot windowです。全class合計のresident上限4MiBという意味ではなく、大regionでは最低1個をhot扱いにします。64bitのregion/slot丸めや、少数のlive allocationが残るpageも解釈に含める必要があります。
- `decommit_unix.go`はcold領域へMADV_DONTNEEDを使用します。mappingを維持したまま物理page返却を試みるため、mapped bytesが一定でもRSSは変わり得ます。page headerを残すこと、OS page sizeの条件、madviseの成否等もあるので、decommit経路が存在するだけで所望のRSS返却が保証されたとは言えません。

固定サイズ・固定並行量でhiLiveが落ち着けばpoolも一定範囲へ落ち着くという仮説は合理的です。ただし、実際にhiLive/活用classが落ち着いたことを測らず、ソースの上限式だけから今回のRSSを説明し切ることはできません。

`libc.MemStat().Bytes`はOS mapping量で空きpoolも含み、`Allocs`は件数であってlive要求bytesではありません。libcのallocatorはpackage共通で、SQLiteだけを計る専用counterでもありません。通常buildではmemory.countersが無効で0になるため、0を未使用と解釈しません。SQLiteのCloseと、この共通allocator自身のClose/Trimは別です。DB Close後にもmappingが残ることは、直ちに解放漏れを示しません。

## 前回60分profile・傾きの再解釈

前回の2本はretry期限回収の1行修正前後であり、元RCと完成版全体の比較ではありません。同一ホストで約39分49秒重なり、前半・後半で並行負荷の有無も逆になっています。payload、退役active、履歴DBも増加していました。このため、after末尾の傾き低下をretry修正の効果として帰属できません。

さらに、bufio.Scannerを使うSSE clientとsnapshot GETのdecoderがアプリと同じGo processにありました。profile上のscanner bufferは試験client由来であり、それをproduction serverの保持rootと呼ぶのは不適切です。約+5.9MiB/時という前回のRSS傾きは、その合成processの記述値であって、server単体のリーク速度ではありません。

snapshot Incident clone/copyTime・JSON等も差分候補でしたが、件数/文字列長や処理率が変化するfixtureでは、正当なworking set変化と保持漏れをprofile差分だけで分離できません。sampled profileの推定objectsとMemStats.HeapObjectsの実件数も区別します。終了profileはSSE/HTTP停止後なので、そこでの減少を稼働中の保持不在の証拠にしません。

## 今回も完全には分離できない経路

**SQLite。** `sqlite_fixed`は固定Topology1行のSaveとLoadをそれぞれ4Hzで行い、JSON marshal、SQL bind、driver内のcopy、SQLite処理、Scan、JSON unmarshalを含みます。本番のMonitorは収集ごとにSaveしますが、Loadは起動時の`New`だけです。したがって、このcaseの高頻度Loadは本番の読込経路を増幅しています。ここで差が出ても、同じ大きさのRSS差が本番の定常書込だけで生じるとは言えません。

`sqlite_compact`も比較のため同じSave/Load cadenceを維持します。full fixtureをmetadataへ絞るため、保存・読込bytesは意図的に異なります。差分は「この増幅したround-trip条件で不要fieldを除いた影響」として解釈し、productionのRSS改善量へ直接換算しません。実際のSave-only/read-on-startup構成との寄与分離は別の確認条件です。

**collector。** 実Monitor.RefreshNowを使いますが、discovery Sourceとrepositoryは固定fixtureです。CQL OPTIONS/TCP/HTTP用の小さなserverも同じ測定processに含まれます。systemctl等の読み取り専用補助probeはchild processを起動し得ます。親RSSはそのchild自身のRSSを含みませんが、親のexec・pipe・buffer等の割り当ては含みます。このcaseはcollector＋fixture等の有限構成であり、本番collector全体、実gocql session/再接続/認証/TLS/内部decodeの長期挙動を分解したものではありません。

**JSONとHTTP/SSE。** json_onlyはPublish4HzとSnapshot clone/encode約9Hz、json_sseはPublish4Hz、外部persistent SSE2本とGET1Hzです。純粋なcodec単体対ネットワークという比較ではなく、clone＋codec対API/HTTP/SSE経路を含む構成の比較です。件数・bytesの実測、setup bootstrapからの差分、timestamp桁の変動、channel coalescingやprofile採取時のtick遅延を確認してから差を述べます。外部client/receiverのRSSを親RSSへ加算してserver単体値にはしません。

## Topology縮小の意味と限界

現在のコードでは、Monitorの起動時Load consumerはcluster一致を確認し、Nodesのmetadataを`known`へ取り込みます。元のlive snapshot全体にあるincidents/events/metrics/観測graphを、このconsumerが再利用する契約はありません。`topologyMetadata`はこの用途に必要なidentity、順序、metadataと元の鮮度時刻を保持し、callerから保存する対象を絞ります。Store.SaveTopology自体の汎用round-tripは維持しています。

これは重複したJSON/BLOB処理・保存量を減らす根拠のある修正です。意味上の妥当性は旧full/newmetadata、cluster mismatch、metadata鮮度、fallback、配信snapshot不変の回帰で評価します。しかし「不要な処理を減らせた」ことと「数週間のRSS微増の主因を除けた」ことは別です。GC外pool・Go JSON/arena・他の経路が残るため、実測が完了する前にRSS改善や原因確定を宣言しません。

## 自然GCと報告時の判断

今回の傾きには、warmup後の通常sampleだけを用い、baseline、warmup境界、終了後postStopGCを混ぜません。NumGC、LastGC/GC age、NextGC、HeapInuse/Idle/Released、HeapObjects等と併記し、GCの前後で値が上下することを確認します。idleのような低allocationケースでは、sampler自体のmap/JSONやprofile採取によるallocationが長く未回収のまま見える可能性があります。HeapAlloc/HeapObjectsの上昇だけではlive保持の増加と判定しません。

自然GCのheap profileは直近GC時点の情報・sampled推定を含み、測定時刻の全live objectを厳密に数えたものではありません。warmupで全ての高水位へ到達したと仮定せず、区間中央値・末尾区間・GC位相・処理率を合わせて記述します。profile採取やsamplerもallocationを伴うため、idle controlとの差を評価します。

停止後GCは別diagnosticです。core参照をKeepAliveしても、HTTP接続・worker・client停止でworking setが変わっているため、その減少だけでリーク不在とは言えません。逆に、停止後RSSが開始値へ戻らなくても、arena/pool保持があるので、それだけで漏れとも言えません。

逐次実行・新process・同binary/hash・同設定は前回の交絡を減らしますが、caseごと1回、異なる長さ・異なる実行順の短い試験です。差分RSSをcomponentごとの正確な内訳へ分解せず、傾きは観測した有限条件の記述値として扱います。閾値未到達、横ばい、PASS、候補の絞り込みのいずれも、数週間の本番相当問題の解消判定には置き換えません。
