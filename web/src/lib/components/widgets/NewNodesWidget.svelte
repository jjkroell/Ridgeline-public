<script lang="ts">
	import WidgetShell from './WidgetShell.svelte';
	import WidgetNodeRow from './WidgetNodeRow.svelte';
	import { overview } from '$lib/overview.svelte';
	import { ago } from '$lib/format';
	import type { Node } from '$lib/api';

	let { base = '', nodes = [] }: { base?: string; nodes?: Node[] } = $props();
	const m = overview.meta('newnodes')!;

	const WINDOW_MS = 7 * 24 * 3600 * 1000;
	// Nodes first heard within the last 7 days, newest arrival first.
	const fresh = $derived(
		nodes
			.filter((n) => n.firstSeen && Date.now() - +new Date(n.firstSeen) < WINDOW_MS)
			.sort((a, b) => (b.firstSeen ?? '').localeCompare(a.firstSeen ?? ''))
			.slice(0, 7)
	);
</script>

<WidgetShell title={m.title} icon={m.icon} color="signal" href="{base}/nodes" linkLabel="All →">
	{#if fresh.length}
		<div class="divide-line/50 divide-y">
			{#each fresh as n (n.publicKey)}
				<WidgetNodeRow node={n} {base} sub="first heard {ago(n.firstSeen)} ago" />
			{/each}
		</div>
	{:else}
		<div class="text-fg-faint px-5 py-10 text-center text-sm">No new nodes in the last 7 days.</div>
	{/if}
</WidgetShell>
