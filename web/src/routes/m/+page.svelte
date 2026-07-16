<script lang="ts">
	import { onMount } from 'svelte';
	import { api, type Stats, type Node } from '$lib/api';
	import { live } from '$lib/live.svelte';
	import { ago, fmtNum } from '$lib/format';
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

	const kpis = $derived([
		{ label: 'Nodes', value: stats?.nodes ?? 0, accent: true },
		{ label: 'Observers', value: stats?.observers ?? 0, accent: false },
		{ label: 'Packets', value: stats?.observations ?? 0, accent: false },
		{ label: 'Session', value: live.total, accent: false }
	]);
</script>

<div class="px-4 py-4">
	<div class="mb-3 flex items-center gap-2">
		<div class="text-fg-faint flex items-center gap-2 font-mono text-[0.68rem]">
			<span>LAST PACKET</span>
			<span class="text-signal tnum">{ago(stats?.lastPacketAt)} ago</span>
		</div>
		<button
			onclick={() => (overview.editing = !overview.editing)}
			class="ml-auto flex items-center gap-1.5 rounded-full border px-3 py-1.5 text-xs font-600 transition-colors {overview.editing
				? 'border-signal/50 text-signal bg-signal/10'
				: 'border-line text-fg-dim'}"
		>
			<svg viewBox="0 0 24 24" class="h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
				<path d="M4 6h10M4 12h7M4 18h13M16 4v4M20 10v4M12 16v4" />
			</svg>
			Customize
		</button>
	</div>

	{#if error}
		<div class="border-coral/40 bg-coral/5 text-coral mb-4 rounded-xl border px-4 py-3 text-sm">
			Can't reach the daemon — {error}
		</div>
	{/if}

	<!-- Fixed KPI tiles -->
	<div class="grid grid-cols-2 gap-3">
		{#each kpis as k (k.label)}
			<div class="border-line/60 bg-panel relative overflow-hidden rounded-2xl border px-4 py-4">
				{#if k.accent}<div class="from-signal/[0.08] absolute inset-0 bg-gradient-to-br to-transparent"></div>{/if}
				<div class="relative">
					<div class="label">{k.label}</div>
					<div class="font-display tnum mt-1.5 text-3xl font-700 {k.accent ? 'text-signal glow-signal' : 'text-fg'}">{fmtNum(k.value)}</div>
				</div>
			</div>
		{/each}
	</div>

	<!-- Customizable dashboard -->
	{#if overview.editing}
		<OverviewCustomizer />
	{/if}
	<OverviewGrid base="/m" {nodes} {stats} />
</div>
