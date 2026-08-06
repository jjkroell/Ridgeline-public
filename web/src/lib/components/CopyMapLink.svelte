<script lang="ts">
	import { copyText } from '$lib/map-share';
	import Tooltip from './Tooltip.svelte';

	interface Props {
		/** Builds the URL at click time, so it always reflects the live camera. */
		url: () => string;
		/** Compact styling for the mobile app bar. */
		compact?: boolean;
	}
	let { url, compact = false }: Props = $props();

	let state = $state<'idle' | 'ok' | 'fail'>('idle');
	let timer: ReturnType<typeof setTimeout> | null = null;

	async function copy() {
		const ok = await copyText(url());
		state = ok ? 'ok' : 'fail';
		if (timer) clearTimeout(timer);
		timer = setTimeout(() => (state = 'idle'), 2200);
	}

	const label = $derived(state === 'ok' ? 'Copied' : state === 'fail' ? 'Press ⌘C' : 'Copy link');
</script>

<Tooltip text="Copy a link to this exact view — centre, zoom, basemap and filters">
	<button
		type="button"
		onclick={copy}
		class="border-line/70 bg-panel/90 text-fg-dim hover:text-fg hover:border-line flex items-center gap-1.5 rounded-lg border backdrop-blur transition-colors {compact
			? 'px-2 py-1 text-[0.65rem]'
			: 'px-2.5 py-1.5 text-[0.7rem]'}"
		aria-label="Copy a link to this map view"
	>
		{#if state === 'ok'}
			<svg viewBox="0 0 24 24" class="h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="2.5">
				<path d="M20 6 9 17l-5-5" stroke-linecap="round" stroke-linejoin="round" />
			</svg>
		{:else}
			<svg viewBox="0 0 24 24" class="h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="2">
				<rect x="9" y="9" width="12" height="12" rx="2" />
				<path d="M5 15V5a2 2 0 0 1 2-2h10" stroke-linecap="round" />
			</svg>
		{/if}
		<span class="font-mono">{label}</span>
	</button>
</Tooltip>
