<script lang="ts">
	import Tooltip from './Tooltip.svelte';

	// The packet's routing mode. The distinction that matters operationally is
	// scoped vs unscoped FLOOD: a region-scoped mesh expects TRANSPORT_FLOOD, and
	// a repeater running `flood.max.unscoped 0` forwards no plain FLOOD at all.
	// Seeing which is which in the feed is how you catch a node forwarding
	// traffic its config should be dropping.
	let { type, tip = true }: { type: string; tip?: boolean } = $props();

	const colors: Record<string, string> = {
		// Amber: the one worth noticing on a scoped mesh.
		Flood: 'var(--color-amber)',
		TransportFlood: 'var(--color-signal)',
		Direct: 'var(--color-fg-dim)',
		TransportDirect: 'var(--color-sky)'
	};
	const abbr: Record<string, string> = {
		Flood: 'FLOOD',
		TransportFlood: 'T·FLOOD',
		Direct: 'DIRECT',
		TransportDirect: 'T·DIRECT'
	};
	const tips: Record<string, string> = {
		Flood: 'Unscoped flood — broadcast mesh-wide with no region scope. On a mesh using region scoping a repeater set to flood.max.unscoped 0 should not forward these.',
		TransportFlood: 'Transport flood — a flood carrying a region scope, so repeaters can limit how far it travels.',
		Direct: 'Direct — addressed along a known return path rather than broadcast.',
		TransportDirect: 'Transport direct — addressed, carrying a region scope.'
	};

	const color = $derived(colors[type] ?? 'var(--color-fg-faint)');
	const label = $derived(abbr[type] ?? type);
</script>

{#snippet tag()}
	<span
		class="font-mono rounded-[var(--radius)] px-1.5 py-0.5 text-[0.6rem] tracking-wide whitespace-nowrap"
		style="color:{color}; background:color-mix(in srgb, {color} 10%, transparent)"
	>
		{label}
	</span>
{/snippet}

{#if tip}
	<Tooltip text={tips[type] ?? type}>{@render tag()}</Tooltip>
{:else}
	{@render tag()}
{/if}
