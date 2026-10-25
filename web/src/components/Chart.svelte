<script>import {chartPaths,formatMS,time,finite} from '../lib.js';export let samples=[];export let field='average_latency_ms';export let title='CQL応答';export let unit='ms';export let small=false;$: data=chartPaths(samples,s=>s[field]);$: value=[...samples].reverse().map(s=>s[field]).find(finite);</script>
<div class:small class="chart-block">
  <div class="chart-top"><span class="quiet">{title}</span><strong class="mono">{finite(value)?(unit==='ms'?formatMS(value):value.toLocaleString('ja-JP')+(unit?' '+unit:'')):'—'}</strong></div>
  {#if data.path}
    <svg class="chart" viewBox="0 0 580 128" role="img" aria-label={`${title}の履歴。${samples.length}サンプル`} preserveAspectRatio="none"><path class="chart-grid" d="M12 30H568 M12 74H568 M12 118H568"/><path d={data.path} class="chart-line" vector-effect="non-scaling-stroke"/></svg>
    <div class="chart-axis"><span>{time(samples[0]?.time)}</span><span>{time(samples.at(-1)?.time)}</span></div>
  {:else}<div class="chart-empty">履歴を収集中<span>未取得の値は0に変換しません</span></div>{/if}
</div>
