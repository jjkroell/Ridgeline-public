<script lang="ts">
	// Filter sheet for the Nodes lists, shared by desktop (/nodes) and mobile
	// (/m/nodes). Holds the role choice and the favorites/claimed switches so the
	// list header stays quiet — see NodeFilters for the matching order rules.
	import Modal from './Modal.svelte';
	import { ROLE_OPTIONS, type NodeFilters } from '$lib/node-filters.svelte';
	import type { Node } from '$lib/api';

	let {
		filters,
		nodes,
		onclose
	}: { filters: NodeFilters; nodes: Node[]; onclose: () => void } = $props();

	// Per-role counts over the whole inventory — worth showing because they tell
	// you what's out there before you narrow to it.
	const counts = $derived.by(() => {
		const c: Record<string, number> = { all: nodes.length };
		for (const n of nodes) c[n.role] = (c[n.role] ?? 0) + 1;
		return c;
	});
</script>

<Modal {onclose}>
	<div class="border-line/70 flex items-center gap-3 border-b px-5 py-4">
		<h2 class="font-display text-fg text-base font-700">Filter nodes</h2>
		<button
			onclick={() => filters.clear()}
			disabled={filters.activeCount === 0}
			class="label hover:text-signal ml-auto transition-colors disabled:opacity-40 disabled:hover:text-fg-faint"
			>Clear all</button
		>
	</div>

	<div class="overflow-y-auto px-5 py-4">
		<div class="label mb-2">Role</div>
		<div class="flex flex-col gap-1">
			{#each ROLE_OPTIONS as r (r.key)}
				{@const n = counts[r.key] ?? 0}
				<button
					onclick={() => (filters.role = r.key)}
					class="flex items-center gap-3 rounded-[var(--radius)] border px-3 py-2 text-left text-sm transition-colors
						{filters.role === r.key
						? 'border-signal/50 bg-signal/10 text-signal'
						: 'border-transparent text-fg-dim hover:border-line hover:text-fg'}"
				>
					<span class="flex-1">{r.label}</span>
					<span class="tnum font-mono text-xs {filters.role === r.key ? '' : 'text-fg-faint'}"
						>{n}</span
					>
				</button>
			{/each}
		</div>

		<div class="label mt-5 mb-2">Show only</div>
		<div class="flex flex-col gap-1">
			<label
				class="border-line/60 hover:border-line-bright flex cursor-pointer items-center gap-3 rounded-[var(--radius)] border px-3 py-2.5 transition-colors"
			>
				<input type="checkbox" bind:checked={filters.favOnly} class="accent-amber" />
				<span class="text-fg flex-1 text-sm">Favorites</span>
				<span class="text-fg-faint text-xs">Nodes you've starred</span>
			</label>
			<label
				class="border-line/60 hover:border-line-bright flex cursor-pointer items-center gap-3 rounded-[var(--radius)] border px-3 py-2.5 transition-colors"
			>
				<input type="checkbox" bind:checked={filters.claimedOnly} class="accent-signal" />
				<span class="text-fg flex-1 text-sm">Claimed</span>
				<span class="text-fg-faint text-xs">Yours first, then other operators</span>
			</label>
		</div>
	</div>

	<div class="border-line/70 border-t px-5 py-3">
		<button
			onclick={onclose}
			class="border-signal/40 bg-signal/15 text-signal hover:bg-signal/25 w-full rounded-[var(--radius)] border px-4 py-2.5 text-sm font-600 transition-colors"
			>Done</button
		>
	</div>
</Modal>
