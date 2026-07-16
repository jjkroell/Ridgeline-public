<script lang="ts">
	import Tooltip from './Tooltip.svelte';

	// `tip` gates the hover tooltip — off where the full type is already obvious
	// from surrounding context (e.g. inside the packet modal header).
	let { type, tip = true }: { type: string; tip?: boolean } = $props();

	const colors: Record<string, string> = {
		Advert: 'var(--color-signal)',
		TextMessage: 'var(--color-sky)',
		GroupText: 'var(--color-violet)',
		Trace: 'var(--color-amber)',
		Ack: 'var(--color-fg-dim)',
		Request: 'var(--color-lime)',
		Response: 'var(--color-lime)',
		Path: 'var(--color-fg-dim)'
	};
	const color = $derived(colors[type] ?? 'var(--color-fg-faint)');

	// Short labels to keep the badge (and the feed's Type column) compact.
	// Full name stays available on hover via the title attribute.
	const abbr: Record<string, string> = {
		Advert: 'Adv',
		TextMessage: 'DM',
		GroupText: 'Grp Msg',
		Request: 'Req',
		Response: 'Resp',
		Control: 'Ctrl',
		AnonRequest: 'AnonRqst'
	};
	const label = $derived(abbr[type] ?? type);
</script>

{#snippet tag()}
	<span
		class="font-mono rounded-[var(--radius)] px-1.5 py-0.5 text-[0.66rem] tracking-wide whitespace-nowrap"
		style="color:{color}; background:color-mix(in srgb, {color} 10%, transparent)"
	>
		{label}
	</span>
{/snippet}

{#if tip}
	<Tooltip text={type}>{@render tag()}</Tooltip>
{:else}
	{@render tag()}
{/if}
