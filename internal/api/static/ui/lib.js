export const VERSION = '0.6.2';
export const routes = ['dashboard','nodes','topology','metrics','alerts','events','query','settings'];
export const titles = {dashboard:'概要',nodes:'ノード',topology:'接続図',metrics:'メトリクス',alerts:'アラート',events:'イベント',query:'CQLコンソール',settings:'設定'};
export const labels = {healthy:'正常',warning:'注意',critical:'異常',unknown:'未確認',stale:'期限切れ',ok:'応答あり',error:'失敗',disabled:'対象外',connecting:'接続中',live:'ライブ',reconnecting:'再接続中',paused:'表示を一時停止'};
export function finite(v){return typeof v==='number'&&Number.isFinite(v);}
export function stamp(v){const t=Date.parse(v);return Number.isFinite(t)&&t>0?t:0;}
export function fresh(check, now, ttl=30){const t=stamp(check?.checked_at);return !!t&&now-t<=ttl*1000&&t-now<=5000&&!['unknown','disabled','stale'].includes(check?.state);}
export function checkState(check,now,ttl=30){if(check?.state==='disabled')return 'disabled';if(!stamp(check?.checked_at))return 'unknown';if(!fresh(check,now,ttl))return 'stale';return check.ok?'ok':'error';}
export function nodeState(node,snap,now){if(!snap||!stamp(snap.generated_at)||stamp(snap.generated_at)-now>5000||now-stamp(snap.generated_at)>(snap.stale_after_seconds||30)*1000)return 'unknown';return node?.state||'unknown';}
export function clusterState(s,now){if(!s||!stamp(s.generated_at)||now-stamp(s.generated_at)>(s.stale_after_seconds||30)*1000||stamp(s.generated_at)-now>5000)return 'unknown';return s.health||'unknown';}
export function latency(c,now,ttl=30){if(!fresh(c,now,ttl)||!c?.ok||!finite(c.latency)||c.latency<0)return '—';return formatMS(c.latency/1e6);}
export function formatMS(v){return finite(v)?(v<1?v.toFixed(2):v<100?v.toFixed(1):v.toFixed(0))+' ms':'—';}
export function bytes(v){if(!finite(v)||v<0)return '—';if(v===0)return '0 B';const u=['B','KiB','MiB','GiB','TiB'];const i=Math.min(4,Math.floor(Math.log(v)/Math.log(1024)));return `${(v/1024**i).toFixed(i?1:0)} ${u[i]}`;}
export function age(v,now){const t=stamp(v);if(!t)return '未確認';let s=Math.max(0,Math.floor((now-t)/1000));if(s<60)return `${s}秒前`;if(s<3600)return `${Math.floor(s/60)}分前`;return `${Math.floor(s/3600)}時間前`;}
export function date(v){const t=stamp(v);return t?new Date(t).toLocaleString('ja-JP',{month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',second:'2-digit'}):'—';}
export function time(v){const t=stamp(v);return t?new Date(t).toLocaleTimeString('ja-JP',{hour:'2-digit',minute:'2-digit'}):'—';}
export function key(n){return n?.host_id||n?.id||n?.address||'';}
export function shortStatus(n){if(!n?.metadata_known)return '未確認';return `${n.up?'U':'D'}${({NORMAL:'N',JOINING:'J',LEAVING:'L',MOVING:'M'})[n.status]||n.status||'?'}`;}
export function edgeState(e,s,now){if(!e||!stamp(e.observed_at)||stamp(e.observed_at)-now>5000||e.stale||now-stamp(e.observed_at)>(s?.stale_after_seconds||30)*1000)return 'unknown';const cs=checkState(e.cql,now,s?.stale_after_seconds),is=checkState(e.internode,now,s?.stale_after_seconds);if(cs==='error'||is==='error')return 'critical';if(cs==='ok'&&is==='ok')return 'healthy';return 'unknown';}
export function storageGet(name,def){try{return JSON.parse(localStorage.getItem('scyllascope.v6.'+name))??def;}catch{return def;}}
export function storageSet(name,value){try{localStorage.setItem('scyllascope.v6.'+name,JSON.stringify(value));}catch{}}
export function sessionGet(name,def){try{return JSON.parse(sessionStorage.getItem('scyllascope.v6.'+name))??def;}catch{return def;}}
export function sessionSet(name,value){try{sessionStorage.setItem('scyllascope.v6.'+name,JSON.stringify(value));}catch{}}
export function plain(v){if(v===null||v===undefined)return 'NULL';if(typeof v==='object')return JSON.stringify(v);return String(v);}
export function csv(rows,cols){const esc=v=>{let t=plain(v);if(/^[\s]*[=+\-@]/.test(t))t="'"+t;return '"'+t.replaceAll('"','""')+'"';};return '\ufeff'+[cols.map(esc).join(','),...rows.map(r=>cols.map(c=>esc(r[c])).join(','))].join('\r\n');}
export function download(name,text,type='application/json'){const url=URL.createObjectURL(new Blob([text],{type}));const a=document.createElement('a');a.href=url;a.download=name;a.click();setTimeout(()=>URL.revokeObjectURL(url),5000);}
export async function copy(text){if(navigator.clipboard&&window.isSecureContext){await navigator.clipboard.writeText(text);return;}const a=document.createElement('textarea');a.value=text;a.style.position='fixed';a.style.opacity='0';document.body.append(a);a.select();const ok=document.execCommand('copy');a.remove();if(!ok)throw Error('コピーできません。値を選択してコピーしてください。');}
// Null samples break a path rather than presenting missing data as zero.
export function chartPaths(samples,value,width=580,height=128){const vals=samples.map(s=>value(s));const valid=vals.filter(finite);if(!valid.length)return {path:'',max:0,min:0};const low=Math.min(0,...valid),high=Math.max(...valid,low+.001);const times=samples.map(s=>stamp(s.time||s.last_updated_at));const lo=times[0],span=Math.max(1,times.at(-1)-lo);let path='',pen=false;for(let i=0;i<vals.length;i++){if(!finite(vals[i])){pen=false;continue;}const x=12+(times[i]-lo)/span*(width-24),y=height-10-(vals[i]-low)/(high-low)*(height-26);path+=(pen?'L':'M')+x.toFixed(2)+','+y.toFixed(2);pen=true;}return {path,max:high,min:low};}
export function ensureSnapshot(v){if(!v||!Array.isArray(v.nodes)||!Array.isArray(v.edges)||!v.generated_at)throw Error('監視応答の形式を確認できません');return {...v,nodes:v.nodes.filter(n=>n&&typeof n.address==='string'),edges:v.edges.filter(e=>e&&typeof e.from==='string'&&typeof e.to==='string'),warnings:Array.isArray(v.warnings)?v.warnings:[],events:Array.isArray(v.events)?v.events:[],incidents:Array.isArray(v.incidents)?v.incidents:[]};}
