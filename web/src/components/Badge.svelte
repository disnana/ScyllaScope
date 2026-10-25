<script>import {labels} from '../lib.js';export let state='unknown';export let text='';export let dot=true;</script>
<span class="badge {state}">{#if dot}<span class="dot"></span>{/if}{text||labels[state]||state}</span>
