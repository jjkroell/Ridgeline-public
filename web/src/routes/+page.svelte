<script lang="ts">
	import { onMount } from 'svelte';
	import Seo from '$lib/components/Seo.svelte';
	import { api, type Stats, type Node } from '$lib/api';
	import { live } from '$lib/live.svelte';
	import { ago, fmtNum } from '$lib/format';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import { announce } from '$lib/announce.svelte';
	import { overview } from '$lib/overview.svelte';
	import OverviewGrid from '$lib/components/widgets/OverviewGrid.svelte';
	import OverviewCustomizer from '$lib/components/widgets/OverviewCustomizer.svelte';

	let stats = $state<Stats | null>(null);
	let nodes = $state<Node[]>([]);
	let error = $state<string | null>(null);

	async function refresh() {
		try {
			[stats, nodes] = await Promise.all([api.stats(), api.nodes()]);
			error = null;
		} catch (e) {
			error = (e as Error).message;
		}
	}

	onMount(() => {
		refresh();
		const t = setInterval(refresh, 5000);
		return () => clearInterval(t);
	});

	const cards = $derived([
		{ label: 'Nodes', value: stats?.nodes ?? 0, accent: true, icon: 'node' },
		{ label: 'Observers', value: stats?.observers ?? 0, accent: false, icon: 'eye' },
		{ label: 'Observations', value: stats?.observations ?? 0, accent: false, icon: 'pulse' },
		{ label: 'Session Events', value: live.total, accent: false, icon: 'bolt' }
	]);

	const icons: Record<string, string> = {
		node: 'M12 8a4 4 0 100 8 4 4 0 000-8zM12 2v4m0 12v4M2 12h4m12 0h4',
		eye: 'M2 12s4-7 10-7 10 7 10 7-4 7-10 7-10-7-10-7z',
		pulse: 'M2 12h4l3 8 4-16 3 8h6',
		bolt: 'M13 2 4 14h7l-1 8 9-12h-7z'
	};
</script>

<Seo
	title="Ridgeline — Live MeshCore Mesh Observatory"
	description="Live dashboard for a MeshCore LoRa mesh — nodes, repeaters, coverage and packets in real time."
	path="/"
/>

<PageHeader eyebrow="Network Observatory" title="Overview">
	<button
		onclick={() => (overview.editing = !overview.editing)}
		class="flex items-center gap-2 rounded-[var(--radius)] border px-3 py-2 text-sm font-600 transition-colors {overview.editing
			? 'border-signal/50 text-signal bg-signal/10'
			: 'border-line text-fg-dim hover:border-signal/50 hover:text-signal'}"
	>
		<svg viewBox="0 0 24 24" class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">
			<path d="M4 6h10M4 12h7M4 18h13M16 4v4M20 10v4M12 16v4" />
		</svg>
		Customize
	</button>
	<button
		onclick={() => announce.show()}
		class="border-line text-fg-dim hover:border-signal/50 hover:text-signal flex items-center gap-2 rounded-[var(--radius)] border px-3 py-2 text-sm font-600 transition-colors"
	>
		<svg viewBox="0 0 24 24" class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">
			<path d="M12 2a7 7 0 0 0-4 12.7V17h8v-2.3A7 7 0 0 0 12 2zM9 21h6M10 17v4m4-4v4" />
		</svg>
		What's new
	</button>
	<div class="font-mono text-fg-dim flex items-center gap-2 text-xs">
		<span class="text-fg-faint">LAST PACKET</span>
		<span class="text-signal tnum">{ago(stats?.lastPacketAt)} ago</span>
	</div>
</PageHeader>

<div class="px-6 py-6 md:px-10">
	{#if error}
		<div class="panel border-coral/40 text-coral mb-6 px-4 py-3 text-sm">
			Can't reach the daemon — {error}
		</div>
	{/if}

	<!-- Fixed stat row -->
	<div class="grid grid-cols-2 gap-3 lg:grid-cols-4">
		{#each cards as c, i (c.label)}
			<div class="panel rise relative overflow-hidden px-5 py-5" style="animation-delay:{i * 50}ms">
				{#if c.accent}
					<div class="from-signal/[0.07] absolute inset-0 bg-gradient-to-br to-transparent"></div>
				{/if}
				<div class="relative">
					<div class="label flex items-center justify-between">
						{c.label}
						<svg viewBox="0 0 24 24" class="h-4 w-4 {c.accent ? 'text-signal' : 'text-fg-faint'}" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d={icons[c.icon]} /></svg>
					</div>
					<div class="font-display tnum mt-3 text-4xl font-700 {c.accent ? 'text-signal glow-signal' : 'text-fg'}">
						{fmtNum(c.value)}
					</div>
				</div>
			</div>
		{/each}
	</div>

	<!-- Customizable dashboard -->
	{#if overview.editing}
		<OverviewCustomizer />
	{/if}
	<OverviewGrid base="" {nodes} {stats} />
</div>
