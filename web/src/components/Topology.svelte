<script>
import Icon from './Icon.svelte';
import { snapshot, clock, selected } from '../state.js';
import {nodeState,shortStatus,edgeState,sessionGet,sessionSet,checkState} from '../lib.js';
export let compact=false;export let scope='all';
let containerWidth=1080, narrow=false;
// Hidden visited pages report width 0. Preserve their last visible layout.
$: if(containerWidth>0)narrow=!compact&&containerWidth<600;
$: viewWidth=narrow?400:1080;
let zoom=sessionGet('topology.zoom',1), pan=sessionGet('topology.pan',{x:0,y:0}), drag=null, svg;
$: allNodes=[...($snapshot?.nodes||[])].sort((a,b)=>a.address===$snapshot?.local_address?-1:b.address===$snapshot?.local_address?1:a.address.localeCompare(b.address));
$: nodes=(scope==='all'?allNodes:[...allNodes.filter(n=>n.id===scope),...allNodes.filter(n=>n.id!==scope)]).slice(0,12);
$: positions=layout(nodes,narrow);
$: edges=makeEdges(nodes,positions,$snapshot?.edges||[],narrow);
$: height=narrow?Math.max(220,nodes.length*155+20):nodes.length<=1?220:nodes.length<=3?350:Math.max(400,Math.ceil(nodes.length/3)*180+60);
function layout(ns,mobile){const out={};ns.forEach((n,i)=>{if(mobile)out[n.id]={x:200,y:80+i*155};else if(ns.length===1)out[n.id]={x:540,y:110};else if(ns.length===2)out[n.id]={x:290+i*500,y:160};else if(ns.length===3)out[n.id]=[{x:540,y:86},{x:230,y:255},{x:850,y:255}][i];else out[n.id]={x:190+(i%3)*350,y:110+Math.floor(i/3)*180};});return out;}
function makeEdges(ns,pos,actual,mobile){const out=[];const edgeMap=new Map(actual.map(e=>[e.from+'>'+e.to,e]));for(let i=0;i<ns.length;i++)for(let j=0;j<ns.length;j++){if(i===j||(scope!=='all'&&ns[i].id!==scope))continue;const a=ns[i],b=ns[j],p=pos[a.id],q=pos[b.id],e=edgeMap.get(a.id+'>'+b.id);let d;const offset=i<j?-5:5;if(mobile){const sign=i<j?-1:1;const lane=200+sign*(140+Math.abs(j-i)*12);d=`M${p.x+sign*110},${p.y+offset}H${lane}V${q.y+offset}H${q.x+sign*110}`;}else if(p.y===q.y){const dir=q.x>p.x?1:-1;d=`M${p.x+dir*110},${p.y+offset}H${q.x-dir*110}`;}else {const dir=q.y>p.y?1:-1;const mid=(p.y+q.y)/2+offset;d=`M${p.x+offset},${p.y+dir*54}V${mid}H${q.x+offset}V${q.y-dir*54}`;}out.push({key:a.id+'>'+b.id,from:a,to:b,e,d});}return out;}
function open(n){selected.set({kind:'node',id:n.id});}
function edge(e){selected.set({kind:'edge',from:e.from.id,to:e.to.id});}
function scale(d){zoom=Math.max(.5,Math.min(2.5,Math.round((zoom+d)*10)/10));sessionSet('topology.zoom',zoom);}
function reset(){zoom=1;pan={x:0,y:0};sessionSet('topology.zoom',zoom);sessionSet('topology.pan',pan);}
function down(e){if(compact||e.button!==0||e.target.closest('[data-item]'))return;drag={x:e.clientX,y:e.clientY,pan:{...pan}};svg.setPointerCapture(e.pointerId);}
function move(e){if(!drag)return;const r=svg.getBoundingClientRect();pan={x:drag.pan.x+(e.clientX-drag.x)*viewWidth/r.width,y:drag.pan.y+(e.clientY-drag.y)*height/r.height};}
function up(){if(drag)sessionSet('topology.pan',pan);drag=null;}
</script>
<div class="topology" class:compact bind:clientWidth={containerWidth}>
{#if allNodes.length>12}<p class="graph-limit">接続図は12台に限定しています（全{allNodes.length}台）。特定の観測元や接続マトリクスで範囲を切り替えてください。</p>{/if}
{#if !compact}<div class="map-controls"><span class="quiet">矢印は観測方向です · ドラッグで移動</span><div class="button-group"><button aria-label="縮小" onclick={()=>scale(-.1)}><Icon name="minus" size={14}/></button><span class="mono">{Math.round(zoom*100)}%</span><button aria-label="拡大" onclick={()=>scale(.1)}><Icon name="plus" size={14}/></button><button aria-label="全体を表示" onclick={reset}><Icon name="fit" size={14}/></button></div></div>{/if}
{#if nodes.length===0}<div class="empty"><Icon name="topology" size={28}/><strong>クラスタ情報を確認しています</strong><span>取得できない場合は診断に理由を表示します。</span></div>{:else}
<svg bind:this={svg} class="map" viewBox={`0 0 ${viewWidth} ${height}`} style:height={compact?'280px':`${Math.min(height,520)}px`} role="group" aria-label="Scyllaクラスタ接続図" onpointerdown={down} onpointermove={move} onpointerup={up} onpointercancel={up}>
<defs>{#each ['healthy','critical','unknown','warning'] as state}<marker id={`arrow-${compact?'small':'full'}-${state}`} markerWidth="6" markerHeight="6" refX="5.5" refY="3" orient="auto"><path d="M0 0L6 3L0 6Z" class="arrow-{state}"/></marker>{/each}</defs>
<g transform={compact?'':`translate(${pan.x},${pan.y}) translate(${viewWidth/2},${height/2}) scale(${zoom}) translate(${-viewWidth/2},${-height/2})`}>
{#each edges as link (link.key)}
{@const status=edgeState(link.e,$snapshot,$clock)}
<g data-item="edge" class="edge-group" role="button" tabindex="0" aria-label={`${link.from.name}から${link.to.name}への接続: ${status}`} onclick={()=>edge(link)} onkeydown={e=>{if(e.key==='Enter'||e.key===' '){e.preventDefault();edge(link);}}}><path class="edge-hit" d={link.d}/><path class="map-edge {status}" d={link.d} marker-end={`url(#arrow-${compact?'small':'full'}-${status})`}/></g>
{/each}
{#each nodes as n (n.host_id||n.id)}
{@const p=positions[n.id]}{@const state=nodeState(n,$snapshot,$clock)}
<g data-item="node" data-node-id={n.id} transform={`translate(${p.x-110},${p.y-54})`} class="map-node {state}" class:chosen={$selected?.kind==='node'&&$selected.id===n.id} role="button" tabindex="0" aria-label={`${n.name}の詳細`} onclick={()=>open(n)} onkeydown={e=>{if(e.key==='Enter'||e.key===' '){e.preventDefault();open(n);}}}>
<rect width="220" height="108" rx="10" class="node-bg"/><circle cx="17" cy="23" r="4" class="status-fill {state}"/>
<text x="29" y="27" class="node-title">{(n.name||n.address).length>25?(n.name||n.address).slice(0,24)+'…':n.name||n.address}</text>
<text x="17" y="47" class="node-address">{n.address}</text><text x="17" y="66" class="node-sub">{n.dc||'DC 未取得'} / {n.rack||'Rack 未取得'}</text>
<text x="17" y="90" class="node-state">{shortStatus(n)}</text><circle cx="70" cy="85" r="3" class="status-fill {checkState(n.cql,$clock,$snapshot?.stale_after_seconds)}"/><text x="78" y="90" class="node-sub">CQL</text><circle cx="125" cy="85" r="3" class="status-fill {checkState(n.internode,$clock,$snapshot?.stale_after_seconds)}"/><text x="133" y="90" class="node-sub">7000</text>{#if n.address===$snapshot?.local_address}<text x="194" y="89" class="node-local">●</text>{/if}
</g>{/each}
</g></svg>{/if}
{#if compact}<div class="mobile-map-list" aria-label="ノードと接続の概要">{#each nodes as n (n.host_id||n.id)}<button onclick={()=>open(n)}><span class="dot {nodeState(n,$snapshot,$clock)}"></span><span><strong>{n.name}</strong><small class="mono">{n.address}</small></span><span class="badge {nodeState(n,$snapshot,$clock)}">{shortStatus(n)}</span><Icon name="chevron" size={15}/></button>{/each}<p>観測方向 {$snapshot?.coverage?.observed_directions||0} / {$snapshot?.coverage?.expected_directions||0} · 接続図は「詳細を開く」へ</p></div>{/if}
<div class="map-legend"><span><i class="line healthy"></i>応答あり</span><span><i class="line critical"></i>接続失敗</span><span><i class="line unknown"></i>未観測 / 期限切れ</span><span class="quiet">ノード・線を選択すると詳細</span></div>
</div>
