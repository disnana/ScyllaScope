// Real compiled state.js + bundled Svelte stores, with explicit platform fixtures.
// This is a lifecycle unit test, not browser E2E or a live cluster test.
import test from 'node:test';
import assert from 'node:assert/strict';
import {get} from '../../internal/api/static/ui/vendor/svelte_store.js';

test('state rejects late callbacks after logout/dispose and can restart',async(t)=>{
 globalThis.location={hash:'#dashboard'};
 globalThis.document={documentElement:{dataset:{}}};
 const storage=()=>{const map=new Map();return {getItem:key=>map.get(key)||null,setItem:(key,v)=>map.set(key,v),removeItem:key=>map.delete(key)};};
 globalThis.localStorage=storage();globalThis.sessionStorage=storage();
 const sources=[];
 class EventSourceFixture{
  constructor(url){this.url=url;this.events={};this.closed=false;sources.push(this);}
  addEventListener(name,fn){this.events[name]=fn;}
  close(){this.closed=true;}
 }
 globalThis.EventSource=EventSourceFixture;
 const fixture={nodes:[],edges:[],generated_at:new Date().toISOString(),incidents:[],events:[]};
 let delayed=null;
 t.mock.method(globalThis,'fetch',async(url,opts)=>{
  if(url.endsWith('/delayed'))return new Promise(resolve=>{delayed=resolve});
  let value={};if(url.endsWith('/auth'))value={required:true,authenticated:true};
  if(url.endsWith('/snapshot'))value=fixture;
  return {ok:true,status:200,json:async()=>value};
 });
 const state=await import('../../internal/api/static/ui/state.js');
 try{
  await state.boot();assert.equal(get(state.connection),'live');assert.equal(sources.length,1);
  const old=sources[0];
  const pending=state.request('delayed');
  state.dispose();assert.equal(old.closed,true);
  await state.boot();assert.equal(sources.length,2);assert.equal(get(state.auth).authenticated,true);
  // Browser event may already have been queued when the old connection closed.
  old.events.auth_expired();assert.equal(get(state.auth).authenticated,true);
  old.events.snapshot({data:JSON.stringify({...fixture,cluster_name:'STALE'})});
  assert.notEqual(get(state.snapshot).cluster_name,'STALE');
  // Even a platform that completes a 401 response after abort must not expire a new session.
  delayed({status:401,ok:false});await assert.rejects(pending);
  assert.equal(get(state.auth).authenticated,true);
  assert.equal(get(state.connection),'live');
  assert.equal(sources[1].closed,false);
  const before=get(state.clock);await new Promise(resolve=>setTimeout(resolve,1100));assert.ok(get(state.clock)>before);
 }finally{state.dispose();}
});
