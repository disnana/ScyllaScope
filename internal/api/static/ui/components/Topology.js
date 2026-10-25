import '../vendor/svelte_internal_disclose-version.js';
import '../vendor/svelte_internal_flags_legacy.js';
import * as $ from '../vendor/svelte_internal_client.js';
import Icon from './Icon.js';
import { snapshot, clock, selected } from '../state.js';

import {
	nodeState,
	shortStatus,
	edgeState,
	sessionGet,
	sessionSet,
	checkState
} from '../lib.js';

var root_1 = $.from_html(`<p class="graph-limit"> </p>`);
var root_2 = $.from_html(`<div class="map-controls"><span class="quiet">矢印は観測方向です · ドラッグで移動</span><div class="button-group"><button aria-label="縮小"><!></button><span class="mono"> </span><button aria-label="拡大"><!></button><button aria-label="全体を表示"><!></button></div></div>`);
var root_3 = $.from_html(`<div class="empty"><!><strong>クラスタ情報を確認しています</strong><span>取得できない場合は診断に理由を表示します。</span></div>`);
var root_5 = $.from_svg(`<marker markerWidth="6" markerHeight="6" refX="5.5" refY="3" orient="auto"><path d="M0 0L6 3L0 6Z"></path></marker>`);
var root_6 = $.from_svg(`<g data-item="edge" class="edge-group" role="button" tabindex="0"><path class="edge-hit"></path><path></path></g>`);
var root_8 = $.from_svg(`<text x="194" y="89" class="node-local">●</text>`);
var root_7 = $.from_svg(`<g data-item="node" role="button" tabindex="0"><rect width="220" height="108" rx="10" class="node-bg"></rect><circle cx="17" cy="23" r="4"></circle><text x="29" y="27" class="node-title"> </text><text x="17" y="47" class="node-address"> </text><text x="17" y="66" class="node-sub"> </text><text x="17" y="90" class="node-state"> </text><circle cx="70" cy="85" r="3"></circle><text x="78" y="90" class="node-sub">CQL</text><circle cx="125" cy="85" r="3"></circle><text x="133" y="90" class="node-sub">7000</text><!></g>`);
var root_4 = $.from_svg(`<svg class="map" role="group" aria-label="Scyllaクラスタ接続図"><defs></defs><g><!><!></g></svg>`);
var root_10 = $.from_html(`<button><span></span><span><strong> </strong><small class="mono"> </small></span><span> </span><!></button>`);
var root_9 = $.from_html(`<div class="mobile-map-list" aria-label="ノードと接続の概要"><!><p> </p></div>`);
var root = $.from_html(`<div><!> <!> <!> <!> <div class="map-legend"><span><i class="line healthy"></i>応答あり</span><span><i class="line critical"></i>接続失敗</span><span><i class="line unknown"></i>未観測 / 期限切れ</span><span class="quiet">ノード・線を選択すると詳細</span></div></div>`);

export default function Topology($$anchor, $$props) {
	$.push($$props, false);

	const $snapshot = () => $.store_get(snapshot, '$snapshot', $$stores);
	const $clock = () => $.store_get(clock, '$clock', $$stores);
	const $selected = () => $.store_get(selected, '$selected', $$stores);
	const [$$stores, $$cleanup] = $.setup_stores();
	const viewWidth = $.mutable_source();
	const allNodes = $.mutable_source();
	const nodes = $.mutable_source();
	const positions = $.mutable_source();
	const edges = $.mutable_source();
	const height = $.mutable_source();
	let compact = $.prop($$props, 'compact', 8, false);
	let scope = $.prop($$props, 'scope', 8, 'all');
	let containerWidth = $.mutable_source(1080);
	let narrow = $.mutable_source(false);

	let // Hidden visited pages report width 0. Preserve their last visible layout.
	zoom = $.mutable_source(sessionGet('topology.zoom', 1));

	let pan = $.mutable_source(sessionGet('topology.pan', { x: 0, y: 0 }));
	let drag = null;
	let svg = $.mutable_source();

	function layout(ns, mobile) {
		const out = {};

		ns.forEach((n, i) => {
			if (mobile) out[n.id] = { x: 200, y: 80 + i * 155 }; else if (ns.length === 1) out[n.id] = { x: 540, y: 110 }; else if (ns.length === 2) out[n.id] = { x: 290 + i * 500, y: 160 }; else if (ns.length === 3) out[n.id] = [{ x: 540, y: 86 }, { x: 230, y: 255 }, { x: 850, y: 255 }][i]; else out[n.id] = { x: 190 + i % 3 * 350, y: 110 + Math.floor(i / 3) * 180 };
		});

		return out;
	}

	function makeEdges(ns, pos, actual, mobile) {
		const out = [];
		const edgeMap = new Map(actual.map((e) => [e.from + '>' + e.to, e]));

		for (let i = 0; i < ns.length; i++) for (let j = 0; j < ns.length; j++) {
			if (i === j || scope() !== 'all' && ns[i].id !== scope()) continue;

			const a = ns[i],
				b = ns[j],
				p = pos[a.id],
				q = pos[b.id],
				e = edgeMap.get(a.id + '>' + b.id);

			let d;
			const offset = i < j ? -5 : 5;

			if (mobile) {
				const sign = i < j ? -1 : 1;
				const lane = 200 + sign * (140 + Math.abs(j - i) * 12);

				d = `M${p.x + sign * 110},${p.y + offset}H${lane}V${q.y + offset}H${q.x + sign * 110}`;
			} else if (p.y === q.y) {
				const dir = q.x > p.x ? 1 : -1;

				d = `M${p.x + dir * 110},${p.y + offset}H${q.x - dir * 110}`;
			} else {
				const dir = q.y > p.y ? 1 : -1;
				const mid = (p.y + q.y) / 2 + offset;

				d = `M${p.x + offset},${p.y + dir * 54}V${mid}H${q.x + offset}V${q.y - dir * 54}`;
			}

			out.push({ key: a.id + '>' + b.id, from: a, to: b, e, d });
		}

		return out;
	}

	function open(n) {
		selected.set({ kind: 'node', id: n.id });
	}

	function edge(e) {
		selected.set({ kind: 'edge', from: e.from.id, to: e.to.id });
	}

	function scale(d) {
		$.set(zoom, Math.max(.5, Math.min(2.5, Math.round(($.get(zoom) + d) * 10) / 10)));
		sessionSet('topology.zoom', $.get(zoom));
	}

	function reset() {
		$.set(zoom, 1);
		$.set(pan, { x: 0, y: 0 });
		sessionSet('topology.zoom', $.get(zoom));
		sessionSet('topology.pan', $.get(pan));
	}

	function down(e) {
		if (compact() || e.button !== 0 || e.target.closest('[data-item]')) return;

		drag = { x: e.clientX, y: e.clientY, pan: { ...$.get(pan) } };
		$.get(svg).setPointerCapture(e.pointerId);
	}

	function move(e) {
		if (!drag) return;

		const r = $.get(svg).getBoundingClientRect();

		$.set(pan, {
			x: drag.pan.x + (e.clientX - drag.x) * $.get(viewWidth) / r.width,
			y: drag.pan.y + (e.clientY - drag.y) * $.get(height) / r.height
		});
	}

	function up() {
		if (drag) sessionSet('topology.pan', $.get(pan));

		drag = null;
	}

	$.legacy_pre_effect(() => ($.get(containerWidth), $.deep_read_state(compact())), () => {
		if ($.get(containerWidth) > 0) $.set(narrow, !compact() && $.get(containerWidth) < 600);
	});

	$.legacy_pre_effect(() => ($.get(narrow)), () => {
		$.set(viewWidth, $.get(narrow) ? 400 : 1080);
	});

	$.legacy_pre_effect(() => ($snapshot()), () => {
		$.set(allNodes, [...$snapshot()?.nodes || []].sort((a, b) => a.address === $snapshot()?.local_address
			? -1
			: b.address === $snapshot()?.local_address ? 1 : a.address.localeCompare(b.address)));
	});

	$.legacy_pre_effect(() => ($.deep_read_state(scope()), $.get(allNodes)), () => {
		$.set(nodes, (scope() === 'all'
			? $.get(allNodes)
			: [
				...$.get(allNodes).filter((n) => n.id === scope()),
				...$.get(allNodes).filter((n) => n.id !== scope())
			]).slice(0, 12));
	});

	$.legacy_pre_effect(() => ($.get(nodes), $.get(narrow)), () => {
		$.set(positions, layout($.get(nodes), $.get(narrow)));
	});

	$.legacy_pre_effect(() => ($.get(nodes), $.get(positions), $snapshot(), $.get(narrow)), () => {
		$.set(edges, makeEdges($.get(nodes), $.get(positions), $snapshot()?.edges || [], $.get(narrow)));
	});

	$.legacy_pre_effect(() => ($.get(narrow), $.get(nodes)), () => {
		$.set(height, $.get(narrow)
			? Math.max(220, $.get(nodes).length * 155 + 20)
			: $.get(nodes).length <= 1
				? 220
				: $.get(nodes).length <= 3
					? 350
					: Math.max(400, Math.ceil($.get(nodes).length / 3) * 180 + 60));
	});

	$.legacy_pre_effect_reset();
	$.init();

	var div = root();
	let classes;
	var node = $.child(div);

	{
		var consequent = ($$anchor) => {
			var p_1 = root_1();
			var text = $.child(p_1);

			$.reset(p_1);
			$.template_effect(() => $.set_text(text, `接続図は12台に限定しています（全${($.get(allNodes), $.untrack(() => $.get(allNodes).length)) ?? ''}台）。特定の観測元や接続マトリクスで範囲を切り替えてください。`));
			$.append($$anchor, p_1);
		};

		$.if(node, ($$render) => {
			if ((
				$.get(allNodes),
				$.untrack(() => $.get(allNodes).length > 12)
			)) $$render(consequent);
		});
	}

	var node_1 = $.sibling(node, 2);

	{
		var consequent_1 = ($$anchor) => {
			var div_1 = root_2();
			var div_2 = $.sibling($.child(div_1));
			var button = $.child(div_2);

			button.__click = () => scale(-.1);

			var node_2 = $.child(button);

			Icon(node_2, { name: 'minus', size: 14 });
			$.reset(button);

			var span = $.sibling(button);
			var text_1 = $.child(span);

			$.reset(span);

			var button_1 = $.sibling(span);

			button_1.__click = () => scale(.1);

			var node_3 = $.child(button_1);

			Icon(node_3, { name: 'plus', size: 14 });
			$.reset(button_1);

			var button_2 = $.sibling(button_1);

			button_2.__click = reset;

			var node_4 = $.child(button_2);

			Icon(node_4, { name: 'fit', size: 14 });
			$.reset(button_2);
			$.reset(div_2);
			$.reset(div_1);

			$.template_effect(($0) => $.set_text(text_1, `${$0 ?? ''}%`), [
				() => ($.get(zoom), $.untrack(() => Math.round($.get(zoom) * 100)))
			]);

			$.append($$anchor, div_1);
		};

		$.if(node_1, ($$render) => {
			if (!compact()) $$render(consequent_1);
		});
	}

	var node_5 = $.sibling(node_1, 2);

	{
		var consequent_2 = ($$anchor) => {
			var div_3 = root_3();
			var node_6 = $.child(div_3);

			Icon(node_6, { name: 'topology', size: 28 });
			$.next(2);
			$.reset(div_3);
			$.append($$anchor, div_3);
		};

		var alternate = ($$anchor) => {
			var svg_1 = root_4();

			svg_1.__pointerdown = down;
			svg_1.__pointermove = move;
			svg_1.__pointerup = up;

			let styles;
			var defs = $.child(svg_1);

			$.each(defs, 4, () => ['healthy', 'critical', 'unknown', 'warning'], $.index, ($$anchor, state) => {
				var marker = root_5();
				var path = $.child(marker);

				$.reset(marker);

				$.template_effect(() => {
					$.set_attribute(marker, 'id', `arrow-${compact() ? 'small' : 'full'}-${state}`);
					$.set_class(path, 0, `arrow-${state ?? ''}`);
				});

				$.append($$anchor, marker);
			});

			$.reset(defs);

			var g = $.sibling(defs);
			var node_7 = $.child(g);

			$.each(node_7, 1, () => $.get(edges), (link) => link.key, ($$anchor, link) => {
				const status = $.derived_safe_equal(() => (
					$.deep_read_state(edgeState),
					$.get(link),
					$snapshot(),
					$clock(),
					$.untrack(() => edgeState($.get(link).e, $snapshot(), $clock()))
				));

				var g_1 = root_6();

				g_1.__click = () => edge($.get(link));

				g_1.__keydown = (e) => {
					if (e.key === 'Enter' || e.key === ' ') {
						e.preventDefault();
						edge($.get(link));
					}
				};

				var path_1 = $.child(g_1);
				var path_2 = $.sibling(path_1);

				$.reset(g_1);

				$.template_effect(() => {
					$.set_attribute(g_1, 'aria-label', (
						$.get(link),
						$.deep_read_state($.get(status)),
						$.untrack(() => `${$.get(link).from.name}から${$.get(link).to.name}への接続: ${$.get(status)}`)
					));

					$.set_attribute(path_1, 'd', ($.get(link), $.untrack(() => $.get(link).d)));
					$.set_class(path_2, 0, `map-edge ${$.get(status) ?? ''}`);
					$.set_attribute(path_2, 'd', ($.get(link), $.untrack(() => $.get(link).d)));
					$.set_attribute(path_2, 'marker-end', `url(#arrow-${compact() ? 'small' : 'full'}-${$.get(status)})`);
				});

				$.append($$anchor, g_1);
			});

			var node_8 = $.sibling(node_7);

			$.each(node_8, 1, () => $.get(nodes), (n) => n.host_id || n.id, ($$anchor, n) => {
				const p = $.derived_safe_equal(() => (
					$.get(positions),
					$.get(n),
					$.untrack(() => $.get(positions)[$.get(n).id])
				));

				const state = $.derived_safe_equal(() => (
					$.deep_read_state(nodeState),
					$.get(n),
					$snapshot(),
					$clock(),
					$.untrack(() => nodeState($.get(n), $snapshot(), $clock()))
				));

				var g_2 = root_7();
				let classes_1;

				g_2.__click = () => open($.get(n));

				g_2.__keydown = (e) => {
					if (e.key === 'Enter' || e.key === ' ') {
						e.preventDefault();
						open($.get(n));
					}
				};

				var circle = $.sibling($.child(g_2));
				var text_2 = $.sibling(circle);
				var text_3 = $.child(text_2, true);

				$.reset(text_2);

				var text_4 = $.sibling(text_2);
				var text_5 = $.child(text_4, true);

				$.reset(text_4);

				var text_6 = $.sibling(text_4);
				var text_7 = $.child(text_6);

				$.reset(text_6);

				var text_8 = $.sibling(text_6);
				var text_9 = $.child(text_8, true);

				$.reset(text_8);

				var circle_1 = $.sibling(text_8);
				var circle_2 = $.sibling(circle_1, 2);
				var node_9 = $.sibling(circle_2, 2);

				{
					var consequent_3 = ($$anchor) => {
						var text_10 = root_8();

						$.append($$anchor, text_10);
					};

					$.if(node_9, ($$render) => {
						if ((
							$.get(n),
							$snapshot(),
							$.untrack(() => $.get(n).address === $snapshot()?.local_address)
						)) $$render(consequent_3);
					});
				}

				$.reset(g_2);

				$.template_effect(
					($0, $1, $2, $3) => {
						$.set_attribute(g_2, 'data-node-id', ($.get(n), $.untrack(() => $.get(n).id)));

						$.set_attribute(g_2, 'transform', (
							$.deep_read_state($.get(p)),
							$.untrack(() => `translate(${$.get(p).x - 110},${$.get(p).y - 54})`)
						));

						classes_1 = $.set_class(g_2, 0, `map-node ${$.get(state) ?? ''}`, null, classes_1, {
							chosen: $selected()?.kind === 'node' && $selected().id === $.get(n).id
						});

						$.set_attribute(g_2, 'aria-label', ($.get(n), $.untrack(() => `${$.get(n).name}の詳細`)));
						$.set_class(circle, 0, `status-fill ${$.get(state) ?? ''}`);
						$.set_text(text_3, $0);
						$.set_text(text_5, ($.get(n), $.untrack(() => $.get(n).address)));
						$.set_text(text_7, `${($.get(n), $.untrack(() => $.get(n).dc || 'DC 未取得')) ?? ''} / ${($.get(n), $.untrack(() => $.get(n).rack || 'Rack 未取得')) ?? ''}`);
						$.set_text(text_9, $1);
						$.set_class(circle_1, 0, `status-fill ${$2 ?? ''}`);
						$.set_class(circle_2, 0, `status-fill ${$3 ?? ''}`);
					},
					[
						() => (
							$.get(n),
							$.untrack(() => ($.get(n).name || $.get(n).address).length > 25
								? ($.get(n).name || $.get(n).address).slice(0, 24) + '…'
								: $.get(n).name || $.get(n).address)
						),

						() => (
							$.deep_read_state(shortStatus),
							$.get(n),
							$.untrack(() => shortStatus($.get(n)))
						),

						() => (
							$.deep_read_state(checkState),
							$.get(n),
							$clock(),
							$snapshot(),
							$.untrack(() => checkState($.get(n).cql, $clock(), $snapshot()?.stale_after_seconds))
						),

						() => (
							$.deep_read_state(checkState),
							$.get(n),
							$clock(),
							$snapshot(),
							$.untrack(() => checkState($.get(n).internode, $clock(), $snapshot()?.stale_after_seconds))
						)
					]
				);

				$.append($$anchor, g_2);
			});

			$.reset(g);
			$.reset(svg_1);
			$.bind_this(svg_1, ($$value) => $.set(svg, $$value), () => $.get(svg));

			$.template_effect(
				($0) => {
					$.set_attribute(svg_1, 'viewBox', `0 0 ${$.get(viewWidth)} ${$.get(height)}`);
					styles = $.set_style(svg_1, '', styles, $0);

					$.set_attribute(g, 'transform', (
						$.deep_read_state(compact()),
						$.get(pan),
						$.get(viewWidth),
						$.get(height),
						$.get(zoom),
						$.untrack(() => compact()
							? ''
							: `translate(${$.get(pan).x},${$.get(pan).y}) translate(${$.get(viewWidth) / 2},${$.get(height) / 2}) scale(${$.get(zoom)}) translate(${-$.get(viewWidth) / 2},${-$.get(height) / 2})`)
					));
				},
				[
					() => ({
						height: (
							$.deep_read_state(compact()),
							$.get(height),
							$.untrack(() => compact() ? '280px' : `${Math.min($.get(height), 520)}px`)
						)
					})
				]
			);

			$.event('pointercancel', svg_1, up);
			$.append($$anchor, svg_1);
		};

		$.if(node_5, ($$render) => {
			if (($.get(nodes), $.untrack(() => $.get(nodes).length === 0))) $$render(consequent_2); else $$render(alternate, false);
		});
	}

	var node_10 = $.sibling(node_5, 2);

	{
		var consequent_4 = ($$anchor) => {
			var div_4 = root_9();
			var node_11 = $.child(div_4);

			$.each(node_11, 1, () => $.get(nodes), (n) => n.host_id || n.id, ($$anchor, n) => {
				var button_3 = root_10();

				button_3.__click = () => open($.get(n));

				var span_1 = $.child(button_3);
				var span_2 = $.sibling(span_1);
				var strong = $.child(span_2);
				var text_11 = $.child(strong, true);

				$.reset(strong);

				var small = $.sibling(strong);
				var text_12 = $.child(small, true);

				$.reset(small);
				$.reset(span_2);

				var span_3 = $.sibling(span_2);
				var text_13 = $.child(span_3, true);

				$.reset(span_3);

				var node_12 = $.sibling(span_3);

				Icon(node_12, { name: 'chevron', size: 15 });
				$.reset(button_3);

				$.template_effect(
					($0, $1, $2) => {
						$.set_class(span_1, 1, `dot ${$0 ?? ''}`);
						$.set_text(text_11, ($.get(n), $.untrack(() => $.get(n).name)));
						$.set_text(text_12, ($.get(n), $.untrack(() => $.get(n).address)));
						$.set_class(span_3, 1, `badge ${$1 ?? ''}`);
						$.set_text(text_13, $2);
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
							$.deep_read_state(nodeState),
							$.get(n),
							$snapshot(),
							$clock(),
							$.untrack(() => nodeState($.get(n), $snapshot(), $clock()))
						),

						() => (
							$.deep_read_state(shortStatus),
							$.get(n),
							$.untrack(() => shortStatus($.get(n)))
						)
					]
				);

				$.append($$anchor, button_3);
			});

			var p_2 = $.sibling(node_11);
			var text_14 = $.child(p_2);

			$.reset(p_2);
			$.reset(div_4);

			$.template_effect(() => $.set_text(text_14, `観測方向 ${(
				$snapshot(),
				$.untrack(() => $snapshot()?.coverage?.observed_directions || 0)
			) ?? ''} / ${(
				$snapshot(),
				$.untrack(() => $snapshot()?.coverage?.expected_directions || 0)
			) ?? ''} · 接続図は「詳細を開く」へ`));

			$.append($$anchor, div_4);
		};

		$.if(node_10, ($$render) => {
			if (compact()) $$render(consequent_4);
		});
	}

	$.next(2);
	$.reset(div);
	$.template_effect(() => classes = $.set_class(div, 1, 'topology', null, classes, { compact: compact() }));
	$.bind_element_size(div, 'clientWidth', ($$value) => $.set(containerWidth, $$value));
	$.append($$anchor, div);
	$.pop();
	$$cleanup();
}

$.delegate([
	'click',
	'pointerdown',
	'pointermove',
	'pointerup',
	'keydown'
]);