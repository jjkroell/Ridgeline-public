<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import HashIdGuide from '$lib/components/HashIdGuide.svelte';
	import Seo from '$lib/components/Seo.svelte';
</script>

<Seo
	title="Multi-byte Hash IDs"
	description="Why MeshCore meshes outgrow 1-byte path IDs, and how to move a repeater, room server or companion to 2- or 3-byte hash IDs."
	path="/hash-ids"
/>

<PageHeader eyebrow="Guide" title="Multi-byte Hash IDs">
	{#snippet children()}
		<a
			href="/identity"
			class="border-line text-fg-dim hover:border-line-bright hover:text-fg rounded-[var(--radius)] border px-3 py-1.5 text-sm transition-colors"
			>Open the planner →</a
		>
	{/snippet}
</PageHeader>

<HashIdGuide />
