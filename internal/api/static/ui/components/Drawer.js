import '../vendor/svelte_internal_disclose-version.js';
import '../vendor/svelte_internal_flags_legacy.js';
import * as $ from '../vendor/svelte_internal_client.js';
import { tick } from '../vendor/svelte_svelte.js';
import { snapshot, selected, clock, notify, request } from '../state.js';

import {
	nodeState,
	shortStatus,
	latency,
	bytes,
	age,
	date,
	copy,
	edgeState,
	labels,
	checkState
} from '../lib.js';

import Badge from './Badge.js';
import Check from './Check.js';
import Icon from './Icon.js';
import Chart from './Chart.js';

var root_2 = $.from_html(`<button> </button>`);
var root_4 = $.from_html(`<p> </p>`);
var root_3 = $.from_html(`<div class="notice warning"></div>`);
var root_5 = $.from_html(`<div><span> </span><!></div>`);
var root_6 = $.from_html(`<p class="error-text"> </p>`);
var root_7 = $.from_html(`<button class="connection-row"><span class="wrap"> </span><!></button>`);
var root_8 = $.from_html(`<p class="quiet">方向別の観測がありません。各ノードにScyllaScopeと共通のcluster_tokenが必要です。</p>`);
var root_9 = $.from_html(`<div><code> </code><strong class="mono"> </strong></div>`);
var root_10 = $.from_html(`<p class="quiet">該当するメトリクスがありません。</p>`);
var root_1 = $.from_html(`<div class="drawer-meta"><!><span class="mono"> </span><button class="icon-button" aria-label="IPをコピー"><!></button></div> <nav class="tabs" aria-label="ノード詳細タブ"></nav> <div class="drawer-body"><section><!> <div class="checks-list"></div> <dl class="definition"><dt>データセンター / Rack</dt><dd> </dd><dt>Scyllaバージョン</dt><dd class="wrap"> </dd><dt>Shards</dt><dd> </dd><dt>起動引数のメモリ上限</dt><dd> </dd><dt>データ量</dt><dd> </dd><dt>Scopeバージョン</dt><dd> </dd><dt>最終観測</dt><dd> </dd></dl> <details><summary>識別情報とチェックの説明</summary><dl class="definition"><dt>Host ID</dt><dd class="mono wrap"> </dd><dt>観測元</dt><dd> </dd></dl><p class="hint">CQLはOPTIONS応答、7000はTCP接続を確認します。クエリの実行時間やRaft合意・データ複製の正常性を保証するものではありません。</p></details> <div class="panel flat"><button class="link-button history-refresh">履歴を更新</button><!><!></div></section> <section><h3>このノードに関わる接続</h3><!></section> <section><p class="hint">ノード自身が取得したメトリクス。カウンターは累積値、ゲージは観測時点の値です。</p><label class="search"><!><input aria-label="メトリクス検索" placeholder="名前で絞り込み"/></label><div class="metric-list"></div></section></div>`, 1);
var root_13 = $.from_html(`<p class="notice warning">双方向で結果が異なります。片方向の経路・フィルタを確認してください。</p>`);
var root_14 = $.from_html(`<p class="notice">この方向の情報は未取得です。接続失敗とは区別しています。</p>`);
var root_12 = $.from_html(`<div class="drawer-body"><p class="edge-title"> <!> </p><!><div class="checks-list"><div><span>CQLプロトコル</span><!></div><div><span>ノード間TCP</span><!></div><div><span>ScyllaScope API</span><!></div></div><dl class="definition"><dt>観測方向</dt><dd> </dd><dt>観測時刻</dt><dd> </dd><dt>逆方向の結果</dt><dd><!></dd></dl><!><!><details><summary>各チェックの詳細</summary><pre> </pre></details><p class="hint">ノード間の到達性の観測結果です。レプリケーションやRaftの合意結果ではありません。</p></div>`);
var root_15 = $.from_html(`<div class="empty"><span>このノードは現在の観測一覧にありません。</span></div>`);
var root = $.from_html(`<dialog class="drawer" aria-label="監視の詳細"><div class="drawer-inner"><header class="drawer-header"><div><span class="eyebrow"> </span><h2> </h2></div><button class="icon-button" aria-label="詳細を閉じる"><!></button></header> <!></div></dialog>`);

export default function Drawer($$anchor, $$props) {
	$.push($$props, false);

	const $selected = () => $.store_get(selected, '$selected', $$stores);
	const $snapshot = () => $.store_get(snapshot, '$snapshot', $$stores);
	const $clock = () => $.store_get(clock, '$clock', $$stores);
	const [$$stores, $$cleanup] = $.setup_stores();
	const node = $.mutable_source();
	const from = $.mutable_source();
	const to = $.mutable_source();
	const edge = $.mutable_source();
	const reverse = $.mutable_source();
	let dialog = $.mutable_source();
	let tab = $.mutable_source('overview');
	let metricOpen = $.mutable_source(false);
	let rawOpen = $.mutable_source(false);
	let metricFilter = $.mutable_source('');
	let rows = $.mutable_source([]);
	let nodeHistory = $.mutable_source([]);
	let historyError = $.mutable_source('');
	let lastNode = $.mutable_source('');
	let opened = $.mutable_source(false);
	let focusBefore;

	async function show() {
		$.set(opened, true);
		focusBefore = document.activeElement;
		await tick();

		if (!$.get(dialog).open) $.get(dialog).showModal();
	}

	function close() {
		selected.set(null);
		focusBefore?.focus?.();
	}

	async function loadHistory(address) {
		try {
			const h = await request('history?hours=6&node=' + encodeURIComponent(address));

			if ($.get(lastNode) === address) $.set(nodeHistory, h.map((n) => ({
				time: n.last_updated_at,
				average_latency_ms: n.cql?.ok ? n.cql.latency / 1e6 : null
			})));
		} catch(e) {
			if ($.get(lastNode) === address) $.set(historyError, e.message);
		}
	}

	async function doCopy(v) {
		try {
			await copy(v);
			notify('コピーしました');
		} catch(e) {
			notify(e.message, 'error');
		}
	}

	$.legacy_pre_effect(() => ($selected(), $snapshot()), () => {
		$.set(node, $selected()?.kind === 'node'
			? $snapshot()?.nodes.find((n) => n.id === $selected().id)
			: null);
	});

	$.legacy_pre_effect(() => ($snapshot(), $selected()), () => {
		$.set(from, $snapshot()?.nodes.find((n) => n.id === $selected()?.from));
	});

	$.legacy_pre_effect(() => ($snapshot(), $selected()), () => {
		$.set(to, $snapshot()?.nodes.find((n) => n.id === $selected()?.to));
	});

	$.legacy_pre_effect(() => ($snapshot(), $selected()), () => {
		$.set(edge, $snapshot()?.edges.find((e) => e.from === $selected()?.from && e.to === $selected()?.to));
	});

	$.legacy_pre_effect(() => ($snapshot(), $selected()), () => {
		$.set(reverse, $snapshot()?.edges.find((e) => e.from === $selected()?.to && e.to === $selected()?.from));
	});

	$.legacy_pre_effect(() => ($.get(dialog), $selected(), $.get(opened)), () => {
		if ($.get(dialog) && $selected() && !$.get(opened)) show(); else if ($.get(dialog) && !$selected() && $.get(opened)) {
			$.set(opened, false);
			$.get(dialog).close();
		}
	});

	$.legacy_pre_effect(() => ($.get(node), $.get(lastNode)), () => {
		if ($.get(node) && $.get(node).address !== $.get(lastNode)) {
			$.set(lastNode, $.get(node).address);
			$.set(tab, 'overview');
			$.set(metricFilter, '');
			$.set(metricOpen, false);
			$.set(rawOpen, false);
			$.set(nodeHistory, []);
			$.set(historyError, '');
			loadHistory($.get(node).address);
		}
	});

	$.legacy_pre_effect(() => ($.get(node), $.get(metricFilter)), () => {
		$.set(rows, Object.entries($.get(node)?.metric_values || {}).filter(([name]) => name.toLowerCase().includes($.get(metricFilter).toLowerCase())));
	});

	$.legacy_pre_effect_reset();
	$.init();

	var dialog_1 = root();

	dialog_1.__click = (e) => {
		if (e.target === $.get(dialog)) close();
	};

	var div = $.child(dialog_1);
	var header = $.child(div);
	var div_1 = $.child(header);
	var span = $.child(div_1);
	var text = $.child(span, true);

	$.reset(span);

	var h2 = $.sibling(span);
	var text_1 = $.child(h2, true);

	$.reset(h2);
	$.reset(div_1);

	var button = $.sibling(div_1);

	button.__click = close;

	var node_1 = $.child(button);

	Icon(node_1, { name: 'close' });
	$.reset(button);
	$.reset(header);

	var node_2 = $.sibling(header, 2);

	{
		var consequent_2 = ($$anchor) => {
			var fragment = root_1();
			var div_2 = $.first_child(fragment);
			var node_3 = $.child(div_2);

			{
				let $0 = $.derived_safe_equal(() => (
					$.deep_read_state(nodeState),
					$.get(node),
					$snapshot(),
					$clock(),
					$.untrack(() => nodeState($.get(node), $snapshot(), $clock()))
				));

				let $1 = $.derived_safe_equal(() => (
					$.deep_read_state(shortStatus),
					$.get(node),
					$.untrack(() => shortStatus($.get(node)))
				));

				Badge(node_3, {
					get state() {
						return $.get($0);
					},

					get text() {
						return $.get($1);
					}
				});
			}

			var span_1 = $.sibling(node_3);
			var text_2 = $.child(span_1, true);

			$.reset(span_1);

			var button_1 = $.sibling(span_1);

			button_1.__click = () => doCopy($.get(node).address);

			var node_4 = $.child(button_1);

			Icon(node_4, { name: 'copy', size: 14 });
			$.reset(button_1);
			$.reset(div_2);

			var nav = $.sibling(div_2, 2);

			$.each(
				nav,
				4,
				() => [
					['overview', '概要'],
					['connections', '接続'],
					['metrics', 'メトリクス']
				],
				$.index,
				($$anchor, $$item) => {
					var $$array = $.derived(() => $.to_array($$item, 2));
					let id = () => $.get($$array)[0];
					let label = () => $.get($$array)[1];
					var button_2 = root_2();

					button_2.__click = () => $.set(tab, id());

					let classes;
					var text_3 = $.child(button_2, true);

					$.reset(button_2);

					$.template_effect(() => {
						classes = $.set_class(button_2, 1, '', null, classes, { active: $.get(tab) === id() });
						$.set_text(text_3, label());
					});

					$.append($$anchor, button_2);
				}
			);

			$.reset(nav);

			var div_3 = $.sibling(nav, 2);
			var section = $.child(div_3);
			var node_5 = $.child(section);

			{
				var consequent = ($$anchor) => {
					var div_4 = root_3();

					$.each(div_4, 5, () => ($.get(node), $.untrack(() => $.get(node).reasons)), $.index, ($$anchor, reason) => {
						var p = root_4();
						var text_4 = $.child(p, true);

						$.reset(p);
						$.template_effect(() => $.set_text(text_4, $.get(reason)));
						$.append($$anchor, p);
					});

					$.reset(div_4);
					$.append($$anchor, div_4);
				};

				$.if(node_5, ($$render) => {
					if (($.get(node), $.untrack(() => $.get(node).reasons?.length))) $$render(consequent);
				});
			}

			var div_5 = $.sibling(node_5, 2);

			$.each(
				div_5,
				5,
				() => (
					$.get(node),
					$.untrack(() => [
						['CQLプロトコル', $.get(node).cql],
						['ノード間TCP', $.get(node).internode],
						['ScyllaScope', $.get(node).watch_api],
						['systemd', $.get(node).local_service],
						['ローカルREST', $.get(node).rest],
						['Scyllaメトリクス', $.get(node).metrics]
					])
				),
				$.index,
				($$anchor, $$item) => {
					var $$array_1 = $.derived(() => $.to_array($.get($$item), 2));
					let name = () => $.get($$array_1)[0];
					let c = () => $.get($$array_1)[1];
					var div_6 = root_5();
					var span_2 = $.child(div_6);
					var text_5 = $.child(span_2, true);

					$.reset(span_2);

					var node_6 = $.sibling(span_2);

					Check(node_6, {
						get value() {
							return c();
						},
						timings: true,
						get ttl() {
							return (
								$snapshot(),
								$.untrack(() => $snapshot().stale_after_seconds)
							);
						}
					});

					$.reset(div_6);
					$.template_effect(() => $.set_text(text_5, name()));
					$.append($$anchor, div_6);
				}
			);

			$.reset(div_5);

			var dl = $.sibling(div_5, 2);
			var dd = $.sibling($.child(dl));
			var text_6 = $.child(dd);

			$.reset(dd);

			var dd_1 = $.sibling(dd, 2);
			var text_7 = $.child(dd_1, true);

			$.reset(dd_1);

			var dd_2 = $.sibling(dd_1, 2);
			var text_8 = $.child(dd_2, true);

			$.reset(dd_2);

			var dd_3 = $.sibling(dd_2, 2);
			var text_9 = $.child(dd_3, true);

			$.reset(dd_3);

			var dd_4 = $.sibling(dd_3, 2);
			var text_10 = $.child(dd_4, true);

			$.reset(dd_4);

			var dd_5 = $.sibling(dd_4, 2);
			var text_11 = $.child(dd_5, true);

			$.reset(dd_5);

			var dd_6 = $.sibling(dd_5, 2);
			var text_12 = $.child(dd_6, true);

			$.reset(dd_6);
			$.reset(dl);

			var details = $.sibling(dl, 2);
			var dl_1 = $.sibling($.child(details));
			var dd_7 = $.sibling($.child(dl_1));
			var text_13 = $.child(dd_7, true);

			$.reset(dd_7);

			var dd_8 = $.sibling(dd_7, 2);
			var text_14 = $.child(dd_8, true);

			$.reset(dd_8);
			$.reset(dl_1);
			$.next();
			$.reset(details);

			var div_7 = $.sibling(details, 2);
			var button_3 = $.child(div_7);

			button_3.__click = () => loadHistory($.get(node).address);

			var node_7 = $.sibling(button_3);

			Chart(node_7, {
				get samples() {
					return $.get(nodeHistory);
				},
				title: 'この観測元からのCQL応答（6時間）'
			});

			var node_8 = $.sibling(node_7);

			{
				var consequent_1 = ($$anchor) => {
					var p_1 = root_6();
					var text_15 = $.child(p_1, true);

					$.reset(p_1);
					$.template_effect(() => $.set_text(text_15, $.get(historyError)));
					$.append($$anchor, p_1);
				};

				$.if(node_8, ($$render) => {
					if ($.get(historyError)) $$render(consequent_1);
				});
			}

			$.reset(div_7);
			$.reset(section);

			var section_1 = $.sibling(section, 2);
			var node_9 = $.sibling($.child(section_1));

			$.each(
				node_9,
				1,
				() => (
					$snapshot(),
					$.get(node),
					$.untrack(() => ($snapshot().edges || []).filter((e) => e.from === $.get(node).id || e.to === $.get(node).id))
				),
				(e) => e.from + '>' + e.to,
				($$anchor, e) => {
					var button_4 = root_7();

					button_4.__click = () => selected.set({ kind: 'edge', from: $.get(e).from, to: $.get(e).to });

					var span_3 = $.child(button_4);
					var text_16 = $.child(span_3);

					$.reset(span_3);

					var node_10 = $.sibling(span_3);

					{
						let $0 = $.derived_safe_equal(() => (
							$.deep_read_state(edgeState),
							$.get(e),
							$snapshot(),
							$clock(),
							$.untrack(() => edgeState($.get(e), $snapshot(), $clock()))
						));

						Badge(node_10, {
							get state() {
								return $.get($0);
							}
						});
					}

					$.reset(button_4);

					$.template_effect(($0, $1) => $.set_text(text_16, `${$0 ?? ''} → ${$1 ?? ''}`), [
						() => (
							$snapshot(),
							$.get(e),
							$.untrack(() => $snapshot().nodes.find((n) => n.id === $.get(e).from)?.name || $.get(e).from)
						),

						() => (
							$snapshot(),
							$.get(e),
							$.untrack(() => $snapshot().nodes.find((n) => n.id === $.get(e).to)?.name || $.get(e).to)
						)
					]);

					$.append($$anchor, button_4);
				},
				($$anchor) => {
					var p_2 = root_8();

					$.append($$anchor, p_2);
				}
			);

			$.reset(section_1);

			var section_2 = $.sibling(section_1, 2);
			var label_1 = $.sibling($.child(section_2));
			var node_11 = $.child(label_1);

			Icon(node_11, { name: 'search', size: 16 });

			var input = $.sibling(node_11);

			$.remove_input_defaults(input);
			$.reset(label_1);

			var div_8 = $.sibling(label_1);

			$.each(
				div_8,
				5,
				() => $.get(rows),
				([name, value]) => name,
				($$anchor, $$item) => {
					var $$array_2 = $.derived(() => $.to_array($.get($$item), 2));
					let name = () => $.get($$array_2)[0];
					let value = () => $.get($$array_2)[1];
					var div_9 = root_9();
					var code = $.child(div_9);
					var text_17 = $.child(code, true);

					$.reset(code);

					var strong = $.sibling(code);
					var text_18 = $.child(strong, true);

					$.reset(strong);
					$.reset(div_9);

					$.template_effect(
						($0) => {
							$.set_text(text_17, name());
							$.set_text(text_18, $0);
						},
						[
							() => (
								value(),
								$.untrack(() => Number(value()).toLocaleString('ja-JP', { maximumFractionDigits: 4 }))
							)
						]
					);

					$.append($$anchor, div_9);
				},
				($$anchor) => {
					var p_3 = root_10();

					$.append($$anchor, p_3);
				}
			);

			$.reset(div_8);
			$.reset(section_2);
			$.reset(div_3);

			$.template_effect(
				($0, $1) => {
					$.set_text(text_2, ($.get(node), $.untrack(() => $.get(node).address)));
					$.set_attribute(section, 'hidden', $.get(tab) !== 'overview');
					$.set_text(text_6, `${($.get(node), $.untrack(() => $.get(node).dc || '—')) ?? ''} / ${($.get(node), $.untrack(() => $.get(node).rack || '—')) ?? ''}`);
					$.set_text(text_7, ($.get(node), $.untrack(() => $.get(node).version || '未取得')));
					$.set_text(text_8, ($.get(node), $.untrack(() => $.get(node).shards || '未取得')));

					$.set_text(text_9, (
						$.get(node),
						$.untrack(() => $.get(node).labels?.memory || '未指定 / 未取得')
					));

					$.set_text(text_10, $0);

					$.set_text(text_11, (
						$.get(node),
						$.untrack(() => $.get(node).agent_version || '未取得')
					));

					$.set_text(text_12, $1);
					$.set_text(text_13, ($.get(node), $.untrack(() => $.get(node).host_id || '未取得')));

					$.set_text(text_14, (
						$snapshot(),
						$.untrack(() => $snapshot().local_node_name || $snapshot().local_node)
					));

					$.set_attribute(section_1, 'hidden', $.get(tab) !== 'connections');
					$.set_attribute(section_2, 'hidden', $.get(tab) !== 'metrics');
				},
				[
					() => (
						$.get(node),
						$.deep_read_state(bytes),
						$.untrack(() => $.get(node).load_known ? bytes($.get(node).load_bytes) : '未取得')
					),

					() => (
						$.deep_read_state(date),
						$.get(node),
						$.untrack(() => date($.get(node).last_updated_at))
					)
				]
			);

			$.bind_property('open', 'toggle', details, ($$value) => $.set(rawOpen, $$value), () => $.get(rawOpen));
			$.bind_value(input, () => $.get(metricFilter), ($$value) => $.set(metricFilter, $$value));
			$.append($$anchor, fragment);
		};

		var alternate_1 = ($$anchor) => {
			var fragment_1 = $.comment();
			var node_12 = $.first_child(fragment_1);

			{
				var consequent_5 = ($$anchor) => {
					var div_10 = root_12();
					var p_4 = $.child(div_10);
					var text_19 = $.child(p_4, true);
					var node_13 = $.sibling(text_19);

					Icon(node_13, { name: 'arrow' });

					var text_20 = $.sibling(node_13, 1, true);

					$.reset(p_4);

					var node_14 = $.sibling(p_4);

					{
						let $0 = $.derived_safe_equal(() => (
							$.deep_read_state(edgeState),
							$.get(edge),
							$snapshot(),
							$clock(),
							$.untrack(() => edgeState($.get(edge), $snapshot(), $clock()))
						));

						Badge(node_14, {
							get state() {
								return $.get($0);
							}
						});
					}

					var div_11 = $.sibling(node_14);
					var div_12 = $.child(div_11);
					var node_15 = $.sibling($.child(div_12));

					{
						let $0 = $.derived_safe_equal(() => ($.get(edge), $.untrack(() => $.get(edge)?.cql)));

						let $1 = $.derived_safe_equal(() => (
							$snapshot(),
							$.untrack(() => $snapshot()?.stale_after_seconds)
						));

						Check(node_15, {
							get value() {
								return $.get($0);
							},
							timings: true,
							get ttl() {
								return $.get($1);
							}
						});
					}

					$.reset(div_12);

					var div_13 = $.sibling(div_12);
					var node_16 = $.sibling($.child(div_13));

					{
						let $0 = $.derived_safe_equal(() => ($.get(edge), $.untrack(() => $.get(edge)?.internode)));

						let $1 = $.derived_safe_equal(() => (
							$snapshot(),
							$.untrack(() => $snapshot()?.stale_after_seconds)
						));

						Check(node_16, {
							get value() {
								return $.get($0);
							},
							timings: true,
							get ttl() {
								return $.get($1);
							}
						});
					}

					$.reset(div_13);

					var div_14 = $.sibling(div_13);
					var node_17 = $.sibling($.child(div_14));

					{
						let $0 = $.derived_safe_equal(() => ($.get(edge), $.untrack(() => $.get(edge)?.watch_api)));

						let $1 = $.derived_safe_equal(() => (
							$snapshot(),
							$.untrack(() => $snapshot()?.stale_after_seconds)
						));

						Check(node_17, {
							get value() {
								return $.get($0);
							},
							timings: true,
							get ttl() {
								return $.get($1);
							}
						});
					}

					$.reset(div_14);
					$.reset(div_11);

					var dl_2 = $.sibling(div_11);
					var dd_9 = $.sibling($.child(dl_2));
					var text_21 = $.child(dd_9);

					$.reset(dd_9);

					var dd_10 = $.sibling(dd_9, 2);
					var text_22 = $.child(dd_10, true);

					$.reset(dd_10);

					var dd_11 = $.sibling(dd_10, 2);
					var node_18 = $.child(dd_11);

					{
						let $0 = $.derived_safe_equal(() => (
							$.deep_read_state(edgeState),
							$.get(reverse),
							$snapshot(),
							$clock(),
							$.untrack(() => edgeState($.get(reverse), $snapshot(), $clock()))
						));

						Badge(node_18, {
							get state() {
								return $.get($0);
							}
						});
					}

					$.reset(dd_11);
					$.reset(dl_2);

					var node_19 = $.sibling(dl_2);

					{
						var consequent_3 = ($$anchor) => {
							var p_5 = root_13();

							$.append($$anchor, p_5);
						};

						$.if(node_19, ($$render) => {
							if (($.get(edge), $.untrack(() => $.get(edge)?.asymmetric))) $$render(consequent_3);
						});
					}

					var node_20 = $.sibling(node_19);

					{
						var consequent_4 = ($$anchor) => {
							var p_6 = root_14();

							$.append($$anchor, p_6);
						};

						$.if(node_20, ($$render) => {
							if (!$.get(edge)) $$render(consequent_4);
						});
					}

					var details_1 = $.sibling(node_20);
					var pre = $.sibling($.child(details_1));
					var text_23 = $.child(pre, true);

					$.reset(pre);
					$.reset(details_1);
					$.next();
					$.reset(div_10);

					$.template_effect(
						($0, $1) => {
							$.set_text(text_19, (
								$.get(from),
								$selected(),
								$.untrack(() => $.get(from)?.name || $selected().from)
							));

							$.set_text(text_20, (
								$.get(to),
								$selected(),
								$.untrack(() => $.get(to)?.name || $selected().to)
							));

							$.set_text(text_21, `${($.get(from), $.untrack(() => $.get(from)?.name)) ?? ''} → ${($.get(to), $.untrack(() => $.get(to)?.name)) ?? ''}`);
							$.set_text(text_22, $0);
							$.set_text(text_23, $1);
						},
						[
							() => (
								$.deep_read_state(date),
								$.get(edge),
								$.untrack(() => date($.get(edge)?.observed_at))
							),

							() => (
								$.get(edge),
								$.untrack(() => JSON.stringify(
									{
										cql: $.get(edge)?.cql,
										internode: $.get(edge)?.internode,
										watch_api: $.get(edge)?.watch_api
									},
									null,
									2
								))
							)
						]
					);

					$.append($$anchor, div_10);
				};

				var alternate = ($$anchor) => {
					var div_15 = root_15();

					$.append($$anchor, div_15);
				};

				$.if(
					node_12,
					($$render) => {
						if (($selected(), $.untrack(() => $selected()?.kind === 'edge'))) $$render(consequent_5); else $$render(alternate, false);
					},
					true
				);
			}

			$.append($$anchor, fragment_1);
		};

		$.if(node_2, ($$render) => {
			if ($.get(node)) $$render(consequent_2); else $$render(alternate_1, false);
		});
	}

	$.reset(div);
	$.reset(dialog_1);
	$.bind_this(dialog_1, ($$value) => $.set(dialog, $$value), () => $.get(dialog));

	$.template_effect(() => {
		$.set_text(text, (
			$selected(),
			$.untrack(() => $selected()?.kind === 'edge' ? 'DIRECTIONAL CONNECTION' : 'NODE DETAILS')
		));

		$.set_text(text_1, (
			$.get(node),
			$selected(),
			$.untrack(() => $.get(node)?.name || ($selected()?.kind === 'edge' ? '接続の詳細' : 'ノードを確認'))
		));
	});

	$.event('close', dialog_1, close);
	$.append($$anchor, dialog_1);
	$.pop();
	$$cleanup();
}

$.delegate(['click']);