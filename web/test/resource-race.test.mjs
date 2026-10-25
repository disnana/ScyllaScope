// Uses the real compiled state module; only browser APIs and network replies are fixtures.
import test from 'node:test';
import assert from 'node:assert/strict';
import {get} from '../../internal/api/static/ui/vendor/svelte_store.js';

test('switching to cached history aborts a pending different range',async(t)=>{
 globalThis.location={hash:'#dashboard'};
 globalThis.document={documentElement:{dataset:{}}};
 const storage=()=>({getItem:()=>null,setItem:()=>{},removeItem:()=>{}});
 globalThis.localStorage=storage();globalThis.sessionStorage=storage();
 const one=[{time:'2026-10-16T00:00:00Z',range:'one'}];
 const six=[{time:'2026-10-16T00:00:00Z',range:'six'}];
 let finishSix;
 const sixStarted=new Promise(resolve=>{
  t.mock.method(globalThis,'fetch',async(url,options)=>{
   if(url.endsWith('hours=1'))return {ok:true,status:200,json:async()=>one};
   if(url.endsWith('hours=6')){
    resolve(options.signal);
    // Deliberately complete despite abort, as an already queued reply can do.
    return new Promise(finish=>{finishSix=()=>finish({ok:true,status:200,json:async()=>six});});
   }
   throw new Error('unexpected URL '+url);
  });
 });
 const state=await import('../../internal/api/static/ui/state.js');
 let pending;
 try{
  await state.loadResource('history',false,1);
  assert.deepEqual(get(state.history),one);
  pending=state.loadResource('history',false,6);
  const signal=await sixStarted;
  await state.loadResource('history',false,1);
  assert.deepEqual(get(state.history),one);
  finishSix();await pending;pending=null;
  assert.deepEqual(get(state.history),one,'late six-hour response overwrote selected cached one-hour data');
  assert.equal(signal.aborted,true,'obsolete request was not cancelled');
 }finally{
  if(pending){finishSix();await pending;}
  state.dispose();
 }
});
