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
		children
	}: {
		onclose: () => void;
		size?: 'lg' | '2xl';
		/** Override the size-derived max-width with an explicit Tailwind class. */
		maxWidth?: string;
		children: Snippet;
	} = $props();
	const maxW = $derived(maxWidth ?? (size === '2xl' ? 'md:max-w-2xl' : 'md:max-w-lg'));
</script>

<svelte:window onkeydown={(e) => e.key === 'Escape' && onclose()} />

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
		class="panel rise flex max-h-[88vh] w-full flex-col {maxW}"
		style="animation-duration:.25s"
		onclick={(e) => e.stopPropagation()}
	>
		{@render children()}
	</div>
</div>
