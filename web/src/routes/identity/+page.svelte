<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import HashIdPlanner from '$lib/components/HashIdPlanner.svelte';
</script>

<svelte:head><title>Node Identity · Ridgeline</title></svelte:head>

<PageHeader eyebrow="Provisioning" title="Node Identity">
	{#snippet children()}
		<span class="text-fg-faint max-w-xs text-right text-xs leading-relaxed">
			Inspect hash-ID collisions and mint a MeshCore key with an available one — all in your browser.
		</span>
	{/snippet}
</PageHeader>

<HashIdPlanner />
