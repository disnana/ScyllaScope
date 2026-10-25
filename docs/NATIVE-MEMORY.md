# modernc SQLiteのGo GC外メモリ診断

今回使用するSQLiteはPure Goですが、全割り当てがGo GC管理下に入るという意味ではありません。`cache_size=-2048`、`mmap_size=0`やGOMEMLIMITだけでプロセス全体のRSSを制限できるとは扱いません。

## ローカル依存ソースで確認した経路

対象はgo.sumに固定された`modernc.org/sqlite v1.59.0`、`modernc.org/libc v1.75.7`、`modernc.org/memory v1.12.1`、linux/amd64です。

- SQLite `lib/sqlite_g_000000000000002b.go:309`の`_sqlite3MemMalloc`は`libc.Xmalloc`を呼びます。
- libc `mem_musl.go:20`の`Xmalloc`は、`libc_musl.go:151`にあるpackage共通の`memory.Allocator`を使用します。
- memory `mmap_unix.go:39`は`unix.MmapPtr`で匿名領域（MAP_PRIVATE|MAP_ANON）をOSから取得します。この領域はGo heap/MemStatsに含まれません。GOMEMLIMITもこのOS mapping全体の上限にはなりません。
- memory `memory.go:131`は空きpoolの上限を4MiB＋過去のlive mapping高水位とします。`:287`の`release`は空regionをpoolへ残し、`:355`以降の`UintptrFree`はサイズclassごとの空carve-target pageも保持します。64bitでは大regionがpower-of-twoに丸められ、小slotにもsize classがあります。
- `decommit_unix.go`にはMADV_DONTNEEDによる返却もあります。poolを保持することと、全mappingが常にresidentであることは同じではありません。サイズclassごとのhot window等によってresidentの戻り方が変わります。
- SQLite `lib/sqlite_g_0000000000000480.go:15259`のcache_sizeはpage cache、`:15324`のmmap_sizeはdatabase-file mappingを対象にしています。上記malloc allocator全体の上限ではありません。

履歴件数やJSON/一時bufferサイズの変化で、allocatorのサイズclass、高水位、部分live pageによる断片化が変化する経路は候補です。履歴DBのディスクサイズ増加そのものがRSSを線形増加させると確認したわけではありません。

## opt-inの短い実SQLite診断

`internal/store/native_memory_test.go`は`memory.counters` build tagと専用環境変数の両方を必要とし、通常buildではファイルごと除外されます。本番ランタイムを変更しません。単一のSnapshot JSONを64、96、128、192、256、384、512KiBの合成payloadで上書き・読込し（各5秒）、64KiBへ縮小後、DB CloseとGC後を記録します。履歴行数を増やす試験ではありません。

```bash
SCYLLASCOPE_NATIVE_MEMORY_DIAGNOSTIC=1 GOMEMLIMIT=128MiB \
  go test -tags=memory.counters -run '^TestNativeMemoryDiagnostic$' \
  -timeout=4m -count=1 -v ./internal/store
```

各段階でGoSys、heap、RSSと`libc.MemStat()`を記録します。`allocator_mapped_bytes`はOSから取得したmapping量で、空きpoolも含み、RSSではありません。`allocator_live_allocations`は未解放allocation数であり、その合計要求bytesではありません。

**通常buildのlibc.MemStatカウンターは無効で0になります。0をメモリ未使用と解釈してはいけません。** 診断buildでは実SQLite Open後にcounterが非0となることも確認します。測定要約は[ネイティブメモリ診断結果](validation/native-memory.json)に保存します。元のpayload、生ログ、DBは公開せず、一時DBは試験終了時に削除します。

## 今回の有限測定結果

Go 1.27.3で最終版診断を実行し、35.03秒・28,888回の上書き/読込で成功しました。初回tag付きコンパイルを含め10分以内に完了しています。

| 段階 | allocator live件数 | allocator mapped bytes | Go heap alloc bytes | GoSys bytes | RSS bytes |
|---|---:|---:|---:|---:|---:|
| Open前 | 2 | 1,048,576 | 225,088 | 12,945,688 | 7,340,032 |
| Open後 | 258 | 1,048,576 | 249,016 | 13,273,368 | 9,867,264 |
| 512KiB payload後 | 388 | 6,029,312 | 405,352 | 30,345,496 | 22,511,616 |
| 64KiBへ縮小後 | 388 | 6,029,312 | 477,160 | 30,345,496 | 20,725,760 |
| Close＋GC後 | 4 | 6,029,312 | 401,416 | 30,345,496 | 20,344,832 |

Close後はlive allocation数が4件まで減った一方、allocator mappingは約6MiB・40領域を保持しました。解放後のpool/page保持と整合しますが、poolと部分live領域の内訳は測っていません。GoSysも測定中に増えており、RSS差の全量をGC外allocatorへ帰属することはできません。この測定を現在の長期runの因果証明に読み替えません。

## 解釈の限界

この別process・別build-tagの短時間試験は、GC外のmapping保持が実際に起きる経路を調べるための診断です。現在の60分runのRSS変動の原因、実環境の数週間の微増の主因、あるいは解消を証明するものではありません。counter tag、payload、DB操作パターンも通常runとは異なります。

GoSys一定でもRSSは変動します。予約済みGo pageのresident化、HeapReleasedの変化、stack、コード、file/SHM領域、GC外allocator等が候補です。GoSysはRSSではないため、単純にRSS−GoSysをGC外allocator使用量と解釈できません。次の切り分けには、固定データ量・固定payloadの比較、allocator live/mapped数の継続記録、HeapInuse/Idle/Released、仮想アドレスやファイルパスを除いたsmaps集計が必要です。現在動作中の長時間測定に計測設定や強制Trimを差し込んではいません。

後半の通常runでsmapsも集計しましたが、名前付きGo VMAラベルは観測できませんでした。匿名領域の合計をそのままC allocator量と解釈しません。`libc.MemStat`はpackage共通allocatorのcounterであり、プロセス全体のRSSやSQLiteだけの要求bytesとは異なります。診断testによる直接importのためlibc v1.75.7をgo.modのdirect欄へ移しましたが、依存versionは変えていません。
