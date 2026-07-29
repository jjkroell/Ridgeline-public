<script lang="ts">
	// Shared theme picker — desktop sidebar and the mobile More sheet render the
	// same swatch row. Each swatch previews the theme it selects: the square is
	// that theme's page ground, the dot its accent, so the choice reads without
	// applying it.
	import { theme, THEMES, type ThemeId } from '$lib/theme.svelte';
	import Tooltip from '$lib/components/Tooltip.svelte';

	let { compact = false }: { compact?: boolean } = $props();

	const box = $derived(compact ? 'h-7 w-7' : 'h-6 w-6');
	const dot = $derived(compact ? 'h-2.5 w-2.5' : 'h-2 w-2');

	function pick(id: ThemeId) {
		theme.set(id);
	}
</script>

<div class="flex flex-col gap-1.5">
	<!-- .label is tracked out 0.14em, which leaves a trailing gap after the last
	     glyph; the matching indent cancels it so the text optically centres. -->
	<span class="label {compact ? '' : 'text-center [text-indent:0.14em]'}">Theme</span>
	<!-- Centred in the desktop sidebar, where the row is narrower than the column.
	     Left-aligned when compact: the mobile sheet is full-width, so centring
	     would strand the swatches away from their label. -->
	<div
		class="flex items-center gap-1.5 {compact ? '' : 'justify-center'}"
		role="radiogroup"
		aria-label="Colour theme"
	>
		{#each THEMES as t (t.id)}
			<Tooltip text={t.label}>
				<button
					type="button"
					role="radio"
					aria-checked={theme.id === t.id}
					aria-label={t.label}
					onclick={() => pick(t.id)}
					class="{box} grid place-items-center rounded-[var(--radius)] border transition-transform hover:scale-110"
					style="background:{t.swatch[0]}; border-color:{t.swatch[1]}; {theme.id === t.id
						? 'outline:2px solid var(--color-signal); outline-offset:1px;'
						: ''}"
				>
					<span class="{dot} rounded-full" style="background:{t.swatch[2]}"></span>
				</button>
			</Tooltip>
		{/each}
	</div>
</div>
