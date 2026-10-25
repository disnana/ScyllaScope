import '../vendor/svelte_internal_disclose-version.js';
import '../vendor/svelte_internal_flags_legacy.js';
import * as $ from '../vendor/svelte_internal_client.js';
import { onMount } from '../vendor/svelte_svelte.js';

import {
	snapshot,
	clock,
	events,
	history,
	errors,
	loadResource,
	navigate
} from '../state.js';

import { fresh, bytes, formatMS, date, time, clusterState } from '../lib.js';
import Icon from '../components/Icon.js';
import Badge from '../components/Badge.js';
import NodeTable from '../components/NodeTable.js';
import Topology from '../components/Topology.js';
import Chart from '../components/Chart.js';

var root_3 = $.from_html(`<p> </p>`);
var root_4 = $.from_html(`<p> </p>`);
var root_5 = $.from_html(`<p>DBメンバーと観測エージェントは別です。各ノードにScopeを配置し、同じcluster_tokenを設定すると方向別の結果を共有します。</p>`);
var root_2 = $.from_html(`<details class="coverage-bar"><summary><!><span> </span><span class="quiet">詳細</span></summary><div><!><!><!></div></details>`);
var root_7 = $.from_html(`<button><!><p> </p></button>`);
var root_6 = $.from_html(`<div class="alert-mini"></div>`);
var root_8 = $.from_html(`<div class="clear-state"><span class="clear-icon"><!></span><div><strong>確定したアラートなし</strong><p>未観測の状態は正常に含めません</p></div></div>`);
var root_9 = $.from_html(`<div class="timeline-row"><time> </time><span></span><p> </p></div>`);
var root_10 = $.from_html(`<p class="quiet">イベントはまだありません</p>`);
var root_1 = $.from_html(`<div class="kpis"><div class="kpi"><span><!> Scyllaメンバー</span><strong> <small> </small></strong><div> </div></div><div class="kpi"><span><!> 観測エージェント</span><strong> <small> </small></strong><div> </div></div><button class="kpi clickable"><span><!> アクティブアラート</span><strong> </strong><div>この観測元で確定した異常</div></button><div class="kpi"><span><!> CQLプロトコル応答</span><strong class="metric-size"> </strong><div>この観測元からの平均 · OPTIONS</div></div></div> <!> <div class="dashboard-grid"><section class="panel topology-panel"><header class="panel-header"><h2>クラスタ接続図 <span class="muted-badge"> </span></h2><button class="link-button">詳細を開く <!></button></header><!></section><aside class="dashboard-side"><section class="panel"><header class="panel-header"><h2>アラート</h2><button class="link-button">すべて <!></button></header><!></section><section class="panel"><header class="panel-header"><h2>最近のイベント</h2><button class="link-button">履歴 <!></button></header><div class="timeline"></div></section></aside></div> <section class="panel"><header class="panel-header"><h2>ノードの状態</h2><button class="link-button">ノード一覧 <!></button></header><!></section> <div class="summary-footer"><span><span class="dot live"></span> </span><span> </span><button class="link-button">履歴グラフを開く <!></button></div>`, 1);
var root_11 = $.from_html(`<div class="empty"><!><h2>最初の観測を待っています</h2><p>接続できない場合でも、状態を正常として表示しません。</p></div>`);

export default function Dashboard($$anchor, $$props) {
	$.push($$props, false);

	const $snapshot = () => $.store_get(snapshot, '$snapshot', $$stores);
	const $clock = () => $.store_get(clock, '$clock', $$stores);
	const $events = () => $.store_get(events, '$events', $$stores);
	const [$$stores, $$cleanup] = $.setup_stores();
	const s = $.mutable_source();
	const cov = $.mutable_source();
	const active = $.mutable_source();
	const lat = $.mutable_source();
	const avg = $.mutable_source();
	const total = $.mutable_source();
	let coverageOpen = $.mutable_source(false);

	$.legacy_pre_effect(() => ($snapshot()), () => {
		$.set(s, $snapshot());
	});

	$.legacy_pre_effect(() => ($.get(s)), () => {
		$.set(cov, $.get(s)?.coverage || {});
	});

	$.legacy_pre_effect(() => ($.get(s)), () => {
		$.set(active, ($.get(s)?.incidents || []).filter((i) => !i.resolved_at));
	});

	$.legacy_pre_effect(() => ($.get(s), fresh, $clock()), () => {
		$.set(lat, ($.get(s)?.nodes || []).filter((n) => fresh(n.cql, $clock(), $.get(s).stale_after_seconds) && n.cql.ok).map((n) => n.cql.latency / 1e6));
	});

	$.legacy_pre_effect(() => ($.get(lat)), () => {
		$.set(avg, $.get(lat).length
			? $.get(lat).reduce((a, b) => a + b, 0) / $.get(lat).length
			: null);
	});

	$.legacy_pre_effect(() => ($.get(s)), () => {
		$.set(total, $.get(s)?.nodes?.length && $.get(s).nodes.every((n) => n.load_known && n.metadata_known)
			? $.get(s).nodes.reduce((a, n) => a + n.load_bytes, 0)
			: null);
	});

	$.legacy_pre_effect_reset();
	$.init();

	var fragment = $.comment();
	var node = $.first_child(fragment);

	{
		var consequent_4 = ($$anchor) => {
			var fragment_1 = root_1();
			var div = $.first_child(fragment_1);
			var div_1 = $.child(div);
			var span = $.child(div_1);
			var node_1 = $.child(span);

			Icon(node_1, { name: 'nodes', size: 16 });
			$.next();
			$.reset(span);

			var strong = $.sibling(span);
			var text = $.child(strong, true);
			var small = $.sibling(text);
			var text_1 = $.child(small);

			$.reset(small);
			$.reset(strong);

			var div_2 = $.sibling(strong);
			var text_2 = $.child(div_2);

			$.reset(div_2);
			$.reset(div_1);

			var div_3 = $.sibling(div_1);
			var span_1 = $.child(div_3);
			var node_2 = $.child(span_1);

			Icon(node_2, { name: 'topology', size: 16 });
			$.next();
			$.reset(span_1);

			var strong_1 = $.sibling(span_1);
			var text_3 = $.child(strong_1, true);
			var small_1 = $.sibling(text_3);
			var text_4 = $.child(small_1);

			$.reset(small_1);
			$.reset(strong_1);

			var div_4 = $.sibling(strong_1);
			var text_5 = $.child(div_4);

			$.reset(div_4);
			$.reset(div_3);

			var button = $.sibling(div_3);

			button.__click = () => navigate('alerts');

			var span_2 = $.child(button);
			var node_3 = $.child(span_2);

			Icon(node_3, { name: 'alerts', size: 16 });
			$.next();
			$.reset(span_2);

			var strong_2 = $.sibling(span_2);
			let classes;
			var text_6 = $.child(strong_2, true);

			$.reset(strong_2);
			$.next();
			$.reset(button);

			var div_5 = $.sibling(button);
			var span_3 = $.child(div_5);
			var node_4 = $.child(span_3);

			Icon(node_4, { name: 'bolt', size: 16 });
			$.next();
			$.reset(span_3);

			var strong_3 = $.sibling(span_3);
			var text_7 = $.child(strong_3, true);

			$.reset(strong_3);
			$.next();
			$.reset(div_5);
			$.reset(div);

			var node_5 = $.sibling(div, 2);

			{
				var consequent_2 = ($$anchor) => {
					var details = root_2();
					var summary = $.child(details);
					var node_6 = $.child(summary);

					Icon(node_6, { name: 'info', size: 14 });

					var span_4 = $.sibling(node_6);
					var text_8 = $.child(span_4, true);

					$.reset(span_4);
					$.next();
					$.reset(summary);

					var div_6 = $.sibling(summary);
					var node_7 = $.child(div_6);

					$.each(node_7, 1, () => ($.get(s), $.untrack(() => $.get(s).warnings)), $.index, ($$anchor, w) => {
						var p = root_3();
						var text_9 = $.child(p, true);

						$.reset(p);
						$.template_effect(() => $.set_text(text_9, $.get(w)));
						$.append($$anchor, p);
					});

					var node_8 = $.sibling(node_7);

					{
						var consequent = ($$anchor) => {
							var p_1 = root_4();
							var text_10 = $.child(p_1, true);

							$.reset(p_1);
							$.template_effect(() => $.set_text(text_10, ($.get(s), $.untrack(() => $.get(s).discovery.message))));
							$.append($$anchor, p_1);
						};

						$.if(node_8, ($$render) => {
							if (($.get(s), $.untrack(() => !$.get(s).discovery.ok))) $$render(consequent);
						});
					}

					var node_9 = $.sibling(node_8);

					{
						var consequent_1 = ($$anchor) => {
							var p_2 = root_5();

							$.append($$anchor, p_2);
						};

						$.if(node_9, ($$render) => {
							if ((
								$.get(cov),
								$.get(s),
								$.untrack(() => $.get(cov).agents_ready < $.get(s).nodes.length)
							)) $$render(consequent_1);
						});
					}

					$.reset(div_6);
					$.reset(details);

					$.template_effect(() => $.set_text(text_8, (
						$.get(s),
						$.get(cov),
						$.untrack(() => !$.get(s).discovery.ok
							? 'クラスタ一覧を取得できていません'
							: $.get(cov).agents_ready < $.get(s).nodes.length
								? `分散観測 ${$.get(cov).agents_ready}/${$.get(s).nodes.length} 台 · 未観測の方向があります`
								: `確認事項 ${$.get(s).warnings.length}件`)
					)));

					$.bind_property('open', 'toggle', details, ($$value) => $.set(coverageOpen, $$value), () => $.get(coverageOpen));
					$.append($$anchor, details);
				};

				$.if(node_5, ($$render) => {
					if ((
						$.get(s),
						$.get(cov),
						$.untrack(() => $.get(s).warnings.length || !$.get(s).discovery.ok || $.get(cov).agents_ready < $.get(s).nodes.length)
					)) $$render(consequent_2);
				});
			}

			var div_7 = $.sibling(node_5, 2);
			var section = $.child(div_7);
			var header = $.child(section);
			var h2 = $.child(header);
			var span_5 = $.sibling($.child(h2));
			var text_11 = $.child(span_5);

			$.reset(span_5);
			$.reset(h2);

			var button_1 = $.sibling(h2);

			button_1.__click = () => navigate('topology');

			var node_10 = $.sibling($.child(button_1));

			Icon(node_10, { name: 'external', size: 13 });
			$.reset(button_1);
			$.reset(header);

			var node_11 = $.sibling(header);

			Topology(node_11, { compact: true });
			$.reset(section);

			var aside = $.sibling(section);
			var section_1 = $.child(aside);
			var header_1 = $.child(section_1);
			var button_2 = $.sibling($.child(header_1));

			button_2.__click = () => navigate('alerts');

			var node_12 = $.sibling($.child(button_2));

			Icon(node_12, { name: 'chevron', size: 13 });
			$.reset(button_2);
			$.reset(header_1);

			var node_13 = $.sibling(header_1);

			{
				var consequent_3 = ($$anchor) => {
					var div_8 = root_6();

					$.each(div_8, 5, () => ($.get(active), $.untrack(() => $.get(active).slice(0, 2))), (i) => i.id, ($$anchor, i) => {
						var button_3 = root_7();

						button_3.__click = () => navigate('alerts');

						var node_14 = $.child(button_3);

						Badge(node_14, {
							get state() {
								return ($.get(i), $.untrack(() => $.get(i).severity));
							}
						});

						var p_3 = $.sibling(node_14);
						var text_12 = $.child(p_3, true);

						$.reset(p_3);
						$.reset(button_3);
						$.template_effect(() => $.set_text(text_12, ($.get(i), $.untrack(() => $.get(i).message))));
						$.append($$anchor, button_3);
					});

					$.reset(div_8);
					$.append($$anchor, div_8);
				};

				var alternate = ($$anchor) => {
					var div_9 = root_8();
					var span_6 = $.child(div_9);
					var node_15 = $.child(span_6);

					Icon(node_15, { name: 'check', size: 20 });
					$.reset(span_6);
					$.next();
					$.reset(div_9);
					$.append($$anchor, div_9);
				};

				$.if(node_13, ($$render) => {
					if (($.get(active), $.untrack(() => $.get(active).length))) $$render(consequent_3); else $$render(alternate, false);
				});
			}

			$.reset(section_1);

			var section_2 = $.sibling(section_1);
			var header_2 = $.child(section_2);
			var button_4 = $.sibling($.child(header_2));

			button_4.__click = () => navigate('events');

			var node_16 = $.sibling($.child(button_4));

			Icon(node_16, { name: 'chevron', size: 13 });
			$.reset(button_4);
			$.reset(header_2);

			var div_10 = $.sibling(header_2);

			$.each(
				div_10,
				5,
				() => ($events(), $.untrack(() => $events().slice(0, 4))),
				(ev) => ev.id,
				($$anchor, ev) => {
					var div_11 = root_9();
					var time_1 = $.child(div_11);
					var text_13 = $.child(time_1, true);

					$.reset(time_1);

					var span_7 = $.sibling(time_1);
					var p_4 = $.sibling(span_7);
					var text_14 = $.child(p_4, true);

					$.reset(p_4);
					$.reset(div_11);

					$.template_effect(
						($0, $1) => {
							$.set_attribute(time_1, 'title', $0);
							$.set_text(text_13, $1);

							$.set_class(span_7, 1, `dot ${(
								$.get(ev),
								$.untrack(() => $.get(ev).severity === 'critical'
									? 'critical'
									: $.get(ev).severity === 'warning' ? 'warning' : 'healthy')
							) ?? ''}`);

							$.set_text(text_14, ($.get(ev), $.untrack(() => $.get(ev).message)));
						},
						[
							() => (
								$.deep_read_state(date),
								$.get(ev),
								$.untrack(() => date($.get(ev).time))
							),

							() => (
								$.deep_read_state(time),
								$.get(ev),
								$.untrack(() => time($.get(ev).time))
							)
						]
					);

					$.append($$anchor, div_11);
				},
				($$anchor) => {
					var p_5 = root_10();

					$.append($$anchor, p_5);
				}
			);

			$.reset(div_10);
			$.reset(section_2);
			$.reset(aside);
			$.reset(div_7);

			var section_3 = $.sibling(div_7, 2);
			var header_3 = $.child(section_3);
			var button_5 = $.sibling($.child(header_3));

			button_5.__click = () => navigate('nodes');

			var node_17 = $.sibling($.child(button_5));

			Icon(node_17, { name: 'chevron', size: 13 });
			$.reset(button_5);
			$.reset(header_3);

			var node_18 = $.sibling(header_3);

			NodeTable(node_18, {
				get nodes() {
					return ($.get(s), $.untrack(() => $.get(s).nodes));
				}
			});

			$.reset(section_3);

			var div_12 = $.sibling(section_3, 2);
			var span_8 = $.child(div_12);
			var text_15 = $.sibling($.child(span_8));

			$.reset(span_8);

			var span_9 = $.sibling(span_8);
			var text_16 = $.child(span_9);

			$.reset(span_9);

			var button_6 = $.sibling(span_9);

			button_6.__click = () => navigate('metrics');

			var node_19 = $.sibling($.child(button_6));

			Icon(node_19, { name: 'arrow', size: 13 });
			$.reset(button_6);
			$.reset(div_12);

			$.template_effect(
				($0, $1, $2) => {
					$.set_text(text, (
						$.get(cov),
						$.untrack(() => $.get(cov).discovered_nodes ?? '—')
					));

					$.set_text(text_1, `/ ${(
						$.get(cov),
						$.untrack(() => $.get(cov).expected_nodes ?? '—')
					) ?? ''}`);

					$.set_text(text_2, `到達性 ${(
						$.get(cov),
						$.untrack(() => $.get(cov).reachable_nodes ?? '—')
					) ?? ''} 台 · ${$0 ?? ''}`);

					$.set_text(text_3, ($.get(cov), $.untrack(() => $.get(cov).agents_ready ?? '—')));
					$.set_text(text_4, `/ ${($.get(s), $.untrack(() => $.get(s).nodes.length)) ?? ''}`);

					$.set_text(text_5, `${(
						$.get(cov),
						$.untrack(() => $.get(cov).observed_directions ?? 0)
					) ?? ''} / ${(
						$.get(cov),
						$.untrack(() => $.get(cov).expected_directions ?? 0)
					) ?? ''} 方向の観測`);

					classes = $.set_class(strong_2, 1, '', null, classes, { critical: $.get(active).length > 0 });
					$.set_text(text_6, ($.get(active), $.untrack(() => $.get(active).length)));
					$.set_text(text_7, $1);
					$.set_text(text_11, `${($.get(s), $.untrack(() => $.get(s).nodes.length)) ?? ''} nodes`);

					$.set_text(text_15, ` ${(
						$.get(s),
						$.untrack(() => $.get(s).local_node_name || $.get(s).local_node)
					) ?? ''} からの観測`);

					$.set_text(text_16, `SQLite履歴 · ${$2 ?? ''}`);
				},
				[
					() => (
						$.deep_read_state(bytes),
						$.get(total),
						$.untrack(() => bytes($.get(total)))
					),

					() => (
						$.deep_read_state(formatMS),
						$.get(avg),
						$.untrack(() => formatMS($.get(avg)))
					),

					() => (
						$.deep_read_state(date),
						$.get(s),
						$.untrack(() => date($.get(s).generated_at))
					)
				]
			);

			$.append($$anchor, fragment_1);
		};

		var alternate_1 = ($$anchor) => {
			var div_13 = root_11();
			var node_20 = $.child(div_13);

			Icon(node_20, { name: 'nodes', size: 32 });
			$.next(2);
			$.reset(div_13);
			$.append($$anchor, div_13);
		};

		$.if(node, ($$render) => {
			if ($.get(s)) $$render(consequent_4); else $$render(alternate_1, false);
		});
	}

	$.append($$anchor, fragment);
	$.pop();
	$$cleanup();
}

$.delegate(['click']);