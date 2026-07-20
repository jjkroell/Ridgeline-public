<script lang="ts">
	import { onMount } from 'svelte';
	import WidgetShell from './WidgetShell.svelte';
	import { overview } from '$lib/overview.svelte';
	import { api, type Observer } from '$lib/api';
	import { ago } from '$lib/format';

	let { base = '' }: { base?: string } = $props();
	const m = overview.meta('observers')!;

	let observers = $state<Observer[]>([]);

	async function load() {
		try {
			observers = await api.observers();
		} catch {
			/* keep last */
		}
	}
	onMount(() => {
		load();
		const t = setInterval(load, 15000);
		return () => clearInterval(t);
	});

	// Heard within 15 min = online (observers report frequently).
	function online(o: Observer): boolean {
		return o.lastSeen ? Date.now() - +new Date(o.lastSeen) < 15 * 60 * 1000 : false;
	}
	const sorted = $derived([...observers].sort((a, b) => (b.lastSeen ?? '').localeCompare(a.lastSeen ?? '')));
</script>

<WidgetShell title={m.title} icon={m.icon} href="{base}/observers" linkLabel="All →">
	{#if sorted.length === 0}
		<div class="text-fg-faint px-5 py-10 text-center text-sm">No observers yet.</div>
	{:else}
		<div class="divide-line/50 divide-y">
			{#each sorted.slice(0, 8) as o (o.id)}
				<a href="{base}/observers/{encodeURIComponent(o.id)}" class="panel-hover flex items-center gap-3 px-5 py-2.5">
					<span class="h-2 w-2 shrink-0 rounded-full" style="background:{online(o) ? 'var(--color-signal)' : 'var(--color-fg-faint)'}"></span>
					<div class="min-w-0 flex-1">
						<div class="text-fg truncate text-sm font-medium">{o.name ?? o.id}</div>
						<div class="font-mono text-fg-faint mt-0.5 text-[0.68rem]">heard {ago(o.lastSeen)} ago</div>
					</div>
					{#if o.status?.batteryMv}
						<span class="font-mono text-fg-dim shrink-0 text-xs tnum">{(o.status.batteryMv / 1000).toFixed(2)}V</span>
					{/if}
				</a>
			{/each}
		</div>
	{/if}
</WidgetShell>
