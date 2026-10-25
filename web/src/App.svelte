<script>
import {onMount,tick} from 'svelte';
import {snapshot,clock,auth,config,route,connection,preferences,paused,toast,errors,selected,boot,dispose,setupHash,navigate,refresh,login,logout,notify} from './state.js';
import {routes,titles,clusterState,age,sessionGet,sessionSet} from './lib.js';
import Icon from './components/Icon.svelte';import Badge from './components/Badge.svelte';import Drawer from './components/Drawer.svelte';
import Dashboard from './pages/Dashboard.svelte';import Nodes from './pages/Nodes.svelte';import Connections from './pages/Connections.svelte';import Metrics from './pages/Metrics.svelte';import Alerts from './pages/Alerts.svelte';import Events from './pages/Events.svelte';import Query from './pages/Query.svelte';import Settings from './pages/Settings.svelte';
const icons={dashboard:'grid',nodes:'nodes',topology:'topology',metrics:'metrics',alerts:'alerts',events:'events',query:'query',settings:'settings'};
let visited=new Set(['dashboard',$route]),main,previous=$route,scrolls=sessionGet('scrolls',{}),mobile=false,busy=false,token='',logging=false,loginError='',mounted=false;
$: if(!visited.has($route))visited=new Set([...visited,$route]);
$: if(mounted&&$route!==previous){const next=$route;previous=next;mobile=false;restoreScroll(next);}
let restoring=false;
async function restoreScroll(id){restoring=true;await tick();if(main)main.scrollTop=scrolls[id]||0;await new Promise(requestAnimationFrame);restoring=false;}
function saveScroll(){if(main&&!restoring){scrolls[previous]=main.scrollTop;sessionSet('scrolls',scrolls);}}
async function update(){busy=true;try{await refresh();}catch(e){notify(e.message,'error');}finally{busy=false;}}
async function enter(){logging=true;loginError='';try{await login(token);token='';}catch(e){loginError=e.message;}finally{logging=false;}}
async function leave(){try{await logout();}catch(e){notify(e.message,'error');}}
function keydown(e){if(e.altKey&&!e.ctrlKey&&!e.metaKey&&/^[1-8]$/.test(e.key)){e.preventDefault();navigate(routes[Number(e.key)-1]);}if(e.key==='Escape')mobile=false;}
onMount(()=>{const cleanup=setupHash();mounted=true;boot();return ()=>{cleanup();dispose();};});
</script>
<svelte:window onkeydown={keydown}/>
{#if !$auth}
<main class="login-page"><section class="login-card"><h1>ScyllaScope</h1><p>接続を確認しています…</p>{#if $errors.boot}<p class="error-text">{$errors.boot}</p><button onclick={boot}>再試行</button>{/if}</section></main>
{:else if $auth.required&&!$auth.authenticated}
<main class="login-page"><form class="login-card" onsubmit={e=>{e.preventDefault();enter();}}><div class="brand"><span class="brand-icon"><Icon name="box" size={26}/></span><span><strong>ScyllaScope</strong><small>Distributed cluster monitor</small></span></div><h1>管理画面にログイン</h1><p class="quiet">このノードのUIトークンを入力してください。</p><input type="hidden" name="username" value="ScyllaScope" autocomplete="username"/><label>UIトークン<input type="password" bind:value={token} required minlength="32" autocomplete="current-password" placeholder="/etc/scyllascope/ui-token"/></label><button class="primary" disabled={logging}>{logging?'認証中…':'ログイン'}</button>{#if loginError}<p class="error-text" role="alert">{loginError}</p>{/if}<details><summary>トークンの確認方法</summary><pre>sudo cat /etc/scyllascope/ui-token</pre><p class="hint">ノード間共有のcluster_tokenとは別です。URLや診断ログには含めません。</p></details></form></main>
{:else}
<div class="shell"><a href="#main" class="skip" onclick={e=>{e.preventDefault();main?.focus();}}>メインへ移動</a>
{#if mobile}<button class="nav-scrim" aria-label="メニューを閉じる" onclick={()=>mobile=false}></button>{/if}
<aside class="sidebar" class:mobile-open={mobile}><a href="#dashboard" class="brand"><span class="brand-icon"><Icon name="box" size={23}/></span><span><strong>ScyllaScope</strong><small>CLUSTER MONITOR</small></span></a><div class="workspace"><span class="dot {clusterState($snapshot,$clock)}"></span><span><strong>{$snapshot?.cluster_name||'Scylla Cluster'}</strong><small>分散ノード監視</small></span></div><nav aria-label="メインナビゲーション">{#each routes as id}<a href={'#'+id} aria-current={$route===id?'page':undefined} class:active={$route===id}><Icon name={icons[id]} size={17}/><span>{titles[id]}</span>{#if id==='alerts'&&($snapshot?.incidents||[]).filter(i=>!i.resolved_at).length}<span class="nav-count">{$snapshot.incidents.filter(i=>!i.resolved_at).length}</span>{/if}</a>{/each}</nav><div class="sidebar-bottom"><Badge state={clusterState($snapshot,$clock)}/><span>v0.6.2 · Svelte SPA</span><button class="link-button" onclick={()=>navigate('settings')}>設定・診断 <Icon name="chevron" size={12}/></button></div></aside>
<div class="workspace-main"><header class="topbar"><div class="breadcrumb"><button class="icon-button mobile-menu" aria-label="メニューを開く" onclick={()=>mobile=true}><Icon name="menu"/></button><span>{$snapshot?.cluster_name||'ワークスペース'}</span><span class="divider">/</span><strong>{titles[$route]}</strong></div><div class="top-actions"><span class="stream-state" class:disconnected={$connection!=='live'}><span class="dot {$connection==='live'?'healthy':'warning'}"></span>{$paused?'表示停止中':$connection==='live'?'ライブ更新':$connection==='connecting'?'接続中':'再接続中'}</span><button class="icon-button" aria-label={$paused?'ライブ表示を再開':'ライブ表示を一時停止'} title="表示のみ停止。サーバー監視・通知は継続" onclick={()=>paused.update(v=>!v)}><Icon name={$paused?'play':'pause'} size={16}/></button><button class="icon-button" aria-label="テーマ切替" onclick={()=>preferences.update(p=>({...p,theme:p.theme==='dark'?'light':'dark'}))}><Icon name={$preferences.theme==='dark'?'sun':'moon'} size={17}/></button>{#if $auth?.authenticated}<button class="icon-button" aria-label="ログアウト" onclick={leave}><Icon name="logout" size={17}/></button>{/if}</div></header>
<main id="main" class="main-scroll" bind:this={main} onscroll={saveScroll} tabindex="-1"><div class="content"><header class="page-header"><div><div class="eyebrow">{$route==='dashboard'?'SCYLLA OPERATIONS':'WORKSPACE / '+$route.toUpperCase()}</div><div class="title-row"><h1>{$route==='dashboard'?'クラスター概要':titles[$route]}</h1>{#if $route==='dashboard'}<Badge state={clusterState($snapshot,$clock)}/>{/if}{#if $snapshot?.demo}<span class="badge warning">DEMO DATA</span>{/if}</div><p>観測元: {$snapshot?.local_node_name||$snapshot?.local_node||'確認中'} <span class="mono">{$snapshot?.local_address||''}</span></p></div><div class="page-actions"><span class="quiet last-update">{$snapshot?age($snapshot.generated_at,$clock):'初回取得中'}</span><button disabled={busy||!$auth?.authenticated} onclick={update}><Icon name="refresh" size={14}/>{busy?'確認中…':'今すぐ確認'}</button></div></header>
{#if $errors.boot}<div class="notice warning" role="alert">{$errors.boot}<button onclick={boot}>再試行</button></div>{/if}
{#if $snapshot&&clusterState($snapshot,$clock)==='unknown'&&$snapshot.health!=='unknown'}<p class="stale-banner" role="status"><Icon name="info" size={15}/>最新情報を受信できていません。最後の観測を表示しています。</p>{/if}
{#if $auth?.read_only}<div class="read-only-line"><Icon name="shield" size={13}/>読取専用 · 操作する場合はUIトークンを設定してください</div>{/if}
<div hidden={$route!=='dashboard'} class="page" data-page="dashboard"><Dashboard/></div>
{#if visited.has('nodes')}<div hidden={$route!=='nodes'} class="page" data-page="nodes"><Nodes/></div>{/if}
{#if visited.has('topology')}<div hidden={$route!=='topology'} class="page" data-page="topology"><Connections/></div>{/if}
{#if visited.has('metrics')}<div hidden={$route!=='metrics'} class="page" data-page="metrics"><Metrics/></div>{/if}
{#if visited.has('alerts')}<div hidden={$route!=='alerts'} class="page" data-page="alerts"><Alerts/></div>{/if}
{#if visited.has('events')}<div hidden={$route!=='events'} class="page" data-page="events"><Events/></div>{/if}
{#if visited.has('query')}<div hidden={$route!=='query'} class="page" data-page="query"><Query/></div>{/if}
{#if visited.has('settings')}<div hidden={$route!=='settings'} class="page" data-page="settings"><Settings/></div>{/if}
</div></main></div></div>
<Drawer/>
{/if}
{#if $toast}<div class="toast {$toast.kind}" role="status"><Icon name={$toast.kind==='error'?'info':'check'} size={17}/><span>{$toast.message}</span><button class="icon-button" aria-label="通知を閉じる" onclick={()=>toast.set(null)}><Icon name="close" size={14}/></button></div>{/if}
