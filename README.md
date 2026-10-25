# ScyllaScope

ScyllaDBの各ノードから接続・クラスタ状態を観測する監視UIです。添付 v0.6.3-rc.2 を元に正式版を準備中です。バージョンはRCのまま保持し、正式リリース・マージは承認待ちです。

- RCのSSE、HTTP、CQLキャッシュ上限・解放対策を保持。退役ノードのインシデント判定カウンターを解放します。
- 通知用のページ取得とID検索を追加。表示用500件上限と通知処理を分離し、未送信の障害・復旧をSQLiteに保持します。
- Alertmanager連携で複数観測元の障害・復旧を集約。種類の異なる障害を分け、固定通知担当への依存をなくします。[通知構成](docs/NOTIFICATIONS.md)
- 検証済みビルドからのインストーラー、設定を保持した更新、チェックサム付きバックアップからの切り戻しを用意します。[導入・更新手順](UPGRADE.md)

## ビルド・検証

Go 1.25以上（保守対象の最新パッチを推奨）、Node.js、Python 3が必要です。

```bash
node web/build.mjs
node --test web/test/*.test.mjs
python3 tools/test_build_gate.py
./build.sh
```

`build.sh` は正規依存の検証、全体test/vet/race、本番CLIビルドを必須とします。Goのインストールや実サービスの変更は行いません。`go.sum`を同梱しています。

継続負荷試験は別実行です。

```bash
SCYLLASCOPE_SOAK_SECONDS=600 GOMEMLIMIT=128MiB go test -timeout=15m -count=1 -v ./internal/api ./tests/runtime -run 'TestSoakRuntime|TestSQLiteIncidentSoak'
```

[今回の検証結果と未検証範囲](VALIDATION.md)を確認してください。短時間の負荷試験から数週間のメモリ問題の解消を断定しません。[長期測定計画](docs/MEMORY.md)

## 運用

既存設定の検出元は通常 `/etc/scylla/scylla.yaml`、ScyllaScope設定は `/etc/scyllascope/config.yaml` です。監視UIは閉域ネットワークを基本とし、UIトークンと共通cluster_tokenを使ってください。設定例のアドレス・識別子はすべて合成値です。

ScyllaDBのサービス停止・再起動、設定変更、repair、データ変更は行いません。systemdは専用ユーザーでScyllaScopeだけを管理します。`GOMEMLIMIT=128MiB` はGoのソフト目標であり、RSS上限ではありません。

## ライセンス

リポジトリ既存の[MIT LICENSE](LICENSE)を変更せず保持しています。第三者コンポーネントの表示は[THIRD_PARTY.md](THIRD_PARTY.md)にあります。
