<script lang="ts">
	import WidgetShell from './WidgetShell.svelte';
	import WidgetNodeRow from './WidgetNodeRow.svelte';
	import { overview } from '$lib/overview.svelte';
	import type { Node } from '$lib/api';

	let { base = '', nodes = [] }: { base?: string; nodes?: Node[] } = $props();
	const m = overview.meta('backbone')!;

	// Busiest relays right now — ranked by packets relayed in the last hour.
	const top = $derived(
		nodes
			.filter((n) => (n.relayCount1h ?? 0) > 0)
			.sort((a, b) => (b.relayCount1h ?? 0) - (a.relayCount1h ?? 0))
			.slice(0, 7)
	);
</script>

<WidgetShell title={m.title} icon={m.icon} href="{base}/analytics" linkLabel="Analytics →">
	{#if top.length}
		<div class="divide-line/50 divide-y">
			{#each top as n (n.publicKey)}
				<WidgetNodeRow node={n} {base} sub="relaying now">
					{#snippet trailing(node: Node)}
						<span class="font-mono text-signal bg-signal/10 shrink-0 rounded-[var(--radius)] px-2 py-0.5 text-xs tnum">
							{node.relayCount1h}/hr
						</span>
					{/snippet}
				</WidgetNodeRow>
			{/each}
		</div>
	{:else}
		<div class="text-fg-faint px-5 py-10 text-center text-sm">No relay activity in the last hour.</div>
	{/if}
</WidgetShell>
