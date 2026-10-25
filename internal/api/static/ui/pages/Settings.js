import '../vendor/svelte_internal_disclose-version.js';
import '../vendor/svelte_internal_flags_legacy.js';
import * as $ from '../vendor/svelte_internal_client.js';

import {
	config,
	preferences,
	auth,
	request,
	notify,
	exportDiagnostics,
	snapshot
} from '../state.js';

import { copy, download, date } from '../lib.js';
import Icon from '../components/Icon.js';
import Check from '../components/Check.js';

var root_1 = $.from_html(`<button> </button>`);
var root_3 = $.from_html(`<div class="diagnostic-node"><strong> </strong><!><!></div>`);
var root_4 = $.from_html(`<p class="hint"> </p>`);
var root_2 = $.from_html(`<div class="diagnostic"><h3> </h3><div class="checks-list"><div><span>クラスタ一覧</span><!></div></div><!><!><details><summary>JSONを見る</summary><pre> </pre></details></div>`);
var root_5 = $.from_html(`<p class="error-text" role="alert"> </p>`);
var root = $.from_html(`<section class="panel"><nav class="tabs settings-tabs" aria-label="設定タブ"></nav><div class="settings-body"><section><h2>このブラウザの表示設定</h2><p class="hint">サーバーの監視設定とは別に保存されます。自動更新中も変更を維持します。</p><div class="settings-row"><div><strong>テーマ</strong><p>ダーク / ライト</p></div><select aria-label="テーマ"><option>ダーク</option><option>ライト</option></select></div><div class="settings-row"><div><strong>情報密度</strong><p>既定はコンパクト。操作領域を広くする場合は標準。</p></div><select aria-label="情報密度"><option>コンパクト</option><option>標準</option></select></div><div class="settings-row"><div><strong>ショートカット</strong><p>Alt + 1〜8: ページ切替 / Esc: 詳細を閉じる / Ctrl + Enter: CQL実行</p></div></div></section> <section><h2>実行中の監視設定</h2><p class="hint">サーバー設定は読み取り表示です。反映には設定ファイルを編集してScopeのみ再起動します。</p><dl class="definition"><dt>監視間隔 / タイムアウト</dt><dd> </dd><dt>異常確定 / 復旧確定</dt><dd> </dd><dt>期待するメンバー数</dt><dd> </dd><dt>保存期間 / サンプル間隔</dt><dd> </dd><dt>Discord</dt><dd> </dd><dt>汎用Webhook</dt><dd> </dd><dt>通知元</dt><dd> </dd></dl><button><!> </button><details class="config-example"><summary>設定例を表示</summary><pre></pre><p class="hint">例を丸ごと既存ファイルに上書きしないでください。認証トークン・CQL認証・通知URLを消さないよう、必要な項目だけ変更します。</p><button>設定例を保存</button></details></section> <section><h2>接続・認証</h2><dl class="definition"><dt>UI認証</dt><dd> </dd><dt>ノード間の認証</dt><dd> </dd><dt>CQLコンソール</dt><dd> </dd><dt>Scope待受</dt><dd class="mono"> </dd><dt>CQL接続先</dt><dd class="mono"> </dd><dt>ノード間 / メトリクス</dt><dd> </dd><dt>Scylla REST</dt><dd class="mono wrap"> </dd></dl><details><summary>設定の読み取り元</summary><dl class="definition"><dt>Scope override</dt><dd class="mono wrap"> </dd><dt>Scyllaの実設定</dt><dd class="mono wrap"> </dd></dl></details><p class="notice">管理UIはTailscaleなどの信頼できるネットワーク内に限定してください。多人数のRBAC・SSO・操作承認ワークフローは、この版には含めていません。</p></section> <section><h2>読み取り専用の診断</h2><p class="hint">接続・メンバー取得・監視の範囲を再確認します。DBの再起動やrepairは実行しません。</p><div class="button-group"><button class="primary"><!> </button><button><!>診断JSON</button></div><p class="hint">出力にはIP・ノード名を含みます。共有前に確認してください。パスワード・トークン・DB行データは出力しません。</p><!></section> <!></div></section>`);

export default function Settings($$anchor, $$props) {
	$.push($$props, false);

	const $preferences = () => $.store_get(preferences, '$preferences', $$stores);
	const $config = () => $.store_get(config, '$config', $$stores);
	const $auth = () => $.store_get(auth, '$auth', $$stores);
	const $snapshot = () => $.store_get(snapshot, '$snapshot', $$stores);
	const [$$stores, $$cleanup] = $.setup_stores();
	let tab = $.mutable_source('general');
	let busy = $.mutable_source('');
	let diagnostic = $.mutable_source(null);
	let error = $.mutable_source('');
	let sourceOpen = $.mutable_source(false);

	async function action(name) {
		if ($.get(busy)) return;

		$.set(busy, name);
		$.set(error, '');

		try {
			if (name === 'test') {
				await request('notifications/test', { method: 'POST', body: '{}' });
				notify('テスト通知を送信しました');
			} else if (name === 'doctor') $.set(diagnostic, await request('doctor', { method: 'POST', body: '{}' })); else if (name === 'export') download('scyllascope-diagnostics.json', await exportDiagnostics());
		} catch(e) {
			$.set(error, e.message);
			notify(e.message, 'error');
		} finally {
			$.set(busy, '');
		}
	}

	const example = `# /etc/scyllascope/config.yaml（既存の認証情報は保持）\nmonitoring:\n  expected_nodes: 3\n  interval: 10s\n  timeout: 3s\n  failure_window: 3\n  recovery_window: 2\n# 全観測ノードで同じ値を設定\n# cluster_token: "openssl rand -hex 32 で一度だけ生成した値"\nnotifications:\n  discord_webhook: ""\n  generic_webhook: ""\n# 変更後: sudo systemctl restart scyllascope\n`;

	$.init();

	var section = root();
	var nav = $.child(section);

	$.each(
		nav,
		4,
		() => [
			['general', '表示'],
			['monitor', '監視・通知'],
			['security', '接続・セキュリティ'],
			['diagnostics', '診断']
		],
		$.index,
		($$anchor, $$item) => {
			var $$array = $.derived(() => $.to_array($$item, 2));
			let id = () => $.get($$array)[0];
			let label = () => $.get($$array)[1];
			var button = root_1();

			button.__click = () => $.set(tab, id());

			let classes;
			var text = $.child(button, true);

			$.reset(button);

			$.template_effect(() => {
				classes = $.set_class(button, 1, '', null, classes, { active: $.get(tab) === id() });
				$.set_text(text, label());
			});

			$.append($$anchor, button);
		}
	);

	$.reset(nav);

	var div = $.sibling(nav);
	var section_1 = $.child(div);
	var div_1 = $.sibling($.child(section_1), 2);
	var select = $.sibling($.child(div_1));

	select.__change = (e) => preferences.update((p) => ({ ...p, theme: e.target.value }));

	var option = $.child(select);

	option.value = option.__value = 'dark';

	var option_1 = $.sibling(option);

	option_1.value = option_1.__value = 'light';
	$.reset(select);

	var select_value;

	$.init_select(select);
	$.reset(div_1);

	var div_2 = $.sibling(div_1);
	var select_1 = $.sibling($.child(div_2));

	select_1.__change = (e) => preferences.update((p) => ({ ...p, density: e.target.value }));

	var option_2 = $.child(select_1);

	option_2.value = option_2.__value = 'compact';

	var option_3 = $.sibling(option_2);

	option_3.value = option_3.__value = 'comfortable';
	$.reset(select_1);

	var select_1_value;

	$.init_select(select_1);
	$.reset(div_2);
	$.next();
	$.reset(section_1);

	var section_2 = $.sibling(section_1, 2);
	var dl = $.sibling($.child(section_2), 2);
	var dd = $.sibling($.child(dl));
	var text_1 = $.child(dd);

	$.reset(dd);

	var dd_1 = $.sibling(dd, 2);
	var text_2 = $.child(dd_1);

	$.reset(dd_1);

	var dd_2 = $.sibling(dd_1, 2);
	var text_3 = $.child(dd_2, true);

	$.reset(dd_2);

	var dd_3 = $.sibling(dd_2, 2);
	var text_4 = $.child(dd_3);

	$.reset(dd_3);

	var dd_4 = $.sibling(dd_3, 2);
	var text_5 = $.child(dd_4, true);

	$.reset(dd_4);

	var dd_5 = $.sibling(dd_4, 2);
	var text_6 = $.child(dd_5, true);

	$.reset(dd_5);

	var dd_6 = $.sibling(dd_5, 2);
	var text_7 = $.child(dd_6, true);

	$.reset(dd_6);
	$.reset(dl);

	var button_1 = $.sibling(dl);

	button_1.__click = () => action('test');

	var node = $.child(button_1);

	Icon(node, { name: 'alerts', size: 15 });

	var text_8 = $.sibling(node, 1, true);

	$.reset(button_1);

	var details = $.sibling(button_1);
	var pre = $.sibling($.child(details));

	pre.textContent = '# /etc/scyllascope/config.yaml（既存の認証情報は保持）\nmonitoring:\n  expected_nodes: 3\n  interval: 10s\n  timeout: 3s\n  failure_window: 3\n  recovery_window: 2\n# 全観測ノードで同じ値を設定\n# cluster_token: "openssl rand -hex 32 で一度だけ生成した値"\nnotifications:\n  discord_webhook: ""\n  generic_webhook: ""\n# 変更後: sudo systemctl restart scyllascope\n';

	var button_2 = $.sibling(pre, 2);

	button_2.__click = () => download('scyllascope-monitoring-example.yaml', example, 'text/yaml');
	$.reset(details);
	$.reset(section_2);

	var section_3 = $.sibling(section_2, 2);
	var dl_1 = $.sibling($.child(section_3));
	var dd_7 = $.sibling($.child(dl_1));
	var text_9 = $.child(dd_7, true);

	$.reset(dd_7);

	var dd_8 = $.sibling(dd_7, 2);
	var text_10 = $.child(dd_8, true);

	$.reset(dd_8);

	var dd_9 = $.sibling(dd_8, 2);
	var text_11 = $.child(dd_9, true);

	$.reset(dd_9);

	var dd_10 = $.sibling(dd_9, 2);
	var text_12 = $.child(dd_10, true);

	$.reset(dd_10);

	var dd_11 = $.sibling(dd_10, 2);
	var text_13 = $.child(dd_11);

	$.reset(dd_11);

	var dd_12 = $.sibling(dd_11, 2);
	var text_14 = $.child(dd_12);

	$.reset(dd_12);

	var dd_13 = $.sibling(dd_12, 2);
	var text_15 = $.child(dd_13, true);

	$.reset(dd_13);
	$.reset(dl_1);

	var details_1 = $.sibling(dl_1);
	var dl_2 = $.sibling($.child(details_1));
	var dd_14 = $.sibling($.child(dl_2));
	var text_16 = $.child(dd_14, true);

	$.reset(dd_14);

	var dd_15 = $.sibling(dd_14, 2);
	var text_17 = $.child(dd_15, true);

	$.reset(dd_15);
	$.reset(dl_2);
	$.reset(details_1);
	$.next();
	$.reset(section_3);

	var section_4 = $.sibling(section_3, 2);
	var div_3 = $.sibling($.child(section_4), 2);
	var button_3 = $.child(div_3);

	button_3.__click = () => action('doctor');

	var node_1 = $.child(button_3);

	Icon(node_1, { name: 'shield', size: 15 });

	var text_18 = $.sibling(node_1, 1, true);

	$.reset(button_3);

	var button_4 = $.sibling(button_3);

	button_4.__click = () => action('export');

	var node_2 = $.child(button_4);

	Icon(node_2, { name: 'download', size: 15 });
	$.next();
	$.reset(button_4);
	$.reset(div_3);

	var node_3 = $.sibling(div_3, 2);

	{
		var consequent = ($$anchor) => {
			var div_4 = root_2();
			var h3 = $.child(div_4);
			var text_19 = $.child(h3);

			$.reset(h3);

			var div_5 = $.sibling(h3);
			var div_6 = $.child(div_5);
			var node_4 = $.sibling($.child(div_6));

			{
				let $0 = $.derived_safe_equal(() => $snapshot()?.stale_after_seconds);

				Check(node_4, {
					get value() {
						return $.get(diagnostic).discovery;
					},

					get ttl() {
						return $.get($0);
					}
				});
			}

			$.reset(div_6);
			$.reset(div_5);

			var node_5 = $.sibling(div_5);

			$.each(node_5, 1, () => $.get(diagnostic).nodes || [], (n) => n.id, ($$anchor, n) => {
				var div_7 = root_3();
				var strong = $.child(div_7);
				var text_20 = $.child(strong, true);

				$.reset(strong);

				var node_6 = $.sibling(strong);

				Check(node_6, {
					get value() {
						return $.get(n).cql;
					},
					caption: 'CQL'
				});

				var node_7 = $.sibling(node_6);

				Check(node_7, {
					get value() {
						return $.get(n).internode;
					},
					caption: '7000'
				});

				$.reset(div_7);
				$.template_effect(() => $.set_text(text_20, $.get(n).name));
				$.append($$anchor, div_7);
			});

			var node_8 = $.sibling(node_5);

			$.each(node_8, 1, () => $.get(diagnostic).warnings || [], $.index, ($$anchor, w) => {
				var p_1 = root_4();
				var text_21 = $.child(p_1, true);

				$.reset(p_1);
				$.template_effect(() => $.set_text(text_21, $.get(w)));
				$.append($$anchor, p_1);
			});

			var details_2 = $.sibling(node_8);
			var pre_1 = $.sibling($.child(details_2));
			var text_22 = $.child(pre_1, true);

			$.reset(pre_1);
			$.reset(details_2);
			$.reset(div_4);

			$.template_effect(
				($0, $1) => {
					$.set_text(text_19, `診断結果 · ${$0 ?? ''}`);
					$.set_text(text_22, $1);
				},
				[
					() => date($.get(diagnostic).checked_at),
					() => JSON.stringify($.get(diagnostic), null, 2)
				]
			);

			$.append($$anchor, div_4);
		};

		$.if(node_3, ($$render) => {
			if ($.get(diagnostic)) $$render(consequent);
		});
	}

	$.reset(section_4);

	var node_9 = $.sibling(section_4, 2);

	{
		var consequent_1 = ($$anchor) => {
			var p_2 = root_5();
			var text_23 = $.child(p_2, true);

			$.reset(p_2);
			$.template_effect(() => $.set_text(text_23, $.get(error)));
			$.append($$anchor, p_2);
		};

		$.if(node_9, ($$render) => {
			if ($.get(error)) $$render(consequent_1);
		});
	}

	$.reset(div);
	$.reset(section);

	$.template_effect(
		($0) => {
			$.set_attribute(section_1, 'hidden', $.get(tab) !== 'general');

			if (select_value !== (select_value = $preferences().theme)) {
				(
					select.value = (select.__value = $preferences().theme) ?? '',
					$.select_option(select, $preferences().theme)
				);
			}

			if (select_1_value !== (select_1_value = $preferences().density)) {
				(
					select_1.value = (select_1.__value = $preferences().density) ?? '',
					$.select_option(select_1, $preferences().density)
				);
			}

			$.set_attribute(section_2, 'hidden', $.get(tab) !== 'monitor');
			$.set_text(text_1, `${$config()?.interval ?? ''} / ${$config()?.timeout ?? ''}`);
			$.set_text(text_2, `${$config()?.failure_window ?? ''}回 / ${$config()?.recovery_window ?? ''}回`);
			$.set_text(text_3, $config()?.expected_nodes || '自動検出');
			$.set_text(text_4, `${$config()?.history_hours ?? ''}時間 / ${$config()?.sample_interval ?? ''}`);
			$.set_text(text_5, $config()?.notifications?.discord ? '設定済み' : '未設定');
			$.set_text(text_6, $config()?.notifications?.generic ? '設定済み' : '未設定');
			$.set_text(text_7, $config()?.notifications?.sender_node || '各観測ノードが自分の結果を通知');
			button_1.disabled = !$auth()?.authenticated || $.get(busy) !== '' || !($config()?.notifications?.discord || $config()?.notifications?.generic);
			$.set_text(text_8, $.get(busy) === 'test' ? '送信中' : 'テスト通知');
			$.set_attribute(section_3, 'hidden', $.get(tab) !== 'security');
			$.set_text(text_9, $config()?.ui_auth_configured ? '有効（共有管理トークン）' : '無効・読取専用');
			$.set_text(text_10, $config()?.peer_auth_configured ? '設定済み' : '未設定（状態共有しません）');
			$.set_text(text_11, $config()?.console_enabled ? '有効・SELECTのみ' : '無効');
			$.set_text(text_12, $config()?.listen);
			$.set_text(text_13, `${$0 ?? ''}:${$config()?.cql_port ?? ''}`);
			$.set_text(text_14, `${$config()?.internode_port ?? ''} / ${$config()?.prometheus_port ?? ''}`);
			$.set_text(text_15, $config()?.rest_url);
			$.set_text(text_16, $config()?.config_source);
			$.set_text(text_17, $config()?.scylla_config);
			$.set_attribute(section_4, 'hidden', $.get(tab) !== 'diagnostics');
			button_3.disabled = !$auth()?.authenticated || $.get(busy) !== '';
			$.set_text(text_18, $.get(busy) === 'doctor' ? '診断中' : '診断を実行');
			button_4.disabled = $.get(busy) !== '';
		},
		[() => ($config()?.cql_hosts || []).join(', ')]
	);

	$.bind_property('open', 'toggle', details_1, ($$value) => $.set(sourceOpen, $$value), () => $.get(sourceOpen));
	$.append($$anchor, section);
	$.pop();
	$$cleanup();
}

$.delegate(['click', 'change']);