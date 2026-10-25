# メモリ調査と長期受入

RCの対策を確認しました: 型付きsnapshot clone、SSE容量1の更新通知、購読解放、最大32接続と書込期限、peer応答2MiB制限、HTTP idle接続期限、CQL prepared/routingキャッシュ各32、discoveryメタデータ最大2×max_nodes、SQLiteキャッシュ2MiB/mmap無効、UIキャッシュ16件とrequest中止です。これらを保持します。

追加で、未復旧のまま退役したノードの判定カウンターが、SQLite上のインシデントと一緒に毎回tracks mapへ残る経路を修正しました。新しい正常証拠がない時は連続判定カウンターを削除します。インシデント自体は閉じないため、データ不足を復旧とは扱いません。1000個の退役identityを使う回帰試験で保持がなくなることを確認します。表示上限500件の解除を単純な全履歴読込にせず、通知対象だけを256件ずつ読みます。送信時の検索はID指定です。

この経路はidentityが増える運用でメモリ保持を増やします。実環境の数週間の微増の主因だったという証拠はありません。実際のheap profile、identity数、CQLセッション・SQLite量、RSSの時系列が必要です。短いsoakで長期問題が解決したとは判定しません。

受入計画:

1. 同条件の旧版・新版で、最低2〜4週間の実時間観測を実施。更新、ノード入替、peer断、通知先停止と復旧、UI再接続、履歴pruneを含める。
2. `tools/observe_process.py --duration 604800 --interval 60 --output <private-path>` で1週間ずつRSS、FD等を保存し、別ファイルへ2〜4回続ける。出力は公開しない。認証付き `/api/v1/runtime` のheap/GC/goroutineと、監視DBサイズ、ノード/インシデント数を併せて記録。
3. ウォームアップ後の同じ負荷・GC周期で比較。RSSとheap-inuse/heap-alloc、mmap/page cache、GC返却待ちを分ける。FD/goroutine/保持identityに継続増加がないか確認。
4. heapが増える場合は許可されたステージング環境で限定的にheap profileを取得し、差分の保持元を追跡。公開pprofは追加していません。
5. 単一の開始値と終了値だけでは合格にしない。日ごとの傾向、負荷/DBサイズの変化、旧版との差、増加の再現性を根拠に判断。

今回の有限試験はHTTP/SSE模擬データと、実modernc SQLite＋インシデントchurnの2系統です。本番collector/CQL・実ScyllaDB・systemd・実際の数週間稼働は含みません。
