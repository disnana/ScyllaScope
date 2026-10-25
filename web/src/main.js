import { mount } from 'svelte';
import App from './App.svelte';
const target = document.getElementById('app');
target.replaceChildren(); // clear only the static pre-boot fallback, once
mount(App, { target });
// Retire the v0.4/0.5 service worker. Do not cache authenticated API responses,
// and never keep a stale app shell alive across upgrades.
if ('serviceWorker' in navigator) {
  navigator.serviceWorker.getRegistrations().then(rs => Promise.all(rs.map(r => r.unregister()))).catch(()=>{});
  if ('caches' in window) caches.keys().then(ks => Promise.all(ks.filter(k=>/scylla/i.test(k)).map(k=>caches.delete(k)))).catch(()=>{});
}
