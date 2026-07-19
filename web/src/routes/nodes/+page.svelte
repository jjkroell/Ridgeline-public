<script lang="ts">
	import { onMount } from 'svelte';
	import Seo from '$lib/components/Seo.svelte';
	import { api, type Node } from '$lib/api';
	import { shortKey, fmtCoord, fmtNum, nodeStatus, ago, lastHeard } from '$lib/format';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import RoleBadge from '$lib/components/RoleBadge.svelte';
	import Tooltip from '$lib/components/Tooltip.svelte';
	import FavoriteStar from '$lib/components/FavoriteStar.svelte';
	import ClaimedBadge from '$lib/components/ClaimedBadge.svelte';
	import NodeFilterModal from '$lib/components/NodeFilterModal.svelte';
	import { favorites } from '$lib/favorites.svelte';
	import { NodeFilters } from '$lib/node-filters.svelte';

	const GPS_WARNING =
		'GPS coordinates appear corrupt — a statistical outlier versus the rest of the mesh, so this node is hidden from the maps. The node is otherwise valid and still appears in packet paths.';

	let nodes = $state<Node[]>([]);
	let loading = $state(true);
	const filters = new NodeFilters();
	let filterOpen = $state(false);

	async function refresh() {
		try {
			nodes = await api.nodes();
		} finally {
			loading = false;
		}
	}
	onMount(() => {
		refresh();
		const t = setInterval(refresh, 8000);
		return () => clearInterval(t);
	});

	// Filtering + ordering live in NodeFilters so /nodes and /m/nodes agree.
	// NOTE: read `sorted` directly everywhere — aliasing a $derived with a plain
	// `const` captures its value at init and never updates.
	const sorted = $derived(filters.apply(nodes));
</script>

<Seo
	title="MeshCore Node &amp; Repeater Directory"
	description="Every node and repeater on the MeshCore LoRa mesh — status, location and last-heard."
	path="/nodes"
/>

<PageHeader eyebrow="Mesh Inventory" title="Nodes">
	<div class="font-mono text-fg-dim text-xs">
		<span class="text-signal tnum">{fmtNum(sorted.length)}</span>
		<span class="text-fg-faint"> / {fmtNum(nodes.length)} shown</span>
	</div>
</PageHeader>

<div class="px-6 py-6 md:px-10">
	<!-- Controls: the table is the instrument, so these stay quiet — an underlined
	     search field and one filter control that names what's constraining the list. -->
	<div class="border-line/60 mb-5 flex items-center gap-5 border-b pb-2.5">
		<div class="focus-within:border-signal/60 border-line/0 flex flex-1 items-center gap-2 border-b pb-0.5 transition-colors">
			<svg
				viewBox="0 0 24 24"
				class="text-fg-faint h-4 w-4 shrink-0"
				fill="none"
				stroke="currentColor"
				stroke-width="1.6"
				stroke-linecap="round"><circle cx="11" cy="11" r="7" /><path d="m21 21-4.3-4.3" /></svg
			>
			<input
				bind:value={filters.query}
				placeholder="Search name or key"
				class="font-mono placeholder:text-fg-faint min-w-0 flex-1 bg-transparent text-sm outline-none"
			/>
			{#if filters.query}
				<button
					onclick={() => (filters.query = '')}
					aria-label="Clear search"
					class="text-fg-faint hover:text-fg text-lg leading-none transition-colors">×</button
				>
			{/if}
		</div>
		<button
			onclick={() => (filterOpen = true)}
			class="flex shrink-0 items-center gap-2 text-xs transition-colors
				{filters.activeCount ? 'text-signal' : 'text-fg-dim hover:text-fg'}"
		>
			<svg
				viewBox="0 0 24 24"
				class="h-4 w-4"
				fill="none"
				stroke="currentColor"
				stroke-width="1.6"
				stroke-linecap="round"
				stroke-linejoin="round"><path d="M3 5h18M7 12h10M11 19h2" /></svg
			>
			<span class="font-mono">{filters.summary || 'Filter'}</span>
		</button>
	</div>

	<!-- Table -->
	<div class="panel overflow-hidden">
		<div
			class="label border-line/70 grid grid-cols-[1fr_auto_auto_auto] gap-4 border-b px-5 py-3 md:grid-cols-[1.4fr_120px_1fr_80px_70px_70px]"
		>
			<span>Node</span>
			<span class="hidden md:block">Role</span>
			<span class="hidden md:block">Location</span>
			<span class="text-center">
				<Tooltip text="Advert transmissions (re-flood and multi-observer copies of one advert collapsed)"
					>Adverts</Tooltip
				>
			</span>
			<span class="text-center">Heard</span>
			<span class="text-center">Status</span>
		</div>

		{#if loading}
			<div class="text-fg-faint px-5 py-12 text-center text-sm">Loading…</div>
		{:else if sorted.length === 0}
			<div class="px-5 py-12 text-center">
				<p class="text-fg-dim text-sm">
					{#if filters.favOnly && !favorites.count}
						No favorites yet. Star a node to keep it at the top of this list.
					{:else if filters.claimedOnly}
						No claimed nodes match. Claim one from its node page to see it here.
					{:else}
						No nodes match these filters.
					{/if}
				</p>
				{#if filters.activeCount}
					<button
						onclick={() => filters.clear()}
						class="text-signal mt-2 text-xs hover:underline">Clear filters</button
					>
				{/if}
			</div>
		{:else}
			<div class="divide-line/50 divide-y">
				{#each sorted as n (n.publicKey)}
					{@const st = nodeStatus(n)}
					{@const heard = lastHeard(n)}
					<a
						href="/nodes/{n.publicKey}"
						class="panel-hover grid grid-cols-[1fr_auto_auto_auto] items-center gap-4 px-5 py-3 md:grid-cols-[1.4fr_120px_1fr_80px_70px_70px] {n.gpsSuspect
							? 'border-amber/60 bg-amber/[0.07] border-l-2'
							: ''}"
					>
						<div class="min-w-0">
							<div class="flex items-center gap-1.5">
								<Tooltip text={st.label} class="shrink-0"><span class="h-2 w-2 rounded-full" style="background:{st.color}"></span></Tooltip>
								<FavoriteStar pubkey={n.publicKey} size="sm" />
								{#if n.gpsSuspect}
									<Tooltip text={GPS_WARNING}>
										<svg viewBox="0 0 24 24" class="text-amber h-4 w-4 shrink-0" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
											<path d="M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z" />
											<path d="M12 9v4M12 17h.01" />
										</svg>
									</Tooltip>
								{/if}
								<span class="text-fg truncate font-medium">{n.name || shortKey(n.publicKey)}</span>
								{#if n.claimed}<ClaimedBadge pubkey={n.publicKey} size="md" />{/if}
							</div>
							<div class="font-mono text-fg-faint mt-0.5 text-[0.68rem]">
								{shortKey(n.publicKey, 10, 4)}
							</div>
						</div>
						<div class="hidden md:block"><RoleBadge role={n.role} /></div>
						<div class="font-mono text-fg-dim hidden text-xs md:block tnum">
							{fmtCoord(n.latitude, n.longitude)}
						</div>
						<div class="font-mono tnum text-fg-dim text-center text-sm">{n.advertTxCount}</div>
						<div class="font-mono tnum text-fg-dim text-center text-xs">{ago(heard)}</div>
						<div class="font-mono tnum text-center text-xs" style="color:{st.color}">{st.label}</div>
					</a>
				{/each}
			</div>
		{/if}
	</div>
</div>

{#if filterOpen}
	<NodeFilterModal {filters} {nodes} onclose={() => (filterOpen = false)} />
{/if}
