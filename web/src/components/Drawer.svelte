<script>
import {tick} from 'svelte';import {snapshot,selected,clock,notify,request} from '../state.js';import {nodeState,shortStatus,latency,bytes,age,date,copy,edgeState,labels,checkState} from '../lib.js';import Badge from './Badge.svelte';import Check from './Check.svelte';import Icon from './Icon.svelte';import Chart from './Chart.svelte';
let dialog,tab='overview',metricOpen=false,rawOpen=false,metricFilter='',rows=[],nodeHistory=[],historyError='',lastNode='',opened=false,focusBefore;
$: node=$selected?.kind==='node'?$snapshot?.nodes.find(n=>n.id===$selected.id):null;
$: from=$snapshot?.nodes.find(n=>n.id===$selected?.from);$: to=$snapshot?.nodes.find(n=>n.id===$selected?.to);
$: edge=$snapshot?.edges.find(e=>e.from===$selected?.from&&e.to===$selected?.to);
$: reverse=$snapshot?.edges.find(e=>e.from===$selected?.to&&e.to===$selected?.from);
$: if(dialog&&$selected&&!opened)show();else if(dialog&&!$selected&&opened){opened=false;dialog.close();}
$: if(node&&node.address!==lastNode){lastNode=node.address;tab='overview';metricFilter='';metricOpen=false;rawOpen=false;nodeHistory=[];historyError='';loadHistory(node.address);}
$: rows=Object.entries(node?.metric_values||{}).filter(([name])=>name.toLowerCase().includes(metricFilter.toLowerCase()));
async function show(){opened=true;focusBefore=document.activeElement;await tick();if(!dialog.open)dialog.showModal();}
function close(){selected.set(null);focusBefore?.focus?.();}
async function loadHistory(address){try{const h=await request('history?hours=6&node='+encodeURIComponent(address));if(lastNode===address)nodeHistory=h.map(n=>({time:n.last_updated_at,average_latency_ms:n.cql?.ok?n.cql.latency/1e6:null}));}catch(e){if(lastNode===address)historyError=e.message;}}
async function doCopy(v){try{await copy(v);notify('コピーしました');}catch(e){notify(e.message,'error');}}
</script>
<dialog bind:this={dialog} class="drawer" onclose={close} onclick={e=>{if(e.target===dialog)close();}} aria-label="監視の詳細">
<div class="drawer-inner">
<header class="drawer-header"><div><span class="eyebrow">{$selected?.kind==='edge'?'DIRECTIONAL CONNECTION':'NODE DETAILS'}</span><h2>{node?.name||($selected?.kind==='edge'?'接続の詳細':'ノードを確認')}</h2></div><button class="icon-button" aria-label="詳細を閉じる" onclick={close}><Icon name="close"/></button></header>
{#if node}
<div class="drawer-meta"><Badge state={nodeState(node,$snapshot,$clock)} text={shortStatus(node)}/><span class="mono">{node.address}</span><button class="icon-button" aria-label="IPをコピー" onclick={()=>doCopy(node.address)}><Icon name="copy" size={14}/></button></div>
<nav class="tabs" aria-label="ノード詳細タブ">{#each [['overview','概要'],['connections','接続'],['metrics','メトリクス']] as [id,label]}<button class:active={tab===id} onclick={()=>tab=id}>{label}</button>{/each}</nav>
<div class="drawer-body">
<section hidden={tab!=='overview'}>
{#if node.reasons?.length}<div class="notice warning">{#each node.reasons as reason}<p>{reason}</p>{/each}</div>{/if}
<div class="checks-list">{#each [['CQLプロトコル',node.cql],['ノード間TCP',node.internode],['ScyllaScope',node.watch_api],['systemd',node.local_service],['ローカルREST',node.rest],['Scyllaメトリクス',node.metrics]] as [name,c]}<div><span>{name}</span><Check value={c} timings ttl={$snapshot.stale_after_seconds}/></div>{/each}</div>
<dl class="definition"><dt>データセンター / Rack</dt><dd>{node.dc||'—'} / {node.rack||'—'}</dd><dt>Scyllaバージョン</dt><dd class="wrap">{node.version||'未取得'}</dd><dt>Shards</dt><dd>{node.shards||'未取得'}</dd><dt>起動引数のメモリ上限</dt><dd>{node.labels?.memory||'未指定 / 未取得'}</dd><dt>データ量</dt><dd>{node.load_known?bytes(node.load_bytes):'未取得'}</dd><dt>Scopeバージョン</dt><dd>{node.agent_version||'未取得'}</dd><dt>最終観測</dt><dd>{date(node.last_updated_at)}</dd></dl>
<details bind:open={rawOpen}><summary>識別情報とチェックの説明</summary><dl class="definition"><dt>Host ID</dt><dd class="mono wrap">{node.host_id||'未取得'}</dd><dt>観測元</dt><dd>{$snapshot.local_node_name||$snapshot.local_node}</dd></dl><p class="hint">CQLはOPTIONS応答、7000はTCP接続を確認します。クエリの実行時間やRaft合意・データ複製の正常性を保証するものではありません。</p></details>
<div class="panel flat"><button class="link-button history-refresh" onclick={()=>loadHistory(node.address)}>履歴を更新</button><Chart samples={nodeHistory} title="この観測元からのCQL応答（6時間）"/>{#if historyError}<p class="error-text">{historyError}</p>{/if}</div>
</section>
<section hidden={tab!=='connections'}><h3>このノードに関わる接続</h3>{#each ($snapshot.edges||[]).filter(e=>e.from===node.id||e.to===node.id) as e (e.from+'>'+e.to)}<button class="connection-row" onclick={()=>selected.set({kind:'edge',from:e.from,to:e.to})}><span class="wrap">{$snapshot.nodes.find(n=>n.id===e.from)?.name||e.from} → {$snapshot.nodes.find(n=>n.id===e.to)?.name||e.to}</span><Badge state={edgeState(e,$snapshot,$clock)}/></button>{:else}<p class="quiet">方向別の観測がありません。各ノードにScyllaScopeと共通のcluster_tokenが必要です。</p>{/each}</section>
<section hidden={tab!=='metrics'}><p class="hint">ノード自身が取得したメトリクス。カウンターは累積値、ゲージは観測時点の値です。</p><label class="search"><Icon name="search" size={16}/><input aria-label="メトリクス検索" bind:value={metricFilter} placeholder="名前で絞り込み"/></label><div class="metric-list">{#each rows as [name,value] (name)}<div><code>{name}</code><strong class="mono">{Number(value).toLocaleString('ja-JP',{maximumFractionDigits:4})}</strong></div>{:else}<p class="quiet">該当するメトリクスがありません。</p>{/each}</div></section>
</div>
{:else if $selected?.kind==='edge'}
<div class="drawer-body"><p class="edge-title">{from?.name||$selected.from}<Icon name="arrow"/>{to?.name||$selected.to}</p><Badge state={edgeState(edge,$snapshot,$clock)}/><div class="checks-list"><div><span>CQLプロトコル</span><Check value={edge?.cql} timings ttl={$snapshot?.stale_after_seconds}/></div><div><span>ノード間TCP</span><Check value={edge?.internode} timings ttl={$snapshot?.stale_after_seconds}/></div><div><span>ScyllaScope API</span><Check value={edge?.watch_api} timings ttl={$snapshot?.stale_after_seconds}/></div></div><dl class="definition"><dt>観測方向</dt><dd>{from?.name} → {to?.name}</dd><dt>観測時刻</dt><dd>{date(edge?.observed_at)}</dd><dt>逆方向の結果</dt><dd><Badge state={edgeState(reverse,$snapshot,$clock)}/></dd></dl>{#if edge?.asymmetric}<p class="notice warning">双方向で結果が異なります。片方向の経路・フィルタを確認してください。</p>{/if}{#if !edge}<p class="notice">この方向の情報は未取得です。接続失敗とは区別しています。</p>{/if}<details><summary>各チェックの詳細</summary><pre>{JSON.stringify({cql:edge?.cql,internode:edge?.internode,watch_api:edge?.watch_api},null,2)}</pre></details><p class="hint">ノード間の到達性の観測結果です。レプリケーションやRaftの合意結果ではありません。</p></div>
{:else}<div class="empty"><span>このノードは現在の観測一覧にありません。</span></div>{/if}
</div></dialog>
