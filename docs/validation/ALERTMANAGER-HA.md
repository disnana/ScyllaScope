# 実Alertmanager 2 processの停止・復帰試験

2026-10-25、隔離された開発環境でAlertmanager 0.28.1の実バイナリ2個を起動して検証しました。入力は合成インシデント、HTTP受信口とHA gossipはすべてloopbackです。本番サーバー・ScyllaDBには接続していません。本番ランタイムコードは変更していません。

`tools/test_alertmanager_ha.py` は独立した2つの永続storage、同一のローカルwebhook受信口、相互のcluster.peerを用意します。どちらが実際に最初の通知を配送したかをreceiverで識別し、その配送担当processを停止するため、任意の1台を止めるだけの試験とは区別しています。

実施した確認：

1. `/api/v2/status` で両方のclusterが2 peerを認識。
2. 2観測元の同じCQL障害を両受信口へ送信し、初回は2観測元を含む1通知へ集約。
3. 実際にその通知を配送したprocessをSIGTERMで停止。
4. 残ったprocessに別のinternode障害を送信し、通知が到達。
5. CQL障害の片側だけ復旧しても全体をresolvedにはしないことを確認し、もう片側を復旧するとresolved通知が到達。
6. internode障害の復旧通知も残ったprocessから到達。
7. 停止したprocessを同じstorageで再起動し、2 peerへの再joinを確認。
8. 両受信口へ完了済みのresolved状態を再送し、5秒間に追加の重複通知がないことを確認。
9. 再join後に新しいservice障害を両受信口へ送信し、通知が到達。

実行要約は[alertmanager-ha.json](alertmanager-ha.json)です。受信した元payload・IP/ポート・processログは要約に含めず、一時領域を試験終了時に削除しています。起動したprocessは成功時・失敗時とも終了させます。

```bash
python3 tools/test_alertmanager_ha.py /path/to/alertmanager \
  --output docs/validation/alertmanager-ha.json
```

試験を有限時間にするため、peer timeout=2秒、settle timeout=3秒、pushpull/reconnect interval=1秒、group wait/interval=1秒を使用しました。これらを本番推奨値とは扱いません。repeat_intervalは1時間です。観測元の状態変化によるgroup更新通知は発生し得ます。

保証しない範囲：SIGKILL・ディスク喪失、実ネットワーク分断・遅延/パケット喪失・clock skew、異なるホスト間のHA、長期間のrepeat/dedup、通知のexactly-once。Alertmanagerのactive alerts自体は再起動時の状態再送が必要であり、永続storageを使ったことだけで全アラートの永続配送を保証しません。ScyllaScopeのSQLite pending・再試行・有限workerの試験は別の回帰/継続負荷で実施しています。
