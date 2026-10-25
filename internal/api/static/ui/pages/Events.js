import '../vendor/svelte_internal_disclose-version.js';
import '../vendor/svelte_internal_flags_legacy.js';
import * as $ from '../vendor/svelte_internal_client.js';
import { onMount } from '../vendor/svelte_svelte.js';
import { events, errors, loadResource } from '../state.js';
import { date, sessionGet, sessionSet, download, csv } from '../lib.js';
import Icon from '../components/Icon.js';

var root_1 = $.from_html(`<p class="notice warning"> </p>`);
var root_3 = $.from_html(`<div class="event-detail"><span> </span><span> </span><span> </span></div>`);
var root_2 = $.from_html(`<tr><td class="mono nowrap"> </td><td><span></span><span class="quiet"> </span></td><td><button class="event-expand"> </button><!></td></tr>`);
var root_4 = $.from_html(`<tr><td colspan="3" class="empty-row">該当するイベントはありません</td></tr>`);
var root = $.from_html(`<section class="panel"><div class="toolbar"><label class="search"><!><input placeholder="イベントを検索" aria-label="イベント検索"/></label><select aria-label="重要度"><option>すべての重要度</option><option>異常</option><option>注意</option><option>情報</option></select><button><!>履歴を更新</button><button><!>CSV</button><span class="quiet count"> </span></div><!><div class="table-scroll"><table class="events-table"><thead><tr><th>時刻</th><th>種別</th><th>内容</th></tr></thead><tbody></tbody></table></div></section>`);

export default function Events($$anchor, $$props) {
	$.push($$props, false);

	const $events = () => $.store_get(events, '$events', $$stores);
	const $errors = () => $.store_get(errors, '$errors', $$stores);
	const [$$stores, $$cleanup] = $.setup_stores();
	const shown = $.mutable_source();
	let search = $.mutable_source(sessionGet('events.search', ''));
	let level = $.mutable_source(sessionGet('events.level', 'all'));
	let expanded = $.mutable_source(sessionGet('events.expanded', null));

	onMount(() => loadResource('events'));

	$.legacy_pre_effect(() => (sessionSet, $.get(search)), () => {
		sessionSet('events.search', $.get(search));
	});

	$.legacy_pre_effect(() => (sessionSet, $.get(level)), () => {
		sessionSet('events.level', $.get(level));
	});

	$.legacy_pre_effect(() => (sessionSet, $.get(expanded)), () => {
		sessionSet('events.expanded', $.get(expanded));
	});

	$.legacy_pre_effect(() => ($events(), $.get(level), $.get(search)), () => {
		$.set(shown, $events().filter((e) => ($.get(level) === 'all' || e.severity === $.get(level)) && (e.message + ' ' + e.node + ' ' + e.type).toLowerCase().includes($.get(search).toLowerCase())));
	});

	$.legacy_pre_effect_reset();
	$.init();

	var section = root();
	var div = $.child(section);
	var label = $.child(div);
	var node = $.child(label);

	Icon(node, { name: 'search', size: 16 });

	var input = $.sibling(node);

	$.remove_input_defaults(input);
	$.reset(label);

	var select = $.sibling(label);

	$.template_effect(() => {
		$.get(level);
		$.invalidate_inner_signals(() => {});
	});

	var option = $.child(select);

	option.value = option.__value = 'all';

	var option_1 = $.sibling(option);

	option_1.value = option_1.__value = 'critical';

	var option_2 = $.sibling(option_1);

	option_2.value = option_2.__value = 'warning';

	var option_3 = $.sibling(option_2);

	option_3.value = option_3.__value = 'info';
	$.reset(select);

	var button = $.sibling(select);

	button.__click = () => loadResource('events', true);

	var node_1 = $.child(button);

	Icon(node_1, { name: 'refresh', size: 14 });
	$.next();
	$.reset(button);

	var button_1 = $.sibling(button);

	button_1.__click = () => download('scyllascope-events.csv', csv($.get(shown), ['time', 'severity', 'type', 'node', 'message']), 'text/csv;charset=utf-8');

	var node_2 = $.child(button_1);

	Icon(node_2, { name: 'download', size: 14 });
	$.next();
	$.reset(button_1);

	var span = $.sibling(button_1);
	var text = $.child(span);

	$.reset(span);
	$.reset(div);

	var node_3 = $.sibling(div);

	{
		var consequent = ($$anchor) => {
			var p = root_1();
			var text_1 = $.child(p, true);

			$.reset(p);
			$.template_effect(() => $.set_text(text_1, ($errors(), $.untrack(() => $errors().events))));
			$.append($$anchor, p);
		};

		$.if(node_3, ($$render) => {
			if (($errors(), $.untrack(() => $errors().events))) $$render(consequent);
		});
	}

	var div_1 = $.sibling(node_3);
	var table = $.child(div_1);
	var tbody = $.sibling($.child(table));

	$.each(
		tbody,
		5,
		() => $.get(shown),
		(e) => e.id,
		($$anchor, e) => {
			var tr = root_2();
			var td = $.child(tr);
			var text_2 = $.child(td, true);

			$.reset(td);

			var td_1 = $.sibling(td);
			var span_1 = $.child(td_1);
			var span_2 = $.sibling(span_1);
			var text_3 = $.child(span_2, true);

			$.reset(span_2);
			$.reset(td_1);

			var td_2 = $.sibling(td_1);
			var button_2 = $.child(td_2);

			button_2.__click = () => $.set(expanded, $.get(expanded) === $.get(e).id ? null : $.get(e).id);

			var text_4 = $.child(button_2, true);

			$.reset(button_2);

			var node_4 = $.sibling(button_2);

			{
				var consequent_1 = ($$anchor) => {
					var div_2 = root_3();
					var span_3 = $.child(div_2);
					var text_5 = $.child(span_3);

					$.reset(span_3);

					var span_4 = $.sibling(span_3);
					var text_6 = $.child(span_4);

					$.reset(span_4);

					var span_5 = $.sibling(span_4);
					var text_7 = $.child(span_5);

					$.reset(span_5);
					$.reset(div_2);

					$.template_effect(() => {
						$.set_text(text_5, `ノード: ${($.get(e), $.untrack(() => $.get(e).node || '—')) ?? ''}`);
						$.set_text(text_6, `ID: ${($.get(e), $.untrack(() => $.get(e).id)) ?? ''}`);
						$.set_text(text_7, `重要度: ${($.get(e), $.untrack(() => $.get(e).severity)) ?? ''}`);
					});

					$.append($$anchor, div_2);
				};

				$.if(node_4, ($$render) => {
					if ((
						$.get(expanded),
						$.get(e),
						$.untrack(() => $.get(expanded) === $.get(e).id)
					)) $$render(consequent_1);
				});
			}

			$.reset(td_2);
			$.reset(tr);

			$.template_effect(
				($0) => {
					$.set_text(text_2, $0);
					$.set_class(span_1, 1, `dot ${($.get(e), $.untrack(() => $.get(e).severity)) ?? ''}`);
					$.set_text(text_3, ($.get(e), $.untrack(() => $.get(e).type)));

					$.set_attribute(button_2, 'aria-expanded', (
						$.get(expanded),
						$.get(e),
						$.untrack(() => $.get(expanded) === $.get(e).id)
					));

					$.set_text(text_4, ($.get(e), $.untrack(() => $.get(e).message)));
				},
				[
					() => (
						$.deep_read_state(date),
						$.get(e),
						$.untrack(() => date($.get(e).time))
					)
				]
			);

			$.append($$anchor, tr);
		},
		($$anchor) => {
			var tr_1 = root_4();

			$.append($$anchor, tr_1);
		}
	);

	$.reset(tbody);
	$.reset(table);
	$.reset(div_1);
	$.reset(section);
	$.template_effect(() => $.set_text(text, `${($.get(shown), $.untrack(() => $.get(shown).length)) ?? ''} 件（最大500件）`));
	$.bind_value(input, () => $.get(search), ($$value) => $.set(search, $$value));
	$.bind_select_value(select, () => $.get(level), ($$value) => $.set(level, $$value));
	$.append($$anchor, section);
	$.pop();
	$$cleanup();
}

$.delegate(['click']);