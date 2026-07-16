<script lang="ts">
	import { onMount } from 'svelte';
	import { api, type Observer } from '$lib/api';
	import { ago, fmtNum, isFresh } from '$lib/format';

	let observers = $state<Observer[]>([]);

	async function refresh() {
		try {
			observers = await api.observers();
		} catch {
			/* keep last */
		}
	}
	onMount(() => {
		refresh();
		const t = setInterval(refresh, 15000);
		return () => clearInterval(t);
	});

	function radioLine(o: Observer): string {
		const s = o.status;
		if (!s) return '';
		const p: string[] = [];
		if (s.freqMhz != null) p.push(`${+s.freqMhz.toFixed(3)}`);
		if (s.spreadingFactor != null) p.push(`SF${s.spreadingFactor}`);
		return p.join(' · ');
	}
</script>

<div class="px-4 py-4">
	<div class="text-fg-faint mb-2 px-1 font-mono text-[0.62rem]">{observers.length} listening posts</div>
	<div class="flex flex-col gap-3">
		{#each observers as o (o.id)}
			{@const reporting = isFresh(o.lastSeen)}
			<a href="/m/observers/{encodeURIComponent(o.id)}" class="border-line/60 bg-panel active:bg-line/40 rounded-2xl border px-4 py-3.5">
				<div class="flex items-center gap-2.5">
					<span class="h-2.5 w-2.5 shrink-0 rounded-full" style="background:{reporting ? 'var(--color-signal)' : 'var(--color-fg-faint)'}"></span>
					<span class="text-fg min-w-0 flex-1 truncate text-sm font-600">{o.id}</span>
					{#if o.region}<span class="label !text-[0.55rem] shrink-0">{o.region}</span>{/if}
				</div>
				<div class="text-fg-faint mt-2 flex items-center gap-2 font-mono text-[0.62rem]">
					<span class={reporting ? 'text-signal' : ''}>{reporting ? 'Reporting' : 'Silent'}</span>
					<span>· {fmtNum(o.packetCount)} pkts</span>
					<span>· {ago(o.lastSeen)}</span>
				</div>
				{#if radioLine(o)}
					<div class="text-fg-faint mt-1 font-mono text-[0.62rem]">{radioLine(o)}{#if o.status?.noiseFloor != null} · noise {o.status.noiseFloor.toFixed(0)}dBm{/if}</div>
				{/if}
			</a>
		{/each}
	</div>
</div>
