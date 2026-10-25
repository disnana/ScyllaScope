import { writable, get } from 'svelte/store';
import { ensureSnapshot, sessionGet, sessionSet, storageGet, storageSet, routes } from './lib.js';
import { boundedCacheSet, fetchJSON } from './lifecycle.js';
export const snapshot=writable(null), clock=writable(Date.now()), auth=writable(null), config=writable(null), connection=writable('connecting'), toast=writable(null), paused=writable(false);
export const events=writable([]), incidents=writable([]), history=writable([]), errors=writable({});
export const route=writable(routes.includes(location.hash.slice(1))?location.hash.slice(1):'dashboard');
export const selected=writable(sessionGet('selected',null));
export const preferences=writable(storageGet('preferences',{theme:'dark',density:'compact'}));
preferences.subscribe(v=>{document.documentElement.dataset.theme=v.theme;document.documentElement.dataset.density=v.density;storageSet('preferences',v);});
selected.subscribe(v=>sessionSet('selected',v));
let source, retry, timer, toastTimer, latest, booted=false, authLock=false, lastID=0;
const active=new Map(), loaded=new Map(); let epoch=0; let lifecycle=new AbortController();
export function notify(message,kind='info'){const id=++lastID;toast.set({message,kind,id});clearTimeout(toastTimer);toastTimer=setTimeout(()=>{if(get(toast)?.id===id)toast.set(null);},6000);}
export async function request(path,options={}){
 const generation=epoch;
 return fetchJSON('/api/v1/'+path,{credentials:'same-origin',cache:'no-store',...options,headers:{...(options.method&&options.method!=='GET'?{'Content-Type':'application/json','X-ScyllaScope-Request':'1'}:{}),...options.headers}},lifecycle.signal,()=>{if(generation===epoch&&!['auth','login'].includes(path))expire();});
}
export function navigate(page){if(routes.includes(page)&&get(route)!==page)location.hash=page;}
function accept(v){const next=ensureSnapshot(v);latest=next;if(!get(paused)){snapshot.set(next);events.update(old=>mergeEvents(old,next.events));incidents.update(old=>mergeIncidents(old,next.incidents));}connection.set('live');}
function mergeEvents(a,b){return [...new Map([...a,...b].map(e=>[e.id,e])).values()].sort((a,b)=>Date.parse(b.time)-Date.parse(a.time)).slice(0,500);}
function mergeIncidents(a,b){return [...new Map([...a.filter(i=>i.resolved_at),...b].map(e=>[e.id,e])).values()].sort((a,b)=>Date.parse(b.updated_at)-Date.parse(a.updated_at)).slice(0,500);}
paused.subscribe(v=>{if(!v&&latest){snapshot.set(latest);events.update(old=>mergeEvents(old,latest.events));incidents.update(old=>mergeIncidents(old,latest.incidents));}});
function connect(){if(source||authLock||lifecycle.signal.aborted)return;const generation=epoch;source=new EventSource('/api/v1/stream');source.addEventListener('snapshot',e=>{if(generation!==epoch)return;try{accept(JSON.parse(e.data));}catch(err){errors.update(v=>({...v,stream:err.message}));}});source.addEventListener('auth_expired',()=>{if(generation===epoch)expire();});source.onerror=()=>{if(generation!==epoch)return;connection.set('reconnecting');source?.close();source=null;clearTimeout(retry);retry=setTimeout(async()=>{try{const a=await request('auth');if(generation!==epoch)return;auth.set(a);if(a.required&&!a.authenticated){expire();return;}}catch{}if(generation===epoch)connect();},3000);};}
export async function boot(){if(lifecycle.signal.aborted)lifecycle=new AbortController();const e=epoch;try{const a=await request('auth');if(e!==epoch)return;auth.set(a);if(a.required&&!a.authenticated)return;await start();}catch(err){if(e!==epoch||err.name==='AbortError')return;errors.update(v=>({...v,boot:err.message}));connection.set('reconnecting');}}
async function start(){authLock=false;const e=epoch;const [c,s]=await Promise.all([request('config'),request('snapshot')]);if(e!==epoch)return;config.set(c);accept(s);connect();if(!booted){timer=setInterval(()=>clock.set(Date.now()),1000);booted=true;}}
export async function login(token){const e=epoch;if(lifecycle.signal.aborted)lifecycle=new AbortController();await request('login',{method:'POST',body:JSON.stringify({token})});if(e!==epoch)return;const a=await request('auth');if(e!==epoch)return;auth.set(a);await start();}
export function dispose(){epoch++;authLock=true;lifecycle.abort();clearTimeout(toastTimer);loaded.clear();booted=false;source?.close();source=null;clearTimeout(retry);clearInterval(timer);for(const c of active.values())c.abort();active.clear();}
function expire(){epoch++;authLock=true;lifecycle.abort();lifecycle=new AbortController();clearInterval(timer);booted=false;source?.close();source=null;clearTimeout(retry);for(const c of active.values())c.abort();active.clear();snapshot.set(null);config.set(null);events.set([]);incidents.set([]);history.set([]);selected.set(null);latest=null;loaded.clear();auth.update(v=>({...v,required:true,authenticated:false}));}
export async function logout(){await request('logout',{method:'POST',body:'{}'});expire();}
export async function refresh(){const e=epoch;const v=await request('refresh',{method:'POST',body:'{}'});if(e!==epoch)return;if(v?.busy){notify('監視を実行中です。完了すると自動更新されます。');return;}accept(v);notify('最新の観測結果に更新しました');}
export async function loadResource(name,force=false,hours=6){active.get(name)?.abort();active.delete(name);const k=name==='history'?`${name}:${hours}`:name;if(!force&&loaded.has(k)&&Date.now()-loaded.get(k).at<15000){const v=loaded.get(k).data;if(name==='history')history.set(v);else if(name==='events')events.set(v);else incidents.set(v);return;}const c=new AbortController();active.set(name,c);const e=epoch;try{const path=name==='history'?`history?hours=${hours}`:name==='incidents'?'incidents?all=1':'events?limit=500';const v=await request(path,{signal:c.signal});if(c.signal.aborted||e!==epoch)return;if(!Array.isArray(v))throw Error('応答の形式を確認できません');if(name==='history')history.set(v);else if(name==='events')events.set(v);else incidents.set(v);boundedCacheSet(loaded,k,{data:v,at:Date.now()},16);errors.update(v=>({...v,[name]:null}));}catch(err){if(e===epoch&&err.name!=='AbortError')errors.update(v=>({...v,[name]:err.message}));}finally{if(active.get(name)===c)active.delete(name);}}
export async function actIncident(id,action,minutes=60){await request('incidents/action',{method:'POST',body:JSON.stringify({id,action,minutes})});await loadResource('incidents',true);notify(action==='ack'?'確認済みにしました。復旧とは別の状態です。':action==='snooze'?'この観測元で通知を一時停止しました':'通知を再開しました');}
export async function exportDiagnostics(){const v=await request('export');return JSON.stringify(v,null,2);}
export function setupHash(){const f=()=>{const p=location.hash.slice(1);route.set(routes.includes(p)?p:'dashboard');};window.addEventListener('hashchange',f);return ()=>window.removeEventListener('hashchange',f);}
