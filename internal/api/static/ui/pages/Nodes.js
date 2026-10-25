import '../vendor/svelte_internal_disclose-version.js';
import '../vendor/svelte_internal_flags_legacy.js';
import * as $ from '../vendor/svelte_internal_client.js';
import { snapshot, clock } from '../state.js';
import { nodeState, sessionGet, sessionSet } from '../lib.js';
import NodeTable from '../components/NodeTable.js';
import Icon from '../components/Icon.js';

var root = $.from_html(`<section class="panel"><div class="toolbar"><label class="search"><!><input id="node-search" aria-label="ノード検索" placeholder="名前・IP・DCで検索"/></label><label class="inline-field">状態 <select aria-label="ノード状態で絞り込み"><option>すべて</option><option>正常</option><option>注意</option><option>異常</option><option>未確認</option></select></label><label class="inline-field">並び順 <select><option>名前</option><option>状態</option><option>データ量</option></select></label><span class="quiet count"> </span></div><!></section> <p class="hint">チェックは表示中の観測元からの結果です。ノード名を選択すると、systemd・ローカルREST・メトリクス・方向別の接続を確認できます。</p>`, 1);

export default function Nodes($$anchor, $$props) {
	$.push($$props, false);

	const $snapshot = () => $.store_get(snapshot, '$snapshot', $$stores);
	const $clock = () => $.store_get(clock, '$clock', $$stores);
	const [$$stores, $$cleanup] = $.setup_stores();
	const nodes = $.mutable_source();
	let search = $.mutable_source(sessionGet('nodes.search', ''));
	let filter = $.mutable_source(sessionGet('nodes.filter', 'all'));
	let sort = $.mutable_source(sessionGet('nodes.sort', 'name'));

	$.legacy_pre_effect(() => (sessionSet, $.get(search)), () => {
		sessionSet('nodes.search', $.get(search));
	});

	$.legacy_pre_effect(() => (sessionSet, $.get(filter)), () => {
		sessionSet('nodes.filter', $.get(filter));
	});

	$.legacy_pre_effect(() => (sessionSet, $.get(sort)), () => {
		sessionSet('nodes.sort', $.get(sort));
	});

	$.legacy_pre_effect(
		() => (
			$snapshot(),
			$.get(filter),
			nodeState,
			$clock(),
			$.get(search),
			$.get(sort)
		),
		() => {
			$.set(nodes, ($snapshot()?.nodes || []).filter((n) => ($.get(filter) === 'all' || nodeState(n, $snapshot(), $clock()) === $.get(filter)) && (n.name + ' ' + n.address + ' ' + n.dc + ' ' + n.rack).toLowerCase().includes($.get(search).toLowerCase())).sort((a, b) => $.get(sort) === 'state'
				? String(a.state).localeCompare(String(b.state))
				: $.get(sort) === 'load'
					? (b.load_bytes || 0) - (a.load_bytes || 0)
					: String(a.name).localeCompare(String(b.name))));
		}
	);

	$.legacy_pre_effect_reset();
	$.init();

	var fragment = root();
	var section = $.first_child(fragment);
	var div = $.child(section);
	var label = $.child(div);
	var node = $.child(label);

	Icon(node, { name: 'search', size: 16 });

	var input = $.sibling(node);

	$.remove_input_defaults(input);
	$.reset(label);

	var label_1 = $.sibling(label);
	var select = $.sibling($.child(label_1));

	$.template_effect(() => {
		$.get(filter);
		$.invalidate_inner_signals(() => {});
	});

	var option = $.child(select);

	option.value = option.__value = 'all';

	var option_1 = $.sibling(option);

	option_1.value = option_1.__value = 'healthy';

	var option_2 = $.sibling(option_1);

	option_2.value = option_2.__value = 'warning';

	var option_3 = $.sibling(option_2);

	option_3.value = option_3.__value = 'critical';

	var option_4 = $.sibling(option_3);

	option_4.value = option_4.__value = 'unknown';
	$.reset(select);
	$.reset(label_1);

	var label_2 = $.sibling(label_1);
	var select_1 = $.sibling($.child(label_2));

	$.template_effect(() => {
		$.get(sort);
		$.invalidate_inner_signals(() => {});
	});

	var option_5 = $.child(select_1);

	option_5.value = option_5.__value = 'name';

	var option_6 = $.sibling(option_5);

	option_6.value = option_6.__value = 'state';

	var option_7 = $.sibling(option_6);

	option_7.value = option_7.__value = 'load';
	$.reset(select_1);
	$.reset(label_2);

	var span = $.sibling(label_2);
	var text = $.child(span);

	$.reset(span);
	$.reset(div);

	var node_1 = $.sibling(div);

	NodeTable(node_1, {
		get nodes() {
			return $.get(nodes);
		},
		detailed: true
	});

	$.reset(section);
	$.next(2);
	$.template_effect(() => $.set_text(text, `${($.get(nodes), $.untrack(() => $.get(nodes).length)) ?? ''} / ${($snapshot(), $.untrack(() => $snapshot()?.nodes.length || 0)) ?? ''} 台`));
	$.bind_value(input, () => $.get(search), ($$value) => $.set(search, $$value));
	$.bind_select_value(select, () => $.get(filter), ($$value) => $.set(filter, $$value));
	$.bind_select_value(select_1, () => $.get(sort), ($$value) => $.set(sort, $$value));
	$.append($$anchor, fragment);
	$.pop();
	$$cleanup();
}