import '../vendor/svelte_internal_disclose-version.js';
import '../vendor/svelte_internal_flags_legacy.js';
import * as $ from '../vendor/svelte_internal_client.js';
import { snapshot, clock, selected } from '../state.js';
import Badge from './Badge.js';
import Check from './Check.js';
import Icon from './Icon.js';
import { nodeState, shortStatus, bytes } from '../lib.js';

var root_1 = $.from_html(`<th>観測エージェント</th><th>DC / Rack</th>`, 1);
var root_3 = $.from_html(`<td><!></td><td class="quiet"> </td>`, 1);
var root_2 = $.from_html(`<tr><td><button class="node-name"><span></span><span><strong> </strong><small class="mono"> </small></span></button></td><td><!></td><td><!></td><td><!></td><!><td class="numeric mono"> </td><td><button class="icon-button"><!></button></td></tr>`);
var root_4 = $.from_html(`<tr><td class="empty-row">表示するノードがありません</td></tr>`);
var root = $.from_html(`<div class="table-scroll"><table class="node-table"><thead><tr><th>ノード</th><th>状態</th><th>CQL応答</th><th>ノード間TCP</th><!><th class="numeric">データ量</th><th><span class="sr-only">詳細</span></th></tr></thead><tbody></tbody></table></div>`);

export default function NodeTable($$anchor, $$props) {
	$.push($$props, false);

	const $snapshot = () => $.store_get(snapshot, '$snapshot', $$stores);
	const $clock = () => $.store_get(clock, '$clock', $$stores);
	const [$$stores, $$cleanup] = $.setup_stores();
	let nodes = $.prop($$props, 'nodes', 24, () => []);
	let detailed = $.prop($$props, 'detailed', 8, false);

	$.init();

	var div = root();
	var table = $.child(div);
	var thead = $.child(table);
	var tr = $.child(thead);
	var node = $.sibling($.child(tr), 4);

	{
		var consequent = ($$anchor) => {
			var fragment = root_1();

			$.next();
			$.append($$anchor, fragment);
		};

		$.if(node, ($$render) => {
			if (detailed()) $$render(consequent);
		});
	}

	$.next(2);
	$.reset(tr);
	$.reset(thead);

	var tbody = $.sibling(thead);

	$.each(
		tbody,
		5,
		nodes,
		(n) => n.host_id || n.id,
		($$anchor, n) => {
			var tr_1 = root_2();
			var td = $.child(tr_1);
			var button = $.child(td);

			button.__click = () => selected.set({ kind: 'node', id: $.get(n).id });

			var span = $.child(button);
			var span_1 = $.sibling(span);
			var strong = $.child(span_1);
			var text = $.child(strong, true);

			$.reset(strong);

			var small = $.sibling(strong);
			var text_1 = $.child(small, true);

			$.reset(small);
			$.reset(span_1);
			$.reset(button);
			$.reset(td);

			var td_1 = $.sibling(td);
			var node_1 = $.child(td_1);

			{
				let $0 = $.derived_safe_equal(() => (
					$.deep_read_state(nodeState),
					$.get(n),
					$snapshot(),
					$clock(),
					$.untrack(() => nodeState($.get(n), $snapshot(), $clock()))
				));

				let $1 = $.derived_safe_equal(() => (
					$.deep_read_state(shortStatus),
					$.get(n),
					$.untrack(() => shortStatus($.get(n)))
				));

				Badge(node_1, {
					get state() {
						return $.get($0);
					},

					get text() {
						return $.get($1);
					}
				});
			}

			$.reset(td_1);

			var td_2 = $.sibling(td_1);
			var node_2 = $.child(td_2);

			{
				let $0 = $.derived_safe_equal(() => (
					$snapshot(),
					$.untrack(() => $snapshot()?.stale_after_seconds)
				));

				Check(node_2, {
					get value() {
						return ($.get(n), $.untrack(() => $.get(n).cql));
					},

					get ttl() {
						return $.get($0);
					},
					timings: true
				});
			}

			$.reset(td_2);

			var td_3 = $.sibling(td_2);
			var node_3 = $.child(td_3);

			{
				let $0 = $.derived_safe_equal(() => (
					$snapshot(),
					$.untrack(() => $snapshot()?.stale_after_seconds)
				));

				Check(node_3, {
					get value() {
						return ($.get(n), $.untrack(() => $.get(n).internode));
					},

					get ttl() {
						return $.get($0);
					}
				});
			}

			$.reset(td_3);

			var node_4 = $.sibling(td_3);

			{
				var consequent_1 = ($$anchor) => {
					var fragment_1 = root_3();
					var td_4 = $.first_child(fragment_1);
					var node_5 = $.child(td_4);

					{
						let $0 = $.derived_safe_equal(() => (
							$snapshot(),
							$.untrack(() => $snapshot()?.stale_after_seconds)
						));

						Check(node_5, {
							get value() {
								return ($.get(n), $.untrack(() => $.get(n).watch_api));
							},

							get ttl() {
								return $.get($0);
							}
						});
					}

					$.reset(td_4);

					var td_5 = $.sibling(td_4);
					var text_2 = $.child(td_5);

					$.reset(td_5);
					$.template_effect(() => $.set_text(text_2, `${($.get(n), $.untrack(() => $.get(n).dc || '—')) ?? ''} / ${($.get(n), $.untrack(() => $.get(n).rack || '—')) ?? ''}`));
					$.append($$anchor, fragment_1);
				};

				$.if(node_4, ($$render) => {
					if (detailed()) $$render(consequent_1);
				});
			}

			var td_6 = $.sibling(node_4);
			var text_3 = $.child(td_6, true);

			$.reset(td_6);

			var td_7 = $.sibling(td_6);
			var button_1 = $.child(td_7);

			button_1.__click = () => selected.set({ kind: 'node', id: $.get(n).id });

			var node_6 = $.child(button_1);

			Icon(node_6, { name: 'chevron', size: 14 });
			$.reset(button_1);
			$.reset(td_7);
			$.reset(tr_1);

			$.template_effect(
				($0, $1) => {
					$.set_attribute(tr_1, 'data-row-id', ($.get(n), $.untrack(() => $.get(n).id)));
					$.set_class(span, 1, `dot ${$0 ?? ''}`);
					$.set_text(text, ($.get(n), $.untrack(() => $.get(n).name || $.get(n).address)));
					$.set_text(text_1, ($.get(n), $.untrack(() => $.get(n).address)));
					$.set_text(text_3, $1);
					$.set_attribute(button_1, 'aria-label', ($.get(n), $.untrack(() => `${$.get(n).name}を開く`)));
				},
				[
					() => (
						$.deep_read_state(nodeState),
						$.get(n),
						$snapshot(),
						$clock(),
						$.untrack(() => nodeState($.get(n), $snapshot(), $clock()))
					),

					() => (
						$.get(n),
						$.deep_read_state(bytes),
						$.untrack(() => $.get(n).load_known && $.get(n).metadata_known ? bytes($.get(n).load_bytes) : '—')
					)
				]
			);

			$.append($$anchor, tr_1);
		},
		($$anchor) => {
			var tr_2 = root_4();
			var td_8 = $.child(tr_2);

			$.reset(tr_2);
			$.template_effect(() => $.set_attribute(td_8, 'colspan', detailed() ? 8 : 6));
			$.append($$anchor, tr_2);
		}
	);

	$.reset(tbody);
	$.reset(table);
	$.reset(div);
	$.append($$anchor, div);
	$.pop();
	$$cleanup();
}

$.delegate(['click']);