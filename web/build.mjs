// Offline, deterministic Svelte 5.48.0 build. No CDN, npm install or network needed.
// The vendored compiler/runtime is the official Svelte code packaged as ESM by
// Gradio 6.x; provenance and checksums are recorded in THIRD_PARTY.md.
import { compile, VERSION } from './vendor/svelte/svelte_compiler.js';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { createHash } from 'node:crypto';
const here = path.dirname(fileURLToPath(import.meta.url));
const src = path.join(here, 'src');
const root = path.resolve(here, '../internal/api/static');
const out = path.join(root, 'ui');
fs.rmSync(out, {recursive: true, force: true});
fs.mkdirSync(out, {recursive: true});
const aliases = {
  'svelte':'svelte_svelte.js', 'svelte/internal/client':'svelte_internal_client.js',
  'svelte/internal/disclose-version':'svelte_internal_disclose-version.js',
  'svelte/internal/flags/legacy':'svelte_internal_flags_legacy.js',
  'svelte/store':'svelte_store.js', 'svelte/transition':'svelte_transition.js',
  'svelte/easing':'svelte_easing.js'
};
const needed = new Set(); let compiled = 0; const warnings = [];
function rewrite(code, target) {
  return code.replace(/((?:from\s*|import\s*)['"])([^'"]+)(['"])/g, (all, before, spec, after) => {
    if (spec in aliases) {
      needed.add(aliases[spec]);
      let p = path.relative(path.dirname(target), path.join(out, 'vendor', aliases[spec])).split(path.sep).join('/');
      return before + (p.startsWith('.') ? p : './'+p) + after;
    }
    if (spec.startsWith('svelte')) throw new Error('Unknown Svelte entrypoint: '+spec);
    return before + spec.replace(/\.svelte$/, '.js') + after;
  });
}
function walk(dir) {
  for (const f of fs.readdirSync(dir, {withFileTypes:true}).sort((a,b)=>a.name.localeCompare(b.name))) {
    const p=path.join(dir,f.name), rel=path.relative(src,p);
    if (f.isDirectory()) {walk(p); continue;}
    const dest=path.join(out,rel.replace(/\.svelte$/,'.js'));
    fs.mkdirSync(path.dirname(dest), {recursive:true});
    if (f.name.endsWith('.svelte')) {
      const res=compile(fs.readFileSync(p,'utf8'), {filename:rel, generate:'client', dev:false, css:'external'});
      if (res.css?.code) throw new Error('Use shared CSS, not component style blocks: '+rel);
      warnings.push(...res.warnings.map(w=>rel+':'+w.start?.line+' '+w.code+' '+w.message));
      fs.writeFileSync(dest,rewrite(res.js.code,dest)); compiled++;
    } else if (f.name.endsWith('.js')) fs.writeFileSync(dest,rewrite(fs.readFileSync(p,'utf8'),dest));
    else fs.copyFileSync(p,dest);
  }
}
walk(src);
function vendor(file) {
  const dst=path.join(out,'vendor',file);
  if(fs.existsSync(dst)) return;
  const code=fs.readFileSync(path.join(here,'vendor/svelte',file),'utf8');
  fs.mkdirSync(path.dirname(dst),{recursive:true});fs.writeFileSync(dst,code);
  for(const m of code.matchAll(/^(?:import|export)[^\n]*?(?:from\s*|import\s*)['"]\.\/([^'"]+)['"]/gm)) vendor(m[1]);
}
for(const f of fs.readdirSync(path.join(here,'vendor/svelte')).filter(f=>f.endsWith('.js')&&!f.includes('compiler'))) vendor(f);
fs.copyFileSync(path.join(here,'vendor/svelte/LICENSE'),path.join(out,'vendor/LICENSE.txt'));
fs.copyFileSync(path.join(here,'index.html'),path.join(root,'index.html'));
let hashes = [];
function hashFiles(d){for(const f of fs.readdirSync(d,{withFileTypes:true}).sort((a,b)=>a.name.localeCompare(b.name))){const p=path.join(d,f.name);if(f.isDirectory())hashFiles(p);else hashes.push(createHash('sha256').update(fs.readFileSync(p)).digest('hex')+'  '+path.relative(root,p).split(path.sep).join('/'));}}
hashFiles(out);
fs.writeFileSync(path.join(root,'ui-manifest.sha256'),hashes.join('\n')+'\n');
console.log(`Svelte ${VERSION}: ${compiled} components, ${hashes.length} local assets. No network.`);
if(warnings.length){console.error(warnings.join('\n'));process.exitCode=1;}
