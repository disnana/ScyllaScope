# RSS切り分け計画の独立レビュー

状態: 計画段階のread-onlyレビュー。ここには新しい実測結果やメモリ問題の合格判定を記載しません。本レビューでは計画文書だけを作成し、ランタイム・harnessを編集せず、コンパイルや試験を起動していません。harnessの最終実装との照合は実行担当者が行います。

前の60分試験はSSE/GETクライアントも同一プロセスに置いており、そのbuffer・HTTP clientの割り当ても測定RSS/heapに含まれていました。外部clientプロセスへ分ける今回の計画は、この交絡を減らします。前回の値を「本番サーバー単体のRSS」と再解釈しないでください。

## 予定する有限実験

すべて同じ `memory.counters` 付きbinary、Go・依存版、GOMEMLIMIT、fixture規模、計測方法で行い、1モードずつ逐次実行します。各ケースは新しい測定対象プロセスを起動してください。同一プロセスでmodeを切り替えると、前ケースのGo arena・native pool・HTTP cacheの高水位が後続ケースへ残り、切り分けになりません。binaryのSHA-256、case、開始/終了時刻を記録します。

| ケース | 全体時間 | warmup | 主に切り分ける経路 |
|---|---:|---:|---|
| idle | 5分 | 2分 | 共通fixtureと計測自体の定常負荷 |
| sqlite_fixed | 15分 | 3分 | 固定payloadのTopology 1行の上書き/読込、実SQLite allocator |
| collector_fixed | 15分 | 3分 | 固定3ノード、bounded fake repository、履歴を蓄積しないcollector |
| json_only | 10分 | 3分 | 固定snapshotのJSON処理 |
| json_sse | 15分 | 3分 | Publish 4Hz、外部2 SSE＋1 GET client、server HTTP/JSON経路 |
| notification | 15分 | 3分 | 実engine、固定500 open incident、AM互換loopback受信口の503/204とrefresh |

warmupは各ケースの全体時間に含めます。基本75分、必要性を結果から説明できる1ケースの追加確認を最大20分として、全体上限95分です。継続的な追加実験へ拡張せず、その時点の結果と未確定事項を報告します。同じホストで別のbuild・負荷試験を並行実行しないでください。

## 実行前に固定・確認する条件

1. **client・receiverの測定境界。** 外部clientのPIDと測定対象server PIDを区別し、serverの`/proc/self`だけをRSSとして記録します。clientのメモリを単純に足して本番server RSSとしません。notificationの偽AM受信口が測定対象と同一プロセスなら、結果はsender＋receiverの合計です。sender単体へ帰属するには受信口も外部化するか、同じ受信口を持つcontrolが必要です。実Alertmanager自体の長期allocator試験とは区別します。
2. **同じ量・速度の処理。** 全ケースの3 nodes/500 incidents、ID・message長・retained row数等を記録します。json_onlyとjson_sseはserialized bytesだけでなく、encode/decode回数・publish回数・成功GET/受信件数・平均処理率も記録します。時間だけが同じでも割り当て総量は一致しません。JSON日時の可変桁、header/event/incidentの変化、レスポンス方式の違いもpayloadのバイト数を変え得るため、min/maxを確認します。
3. **本当に固定されたrepository。** SQLiteは1行のみ、fake repositoryはイベント・履歴・node履歴も上限が固定され、古いsnapshotをsliceやcallbackで蓄積しないことを確認します。sqlite_fixedにもJSON marshal/unmarshalとGo driverの処理は含まれるため、RSS差をすべてnative allocatorへ帰属しません。
4. **本当に成立した負荷。** 外部clientの起動成功・終了状態、各測定区間のSSE接続/受信/GET成功、collector/notification処理の進行を記録します。client停止やrequestが動かない状態の「平坦なRSS」を成功扱いにしません。負荷率の最低条件は実行前に設定し、途切れた区間を結果から隠さず示します。
5. **boundedという事実を観測。** live/retained incident数、既知ノード、queue/inflight/retry件数、worker、subscriber、goroutine、FDを追います。queue128・worker32ならinflightの理論上限は160ですが、実装とfixtureに対応する上限を実行前に照合します。503/204は速い応答であり、ハング/遅延時のbackpressureまで同じケースで検証したとは扱いません。

## 計測と分析の注意

- 測定中に強制GC、cache clear、DB Close/reopen、allocator trim、再起動を入れません。自然GCのHeapAlloc/HeapObjectsに加え、NextGC、NumGC、LastGC、GoSys、HeapInuse/Idle/Released、native live allocation数・mapped bytes、RSSを同じ時系列へ記録します。
- 自然GC下のHeapAlloc傾きにはGC位相が混ざります。GC回数・時間と併記し、同程度のGC位相、区間中央値、ピーク/下限等も確認します。GCがほとんど起きないidleは計測controlであり、保持heapの長期安定性を証明しません。必要に応じてallocation rateも区別します。
- native mapped bytesは予約/空きpoolも含み、RSSでも未解放要求bytesでもありません。live allocationの**個数**が一定でも、1件あたりのサイズ・fragmentationは変わり得ます。GoSys一定＋RSS増加、あるいはRSS−GoSysだけからC allocatorへ原因を断定しません。匿名領域にはGoと非Goの両方が含まれます。
- warmup除外後と最後の区間の記述的な傾きを示します。3分で全ての高水位へ到達したとは仮定せず、GC・queue・payload状態も確認します。ケースごと1回の短い逐次実験で、統計的有意性や因果の完全分離を示すことはできません。実行順によるホスト状態の差も残ります。
- postStopGCは1回だけの別diagnosticとし、通常sampleの傾きへ含めません。coreの参照を保持していても、client停止・HTTP idle解放・worker停止でworking setは変わります。終了後の減少だけをリーク不在の証拠にしません。

## 停止条件と停止後の扱い

エラー、RSS256MiB超、native mapped128MiB超、資源上限超過、各ケースの時間上限で停止します。これらは資源保護・実験成立の上限であり、「未到達ならリークなし」という合格基準ではありません。低速度の漏れは15分で閾値に達しない可能性があります。

サンプル時の閾値は瞬間ピークを取り逃すため、実装上のhard capと観測間隔を併記してください。測定goroutineがblocked operationの後でしか判定しない設計では時間・RSS監視自体が遅れます。request deadline、独立したwall-clock watchdog、外部clientの終了回収があるかを実行前に確認します。監視がAPI応答だけに依存するとserver停止時に制限が効かないため、OSのRSS読取とwall-clockも必要です。

途中停止したケースは「中断/失敗」とし、取得済み区間・停止理由・最大値・未実行ケースを保存します。queueが残る場合は未処理状態を明示し、見えないように空にして再開しません。本番、権限・security設定、ホストsystemd/cgroup、merge/tag/releaseはこの計画の対象外です。

## この実験で分かる範囲

同じ固定規模・有限時間で、増加がSQLite経路、collector、JSON/HTTP/SSE、notificationのどこに現れやすいかという次の調査対象を絞ります。外部client化と固定状態で前回の交絡を減らせますが、各ケースには共有ランタイム・JSON・HTTP・allocator等の重なりがあり、差分RSSを単一componentの使用量へそのまま分解できません。

実ScyllaDBの長期driver挙動、数週間の微増、ノード入替/履歴蓄積/分断を含む本番相当の合格判定は別です。最大95分で候補が絞れない場合も、仮説を未確定のまま報告し、追加の2〜4週間受入が必要であることを残します。
