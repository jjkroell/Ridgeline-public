<script lang="ts">
	import { basemap } from '$lib/basemap.svelte';
	import { BASEMAPS } from '$lib/map-basemap';

	// `compact` trims the trigger to an icon-only chip (phone layouts). `posClass`
	// places the control; the dropdown opens downward, left-aligned.
	let { compact = false, posClass = 'top-3 right-3' }: { compact?: boolean; posClass?: string } = $props();

	let open = $state(false);
	const current = $derived(BASEMAPS.find((b) => b.id === basemap.id) ?? BASEMAPS[0]);

	function pick(id: string) {
		basemap.set(id);
		open = false;
	}
</script>

<div class="absolute {posClass} z-10">
	<button
		onclick={() => (open = !open)}
		class="border-line bg-ink-2/85 hover:bg-panel-2/70 flex items-center gap-2 rounded-[var(--radius)] border backdrop-blur-md transition-colors {compact ? 'p-2.5' : 'w-[176px] max-w-[60vw] px-3 py-2'}"
		aria-label="Base map: {current.label}"
	>
		<svg viewBox="0 0 24 24" class="shrink-0 {compact ? 'text-fg h-6 w-6' : 'text-fg-dim h-4 w-4'}" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">
			<path d="M12 2 2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5" />
		</svg>
		{#if !compact}
			<span class="min-w-0 flex-1 text-left">
				<span class="label block leading-none">Base map</span>
				<span class="text-fg mt-0.5 block truncate text-xs font-600">{current.label}</span>
			</span>
			<svg viewBox="0 0 24 24" class="text-fg-faint ml-auto h-3.5 w-3.5 shrink-0 transition-transform {open ? 'rotate-180' : ''}" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M6 9l6 6 6-6" /></svg>
		{/if}
	</button>

	{#if open}
		<div class="border-line bg-ink-2/95 rise mt-1 w-44 overflow-hidden rounded-[var(--radius)] border backdrop-blur-md">
			{#each BASEMAPS as b (b.id)}
				{@const on = b.id === basemap.id}
				<button
					onclick={() => pick(b.id)}
					class="hover:bg-panel-2/60 flex w-full items-center gap-2 px-3 py-2 text-left transition-colors {on ? 'bg-signal/10' : ''}"
				>
					<span class="h-1.5 w-1.5 shrink-0 rounded-full" style="background:{on ? 'var(--color-signal)' : 'var(--color-fg-faint)'}"></span>
					<span class="min-w-0 flex-1">
						<span class="block truncate text-xs font-600 {on ? 'text-signal' : 'text-fg'}">{b.label}</span>
						<span class="text-fg-faint mt-0.5 block truncate text-[0.62rem] leading-tight">{b.desc}</span>
					</span>
				</button>
			{/each}
		</div>
	{/if}
</div>
