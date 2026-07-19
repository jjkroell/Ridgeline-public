<script lang="ts">
	import { onMount } from 'svelte';
	import { api, type Node } from '$lib/api';
	import { ago, shortKey, nodeStatus, roleColor, roleLabel, lastHeard } from '$lib/format';
	import { favorites } from '$lib/favorites.svelte';
	import ClaimedBadge from '$lib/components/ClaimedBadge.svelte';
	import NodeFilterModal from '$lib/components/NodeFilterModal.svelte';
	import { NodeFilters } from '$lib/node-filters.svelte';

	let nodes = $state<Node[]>([]);
	const filters = new NodeFilters();
	let filterOpen = $state(false);

	async function refresh() {
		try {
			nodes = await api.nodes();
		} catch {
			/* keep last */
		}
	}
	onMount(() => {
		refresh();
		const t = setInterval(refresh, 10000);
		return () => clearInterval(t);
	});

	// Filtering + ordering live in NodeFilters so /nodes and /m/nodes agree.
	const filtered = $derived(filters.apply(nodes));
</script>

<div class="px-4 py-4">
	<!-- Controls: quiet by design — search plus one control that names the active filters. -->
	<div class="border-line bg-panel mb-2 flex items-center gap-2 rounded-xl border px-3 py-2.5">
		<svg viewBox="0 0 24 24" class="text-fg-faint h-4 w-4 shrink-0" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round"><circle cx="11" cy="11" r="7" /><path d="M21 21l-4-4" /></svg>
		<input bind:value={filters.query} placeholder="Search nodes…" class="text-fg placeholder:text-fg-faint min-w-0 flex-1 bg-transparent text-sm outline-none" />
		{#if filters.query}<button onclick={() => (filters.query = '')} aria-label="Clear search" class="text-fg-faint text-lg leading-none">×</button>{/if}
	</div>

	<div class="mb-3 flex items-center gap-3 px-1">
		<button
			onclick={() => (filterOpen = true)}
			class="flex items-center gap-1.5 text-xs {filters.activeCount ? 'text-signal' : 'text-fg-dim'}"
		>
			<svg viewBox="0 0 24 24" class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M3 5h18M7 12h10M11 19h2" /></svg>
			<span class="font-mono">{filters.summary || 'Filter'}</span>
		</button>
	</div>

	<div class="text-fg-faint mb-2 px-1 font-mono text-[0.62rem]">{filtered.length} node{filtered.length === 1 ? '' : 's'}</div>

	<!-- List -->
	<div class="border-line/60 bg-panel divide-line/50 divide-y overflow-hidden rounded-2xl border">
		{#if filtered.length === 0}
			<div class="px-4 py-10 text-center">
				<p class="text-fg-dim text-sm">
					{#if filters.favOnly && !favorites.count}
						No favorites yet. Star a node to keep it at the top.
					{:else if filters.claimedOnly}
						No claimed nodes match.
					{:else}
						No nodes match these filters.
					{/if}
				</p>
				{#if filters.activeCount}
					<button onclick={() => filters.clear()} class="text-signal mt-2 text-xs">Clear filters</button>
				{/if}
			</div>
		{:else}
			{#each filtered as n (n.publicKey)}
				{@const st = nodeStatus(n)}
				{@const heard = lastHeard(n)}
				<a href="/m/nodes/{n.publicKey}" class="active:bg-line/40 flex items-center gap-3 px-4 py-3">
					<span class="h-2.5 w-2.5 shrink-0 rounded-full" style="background:{st.color}"></span>
					<div class="min-w-0 flex-1">
						<div class="flex items-center gap-1.5">
							<span class="text-fg truncate text-sm font-medium">{n.name || shortKey(n.publicKey)}</span>
							{#if n.claimed}<ClaimedBadge pubkey={n.publicKey} />{/if}
						</div>
						<div class="mt-0.5 flex items-center gap-1.5 font-mono text-[0.62rem]">
							<span style="color:{roleColor(n.role)}">{roleLabel(n.role)}</span>
							<span class="text-fg-faint">· {st.label} · {ago(heard)}</span>
						</div>
					</div>
					<button
						onclick={(e) => {
							e.preventDefault();
							favorites.toggle(n.publicKey);
						}}
						class="shrink-0 p-1.5"
						aria-label="Toggle favorite"
					>
						<svg viewBox="0 0 24 24" class="h-5 w-5 {favorites.has(n.publicKey) ? 'text-amber' : 'text-fg-faint'}" fill={favorites.has(n.publicKey) ? 'currentColor' : 'none'} stroke="currentColor" stroke-width="1.5"><path d="M12 2.5l2.9 5.9 6.5.95-4.7 4.6 1.1 6.45L12 17.9l-5.8 3.05 1.1-6.45-4.7-4.6 6.5-.95z" /></svg>
					</button>
				</a>
			{/each}
		{/if}
	</div>
</div>

{#if filterOpen}
	<NodeFilterModal {filters} {nodes} onclose={() => (filterOpen = false)} />
{/if}
