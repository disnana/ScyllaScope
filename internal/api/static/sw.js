// v0.6 retires the legacy offline worker. Monitoring must not appear live from an offline cache.
self.addEventListener('install',()=>self.skipWaiting());
self.addEventListener('activate',event=>event.waitUntil((async()=>{
 for(const key of await caches.keys())if(key.startsWith('scyllascope'))await caches.delete(key);
 await self.clients.claim(); await self.registration.unregister();
})()));
