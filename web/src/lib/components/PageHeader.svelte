<script lang="ts">
	interface Props {
		eyebrow: string;
		title: string;
		/** Optional adornment rendered inline to the left of the title (e.g. a star). */
		titleLeft?: import('svelte').Snippet;
		children?: import('svelte').Snippet;
	}
	let { eyebrow, title, titleLeft, children }: Props = $props();
</script>

<header class="flex flex-wrap items-end justify-between gap-4 px-6 py-6 md:px-10">
	<div class="rise">
		<div class="label mb-2 flex items-center gap-2">
			<span class="bg-signal/70 inline-block h-px w-6"></span>
			{eyebrow}
		</div>
		<div class="flex items-center gap-3">
			{#if titleLeft}{@render titleLeft()}{/if}
			<h1 class="font-display text-fg text-3xl font-700 tracking-tight md:text-4xl">{title}</h1>
		</div>
	</div>
	{#if children}
		<div class="rise flex items-center gap-3" style="animation-delay:60ms">
			{@render children()}
		</div>
	{/if}
</header>
