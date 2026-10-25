import '../vendor/svelte_internal_disclose-version.js';
import '../vendor/svelte_internal_flags_legacy.js';
import * as $ from '../vendor/svelte_internal_client.js';
import { config, auth, request, notify } from '../state.js';
import { plain, copy, download, csv, sessionGet, sessionSet } from '../lib.js';
import Icon from '../components/Icon.js';

var root_1 = $.from_html(`<div class="notice"><!><div><strong>CQLコンソールは無効です</strong><p>有効化する場合は、DBの読み取り専用ユーザーを設定し、Scope設定に <code>console.enabled: true</code> を追加してください。編集欄で下書きはできます。</p></div></div>`);
var root_2 = $.from_html(`<option> </option>`);
var root_3 = $.from_html(`<button>キャンセル</button>`);
var root_4 = $.from_html(`<pre class="notice error-text" role="alert"> </pre>`);
var root_6 = $.from_html(`<p class="notice warning">表示上限で切り詰めています。条件を絞って再実行してください。</p>`);
var root_7 = $.from_html(`<th> </th>`);
var root_9 = $.from_html(`<td><span class="cell-value"> </span></td>`);
var root_8 = $.from_html(`<tr></tr>`);
var root_5 = $.from_html(`<section class="panel"><header class="panel-header"><h2>結果 <span class="muted-badge"> </span></h2><div class="button-group"><button><!>CSV</button><button>JSON</button></div></header><!> <div class="table-scroll query-results" tabindex="0" role="region" aria-label="CQL実行結果"><table><thead><tr></tr></thead><tbody></tbody></table></div></section>`);
var root_10 = $.from_html(`<div class="empty compact-empty"><!><p>実行結果はここに表示されます。監視の自動更新で入力欄や結果を作り直しません。</p></div>`);
var root = $.from_html(`<!> <section class="panel"><header class="panel-header"><h2><!> Read-only CQL</h2><select aria-label="CQLの例"><option>クエリ例を選択</option><!></select></header><div class="query-editor"><textarea aria-label="CQLエディタ" spellcheck="false"></textarea></div><div class="toolbar"><button class="primary"><!> </button><!><span class="hint"> </span><label class="check-field"><input type="checkbox"/>このタブで下書きを保存</label></div></section> <!> <!>`, 1);

export default function Query($$anchor, $$props) {
	$.push($$props, false);

	const $config = () => $.store_get(config, '$config', $$stores);
	const $auth = () => $.store_get(auth, '$auth', $$stores);
	const [$$stores, $$cleanup] = $.setup_stores();

	let // Query text is kept in memory across navigation. Persisting it is opt-in.
	remember = $.mutable_source(sessionGet('query.remember', false));

	let sql = $.mutable_source($.get(remember)
		? sessionGet('query.draft', 'SELECT * FROM system.cluster_status;')
		: 'SELECT * FROM system.cluster_status;');

	let result = $.mutable_source(null);
	let busy = $.mutable_source(false);
	let error = $.mutable_source('');
	let controller = $.mutable_source();
	let template = $.mutable_source('');

	const examples = [
		['cluster', 'クラスタ状態', 'SELECT * FROM system.cluster_status;'],
		['raft', 'Raftメンバー', 'SELECT * FROM system.raft_state;'],
		[
			'keyspaces',
			'Keyspaces',
			'SELECT * FROM system_schema.keyspaces;'
		],
		['tables', 'Tables', 'SELECT * FROM system_schema.tables;']
	];

	async function run() {
		if ($.get(busy)) return;

		$.set(busy, true);
		$.set(error, '');
		$.set(controller, new AbortController());

		try {
			$.set(result, await request('query', {
				method: 'POST',
				body: JSON.stringify({ cql: $.get(sql) }),
				signal: $.get(controller).signal
			}));
		} catch(e) {
			$.set(error, e.name === 'AbortError' ? 'クエリをキャンセルしました' : e.message);
		} finally {
			$.set(busy, false);
			$.set(controller, null);
		}
	}

	function example() {
		const v = examples.find((e) => e[0] === $.get(template));

		if (v) $.set(sql, v[2]);

		$.set(template, '');
	}

	$.legacy_pre_effect(() => (sessionSet, $.get(remember)), () => {
		sessionSet('query.remember', $.get(remember));
	});

	$.legacy_pre_effect(() => ($.get(remember), sessionSet, $.get(sql)), () => {
		if ($.get(remember)) sessionSet('query.draft', $.get(sql)); else sessionSet('query.draft', null);
	});

	$.legacy_pre_effect_reset();
	$.init();

	var fragment = root();
	var node = $.first_child(fragment);

	{
		var consequent = ($$anchor) => {
			var div = root_1();
			var node_1 = $.child(div);

			Icon(node_1, { name: 'shield', size: 18 });
			$.next();
			$.reset(div);
			$.append($$anchor, div);
		};

		$.if(node, ($$render) => {
			if (($config(), $.untrack(() => !$config()?.console_enabled))) $$render(consequent);
		});
	}

	var section = $.sibling(node, 2);
	var header = $.child(section);
	var h2 = $.child(header);
	var node_2 = $.child(h2);

	Icon(node_2, { name: 'query', size: 17 });
	$.next();
	$.reset(h2);

	var select = $.sibling(h2);

	$.template_effect(() => {
		$.get(template);

		$.invalidate_inner_signals(() => {
			example;
			examples;
		});
	});

	select.__change = example;

	var option = $.child(select);

	option.value = option.__value = '';

	var node_3 = $.sibling(option);

	$.each(node_3, 1, () => examples, $.index, ($$anchor, $$item) => {
		var $$array = $.derived(() => $.to_array($.get($$item), 2));
		let id = () => $.get($$array)[0];
		let label = () => $.get($$array)[1];
		var option_1 = root_2();
		var text = $.child(option_1, true);

		$.reset(option_1);

		var option_1_value = {};

		$.template_effect(() => {
			$.set_text(text, label());

			if (option_1_value !== (option_1_value = id())) {
				option_1.value = (option_1.__value = id()) ?? '';
			}
		});

		$.append($$anchor, option_1);
	});

	$.reset(select);
	$.reset(header);

	var div_1 = $.sibling(header);
	var textarea = $.child(div_1);

	$.remove_textarea_child(textarea);

	textarea.__keydown = (e) => {
		if ((e.ctrlKey || e.metaKey) && e.key === 'Enter' && $config()?.console_enabled && $auth()?.authenticated) {
			e.preventDefault();
			run();
		}
	};

	$.reset(div_1);

	var div_2 = $.sibling(div_1);
	var button = $.child(div_2);

	button.__click = run;

	var node_4 = $.child(button);

	Icon(node_4, { name: 'play', size: 14 });

	var text_1 = $.sibling(node_4, 1, true);

	$.reset(button);

	var node_5 = $.sibling(button);

	{
		var consequent_1 = ($$anchor) => {
			var button_1 = root_3();

			button_1.__click = () => $.get(controller)?.abort();
			$.append($$anchor, button_1);
		};

		$.if(node_5, ($$render) => {
			if ($.get(busy)) $$render(consequent_1);
		});
	}

	var span = $.sibling(node_5);
	var text_2 = $.child(span);

	$.reset(span);

	var label_1 = $.sibling(span);
	var input = $.child(label_1);

	$.remove_input_defaults(input);
	$.next();
	$.reset(label_1);
	$.reset(div_2);
	$.reset(section);

	var node_6 = $.sibling(section, 2);

	{
		var consequent_2 = ($$anchor) => {
			var pre = root_4();
			var text_3 = $.child(pre, true);

			$.reset(pre);
			$.template_effect(() => $.set_text(text_3, $.get(error)));
			$.append($$anchor, pre);
		};

		$.if(node_6, ($$render) => {
			if ($.get(error)) $$render(consequent_2);
		});
	}

	var node_7 = $.sibling(node_6, 2);

	{
		var consequent_4 = ($$anchor) => {
			var section_1 = root_5();
			var header_1 = $.child(section_1);
			var h2_1 = $.child(header_1);
			var span_1 = $.sibling($.child(h2_1));
			var text_4 = $.child(span_1);

			$.reset(span_1);
			$.reset(h2_1);

			var div_3 = $.sibling(h2_1);
			var button_2 = $.child(div_3);

			button_2.__click = () => download('scyllascope-query.csv', csv($.get(result).rows, $.get(result).columns), 'text/csv;charset=utf-8');

			var node_8 = $.child(button_2);

			Icon(node_8, { name: 'download', size: 14 });
			$.next();
			$.reset(button_2);

			var button_3 = $.sibling(button_2);

			button_3.__click = () => download('scyllascope-query.json', JSON.stringify($.get(result), null, 2));
			$.reset(div_3);
			$.reset(header_1);

			var node_9 = $.sibling(header_1);

			{
				var consequent_3 = ($$anchor) => {
					var p = root_6();

					$.append($$anchor, p);
				};

				$.if(node_9, ($$render) => {
					if (($.get(result), $.untrack(() => $.get(result).truncated))) $$render(consequent_3);
				});
			}

			var div_4 = $.sibling(node_9, 2);
			var table = $.child(div_4);
			var thead = $.child(table);
			var tr = $.child(thead);

			$.each(tr, 5, () => ($.get(result), $.untrack(() => $.get(result).columns)), $.index, ($$anchor, col) => {
				var th = root_7();
				var text_5 = $.child(th, true);

				$.reset(th);
				$.template_effect(() => $.set_text(text_5, $.get(col)));
				$.append($$anchor, th);
			});

			$.reset(tr);
			$.reset(thead);

			var tbody = $.sibling(thead);

			$.each(tbody, 5, () => ($.get(result), $.untrack(() => $.get(result).rows)), $.index, ($$anchor, row) => {
				var tr_1 = root_8();

				$.each(tr_1, 5, () => ($.get(result), $.untrack(() => $.get(result).columns)), $.index, ($$anchor, col) => {
					var td = root_9();
					var span_2 = $.child(td);
					var text_6 = $.child(span_2, true);

					$.reset(span_2);
					$.reset(td);

					$.template_effect(
						($0, $1) => {
							$.set_attribute(span_2, 'title', $0);
							$.set_text(text_6, $1);
						},
						[
							() => (
								$.deep_read_state(plain),
								$.get(row),
								$.get(col),
								$.untrack(() => plain($.get(row)[$.get(col)]))
							),

							() => (
								$.deep_read_state(plain),
								$.get(row),
								$.get(col),
								$.untrack(() => plain($.get(row)[$.get(col)]))
							)
						]
					);

					$.append($$anchor, td);
				});

				$.reset(tr_1);
				$.append($$anchor, tr_1);
			});

			$.reset(tbody);
			$.reset(table);
			$.reset(div_4);
			$.reset(section_1);
			$.template_effect(() => $.set_text(text_4, `${($.get(result), $.untrack(() => $.get(result).count)) ?? ''} rows · ${($.get(result), $.untrack(() => $.get(result).took_ms)) ?? ''} ms`));
			$.append($$anchor, section_1);
		};

		var alternate = ($$anchor) => {
			var div_5 = root_10();
			var node_10 = $.child(div_5);

			Icon(node_10, { name: 'query', size: 28 });
			$.next();
			$.reset(div_5);
			$.append($$anchor, div_5);
		};

		$.if(node_7, ($$render) => {
			if ($.get(result)) $$render(consequent_4); else $$render(alternate, false);
		});
	}

	$.template_effect(() => {
		button.disabled = (
			$.get(busy),
			$config(),
			$auth(),
			$.untrack(() => $.get(busy) || !$config()?.console_enabled || !$auth()?.authenticated)
		);

		$.set_text(text_1, $.get(busy) ? '実行中…' : 'クエリを実行');
		$.set_text(text_2, `Ctrl / ⌘ + Enter · SELECTのみ · 最大${($config(), $.untrack(() => $config()?.max_rows || 200)) ?? ''}行`);
	});

	$.bind_select_value(select, () => $.get(template), ($$value) => $.set(template, $$value));
	$.bind_value(textarea, () => $.get(sql), ($$value) => $.set(sql, $$value));
	$.bind_checked(input, () => $.get(remember), ($$value) => $.set(remember, $$value));
	$.append($$anchor, fragment);
	$.pop();
	$$cleanup();
}

$.delegate(['change', 'keydown', 'click']);