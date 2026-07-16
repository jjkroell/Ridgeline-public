<script lang="ts">
	// The six-up KPI tile strip used by the analytics and observer pages. Each
	// item is a headline number; `accent` adds the signal-tinted treatment, an
	// explicit `color` overrides the value colour, and `hint` shows an ⓘ tooltip.
	import Tooltip from './Tooltip.svelte';

	export interface Kpi {
		label: string;
		value: string;
		accent?: boolean;
		color?: string;
		hint?: string;
	}
	let { items }: { items: Kpi[] } = $props();
</script>

<div class="grid grid-cols-2 gap-3 md:grid-cols-3 lg:grid-cols-6">
	{#each items as c, i (c.label)}
		<div class="panel rise relative overflow-hidden px-4 py-4" style="animation-delay:{i * 40}ms">
			{#if c.accent}
				<div class="from-signal/[0.07] absolute inset-0 bg-gradient-to-br to-transparent"></div>
			{/if}
			<div class="relative">
				<div class="label flex items-center gap-1">
					{c.label}
					{#if c.hint}
						<Tooltip text={c.hint}><span class="text-fg-faint cursor-help text-[0.6rem]">ⓘ</span></Tooltip>
					{/if}
				</div>
				<div
					class="font-display tnum mt-2 text-2xl font-700 lg:text-3xl {c.color
						? ''
						: c.accent
							? 'text-signal glow-signal'
							: 'text-fg'}"
					style={c.color ? `color:${c.color}` : ''}
				>
					{c.value}
				</div>
			</div>
		</div>
	{/each}
</div>
