import '../vendor/svelte_internal_disclose-version.js';
import '../vendor/svelte_internal_flags_legacy.js';
import * as $ from '../vendor/svelte_internal_client.js';
import { clock } from '../state.js';
import { checkState, latency, labels } from '../lib.js';

var root_1 = $.from_html(`<span class="mono quiet"> </span>`);
var root = $.from_html(`<span><span class="dot"></span> <!></span>`);

export default function Check($$anchor, $$props) {
	$.push($$props, false);

	const $clock = () => $.store_get(clock, '$clock', $$stores);
	const [$$stores, $$cleanup] = $.setup_stores();
	const state = $.mutable_source();
	let value = $.prop($$props, 'value', 8);
	let ttl = $.prop($$props, 'ttl', 8, 30);
	let caption = $.prop($$props, 'caption', 8, '');
	let timings = $.prop($$props, 'timings', 8, false);

	$.legacy_pre_effect(
		() => (
			checkState,
			$.deep_read_state(value()),
			$clock(),
			$.deep_read_state(ttl())
		),
		() => {
			$.set(state, checkState(value(), $clock(), ttl()));
		}
	);

	$.legacy_pre_effect_reset();
	$.init();

	var span = root();
	var text = $.sibling($.child(span), 1, true);
	var node = $.sibling(text);

	{
		var consequent = ($$anchor) => {
			var span_1 = root_1();
			var text_1 = $.child(span_1, true);

			$.reset(span_1);

			$.template_effect(($0) => $.set_text(text_1, $0), [
				() => (
					$.deep_read_state(latency),
					$.deep_read_state(value()),
					$clock(),
					$.deep_read_state(ttl()),
					$.untrack(() => latency(value(), $clock(), ttl()))
				)
			]);

			$.append($$anchor, span_1);
		};

		$.if(node, ($$render) => {
			if (timings()) $$render(consequent);
		});
	}

	$.reset(span);

	$.template_effect(() => {
		$.set_class(span, 1, `check ${$.get(state) ?? ''}`);

		$.set_attribute(span, 'title', (
			$.deep_read_state(value()),
			$.deep_read_state(labels),
			$.get(state),
			$.untrack(() => value()?.message || labels[$.get(state)])
		));

		$.set_text(text, (
			$.deep_read_state(caption()),
			$.deep_read_state(labels),
			$.get(state),
			$.untrack(() => caption() || labels[$.get(state)])
		));
	});

	$.append($$anchor, span);
	$.pop();
	$$cleanup();
}