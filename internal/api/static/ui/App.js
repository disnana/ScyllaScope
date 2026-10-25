import './vendor/svelte_internal_disclose-version.js';
import './vendor/svelte_internal_flags_legacy.js';
import * as $ from './vendor/svelte_internal_client.js';
import { onMount, tick } from './vendor/svelte_svelte.js';

import {
	snapshot,
	clock,
	auth,
	config,
	route,
	connection,
	preferences,
	paused,
	toast,
	errors,
	selected,
	boot,
	dispose,
	setupHash,
	navigate,
	refresh,
	login,
	logout,
	notify
} from './state.js';

import { routes, titles, clusterState, age, sessionGet, sessionSet } from './lib.js';
import Icon from './components/Icon.js';
import Badge from './components/Badge.js';
import Drawer from './components/Drawer.js';
import Dashboard from './pages/Dashboard.js';
import Nodes from './pages/Nodes.js';
import Connections from './pages/Connections.js';
import Metrics from './pages/Metrics.js';
import Alerts from './pages/Alerts.js';
import Events from './pages/Events.js';
import Query from './pages/Query.js';
import Settings from './pages/Settings.js';

var root_2 = $.from_html(`<p class="error-text"> </p><button>再試行</button>`, 1);
var root_1 = $.from_html(`<main class="login-page"><section class="login-card"><h1>ScyllaScope</h1><p>接続を確認しています…</p><!></section></main>`);
var root_5 = $.from_html(`<p class="error-text" role="alert"> </p>`);
var root_4 = $.from_html(`<main class="login-page"><form class="login-card"><div class="brand"><span class="brand-icon"><!></span><span><strong>ScyllaScope</strong><small>Distributed cluster monitor</small></span></div><h1>管理画面にログイン</h1><p class="quiet">このノードのUIトークンを入力してください。</p><input type="hidden" name="username" value="ScyllaScope" autocomplete="username"/><label>UIトークン<input type="password" required minlength="32" autocomplete="current-password" placeholder="/etc/scyllascope/ui-token"/></label><button class="primary"> </button><!><details><summary>トークンの確認方法</summary><pre>sudo cat /etc/scyllascope/ui-token</pre><p class="hint">ノード間共有のcluster_tokenとは別です。URLや診断ログには含めません。</p></details></form></main>`);
var root_7 = $.from_html(`<button class="nav-scrim" aria-label="メニューを閉じる"></button>`);
var root_9 = $.from_html(`<span class="nav-count"> </span>`);
var root_8 = $.from_html(`<a><!><span> </span><!></a>`);
var root_10 = $.from_html(`<button class="icon-button" aria-label="ログアウト"><!></button>`);
var root_12 = $.from_html(`<span class="badge warning">DEMO DATA</span>`);
var root_13 = $.from_html(`<div class="notice warning" role="alert"> <button>再試行</button></div>`);
var root_14 = $.from_html(`<p class="stale-banner" role="status"><!>最新情報を受信できていません。最後の観測を表示しています。</p>`);
var root_15 = $.from_html(`<div class="read-only-line"><!>読取専用 · 操作する場合はUIトークンを設定してください</div>`);
var root_16 = $.from_html(`<div class="page" data-page="nodes"><!></div>`);
var root_17 = $.from_html(`<div class="page" data-page="topology"><!></div>`);
var root_18 = $.from_html(`<div class="page" data-page="metrics"><!></div>`);
var root_19 = $.from_html(`<div class="page" data-page="alerts"><!></div>`);
var root_20 = $.from_html(`<div class="page" data-page="events"><!></div>`);
var root_21 = $.from_html(`<div class="page" data-page="query"><!></div>`);
var root_22 = $.from_html(`<div class="page" data-page="settings"><!></div>`);
var root_6 = $.from_html(`<div class="shell"><a href="#main" class="skip">メインへ移動</a> <!> <aside><a href="#dashboard" class="brand"><span class="brand-icon"><!></span><span><strong>ScyllaScope</strong><small>CLUSTER MONITOR</small></span></a><div class="workspace"><span></span><span><strong> </strong><small>分散ノード監視</small></span></div><nav aria-label="メインナビゲーション"></nav><div class="sidebar-bottom"><!><span>v0.6.2 · Svelte SPA</span><button class="link-button">設定・診断 <!></button></div></aside> <div class="workspace-main"><header class="topbar"><div class="breadcrumb"><button class="icon-button mobile-menu" aria-label="メニューを開く"><!></button><span> </span><span class="divider">/</span><strong> </strong></div><div class="top-actions"><span><span></span> </span><button class="icon-button" title="表示のみ停止。サーバー監視・通知は継続"><!></button><button class="icon-button" aria-label="テーマ切替"><!></button><!></div></header> <main id="main" class="main-scroll" tabindex="-1"><div class="content"><header class="page-header"><div><div class="eyebrow"> </div><div class="title-row"><h1> </h1><!><!></div><p> <span class="mono"> </span></p></div><div class="page-actions"><span class="quiet last-update"> </span><button><!> </button></div></header> <!> <!> <!> <div class="page" data-page="dashboard"><!></div> <!> <!> <!> <!> <!> <!> <!></div></main></div></div> <!>`, 1);
var root_23 = $.from_html(`<div role="status"><!><span> </span><button class="icon-button" aria-label="通知を閉じる"><!></button></div>`);
var root = $.from_html(`<!> <!>`, 1);

export default function App($$anchor, $$props) {
	$.push($$props, false);

	const $route = () => $.store_get(route, '$route', $$stores);
	const $auth = () => $.store_get(auth, '$auth', $$stores);
	const $errors = () => $.store_get(errors, '$errors', $$stores);
	const $snapshot = () => $.store_get(snapshot, '$snapshot', $$stores);
	const $clock = () => $.store_get(clock, '$clock', $$stores);
	const $connection = () => $.store_get(connection, '$connection', $$stores);
	const $paused = () => $.store_get(paused, '$paused', $$stores);
	const $preferences = () => $.store_get(preferences, '$preferences', $$stores);
	const $toast = () => $.store_get(toast, '$toast', $$stores);
	const [$$stores, $$cleanup] = $.setup_stores();

	const icons = {
		dashboard: 'grid',
		nodes: 'nodes',
		topology: 'topology',
		metrics: 'metrics',
		alerts: 'alerts',
		events: 'events',
		query: 'query',
		settings: 'settings'
	};

	let visited = $.mutable_source(new Set(['dashboard', $route()]));
	let main = $.mutable_source();
	let previous = $.mutable_source($route());
	let scrolls = sessionGet('scrolls', {});
	let mobile = $.mutable_source(false);
	let busy = $.mutable_source(false);
	let token = $.mutable_source('');
	let logging = $.mutable_source(false);
	let loginError = $.mutable_source('');
	let mounted = $.mutable_source(false);
	let restoring = false;

	async function restoreScroll(id) {
		restoring = true;
		await tick();

		if ($.get(main)) $.mutate(main, $.get(main).scrollTop = scrolls[id] || 0);

		await new Promise(requestAnimationFrame);
		restoring = false;
	}

	function saveScroll() {
		if ($.get(main) && !restoring) {
			scrolls[$.get(previous)] = $.get(main).scrollTop;
			sessionSet('scrolls', scrolls);
		}
	}

	async function update() {
		$.set(busy, true);

		try {
			await refresh();
		} catch(e) {
			notify(e.message, 'error');
		} finally {
			$.set(busy, false);
		}
	}

	async function enter() {
		$.set(logging, true);
		$.set(loginError, '');

		try {
			await login($.get(token));
			$.set(token, '');
		} catch(e) {
			$.set(loginError, e.message);
		} finally {
			$.set(logging, false);
		}
	}

	async function leave() {
		try {
			await logout();
		} catch(e) {
			notify(e.message, 'error');
		}
	}

	function keydown(e) {
		if (e.altKey && !e.ctrlKey && !e.metaKey && (/^[1-8]$/).test(e.key)) {
			e.preventDefault();
			navigate(routes[Number(e.key) - 1]);
		}

		if (e.key === 'Escape') $.set(mobile, false);
	}

	onMount(() => {
		const cleanup = setupHash();

		$.set(mounted, true);
		boot();

		return () => {
			cleanup();
			dispose();
		};
	});

	$.legacy_pre_effect(() => ($.get(visited), $route()), () => {
		if (!$.get(visited).has($route())) $.set(visited, new Set([...$.get(visited), $route()]));
	});

	$.legacy_pre_effect(() => ($.get(mounted), $route(), $.get(previous)), () => {
		if ($.get(mounted) && $route() !== $.get(previous)) {
			const next = $route();

			$.set(previous, next);
			$.set(mobile, false);
			restoreScroll(next);
		}
	});

	$.legacy_pre_effect_reset();
	$.init();

	var fragment = root();

	$.event('keydown', $.window, keydown);

	var node = $.first_child(fragment);

	{
		var consequent_1 = ($$anchor) => {
			var main_1 = root_1();
			var section = $.child(main_1);
			var node_1 = $.sibling($.child(section), 2);

			{
				var consequent = ($$anchor) => {
					var fragment_1 = root_2();
					var p_1 = $.first_child(fragment_1);
					var text = $.child(p_1, true);

					$.reset(p_1);

					var button = $.sibling(p_1);

					button.__click = function (...$$args) {
						boot?.apply(this, $$args);
					};

					$.template_effect(() => $.set_text(text, ($errors(), $.untrack(() => $errors().boot))));
					$.append($$anchor, fragment_1);
				};

				$.if(node_1, ($$render) => {
					if (($errors(), $.untrack(() => $errors().boot))) $$render(consequent);
				});
			}

			$.reset(section);
			$.reset(main_1);
			$.append($$anchor, main_1);
		};

		var alternate_1 = ($$anchor) => {
			var fragment_2 = $.comment();
			var node_2 = $.first_child(fragment_2);

			{
				var consequent_3 = ($$anchor) => {
					var main_2 = root_4();
					var form = $.child(main_2);
					var div = $.child(form);
					var span = $.child(div);
					var node_3 = $.child(span);

					Icon(node_3, { name: 'box', size: 26 });
					$.reset(span);
					$.next();
					$.reset(div);

					var input = $.sibling(div, 3);
					var label = $.sibling(input);
					var input_1 = $.sibling($.child(label));

					$.remove_input_defaults(input_1);
					$.reset(label);

					var button_1 = $.sibling(label);
					var text_1 = $.child(button_1, true);

					$.reset(button_1);

					var node_4 = $.sibling(button_1);

					{
						var consequent_2 = ($$anchor) => {
							var p_2 = root_5();
							var text_2 = $.child(p_2, true);

							$.reset(p_2);
							$.template_effect(() => $.set_text(text_2, $.get(loginError)));
							$.append($$anchor, p_2);
						};

						$.if(node_4, ($$render) => {
							if ($.get(loginError)) $$render(consequent_2);
						});
					}

					$.next();
					$.reset(form);
					$.reset(main_2);

					$.template_effect(() => {
						button_1.disabled = $.get(logging);
						$.set_text(text_1, $.get(logging) ? '認証中…' : 'ログイン');
					});

					$.event('submit', form, (e) => {
						e.preventDefault();
						enter();
					});

					$.bind_value(input_1, () => $.get(token), ($$value) => $.set(token, $$value));
					$.append($$anchor, main_2);
				};

				var alternate = ($$anchor) => {
					var fragment_3 = root_6();
					var div_1 = $.first_child(fragment_3);
					var a = $.child(div_1);

					a.__click = (e) => {
						e.preventDefault();
						$.get(main)?.focus();
					};

					var node_5 = $.sibling(a, 2);

					{
						var consequent_4 = ($$anchor) => {
							var button_2 = root_7();

							button_2.__click = () => $.set(mobile, false);
							$.append($$anchor, button_2);
						};

						$.if(node_5, ($$render) => {
							if ($.get(mobile)) $$render(consequent_4);
						});
					}

					var aside = $.sibling(node_5, 2);
					let classes;
					var a_1 = $.child(aside);
					var span_1 = $.child(a_1);
					var node_6 = $.child(span_1);

					Icon(node_6, { name: 'box', size: 23 });
					$.reset(span_1);
					$.next();
					$.reset(a_1);

					var div_2 = $.sibling(a_1);
					var span_2 = $.child(div_2);
					var span_3 = $.sibling(span_2);
					var strong = $.child(span_3);
					var text_3 = $.child(strong, true);

					$.reset(strong);
					$.next();
					$.reset(span_3);
					$.reset(div_2);

					var nav = $.sibling(div_2);

					$.each(nav, 5, () => routes, $.index, ($$anchor, id) => {
						var a_2 = root_8();
						let classes_1;
						var node_7 = $.child(a_2);

						Icon(node_7, {
							get name() {
								return ($.get(id), $.untrack(() => icons[$.get(id)]));
							},
							size: 17
						});

						var span_4 = $.sibling(node_7);
						var text_4 = $.child(span_4, true);

						$.reset(span_4);

						var node_8 = $.sibling(span_4);

						{
							var consequent_5 = ($$anchor) => {
								var span_5 = root_9();
								var text_5 = $.child(span_5, true);

								$.reset(span_5);

								$.template_effect(($0) => $.set_text(text_5, $0), [
									() => (
										$snapshot(),
										$.untrack(() => $snapshot().incidents.filter((i) => !i.resolved_at).length)
									)
								]);

								$.append($$anchor, span_5);
							};

							$.if(node_8, ($$render) => {
								if ((
									$.get(id),
									$snapshot(),
									$.untrack(() => $.get(id) === 'alerts' && ($snapshot()?.incidents || []).filter((i) => !i.resolved_at).length)
								)) $$render(consequent_5);
							});
						}

						$.reset(a_2);

						$.template_effect(() => {
							$.set_attribute(a_2, 'href', '#' + $.get(id));
							$.set_attribute(a_2, 'aria-current', $route() === $.get(id) ? 'page' : undefined);
							classes_1 = $.set_class(a_2, 1, '', null, classes_1, { active: $route() === $.get(id) });

							$.set_text(text_4, (
								$.deep_read_state(titles),
								$.get(id),
								$.untrack(() => titles[$.get(id)])
							));
						});

						$.append($$anchor, a_2);
					});

					$.reset(nav);

					var div_3 = $.sibling(nav);
					var node_9 = $.child(div_3);

					{
						let $0 = $.derived_safe_equal(() => (
							$.deep_read_state(clusterState),
							$snapshot(),
							$clock(),
							$.untrack(() => clusterState($snapshot(), $clock()))
						));

						Badge(node_9, {
							get state() {
								return $.get($0);
							}
						});
					}

					var button_3 = $.sibling(node_9, 2);

					button_3.__click = () => navigate('settings');

					var node_10 = $.sibling($.child(button_3));

					Icon(node_10, { name: 'chevron', size: 12 });
					$.reset(button_3);
					$.reset(div_3);
					$.reset(aside);

					var div_4 = $.sibling(aside, 2);
					var header = $.child(div_4);
					var div_5 = $.child(header);
					var button_4 = $.child(div_5);

					button_4.__click = () => $.set(mobile, true);

					var node_11 = $.child(button_4);

					Icon(node_11, { name: 'menu' });
					$.reset(button_4);

					var span_6 = $.sibling(button_4);
					var text_6 = $.child(span_6, true);

					$.reset(span_6);

					var strong_1 = $.sibling(span_6, 2);
					var text_7 = $.child(strong_1, true);

					$.reset(strong_1);
					$.reset(div_5);

					var div_6 = $.sibling(div_5);
					var span_7 = $.child(div_6);
					let classes_2;
					var span_8 = $.child(span_7);
					var text_8 = $.sibling(span_8, 1, true);

					$.reset(span_7);

					var button_5 = $.sibling(span_7);

					button_5.__click = () => paused.update((v) => !v);

					var node_12 = $.child(button_5);

					{
						let $0 = $.derived_safe_equal(() => $paused() ? 'play' : 'pause');

						Icon(node_12, {
							get name() {
								return $.get($0);
							},
							size: 16
						});
					}

					$.reset(button_5);

					var button_6 = $.sibling(button_5);

					button_6.__click = () => preferences.update((p) => ({ ...p, theme: p.theme === 'dark' ? 'light' : 'dark' }));

					var node_13 = $.child(button_6);

					{
						let $0 = $.derived_safe_equal(() => (
							$preferences(),
							$.untrack(() => $preferences().theme === 'dark' ? 'sun' : 'moon')
						));

						Icon(node_13, {
							get name() {
								return $.get($0);
							},
							size: 17
						});
					}

					$.reset(button_6);

					var node_14 = $.sibling(button_6);

					{
						var consequent_6 = ($$anchor) => {
							var button_7 = root_10();

							button_7.__click = leave;

							var node_15 = $.child(button_7);

							Icon(node_15, { name: 'logout', size: 17 });
							$.reset(button_7);
							$.append($$anchor, button_7);
						};

						$.if(node_14, ($$render) => {
							if (($auth(), $.untrack(() => $auth()?.authenticated))) $$render(consequent_6);
						});
					}

					$.reset(div_6);
					$.reset(header);

					var main_3 = $.sibling(header, 2);
					var div_7 = $.child(main_3);
					var header_1 = $.child(div_7);
					var div_8 = $.child(header_1);
					var div_9 = $.child(div_8);
					var text_9 = $.child(div_9, true);

					$.reset(div_9);

					var div_10 = $.sibling(div_9);
					var h1 = $.child(div_10);
					var text_10 = $.child(h1, true);

					$.reset(h1);

					var node_16 = $.sibling(h1);

					{
						var consequent_7 = ($$anchor) => {
							{
								let $0 = $.derived_safe_equal(() => (
									$.deep_read_state(clusterState),
									$snapshot(),
									$clock(),
									$.untrack(() => clusterState($snapshot(), $clock()))
								));

								Badge($$anchor, {
									get state() {
										return $.get($0);
									}
								});
							}
						};

						$.if(node_16, ($$render) => {
							if ($route() === 'dashboard') $$render(consequent_7);
						});
					}

					var node_17 = $.sibling(node_16);

					{
						var consequent_8 = ($$anchor) => {
							var span_9 = root_12();

							$.append($$anchor, span_9);
						};

						$.if(node_17, ($$render) => {
							if (($snapshot(), $.untrack(() => $snapshot()?.demo))) $$render(consequent_8);
						});
					}

					$.reset(div_10);

					var p_3 = $.sibling(div_10);
					var text_11 = $.child(p_3);
					var span_10 = $.sibling(text_11);
					var text_12 = $.child(span_10, true);

					$.reset(span_10);
					$.reset(p_3);
					$.reset(div_8);

					var div_11 = $.sibling(div_8);
					var span_11 = $.child(div_11);
					var text_13 = $.child(span_11, true);

					$.reset(span_11);

					var button_8 = $.sibling(span_11);

					button_8.__click = update;

					var node_18 = $.child(button_8);

					Icon(node_18, { name: 'refresh', size: 14 });

					var text_14 = $.sibling(node_18, 1, true);

					$.reset(button_8);
					$.reset(div_11);
					$.reset(header_1);

					var node_19 = $.sibling(header_1, 2);

					{
						var consequent_9 = ($$anchor) => {
							var div_12 = root_13();
							var text_15 = $.child(div_12, true);
							var button_9 = $.sibling(text_15);

							button_9.__click = function (...$$args) {
								boot?.apply(this, $$args);
							};

							$.reset(div_12);
							$.template_effect(() => $.set_text(text_15, ($errors(), $.untrack(() => $errors().boot))));
							$.append($$anchor, div_12);
						};

						$.if(node_19, ($$render) => {
							if (($errors(), $.untrack(() => $errors().boot))) $$render(consequent_9);
						});
					}

					var node_20 = $.sibling(node_19, 2);

					{
						var consequent_10 = ($$anchor) => {
							var p_4 = root_14();
							var node_21 = $.child(p_4);

							Icon(node_21, { name: 'info', size: 15 });
							$.next();
							$.reset(p_4);
							$.append($$anchor, p_4);
						};

						$.if(node_20, ($$render) => {
							if ((
								$snapshot(),
								$.deep_read_state(clusterState),
								$clock(),
								$.untrack(() => $snapshot() && clusterState($snapshot(), $clock()) === 'unknown' && $snapshot().health !== 'unknown')
							)) $$render(consequent_10);
						});
					}

					var node_22 = $.sibling(node_20, 2);

					{
						var consequent_11 = ($$anchor) => {
							var div_13 = root_15();
							var node_23 = $.child(div_13);

							Icon(node_23, { name: 'shield', size: 13 });
							$.next();
							$.reset(div_13);
							$.append($$anchor, div_13);
						};

						$.if(node_22, ($$render) => {
							if (($auth(), $.untrack(() => $auth()?.read_only))) $$render(consequent_11);
						});
					}

					var div_14 = $.sibling(node_22, 2);
					var node_24 = $.child(div_14);

					Dashboard(node_24, {});
					$.reset(div_14);

					var node_25 = $.sibling(div_14, 2);

					{
						var consequent_12 = ($$anchor) => {
							var div_15 = root_16();
							var node_26 = $.child(div_15);

							Nodes(node_26, {});
							$.reset(div_15);
							$.template_effect(() => $.set_attribute(div_15, 'hidden', $route() !== 'nodes'));
							$.append($$anchor, div_15);
						};

						$.if(node_25, ($$render) => {
							if (($.get(visited), $.untrack(() => $.get(visited).has('nodes')))) $$render(consequent_12);
						});
					}

					var node_27 = $.sibling(node_25, 2);

					{
						var consequent_13 = ($$anchor) => {
							var div_16 = root_17();
							var node_28 = $.child(div_16);

							Connections(node_28, {});
							$.reset(div_16);
							$.template_effect(() => $.set_attribute(div_16, 'hidden', $route() !== 'topology'));
							$.append($$anchor, div_16);
						};

						$.if(node_27, ($$render) => {
							if ((
								$.get(visited),
								$.untrack(() => $.get(visited).has('topology'))
							)) $$render(consequent_13);
						});
					}

					var node_29 = $.sibling(node_27, 2);

					{
						var consequent_14 = ($$anchor) => {
							var div_17 = root_18();
							var node_30 = $.child(div_17);

							Metrics(node_30, {});
							$.reset(div_17);
							$.template_effect(() => $.set_attribute(div_17, 'hidden', $route() !== 'metrics'));
							$.append($$anchor, div_17);
						};

						$.if(node_29, ($$render) => {
							if ((
								$.get(visited),
								$.untrack(() => $.get(visited).has('metrics'))
							)) $$render(consequent_14);
						});
					}

					var node_31 = $.sibling(node_29, 2);

					{
						var consequent_15 = ($$anchor) => {
							var div_18 = root_19();
							var node_32 = $.child(div_18);

							Alerts(node_32, {});
							$.reset(div_18);
							$.template_effect(() => $.set_attribute(div_18, 'hidden', $route() !== 'alerts'));
							$.append($$anchor, div_18);
						};

						$.if(node_31, ($$render) => {
							if ((
								$.get(visited),
								$.untrack(() => $.get(visited).has('alerts'))
							)) $$render(consequent_15);
						});
					}

					var node_33 = $.sibling(node_31, 2);

					{
						var consequent_16 = ($$anchor) => {
							var div_19 = root_20();
							var node_34 = $.child(div_19);

							Events(node_34, {});
							$.reset(div_19);
							$.template_effect(() => $.set_attribute(div_19, 'hidden', $route() !== 'events'));
							$.append($$anchor, div_19);
						};

						$.if(node_33, ($$render) => {
							if ((
								$.get(visited),
								$.untrack(() => $.get(visited).has('events'))
							)) $$render(consequent_16);
						});
					}

					var node_35 = $.sibling(node_33, 2);

					{
						var consequent_17 = ($$anchor) => {
							var div_20 = root_21();
							var node_36 = $.child(div_20);

							Query(node_36, {});
							$.reset(div_20);
							$.template_effect(() => $.set_attribute(div_20, 'hidden', $route() !== 'query'));
							$.append($$anchor, div_20);
						};

						$.if(node_35, ($$render) => {
							if (($.get(visited), $.untrack(() => $.get(visited).has('query')))) $$render(consequent_17);
						});
					}

					var node_37 = $.sibling(node_35, 2);

					{
						var consequent_18 = ($$anchor) => {
							var div_21 = root_22();
							var node_38 = $.child(div_21);

							Settings(node_38, {});
							$.reset(div_21);
							$.template_effect(() => $.set_attribute(div_21, 'hidden', $route() !== 'settings'));
							$.append($$anchor, div_21);
						};

						$.if(node_37, ($$render) => {
							if ((
								$.get(visited),
								$.untrack(() => $.get(visited).has('settings'))
							)) $$render(consequent_18);
						});
					}

					$.reset(div_7);
					$.reset(main_3);
					$.bind_this(main_3, ($$value) => $.set(main, $$value), () => $.get(main));
					$.reset(div_4);
					$.reset(div_1);

					var node_39 = $.sibling(div_1, 2);

					Drawer(node_39, {});

					$.template_effect(
						($0, $1, $2) => {
							classes = $.set_class(aside, 1, 'sidebar', null, classes, { 'mobile-open': $.get(mobile) });
							$.set_class(span_2, 1, `dot ${$0 ?? ''}`);

							$.set_text(text_3, (
								$snapshot(),
								$.untrack(() => $snapshot()?.cluster_name || 'Scylla Cluster')
							));

							$.set_text(text_6, (
								$snapshot(),
								$.untrack(() => $snapshot()?.cluster_name || 'ワークスペース')
							));

							$.set_text(text_7, (
								$.deep_read_state(titles),
								$route(),
								$.untrack(() => titles[$route()])
							));

							classes_2 = $.set_class(span_7, 1, 'stream-state', null, classes_2, { disconnected: $connection() !== 'live' });
							$.set_class(span_8, 1, `dot ${$connection() === 'live' ? 'healthy' : 'warning'}`);

							$.set_text(text_8, $paused()
								? '表示停止中'
								: $connection() === 'live'
									? 'ライブ更新'
									: $connection() === 'connecting' ? '接続中' : '再接続中');

							$.set_attribute(button_5, 'aria-label', $paused() ? 'ライブ表示を再開' : 'ライブ表示を一時停止');
							$.set_text(text_9, $1);

							$.set_text(text_10, (
								$route(),
								$.deep_read_state(titles),
								$.untrack(() => $route() === 'dashboard' ? 'クラスター概要' : titles[$route()])
							));

							$.set_text(text_11, `観測元: ${(
								$snapshot(),
								$.untrack(() => $snapshot()?.local_node_name || $snapshot()?.local_node || '確認中')
							) ?? ''} `);

							$.set_text(text_12, (
								$snapshot(),
								$.untrack(() => $snapshot()?.local_address || '')
							));

							$.set_text(text_13, $2);

							button_8.disabled = (
								$.get(busy),
								$auth(),
								$.untrack(() => $.get(busy) || !$auth()?.authenticated)
							);

							$.set_text(text_14, $.get(busy) ? '確認中…' : '今すぐ確認');
							$.set_attribute(div_14, 'hidden', $route() !== 'dashboard');
						},
						[
							() => (
								$.deep_read_state(clusterState),
								$snapshot(),
								$clock(),
								$.untrack(() => clusterState($snapshot(), $clock()))
							),

							() => (
								$route(),
								$.untrack(() => $route() === 'dashboard'
									? 'SCYLLA OPERATIONS'
									: 'WORKSPACE / ' + $route().toUpperCase())
							),

							() => (
								$snapshot(),
								$.deep_read_state(age),
								$clock(),
								$.untrack(() => $snapshot() ? age($snapshot().generated_at, $clock()) : '初回取得中')
							)
						]
					);

					$.event('scroll', main_3, saveScroll);
					$.append($$anchor, fragment_3);
				};

				$.if(
					node_2,
					($$render) => {
						if ((
							$auth(),
							$.untrack(() => $auth().required && !$auth().authenticated)
						)) $$render(consequent_3); else $$render(alternate, false);
					},
					true
				);
			}

			$.append($$anchor, fragment_2);
		};

		$.if(node, ($$render) => {
			if (!$auth()) $$render(consequent_1); else $$render(alternate_1, false);
		});
	}

	var node_40 = $.sibling(node, 2);

	{
		var consequent_19 = ($$anchor) => {
			var div_22 = root_23();
			var node_41 = $.child(div_22);

			{
				let $0 = $.derived_safe_equal(() => (
					$toast(),
					$.untrack(() => $toast().kind === 'error' ? 'info' : 'check')
				));

				Icon(node_41, {
					get name() {
						return $.get($0);
					},
					size: 17
				});
			}

			var span_12 = $.sibling(node_41);
			var text_16 = $.child(span_12, true);

			$.reset(span_12);

			var button_10 = $.sibling(span_12);

			button_10.__click = () => toast.set(null);

			var node_42 = $.child(button_10);

			Icon(node_42, { name: 'close', size: 14 });
			$.reset(button_10);
			$.reset(div_22);

			$.template_effect(() => {
				$.set_class(div_22, 1, `toast ${($toast(), $.untrack(() => $toast().kind)) ?? ''}`);
				$.set_text(text_16, ($toast(), $.untrack(() => $toast().message)));
			});

			$.append($$anchor, div_22);
		};

		$.if(node_40, ($$render) => {
			if ($toast()) $$render(consequent_19);
		});
	}

	$.append($$anchor, fragment);
	$.pop();
	$$cleanup();
}

$.delegate(['click']);