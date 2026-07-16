<script lang="ts">
	import { onMount } from 'svelte';
	import { api, type MeshAnalytics } from '$lib/api';
	import TopologyGraph from '$lib/components/TopologyGraph.svelte';

	let windowSec = $state(21600);
	const windows = [
		{ label: '1h', sec: 3600 },
		{ label: '6h', sec: 21600 },
		{ label: '24h', sec: 86400 }
	];
	let data = $state<MeshAnalytics | null>(null);
	let loading = $state(true);

	async function refresh() {
		try {
			data = await api.meshAnalytics(windowSec);
		} catch {
			/* keep last */
		} finally {
			loading = false;
		}
	}
	// Reload on window change; poll while mounted (graph only remounts on a window
	// change, so an in-progress pan/zoom survives the 30s refresh).
	$effect(() => {
		windowSec;
		refresh();
	});
	onMount(() => {
		const t = setInterval(refresh, 30000);
		return () => clearInterval(t);
	});

	const topo = $derived(data?.topology);
	const hasTopo = $derived(!!topo && topo.nodes.length > 0 && topo.edges.length > 0);
</script>

<div class="flex h-full flex-col">
	<!-- window + counts -->
	<div class="border-line/70 flex shrink-0 items-center gap-2 border-b px-4 py-2.5">
		{#each windows as w (w.sec)}
			<button
				onclick={() => (windowSec = w.sec)}
				class="rounded-full border px-3 py-1 text-xs font-600 {windowSec === w.sec
					? 'border-signal/50 bg-signal/15 text-signal'
					: 'border-line text-fg-dim'}">{w.label}</button
			>
		{/each}
		{#if hasTopo && topo}
			<span class="text-fg-faint ml-auto font-mono text-[0.62rem] tnum"
				>{topo.nodes.length} relays · {topo.edges.length} links</span
			>
		{/if}
	</div>

	<!-- graph fills the rest of the viewport -->
	<div class="relative min-h-0 flex-1">
		{#if loading && !data}
			<div class="text-fg-faint flex h-full items-center justify-center text-sm">Loading…</div>
		{:else if !hasTopo || !topo}
			<div class="text-fg-faint flex h-full items-center justify-center px-8 text-center text-sm">
				Not enough multi-hop relaying in this window to map the backbone — try a longer window.
			</div>
		{:else}
			{#key windowSec}
				<TopologyGraph nodes={topo.nodes} edges={topo.edges} nodePath="/m/nodes" />
			{/key}
		{/if}
	</div>
</div>
