# 取り込み元と公開範囲

取り込み元: ユーザー提供の `scyllascope-v0.6.3-rc.2.zip`。
ZIP SHA-256: `cd46898f2c4f49f214feb2e17a4ce393b5a0db34982c7de2ce97f6c7e584abf1`。
展開した元ソースのSOURCE-MANIFEST.sha256照合は成功しました。

リポジトリ既存LICENSEはバイト単位で変更せず保持し、元RCのMIT表示はLICENSES/RC-MIT.txtに残しています。Svelteと関連ライセンス、元のvendorチェックサムを保持しています。

公開対象はコード、合成データのテスト、汎用設定例、公開用に書き直した手順と検証要約です。RC内の実環境向けexamples、docs/evidence、docs/rc1、元レビュー・検証の生ログ/JSON、設計参照画像、古いソースパッチ・manifestは取り込んでいません。コードとfixtureの実アドレス・環境固有名称も合成値へ置換しています。100.64.0.1〜3はPrivateAddress判定の試験用CGNアドレスで、実配置先ではありません。

今回の検証でも、公開するのは集計・計測値と実施条件だけです。ローカルの生ログ、ブラウザ画像、バイナリ、BUILD-VERIFIED.sha256、運用設定、ui-token、secrets.env、DB/WAL/SHMはコミット対象外です。

`python3 tools/check_publication.py` はGitのindexに含まれるパス・資格情報パターン・CGNアドレスを確認します。手動レビューと併用しており、あらゆる秘密情報が存在しないことの数学的な証明ではありません。
