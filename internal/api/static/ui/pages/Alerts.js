import '../vendor/svelte_internal_disclose-version.js';
import '../vendor/svelte_internal_flags_legacy.js';
import * as $ from '../vendor/svelte_internal_client.js';
import { onMount } from '../vendor/svelte_svelte.js';

import {
	incidents,
	errors,
	loadResource,
	actIncident,
	notify,
	auth,
	clock,
	snapshot
} from '../state.js';

import Badge from '../components/Badge.js';
import Icon from '../components/Icon.js';
import { date, sessionGet, sessionSet } from '../lib.js';

var root_1 = $.from_html(`<p class="notice warning"> </p>`);
var root_3 = $.from_html(`<span class="error-text"> </span>`);
var root_5 = $.from_html(`<button>通知を再開</button>`);
var root_4 = $.from_html(`<div class="button-group"><button>確認済みにする</button><button>通知を1時間停止</button><!></div>`);
var root_2 = $.from_html(`<article class="incident"><div class="incident-top"><!><span class="quiet"> </span><time> </time></div><h3> </h3><div class="incident-bottom"><div class="quiet"> <!></div><!></div></article>`);
var root_6 = $.from_html(`<div class="empty compact-empty"><!><h3>該当するアラートはありません</h3><p>確認済み・通知停止は、障害の復旧とは別に管理します。</p></div>`);
var root = $.from_html(`<section class="panel"><div class="toolbar"><div class="tabs"><button>アクティブ</button><button>復旧済み</button><button>すべて</button></div><label class="search"><!><input placeholder="ノード・内容で検索" aria-label="アラート検索"/></label><button><!>履歴を更新</button></div> <!> <div class="incident-list"></div></section> <p class="hint">この観測ノードの履歴です。複数ノードが通知する構成では観測元を併記します。別ノードの通知停止や全体共通の確認済み状態としては同期しません。</p>`, 1);

export default function Alerts($$anchor, $$props) {
	$.push($$props, false);

	const $incidents = () => $.store_get(incidents, '$incidents', $$stores);
	const $errors = () => $.store_get(errors, '$errors', $$stores);
	const $clock = () => $.store_get(clock, '$clock', $$stores);
	const $auth = () => $.store_get(auth, '$auth', $$stores);
	const [$$stores, $$cleanup] = $.setup_stores();
	const shown = $.mutable_source();
	let filter = $.mutable_source(sessionGet('alerts.filter', 'active'));
	let search = $.mutable_source(sessionGet('alerts.search', ''));
	let busy = $.mutable_source('');

	onMount(() => loadResource('incidents'));

	async function action(i, a, m = 60) {
		$.set(busy, i.id);

		try {
			await actIncident(i.id, a, m);
		} catch(e) {
			notify(e.message, 'error');
		} finally {
			$.set(busy, '');
		}
	}

	$.legacy_pre_effect(() => (sessionSet, $.get(filter)), () => {
		sessionSet('alerts.filter', $.get(filter));
	});

	$.legacy_pre_effect(() => (sessionSet, $.get(search)), () => {
		sessionSet('alerts.search', $.get(search));
	});

	$.legacy_pre_effect(() => ($incidents(), $.get(filter), $.get(search)), () => {
		$.set(shown, $incidents().filter((i) => ($.get(filter) === 'all' || ($.get(filter) === 'resolved' ? !!i.resolved_at : !i.resolved_at)) && (i.message + ' ' + i.node).toLowerCase().includes($.get(search).toLowerCase())));
	});

	$.legacy_pre_effect_reset();
	$.init();

	var fragment = root();
	var section = $.first_child(fragment);
	var div = $.child(section);
	var div_1 = $.child(div);
	var button = $.child(div_1);

	button.__click = () => $.set(filter, 'active');

	let classes;
	var button_1 = $.sibling(button);

	button_1.__click = () => {
		$.set(filter, 'resolved');
		loadResource('incidents', true);
	};

	let classes_1;
	var button_2 = $.sibling(button_1);

	button_2.__click = () => {
		$.set(filter, 'all');
		loadResource('incidents', true);
	};

	let classes_2;

	$.reset(div_1);

	var label = $.sibling(div_1);
	var node = $.child(label);

	Icon(node, { name: 'search', size: 16 });

	var input = $.sibling(node);

	$.remove_input_defaults(input);
	$.reset(label);

	var button_3 = $.sibling(label);

	button_3.__click = () => loadResource('incidents', true);

	var node_1 = $.child(button_3);

	Icon(node_1, { name: 'refresh', size: 14 });
	$.next();
	$.reset(button_3);
	$.reset(div);

	var node_2 = $.sibling(div, 2);

	{
		var consequent = ($$anchor) => {
			var p = root_1();
			var text = $.child(p, true);

			$.reset(p);
			$.template_effect(() => $.set_text(text, ($errors(), $.untrack(() => $errors().incidents))));
			$.append($$anchor, p);
		};

		$.if(node_2, ($$render) => {
			if (($errors(), $.untrack(() => $errors().incidents))) $$render(consequent);
		});
	}

	var div_2 = $.sibling(node_2, 2);

	$.each(
		div_2,
		5,
		() => $.get(shown),
		(i) => i.id,
		($$anchor, i) => {
			var article = root_2();
			var div_3 = $.child(article);
			var node_3 = $.child(div_3);

			{
				let $0 = $.derived_safe_equal(() => (
					$.get(i),
					$.untrack(() => $.get(i).resolved_at ? 'healthy' : $.get(i).severity)
				));

				let $1 = $.derived_safe_equal(() => (
					$.get(i),
					$.untrack(() => $.get(i).resolved_at ? '復旧済み' : '')
				));

				Badge(node_3, {
					get state() {
						return $.get($0);
					},

					get text() {
						return $.get($1);
					}
				});
			}

			var span = $.sibling(node_3);
			var text_1 = $.child(span);

			$.reset(span);

			var time = $.sibling(span);
			var text_2 = $.child(time, true);

			$.reset(time);
			$.reset(div_3);

			var h3 = $.sibling(div_3);
			var text_3 = $.child(h3, true);

			$.reset(h3);

			var div_4 = $.sibling(h3);
			var div_5 = $.child(div_4);
			var text_4 = $.child(div_5);
			var node_4 = $.sibling(text_4);

			{
				var consequent_1 = ($$anchor) => {
					var span_1 = root_3();
					var text_5 = $.child(span_1);

					$.reset(span_1);
					$.template_effect(() => $.set_text(text_5, `· ${($.get(i), $.untrack(() => $.get(i).delivery_error)) ?? ''}`));
					$.append($$anchor, span_1);
				};

				$.if(node_4, ($$render) => {
					if (($.get(i), $.untrack(() => $.get(i).delivery_error))) $$render(consequent_1);
				});
			}

			$.reset(div_5);

			var node_5 = $.sibling(div_5);

			{
				var consequent_3 = ($$anchor) => {
					var div_6 = root_4();
					var button_4 = $.child(div_6);

					button_4.__click = () => action($.get(i), 'ack');

					var button_5 = $.sibling(button_4);

					button_5.__click = () => action($.get(i), 'snooze', 60);

					var node_6 = $.sibling(button_5);

					{
						var consequent_2 = ($$anchor) => {
							var button_6 = root_5();

							button_6.__click = () => action($.get(i), 'resume');

							$.template_effect(() => button_6.disabled = (
								$auth(),
								$.get(busy),
								$.get(i),
								$.untrack(() => !$auth()?.authenticated || $.get(busy) === $.get(i).id)
							));

							$.append($$anchor, button_6);
						};

						$.if(node_6, ($$render) => {
							if ((
								$.get(i),
								$.untrack(() => $.get(i).acknowledged || $.get(i).silenced_until)
							)) $$render(consequent_2);
						});
					}

					$.reset(div_6);

					$.template_effect(() => {
						button_4.disabled = (
							$auth(),
							$.get(busy),
							$.get(i),
							$.untrack(() => !$auth()?.authenticated || $.get(busy) === $.get(i).id)
						);

						button_5.disabled = (
							$auth(),
							$.get(busy),
							$.get(i),
							$.untrack(() => !$auth()?.authenticated || $.get(busy) === $.get(i).id)
						);
					});

					$.append($$anchor, div_6);
				};

				$.if(node_5, ($$render) => {
					if (($.get(i), $.untrack(() => !$.get(i).resolved_at))) $$render(consequent_3);
				});
			}

			$.reset(div_4);
			$.reset(article);

			$.template_effect(
				($0, $1) => {
					$.set_text(text_1, `観測元: ${($.get(i), $.untrack(() => $.get(i).observer)) ?? ''}`);
					$.set_text(text_2, $0);
					$.set_text(text_3, ($.get(i), $.untrack(() => $.get(i).message)));

					$.set_text(text_4, `${(
						$.get(i),
						$.untrack(() => $.get(i).acknowledged ? '確認済み · ' : '')
					) ?? ''}${$1 ?? ''}`);
				},
				[
					() => (
						$.deep_read_state(date),
						$.get(i),
						$.untrack(() => date($.get(i).opened_at))
					),

					() => (
						$.get(i),
						$clock(),
						$.deep_read_state(date),
						$.untrack(() => $.get(i).silenced_until && Date.parse($.get(i).silenced_until) > $clock()
							? '通知停止中（' + date($.get(i).silenced_until) + 'まで）'
							: $.get(i).last_notified_at ? '最終通知 ' + date($.get(i).last_notified_at) : '通知未送信')
					)
				]
			);

			$.append($$anchor, article);
		},
		($$anchor) => {
			var div_7 = root_6();
			var node_7 = $.child(div_7);

			Icon(node_7, { name: 'check', size: 28 });
			$.next(2);
			$.reset(div_7);
			$.append($$anchor, div_7);
		}
	);

	$.reset(div_2);
	$.reset(section);
	$.next(2);

	$.template_effect(() => {
		classes = $.set_class(button, 1, '', null, classes, { active: $.get(filter) === 'active' });
		classes_1 = $.set_class(button_1, 1, '', null, classes_1, { active: $.get(filter) === 'resolved' });
		classes_2 = $.set_class(button_2, 1, '', null, classes_2, { active: $.get(filter) === 'all' });
	});

	$.bind_value(input, () => $.get(search), ($$value) => $.set(search, $$value));
	$.append($$anchor, fragment);
	$.pop();
	$$cleanup();
}

$.delegate(['click']);