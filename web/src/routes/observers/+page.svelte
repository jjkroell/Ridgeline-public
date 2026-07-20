<script lang="ts">
	import { onMount } from 'svelte';
	import Seo from '$lib/components/Seo.svelte';
	import { api, type Observer, type ObserverCoverage } from '$lib/api';
	import { ago, fmtNum, skewColor, fmtSkew, isFresh } from '$lib/format';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import Tooltip from '$lib/components/Tooltip.svelte';

	let observers = $state<Observer[]>([]);
	let coverage = $state<Record<string, ObserverCoverage>>({});
	let loading = $state(true);

	async function refresh() {
		try {
			const [obs, mesh] = await Promise.all([api.observers(), api.meshAnalytics().catch(() => null)]);
			observers = obs;
			if (mesh) coverage = Object.fromEntries(mesh.observers.map((o) => [o.id, o]));
		} finally {
			loading = false;
		}
	}
	onMount(() => {
		refresh();
		const t = setInterval(refresh, 8000);
		return () => clearInterval(t);
	});
</script>

<Seo
	title="MeshCore Observers — receive-only mesh listeners"
	description="The receive-only observer stations that feed Ridgeline's view of the MeshCore LoRa mesh."
	path="/observers"
/>

<PageHeader eyebrow="Listening Posts" title="Observers">
	<div class="font-mono text-fg-dim text-xs">
		<span class="text-signal tnum">{observers.length}</span> <span class="text-fg-faint">posts</span>
	</div>
</PageHeader>

<div class="px-6 py-6 md:px-10">
	{#if loading}
		<div class="panel text-fg-faint px-5 py-12 text-center text-sm">Loading…</div>
	{:else if observers.length === 0}
		<div class="panel text-fg-faint px-5 py-12 text-center text-sm">
			No observers reporting yet.
		</div>
	{:else}
		<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
			{#each observers as o, i (o.id)}
				{@const cov = coverage[o.id]}
				<a
					href="/observers/{encodeURIComponent(o.id)}"
					class="panel panel-hover rise block px-5 py-4"
					style="animation-delay:{i * 40}ms"
				>
					<div class="flex items-start justify-between">
						<div class="min-w-0">
							<div class="text-fg truncate text-sm font-bold">{o.name ?? o.id}</div>
							{#if o.region}
								<div class="label mt-1">{o.region}</div>
							{/if}
						</div>
						<span class="label shrink-0 {isFresh(o.lastSeen) ? '!text-signal' : '!text-fg-faint'}">
							{isFresh(o.lastSeen) ? 'Reporting' : 'Silent'}
						</span>
					</div>
					<div class="border-line/60 mt-4 flex items-end justify-between border-t pt-3">
						<div>
							<div class="label">Packets</div>
							<div class="font-display tnum text-fg mt-1 text-2xl font-700">
								{fmtNum(o.packetCount)}
							</div>
						</div>
						<div class="text-right">
							<div class="label">Last heard</div>
							<div class="font-mono tnum text-fg-dim mt-1 text-sm">{ago(o.lastSeen)} ago</div>
						</div>
					</div>
					<div class="border-line/60 mt-3 flex items-center justify-between border-t pt-3">
						<Tooltip text="median receive-time deviation from consensus on shared packets (6h) — large = drifting clock">
							<span class="label">Clock skew</span>
						</Tooltip>
						<span class="font-mono tnum text-sm" style="color:{skewColor(cov?.clockSkewMs)}">
							{fmtSkew(cov?.clockSkewMs)}
						</span>
					</div>
				</a>
			{/each}
		</div>
	{/if}
</div>
