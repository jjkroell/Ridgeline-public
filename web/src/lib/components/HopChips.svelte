<script lang="ts">
	// A packet's relay path rendered as role-coloured chips. Each hop prefix is
	// resolved to a located node (linking to its page) or shown as a dashed,
	// unresolved chip. `showIds` displays the node's hash id instead of its name.
	import type { Node } from '$lib/api';
	import { shortKey, roleColor } from '$lib/format';
	import Tooltip from './Tooltip.svelte';

	let {
		hops,
		nodes,
		showIds = false,
		onnavigate
	}: { hops: string[]; nodes: Node[]; showIds?: boolean; onnavigate?: () => void } = $props();

	const resolveHop = (hop: string): Node | undefined => nodes.find((n) => n.publicKey.startsWith(hop));
	const hashId = (n: Node) => n.publicKey.slice(0, Math.max(1, n.hashSize || 2) * 2);
</script>

<div class="flex flex-wrap items-center gap-1.5">
	{#each hops as hop, i (i)}
		{#if i > 0}<span class="text-fg-faint">→</span>{/if}
		{@const n = resolveHop(hop)}
		{#if n}
			<Tooltip text={showIds ? n.name || shortKey(n.publicKey) : hashId(n)}>
				<a
					href="/nodes/{n.publicKey}"
					onclick={onnavigate}
					class="border-line bg-panel-2/60 hover:border-signal/50 rounded-[var(--radius)] border px-2 py-1 text-xs"
					style="color:{roleColor(n.role)}">{showIds ? hashId(n) : n.name || shortKey(n.publicKey)}</a
				>
			</Tooltip>
		{:else}
			<Tooltip text="no located node with this key prefix"
				><span
					class="border-line/60 font-mono text-fg-faint rounded-[var(--radius)] border border-dashed px-2 py-1 text-xs"
					>{hop}</span
				></Tooltip
			>
		{/if}
	{/each}
</div>
