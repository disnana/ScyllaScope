// Framework-independent so lifecycle/resource bounds have direct tests.
export function boundedCacheSet(cache,key,value,limit=16){
 if(!Number.isInteger(limit)||limit<1)throw new RangeError('invalid cache limit');
 cache.delete(key);cache.set(key,value);
 while(cache.size>limit)cache.delete(cache.keys().next().value);
}
export async function fetchJSON(url,options={},lifetime,unauthorized=()=>{},timeoutMS=15000){
 const controller=new AbortController();
 const signals=[options.signal,lifetime].filter(Boolean);
 const abort=()=>controller.abort();
 for(const signal of signals){if(signal.aborted)abort();else signal.addEventListener('abort',abort,{once:true});}
 const timer=setTimeout(abort,timeoutMS);
 try{
  const response=await fetch(url,{...options,signal:controller.signal});
  if(response.status===401){unauthorized();throw new Error('セッションが終了しました。再ログインしてください。');}
  if(!response.ok){const text=await response.text();throw new Error((text||`HTTP ${response.status}`).slice(0,700));}
  if(response.status===204)return null;
  return await response.json();
 }finally{
  clearTimeout(timer);
  for(const signal of signals)signal.removeEventListener('abort',abort);
 }
}
