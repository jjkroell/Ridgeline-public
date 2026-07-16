<script lang="ts">
	import WidgetShell from './WidgetShell.svelte';
	import WidgetNodeRow from './WidgetNodeRow.svelte';
	import { overview } from '$lib/overview.svelte';
	import type { Node } from '$lib/api';

	let { base = '', nodes = [] }: { base?: string; nodes?: Node[] } = $props();
	const m = overview.meta('recentnodes')!;
</script>

<WidgetShell title={m.title} icon={m.icon} href="{base}/nodes" linkLabel="All →">
	{#if nodes.length}
		<div class="divide-line/50 divide-y">
			{#each nodes.slice(0, 7) as n (n.publicKey)}
				<WidgetNodeRow node={n} {base} />
			{/each}
		</div>
	{:else}
		<div class="text-fg-faint px-5 py-10 text-center text-sm">No nodes yet.</div>
	{/if}
</WidgetShell>
