# 継続レビューの試験設計

目的は、短い試験の開始/終了値だけでは分からない保持の増加と、通知失敗・ノード入れ替え時の資源上限を観測することです。本番には接続しません。

- 60分の単一collector/SQLite/alert engineプロセス。途中再起動でリークを隠さない。
- 最初に4096件の永続pendingと600件の既知未復旧履歴を投入。表示の500件working setを開始前から満たし、表示件数増加による正当なheap増大とリークを分ける。128件のキューより多い未処理作業を通知先503/復帰へ流す。
- 250msごとの実collector更新、5秒ごとの合成ノード入れ替え、1分ごと10秒のdiscovery失敗、2分ごと45秒の通知先503。実SQLite履歴保存・prune・実認証HTTP/SSE再接続を同時実行。
- 最初の5分をウォームアップ。30秒ごとに明示GC後のheap allocation、heap objects、heap in-use、RSS、FD/goroutine、known node数、queue/inflight/attempt/counter数を採取。
- 5分・30分・終了時にheap profileを非公開の一時ディレクトリへ保存。後半の回帰傾きと保持オブジェクトの種類を調べる。
- queue<=128、attempt記録<=2048、known<=2×max_nodesを実行中に確認。worker並列化のinflight上限と500件処理は別の回帰試験で確認。
- 初期値まで戻ることを合格条件にしない。後半に持続増加が見える場合は、その要因と未解決事項を報告。

この試験は合成discoveryとloopbackのネットワークfixtureを使用します。CQL native session、OSの本物のサービス管理、実クラスタの数週間の稼働とは区別します。別に隔離ScyllaDBの統合試験と最終HEADのbuild/test/race/browser/PR CIを実施します。60分を数週間の代用とは扱いません。
