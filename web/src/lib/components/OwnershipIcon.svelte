<!--
  A small ownership indicator shown next to a node name: a teal shield-check for
  a node you own, a faint shield-clock for a pending claim, and a blue "share"
  glyph (three connected dots) for a node shared with you. Shared by the My Nodes
  widget and the account page so the indicators read the same everywhere.
-->
<script lang="ts">
	import Tooltip from './Tooltip.svelte';

	let { kind, sharedBy = '' }: { kind: 'owned' | 'pending' | 'shared'; sharedBy?: string } = $props();

	const label = $derived(
		kind === 'owned'
			? 'Owned by you'
			: kind === 'pending'
				? 'Your claim is pending'
				: sharedBy
					? `Shared with you by ${sharedBy}`
					: 'Shared with you'
	);
	const color = $derived(
		kind === 'owned' ? 'text-signal' : kind === 'pending' ? 'text-fg-faint' : 'text-sky'
	);
</script>

<Tooltip text={label} class="shrink-0">
	{#if kind === 'shared'}
		<svg viewBox="0 0 24 24" class="{color} h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-label={label}>
			<circle cx="18" cy="5" r="3" /><circle cx="6" cy="12" r="3" /><circle cx="18" cy="19" r="3" />
			<line x1="8.6" y1="13.5" x2="15.4" y2="17.5" /><line x1="15.4" y1="6.5" x2="8.6" y2="10.5" />
		</svg>
	{:else if kind === 'owned'}
		<svg viewBox="0 0 24 24" class="{color} h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-label={label}>
			<path d="M12 3 4 6v6c0 4.4 3.2 7.6 8 9 4.8-1.4 8-4.6 8-9V6l-8-3z" /><path d="m9 12 2 2 4-4" />
		</svg>
	{:else}
		<svg viewBox="0 0 24 24" class="{color} h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-label={label}>
			<path d="M12 3 4 6v6c0 4.4 3.2 7.6 8 9 4.8-1.4 8-4.6 8-9V6l-8-3z" /><path d="M12 9v3l2 1" />
		</svg>
	{/if}
</Tooltip>
