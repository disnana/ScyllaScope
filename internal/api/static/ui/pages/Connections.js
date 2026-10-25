import '../vendor/svelte_internal_disclose-version.js';
import '../vendor/svelte_internal_flags_legacy.js';
import * as $ from '../vendor/svelte_internal_client.js';
import { snapshot, clock, selected } from '../state.js';
import Topology from '../components/Topology.js';
import { edgeState, labels, sessionGet, sessionSet } from '../lib.js';

var root_1 = $.from_html(`<option> </option>`);
var root_2 = $.from_html(`<p class="graph-limit"> </p>`);
var root_3 = $.from_html(`<th> </th>`);
var root_6 = $.from_html(`<span class="quiet">—</span>`);
var root_7 = $.from_html(`<button><span class="dot"></span> </button>`);
var root_5 = $.from_html(`<td><!></td>`);
var root_4 = $.from_html(`<tr><th> </th><!></tr>`);
var root = $.from_html(`<section class="panel"><header class="panel-header"><div class="tabs"><button>接続図</button><button>接続マトリクス</button></div><label class="inline-field">観測元 <select aria-label="接続図の観測元"><option>すべての観測</option><!></select></label></header><div><!></div><div class="table-scroll"><!><table class="matrix"><thead><tr><th>送信元 ↓ / 宛先 →</th><!></tr></thead><tbody></tbody></table></div></section> <div class="note-grid"><div><h3>矢印は観測方向</h3><p>「AからBへ接続できた」と「BからAへ接続できた」は別の結果です。線を選ぶとCQL・7000・Scope APIを確認できます。</p></div><div><h3>破線は未観測</h3><p>エージェント未導入や古い情報を「DOWN」とは扱いません。データ複製経路やRaftリーダーを表す図ではありません。</p></div></div>`, 1);

export default function Connections($$anchor, $$props) {
	$.push($$props, false);

	const $snapshot = () => $.store_get(snapshot, '$snapshot', $$stores);
	const $clock = () => $.store_get(clock, '$clock', $$stores);
	const [$$stores, $$cleanup] = $.setup_stores();
	const nodes = $.mutable_source();
	const edgeMap = $.mutable_source();
	const rows = $.mutable_source();
	const columns = $.mutable_source();
	let scope = $.mutable_source(sessionGet('topology.scope', 'all'));
	let view = $.mutable_source(sessionGet('topology.view', 'map'));

	function edge(a, b) {
		return $.get(edgeMap).get(a.id + '>' + b.id);
	}

	$.legacy_pre_effect(() => (sessionSet, $.get(scope)), () => {
		sessionSet('topology.scope', $.get(scope));
	});

	$.legacy_pre_effect(() => (sessionSet, $.get(view)), () => {
		sessionSet('topology.view', $.get(view));
	});

	$.legacy_pre_effect(() => ($snapshot()), () => {
		$.set(nodes, $snapshot()?.nodes || []);
	});

	$.legacy_pre_effect(() => ($snapshot()), () => {
		$.set(edgeMap, new Map(($snapshot()?.edges || []).map((e) => [e.from + '>' + e.to, e])));
	});

	$.legacy_pre_effect(() => ($.get(scope), $.get(nodes)), () => {
		$.set(rows, $.get(scope) === 'all'
			? $.get(nodes).slice(0, 12)
			: $.get(nodes).filter((n) => n.id === $.get(scope)));
	});

	$.legacy_pre_effect(() => ($.get(scope), $.get(nodes)), () => {
		$.set(columns, $.get(scope) === 'all' ? $.get(nodes).slice(0, 12) : $.get(nodes));
	});

	$.legacy_pre_effect_reset();
	$.init();

	var fragment = root();
	var section = $.first_child(fragment);
	var header = $.child(section);
	var div = $.child(header);
	var button = $.child(div);

	button.__click = () => $.set(view, 'map');

	let classes;
	var button_1 = $.sibling(button);

	button_1.__click = () => $.set(view, 'matrix');

	let classes_1;

	$.reset(div);

	var label = $.sibling(div);
	var select = $.sibling($.child(label));

	$.template_effect(() => {
		$.get(scope);

		$.invalidate_inner_signals(() => {
			$.get(nodes);
		});
	});

	var option = $.child(select);

	option.value = option.__value = 'all';

	var node = $.sibling(option);

	$.each(node, 1, () => $.get(nodes), (n) => n.id, ($$anchor, n) => {
		var option_1 = root_1();
		var text = $.child(option_1, true);

		$.reset(option_1);

		var option_1_value = {};

		$.template_effect(() => {
			$.set_text(text, ($.get(n), $.untrack(() => $.get(n).name)));

			if (option_1_value !== (option_1_value = ($.get(n), $.untrack(() => $.get(n).id)))) {
				option_1.value = (option_1.__value = ($.get(n), $.untrack(() => $.get(n).id))) ?? '';
			}
		});

		$.append($$anchor, option_1);
	});

	$.reset(select);
	$.reset(label);
	$.reset(header);

	var div_1 = $.sibling(header);
	var node_1 = $.child(div_1);

	Topology(node_1, {
		get scope() {
			return $.get(scope);
		}
	});

	$.reset(div_1);

	var div_2 = $.sibling(div_1);
	var node_2 = $.child(div_2);

	{
		var consequent = ($$anchor) => {
			var p = root_2();
			var text_1 = $.child(p);

			$.reset(p);
			$.template_effect(() => $.set_text(text_1, `一覧は先頭12台。観測元を選択すると、そのノードから全${($.get(nodes), $.untrack(() => $.get(nodes).length)) ?? ''}台への結果を表示します。`));
			$.append($$anchor, p);
		};

		$.if(node_2, ($$render) => {
			if ((
				$.get(nodes),
				$.get(scope),
				$.untrack(() => $.get(nodes).length > 12 && $.get(scope) === 'all')
			)) $$render(consequent);
		});
	}

	var table = $.sibling(node_2);
	var thead = $.child(table);
	var tr = $.child(thead);
	var node_3 = $.sibling($.child(tr));

	$.each(node_3, 1, () => $.get(columns), (n) => n.id, ($$anchor, n) => {
		var th = root_3();
		var text_2 = $.child(th, true);

		$.reset(th);
		$.template_effect(() => $.set_text(text_2, ($.get(n), $.untrack(() => $.get(n).name))));
		$.append($$anchor, th);
	});

	$.reset(tr);
	$.reset(thead);

	var tbody = $.sibling(thead);

	$.each(tbody, 5, () => $.get(rows), (a) => a.id, ($$anchor, a) => {
		var tr_1 = root_4();
		var th_1 = $.child(tr_1);
		var text_3 = $.child(th_1, true);

		$.reset(th_1);

		var node_4 = $.sibling(th_1);

		$.each(node_4, 1, () => $.get(columns), (b) => b.id, ($$anchor, b) => {
			var td = root_5();
			var node_5 = $.child(td);

			{
				var consequent_1 = ($$anchor) => {
					var span = root_6();

					$.append($$anchor, span);
				};

				var alternate = ($$anchor) => {
					const state = $.derived_safe_equal(() => (
						$.deep_read_state(edgeState),
						$.get(a),
						$.get(b),
						$snapshot(),
						$clock(),
						$.untrack(() => edgeState(edge($.get(a), $.get(b)), $snapshot(), $clock()))
					));

					var button_2 = root_7();

					button_2.__click = () => selected.set({ kind: 'edge', from: $.get(a).id, to: $.get(b).id });

					var text_4 = $.sibling($.child(button_2), 1, true);

					$.reset(button_2);

					$.template_effect(() => {
						$.set_class(button_2, 1, `matrix-cell ${$.get(state) ?? ''}`);

						$.set_text(text_4, (
							$.deep_read_state(labels),
							$.deep_read_state($.get(state)),
							$.untrack(() => labels[$.get(state)])
						));
					});

					$.append($$anchor, button_2);
				};

				$.if(node_5, ($$render) => {
					if ((
						$.get(a),
						$.get(b),
						$.untrack(() => $.get(a).id === $.get(b).id)
					)) $$render(consequent_1); else $$render(alternate, false);
				});
			}

			$.reset(td);
			$.append($$anchor, td);
		});

		$.reset(tr_1);
		$.template_effect(() => $.set_text(text_3, ($.get(a), $.untrack(() => $.get(a).name))));
		$.append($$anchor, tr_1);
	});

	$.reset(tbody);
	$.reset(table);
	$.reset(div_2);
	$.reset(section);
	$.next(2);

	$.template_effect(() => {
		classes = $.set_class(button, 1, '', null, classes, { active: $.get(view) === 'map' });
		classes_1 = $.set_class(button_1, 1, '', null, classes_1, { active: $.get(view) === 'matrix' });
		$.set_attribute(div_1, 'hidden', $.get(view) !== 'map');
		$.set_attribute(div_2, 'hidden', $.get(view) !== 'matrix');
	});

	$.bind_select_value(select, () => $.get(scope), ($$value) => $.set(scope, $$value));
	$.append($$anchor, fragment);
	$.pop();
	$$cleanup();
}

$.delegate(['click']);