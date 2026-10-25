import '../vendor/svelte_internal_disclose-version.js';
import '../vendor/svelte_internal_flags_legacy.js';
import * as $ from '../vendor/svelte_internal_client.js';
import { chartPaths, formatMS, time, finite } from '../lib.js';

var root_1 = $.from_html(`<svg class="chart" viewBox="0 0 580 128" role="img" preserveAspectRatio="none"><path class="chart-grid" d="M12 30H568 M12 74H568 M12 118H568"></path><path class="chart-line" vector-effect="non-scaling-stroke"></path></svg> <div class="chart-axis"><span> </span><span> </span></div>`, 1);
var root_2 = $.from_html(`<div class="chart-empty">履歴を収集中<span>未取得の値は0に変換しません</span></div>`);
var root = $.from_html(`<div><div class="chart-top"><span class="quiet"> </span><strong class="mono"> </strong></div> <!></div>`);

export default function Chart($$anchor, $$props) {
	$.push($$props, false);

	const data = $.mutable_source();
	const value = $.mutable_source();
	let samples = $.prop($$props, 'samples', 24, () => []);
	let field = $.prop($$props, 'field', 8, 'average_latency_ms');
	let title = $.prop($$props, 'title', 8, 'CQL応答');
	let unit = $.prop($$props, 'unit', 8, 'ms');
	let small = $.prop($$props, 'small', 8, false);

	$.legacy_pre_effect(
		() => (
			chartPaths,
			$.deep_read_state(samples()),
			$.deep_read_state(field())
		),
		() => {
			$.set(data, chartPaths(samples(), (s) => s[field()]));
		}
	);

	$.legacy_pre_effect(
		() => (
			$.deep_read_state(samples()),
			$.deep_read_state(field()),
			finite
		),
		() => {
			$.set(value, [...samples()].reverse().map((s) => s[field()]).find(finite));
		}
	);

	$.legacy_pre_effect_reset();
	$.init();

	var div = root();
	let classes;
	var div_1 = $.child(div);
	var span = $.child(div_1);
	var text = $.child(span, true);

	$.reset(span);

	var strong = $.sibling(span);
	var text_1 = $.child(strong, true);

	$.reset(strong);
	$.reset(div_1);

	var node = $.sibling(div_1, 2);

	{
		var consequent = ($$anchor) => {
			var fragment = root_1();
			var svg = $.first_child(fragment);
			var path = $.sibling($.child(svg));

			$.reset(svg);

			var div_2 = $.sibling(svg, 2);
			var span_1 = $.child(div_2);
			var text_2 = $.child(span_1, true);

			$.reset(span_1);

			var span_2 = $.sibling(span_1);
			var text_3 = $.child(span_2, true);

			$.reset(span_2);
			$.reset(div_2);

			$.template_effect(
				($0, $1) => {
					$.set_attribute(svg, 'aria-label', (
						$.deep_read_state(title()),
						$.deep_read_state(samples()),
						$.untrack(() => `${title()}の履歴。${samples().length}サンプル`)
					));

					$.set_attribute(path, 'd', ($.get(data), $.untrack(() => $.get(data).path)));
					$.set_text(text_2, $0);
					$.set_text(text_3, $1);
				},
				[
					() => (
						$.deep_read_state(time),
						$.deep_read_state(samples()),
						$.untrack(() => time(samples()[0]?.time))
					),

					() => (
						$.deep_read_state(time),
						$.deep_read_state(samples()),
						$.untrack(() => time(samples().at(-1)?.time))
					)
				]
			);

			$.append($$anchor, fragment);
		};

		var alternate = ($$anchor) => {
			var div_3 = root_2();

			$.append($$anchor, div_3);
		};

		$.if(node, ($$render) => {
			if (($.get(data), $.untrack(() => $.get(data).path))) $$render(consequent); else $$render(alternate, false);
		});
	}

	$.reset(div);

	$.template_effect(
		($0) => {
			classes = $.set_class(div, 1, 'chart-block', null, classes, { small: small() });
			$.set_text(text, title());
			$.set_text(text_1, $0);
		},
		[
			() => (
				$.deep_read_state(finite),
				$.get(value),
				$.deep_read_state(unit()),
				$.deep_read_state(formatMS),
				$.untrack(() => finite($.get(value))
					? unit() === 'ms'
						? formatMS($.get(value))
						: $.get(value).toLocaleString('ja-JP') + (unit() ? ' ' + unit() : '')
					: '—')
			)
		]
	);

	$.append($$anchor, div);
	$.pop();
}