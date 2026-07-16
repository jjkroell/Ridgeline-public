<script lang="ts">
	import { onMount } from 'svelte';
	import Seo from '$lib/components/Seo.svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import WindowToggle from '$lib/components/WindowToggle.svelte';
	import TopologyGraph from '$lib/components/TopologyGraph.svelte';
	import { api, type MeshAnalytics } from '$lib/api';

	const windows = [
		{ label: '1h', sec: 3600, bucket: 2 },
		{ label: '6h', sec: 21600, bucket: 10 },
		{ label: '24h', sec: 86400, bucket: 30 }
	];
	let windowSec = $state(21600);
	let bucketMin = $derived(windows.find((w) => w.sec === windowSec)?.bucket ?? 10);
	let data = $state<MeshAnalytics | null>(null);
	let error = $state<string | null>(null);
	let loading = $state(true);

	async function refresh() {
		try {
			data = await api.meshAnalytics(windowSec, bucketMin);
			error = null;
		} catch (e) {
			error = (e as Error).message;
		} finally {
			loading = false;
		}
	}
	// Reload on window change; poll while mounted (the graph only remounts on a
	// window change, so panning/zoom survives the 30s refresh).
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

<Seo
	title="Topology — Ridgeline"
	description="Interactive mesh topology for the MeshCore network — the relay backbone of nodes that hand off traffic to one another, inferred from observed flood paths."
	path="/topology"
/>

<PageHeader eyebrow="Network Observatory" title="Topology">
	<WindowToggle options={windows} bind:value={windowSec} />
</PageHeader>

<div class="px-6 py-6 md:px-10">
	{#if error}
		<div class="panel border-coral/40 text-coral mb-6 px-4 py-3 text-sm">
			Can't reach the daemon — {error}
		</div>
	{/if}

	<section class="panel rise flex h-[calc(100dvh-13rem)] min-h-[480px] flex-col overflow-hidden">
		<div class="border-line/70 flex items-center gap-3 border-b px-5 py-3.5">
			<h2 class="font-display text-fg text-sm font-700 tracking-wide">MESH TOPOLOGY — RELAY BACKBONE</h2>
			<span class="font-mono text-fg-faint text-[0.68rem]">node ↔ node hand-offs · size = tx forwarded</span>
			{#if hasTopo && topo}
				<span class="font-mono text-fg-faint ml-auto text-[0.68rem]"
					>{topo.nodes.length} relays · {topo.edges.length} links</span
				>
			{/if}
		</div>
		<div class="relative flex-1 overflow-hidden">
			{#if loading && !data}
				<div class="text-fg-faint flex h-full items-center justify-center text-sm">Loading…</div>
			{:else if !hasTopo || !topo}
				<div class="text-fg-faint flex h-full items-center justify-center px-6 text-center text-sm">
					Not enough multi-hop relaying in this window to map the backbone — try a longer window.
				</div>
			{:else}
				{#key windowSec}
					<TopologyGraph nodes={topo.nodes} edges={topo.edges} />
				{/key}
			{/if}
		</div>
	</section>

	<p class="text-fg-faint mt-3 text-center font-mono text-[0.62rem]">
		Inferred from observed flood paths — who hands off to whom across the mesh. Window {data?.windowHours?.toFixed(
			1
		) ?? '—'}h · updates every 30s.
	</p>
</div>
