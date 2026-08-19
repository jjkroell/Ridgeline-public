<script lang="ts">
	// Standard overlay modal shell: dimmed backdrop, centred panel, Escape-to-close
	// and click-outside-to-close. The caller renders the panel's contents (header,
	// body, etc.) as children. Mount it conditionally — it closes on Escape while
	// mounted, so there's no need to guard the key handler.
	import type { Snippet } from 'svelte';
	let {
		onclose,
		size = 'lg',
		maxWidth,
		closeOnEscape = true,
		height,
		children
	}: {
		onclose: () => void;
		size?: 'lg' | '2xl';
		/** Override the size-derived max-width with an explicit Tailwind class. */
		maxWidth?: string;
		/**
		 * Set false while a modal is stacked on top of this one. Both listen on
		 * window, so a single Escape would otherwise close the whole stack instead
		 * of just the topmost.
		 */
		closeOnEscape?: boolean;
		/**
		 * Fixed panel height (a Tailwind class, e.g. "h-[80vh]"). By default a modal
		 * sizes to its content, which is right for most, but wrong for anything
		 * whose content shrinks as you filter it — the panel jumps around under the
		 * cursor. The max-height cap still applies. A child that should absorb the
		 * slack needs `flex-1 min-h-0` to scroll rather than overflow.
		 */
		height?: string;
		children: Snippet;
	} = $props();
	const maxW = $derived(maxWidth ?? (size === '2xl' ? 'md:max-w-2xl' : 'md:max-w-lg'));
</script>

<svelte:window onkeydown={(e) => e.key === 'Escape' && closeOnEscape && onclose()} />

<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
<div
	class="fixed inset-0 z-50 flex items-end justify-center bg-black/60 p-0 backdrop-blur-sm md:items-center md:p-6"
	onclick={onclose}
	role="dialog"
	aria-modal="true"
	tabindex="-1"
>
	<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
	<div
		class="panel rise flex max-h-[88vh] w-full flex-col {maxW} {height ?? ''}"
		style="animation-duration:.25s"
		onclick={(e) => e.stopPropagation()}
	>
		{@render children()}
	</div>
</div>
