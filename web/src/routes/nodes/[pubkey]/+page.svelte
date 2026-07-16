<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { api, type Node } from '$lib/api';
	import { shortKey } from '$lib/format';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import RoleBadge from '$lib/components/RoleBadge.svelte';
	import FavoriteStar from '$lib/components/FavoriteStar.svelte';
	import NodeDetail from '$lib/components/NodeDetail.svelte';

	const pubkey = $derived(page.params.pubkey ?? '');
	let allNodes = $state<Node[]>([]);
	let loading = $state(true);

	const node = $derived(allNodes.find((n) => n.publicKey === pubkey) ?? null);

	async function refresh() {
		try {
			allNodes = await api.nodes();
		} finally {
			loading = false;
		}
	}
	onMount(() => {
		refresh();
		const t = setInterval(refresh, 8000);
		return () => clearInterval(t);
	});
</script>

<PageHeader eyebrow="Node Detail" title={node?.name || shortKey(pubkey, 8, 4)}>
	{#snippet titleLeft()}<FavoriteStar {pubkey} size="md" />{/snippet}
	{#if node}<RoleBadge role={node.role} />{/if}
	<a href="/nodes" class="label hover:text-signal transition-colors">← All nodes</a>
</PageHeader>

<div class="px-6 py-6 md:px-10">
	{#if loading}
		<div class="panel text-fg-faint px-5 py-12 text-center text-sm">Loading…</div>
	{:else}
		<NodeDetail {pubkey} nodes={allNodes} />
	{/if}
</div>
