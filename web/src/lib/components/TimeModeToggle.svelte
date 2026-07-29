<script lang="ts">
	// Segmented control for the live feed's timestamp format. Shared so the
	// desktop toolbar and the mobile feed header stay in step.
	import { timeMode, type TimeMode } from '$lib/time-mode.svelte';
	import Tooltip from '$lib/components/Tooltip.svelte';

	let { compact = false }: { compact?: boolean } = $props();

	const OPTS: { mode: TimeMode; label: string; hint: string }[] = [
		{ mode: 'relative', label: 'Ago', hint: 'Time since the packet was heard' },
		{ mode: 'clock', label: 'Clock', hint: 'Local wall-clock time it was heard' }
	];

	const pad = $derived(compact ? 'px-2 py-1' : 'px-2.5 py-1.5');
	const size = $derived(compact ? 'text-[0.62rem]' : 'text-xs');
</script>

<div
	class="border-line inline-flex shrink-0 items-center overflow-hidden rounded-[var(--radius)] border"
	role="group"
	aria-label="Timestamp format"
>
	{#each OPTS as o (o.mode)}
		{@const on = timeMode.mode === o.mode}
		<Tooltip text={o.hint} class="flex">
			<button
				type="button"
				onclick={() => timeMode.set(o.mode)}
				aria-pressed={on}
				class="{pad} {size} font-mono transition-colors {on
					? 'bg-signal/12 text-signal'
					: 'text-fg-faint hover:text-fg-dim'}"
			>
				{o.label}
			</button>
		</Tooltip>
	{/each}
</div>
