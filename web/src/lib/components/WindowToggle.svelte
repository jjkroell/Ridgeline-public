<script lang="ts">
	// Time-window selector (e.g. 6h / 24h / 3d) bound to a seconds value.
	interface Option {
		label: string;
		sec: number;
	}
	let { options, value = $bindable() }: { options: Option[]; value: number } = $props();
</script>

<div class="bg-panel border-line flex overflow-hidden rounded-[var(--radius)] border text-xs">
	{#each options as o (o.sec)}
		<button
			onclick={() => (value = o.sec)}
			class="font-mono px-3 py-1.5 transition-colors {value === o.sec
				? 'bg-signal/15 text-signal'
				: 'text-fg-dim hover:text-fg'}">{o.label}</button
		>
	{/each}
</div>
