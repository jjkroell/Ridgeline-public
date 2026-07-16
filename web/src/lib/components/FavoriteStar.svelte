<script lang="ts">
	import { favorites } from '$lib/favorites.svelte';
	import Tooltip from './Tooltip.svelte';

	interface Props {
		pubkey: string;
		size?: 'sm' | 'md' | 'lg';
		/** Extra classes for the button. */
		class?: string;
	}
	let { pubkey, size = 'md', class: cls = '' }: Props = $props();

	const active = $derived(favorites.has(pubkey));
	const dim = $derived(size === 'sm' ? 'h-4 w-4' : size === 'lg' ? 'h-7 w-7' : 'h-5 w-5');

	// Stop the click from triggering an enclosing link/button (rows are links).
	function toggle(e: MouseEvent) {
		e.preventDefault();
		e.stopPropagation();
		favorites.toggle(pubkey);
	}
</script>

<Tooltip text={active ? 'Remove from favorites' : 'Add to favorites'} class="shrink-0">
	<button
		type="button"
		onclick={toggle}
		aria-pressed={active}
		aria-label={active ? 'Remove from favorites' : 'Add to favorites'}
		class="shrink-0 transition-colors {active ? 'text-amber' : 'text-fg-faint hover:text-amber'} {cls}"
	>
		<svg
			viewBox="0 0 24 24"
			class={dim}
			fill={active ? 'currentColor' : 'none'}
			stroke="currentColor"
			stroke-width="1.6"
			stroke-linecap="round"
			stroke-linejoin="round"
		>
			<path d="M12 2.5l2.9 5.9 6.5.95-4.7 4.6 1.1 6.45L12 17.9l-5.8 3.05 1.1-6.45-4.7-4.6 6.5-.95z" />
		</svg>
	</button>
</Tooltip>
