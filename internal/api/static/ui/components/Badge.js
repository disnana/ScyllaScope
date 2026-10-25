import '../vendor/svelte_internal_disclose-version.js';
import '../vendor/svelte_internal_flags_legacy.js';
import * as $ from '../vendor/svelte_internal_client.js';
import { labels } from '../lib.js';

var root_1 = $.from_html(`<span class="dot"></span>`);
var root = $.from_html(`<span><!> </span>`);

export default function Badge($$anchor, $$props) {
	$.push($$props, false);

	let state = $.prop($$props, 'state', 8, 'unknown');
	let text = $.prop($$props, 'text', 8, '');
	let dot = $.prop($$props, 'dot', 8, true);

	$.init();

	var span = root();
	var node = $.child(span);

	{
		var consequent = ($$anchor) => {
			var span_1 = root_1();

			$.append($$anchor, span_1);
		};

		$.if(node, ($$render) => {
			if (dot()) $$render(consequent);
		});
	}

	var text_1 = $.sibling(node, 1, true);

	$.reset(span);

	$.template_effect(() => {
		$.set_class(span, 1, `badge ${state() ?? ''}`);

		$.set_text(text_1, (
			$.deep_read_state(text()),
			$.deep_read_state(labels),
			$.deep_read_state(state()),
			$.untrack(() => text() || labels[state()] || state())
		));
	});

	$.append($$anchor, span);
	$.pop();
}