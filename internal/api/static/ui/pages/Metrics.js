import '../vendor/svelte_internal_disclose-version.js';
import '../vendor/svelte_internal_flags_legacy.js';
import * as $ from '../vendor/svelte_internal_client.js';
import { onMount } from '../vendor/svelte_svelte.js';

import {
	snapshot,
	history,
	errors,
	loadResource,
	request,
	notify,
	route
} from '../state.js';

import { sessionGet, sessionSet, finite, download, csv, bytes } from '../lib.js';
import Chart from '../components/Chart.js';
import Icon from '../components/Icon.js';

var root_2 = $.from_html(`<option> </option>`);
var root_1 = $.from_html(`<select aria-label="メトリクス対象ノード"></select>`);
var root_3 = $.from_html(`<p class="notice warning"> </p>`);
var root_4 = $.from_html(`<div><h3> </h3><dl class="definition"><dt>実行中のCompaction</dt><dd> </dd><dt>保留中のCompaction</dt><dd> </dd><dt>Read timeouts（累積）</dt><dd> </dd><dt>Write timeouts（累積）</dt><dd> </dd></dl></div>`);
var root = $.from_html(`<section class="panel"><div class="toolbar"><div class="tabs"><button>クラスタ集計</button><button>ノード</button></div><!><select aria-label="履歴の時間範囲"><option>1時間</option><option>6時間</option><option>24時間</option><option>7日間</option></select><button><!> </button><button><!>CSV</button></div></section> <!><div class="chart-grid-layout"><section class="panel chart-panel"><!></section><section class="panel chart-panel"><!></section><section class="panel chart-panel"><!></section><section class="panel chart-panel"><!></section></div> <p class="hint"> </p> <section class="panel"><header class="panel-header"><h2>Scylla内部の最新メトリクス</h2><span class="quiet">各ノードを選択して詳細</span></header><div class="metric-summary"></div></section>`, 1);

export default function Metrics($$anchor, $$props) {
	$.push($$props, false);

	const $history = () => $.store_get(history, '$history', $$stores);
	const $route = () => $.store_get(route, '$route', $$stores);
	const $snapshot = () => $.store_get(snapshot, '$snapshot', $$stores);
	const $errors = () => $.store_get(errors, '$errors', $$stores);
	const [$$stores, $$cleanup] = $.setup_stores();
	const samples = $.mutable_source();
	let hours = $.mutable_source(sessionGet('metrics.hours', 6));
	let kind = $.mutable_source('cluster');
	let node = $.mutable_source('');
	let nodeSamples = $.mutable_source([]);
	let loading = $.mutable_source(false);
	let requestID = 0;

	async function load(force = false) {
		const id = ++requestID;

		$.set(loading, true);

		try {
			if ($.get(kind) === 'cluster') await loadResource('history', force, Number($.get(hours))); else {
				const r = await request(`history?hours=${$.get(hours)}&node=${encodeURIComponent($.get(node))}`);

				if (id !== requestID) return;

				$.set(nodeSamples, r.map((n) => ({
					time: n.last_updated_at,
					average_latency_ms: n.cql?.ok ? n.cql.latency / 1e6 : null,
					total_load_bytes: n.load_known ? n.load_bytes : null,
					nodes_healthy: n.state === 'healthy' ? 1 : 0,
					cql_healthy: n.cql?.ok ? 1 : 0,
					internode_healthy: n.internode?.ok ? 1 : 0
				})));
			}
		} catch(e) {
			notify(e.message, 'error');
		} finally {
			if (id === requestID) $.set(loading, false);
		}
	}

	onMount(() => {
		load();

		const id = setInterval(
			() => {
				if ($route() === 'metrics' && !$.get(loading)) load(true);
			},
			60000
		);

		return () => clearInterval(id);
	});

	function change() {
		if ($.get(kind) !== 'cluster' && !$.get(node)) $.set(node, $snapshot()?.nodes[0]?.address || '');

		load(false);
	}

	function exportCSV() {
		download(
			'scyllascope-metrics.csv',
			csv($.get(samples), [
				'time',
				'average_latency_ms',
				'total_load_bytes',
				'nodes_healthy',
				'cql_healthy',
				'internode_healthy'
			]),
			'text/csv;charset=utf-8'
		);
	}

	$.legacy_pre_effect(() => (sessionSet, $.get(hours)), () => {
		sessionSet('metrics.hours', Number($.get(hours)));
	});

	$.legacy_pre_effect(() => ($.get(kind), $history(), $.get(nodeSamples)), () => {
		$.set(samples, $.get(kind) === 'cluster' ? $history() : $.get(nodeSamples));
	});

	$.legacy_pre_effect_reset();
	$.init();

	var fragment = root();
	var section = $.first_child(fragment);
	var div = $.child(section);
	var div_1 = $.child(div);
	var button = $.child(div_1);

	button.__click = () => {
		$.set(kind, 'cluster');
		change();
	};

	let classes;
	var button_1 = $.sibling(button);

	button_1.__click = () => {
		$.set(kind, 'node');
		change();
	};

	let classes_1;

	$.reset(div_1);

	var node_1 = $.sibling(div_1);

	{
		var consequent = ($$anchor) => {
			var select = root_1();

			$.template_effect(() => {
				$.get(node);

				$.invalidate_inner_signals(() => {
					change;
					$snapshot();
				});
			});

			select.__change = change;

			$.each(select, 5, () => ($snapshot(), $.untrack(() => $snapshot()?.nodes || [])), (n) => n.id, ($$anchor, n) => {
				var option = root_2();
				var text = $.child(option, true);

				$.reset(option);

				var option_value = {};

				$.template_effect(() => {
					$.set_text(text, ($.get(n), $.untrack(() => $.get(n).name)));

					if (option_value !== (option_value = ($.get(n), $.untrack(() => $.get(n).address)))) {
						option.value = (option.__value = ($.get(n), $.untrack(() => $.get(n).address))) ?? '';
					}
				});

				$.append($$anchor, option);
			});

			$.reset(select);
			$.bind_select_value(select, () => $.get(node), ($$value) => $.set(node, $$value));
			$.append($$anchor, select);
		};

		$.if(node_1, ($$render) => {
			if ($.get(kind) === 'node') $$render(consequent);
		});
	}

	var select_1 = $.sibling(node_1);

	$.template_effect(() => {
		$.get(hours);

		$.invalidate_inner_signals(() => {
			change;
		});
	});

	select_1.__change = change;

	var option_1 = $.child(select_1);

	option_1.value = option_1.__value = 1;

	var option_2 = $.sibling(option_1);

	option_2.value = option_2.__value = 6;

	var option_3 = $.sibling(option_2);

	option_3.value = option_3.__value = 24;

	var option_4 = $.sibling(option_3);

	option_4.value = option_4.__value = 168;
	$.reset(select_1);

	var button_2 = $.sibling(select_1);

	button_2.__click = () => load(true);

	var node_2 = $.child(button_2);

	Icon(node_2, { name: 'refresh', size: 14 });

	var text_1 = $.sibling(node_2, 1, true);

	$.reset(button_2);

	var button_3 = $.sibling(button_2);

	button_3.__click = exportCSV;

	var node_3 = $.child(button_3);

	Icon(node_3, { name: 'download', size: 14 });
	$.next();
	$.reset(button_3);
	$.reset(div);
	$.reset(section);

	var node_4 = $.sibling(section, 2);

	{
		var consequent_1 = ($$anchor) => {
			var p = root_3();
			var text_2 = $.child(p, true);

			$.reset(p);
			$.template_effect(() => $.set_text(text_2, ($errors(), $.untrack(() => $errors().history))));
			$.append($$anchor, p);
		};

		$.if(node_4, ($$render) => {
			if (($errors(), $.untrack(() => $errors().history))) $$render(consequent_1);
		});
	}

	var div_2 = $.sibling(node_4);
	var section_1 = $.child(div_2);
	var node_5 = $.child(section_1);

	Chart(node_5, {
		get samples() {
			return $.get(samples);
		},
		title: 'CQL OPTIONS 応答時間'
	});

	$.reset(section_1);

	var section_2 = $.sibling(section_1);
	var node_6 = $.child(section_2);

	Chart(node_6, {
		get samples() {
			return $.get(samples);
		},
		field: 'nodes_healthy',
		title: '正常ノード数',
		unit: '台'
	});

	$.reset(section_2);

	var section_3 = $.sibling(section_2);
	var node_7 = $.child(section_3);

	Chart(node_7, {
		get samples() {
			return $.get(samples);
		},
		field: 'cql_healthy',
		title: 'CQL応答あり',
		unit: '台'
	});

	$.reset(section_3);

	var section_4 = $.sibling(section_3);
	var node_8 = $.child(section_4);

	Chart(node_8, {
		get samples() {
			return $.get(samples);
		},
		field: 'internode_healthy',
		title: 'ノード間TCP 到達数',
		unit: '台'
	});

	$.reset(section_4);
	$.reset(div_2);

	var p_1 = $.sibling(div_2, 2);
	var text_3 = $.child(p_1);

	$.reset(p_1);

	var section_5 = $.sibling(p_1, 2);
	var div_3 = $.sibling($.child(section_5));

	$.each(div_3, 5, () => ($snapshot(), $.untrack(() => $snapshot()?.nodes || [])), (n) => n.id, ($$anchor, n) => {
		var div_4 = root_4();
		var h3 = $.child(div_4);
		var text_4 = $.child(h3, true);

		$.reset(h3);

		var dl = $.sibling(h3);
		var dd = $.sibling($.child(dl));
		var text_5 = $.child(dd, true);

		$.reset(dd);

		var dd_1 = $.sibling(dd, 2);
		var text_6 = $.child(dd_1, true);

		$.reset(dd_1);

		var dd_2 = $.sibling(dd_1, 2);
		var text_7 = $.child(dd_2, true);

		$.reset(dd_2);

		var dd_3 = $.sibling(dd_2, 2);
		var text_8 = $.child(dd_3, true);

		$.reset(dd_3);
		$.reset(dl);
		$.reset(div_4);

		$.template_effect(() => {
			$.set_text(text_4, ($.get(n), $.untrack(() => $.get(n).name)));

			$.set_text(text_5, (
				$.get(n),
				$.untrack(() => $.get(n).metric_values?.scylla_compaction_manager_compactions ?? '—')
			));

			$.set_text(text_6, (
				$.get(n),
				$.untrack(() => $.get(n).metric_values?.scylla_compaction_manager_pending_compactions ?? '—')
			));

			$.set_text(text_7, (
				$.get(n),
				$.untrack(() => $.get(n).metric_values?.scylla_storage_proxy_coordinator_read_timeouts ?? '—')
			));

			$.set_text(text_8, (
				$.get(n),
				$.untrack(() => $.get(n).metric_values?.scylla_storage_proxy_coordinator_write_timeouts ?? '—')
			));
		});

		$.append($$anchor, div_4);
	});

	$.reset(div_3);
	$.reset(section_5);

	$.template_effect(() => {
		classes = $.set_class(button, 1, '', null, classes, { active: $.get(kind) === 'cluster' });
		classes_1 = $.set_class(button_1, 1, '', null, classes_1, { active: $.get(kind) === 'node' });
		button_2.disabled = $.get(loading);
		$.set_text(text_1, $.get(loading) ? '取得中' : '履歴を更新');
		button_3.disabled = ($.get(samples), $.untrack(() => !$.get(samples).length));
		$.set_text(text_3, `SQLiteに保存した観測履歴 · ${($.get(samples), $.untrack(() => $.get(samples).length)) ?? ''} サンプル。範囲はサーバーの保存期間内で指定してください。欠測区間は線をつなぎません。再起動や単一ノードからの結果を、クエリ性能・複製正常性として扱わない設計です。`);
	});

	$.bind_select_value(select_1, () => $.get(hours), ($$value) => $.set(hours, $$value));
	$.append($$anchor, fragment);
	$.pop();
	$$cleanup();
}

$.delegate(['click', 'change']);