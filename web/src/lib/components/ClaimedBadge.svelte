<!--
  The public "claimed" shield shown on node lists. It's teal (signal) when the
  signed-in user owns the node and violet when it's claimed by someone else, so
  your own nodes stand out. Signed out, nothing is "yours" so every claimed node
  shows the violet "claimed by another" colour.
-->
<script lang="ts">
	import { auth } from '$lib/auth.svelte';
	import Tooltip from './Tooltip.svelte';

	let { pubkey, size = 'sm' }: { pubkey: string; size?: 'sm' | 'md' } = $props();

	const mine = $derived(auth.ownsNode(pubkey));
	const sz = $derived(size === 'md' ? 'h-4 w-4' : 'h-3.5 w-3.5');
</script>

<Tooltip text={mine ? 'Claimed by you' : 'Claimed by another operator'} class="shrink-0">
	<svg
		viewBox="0 0 24 24"
		class="{mine ? 'text-signal' : 'text-violet'} {sz}"
		fill="none"
		stroke="currentColor"
		stroke-width="1.8"
		stroke-linecap="round"
		stroke-linejoin="round"
		aria-label={mine ? 'Claimed by you' : 'Claimed'}
	>
		<path d="M12 3 4 6v6c0 4.4 3.2 7.6 8 9 4.8-1.4 8-4.6 8-9V6l-8-3z" /><path d="m9 12 2 2 4-4" />
	</svg>
</Tooltip>
