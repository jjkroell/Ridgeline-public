<script lang="ts">
	import { linkSegments } from '$lib/linkify';

	let { text }: { text: string } = $props();
	const segments = $derived(linkSegments(text));
</script>

<!-- prettier-ignore -->
{#each segments as s}{#if s.href}<a href={s.href} target="_blank" rel="noopener noreferrer nofollow ugc" class="text-signal decoration-signal/40 hover:decoration-signal underline underline-offset-2">{s.text}</a>{:else}{s.text}{/if}{/each}
