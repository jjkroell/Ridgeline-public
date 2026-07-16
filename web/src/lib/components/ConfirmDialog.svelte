<script lang="ts">
	// Renders the confirmer singleton's current dialog. Mount once per root
	// layout. Uses the shared Modal shell for backdrop / Escape / click-outside.
	import Modal from './Modal.svelte';
	import { confirmer } from '$lib/confirm.svelte';

	const s = $derived(confirmer.state);
</script>

{#if s.open}
	<Modal onclose={() => confirmer.cancel()}>
		<div class="px-6 py-5">
			<h2 class="font-display text-fg text-lg font-700">{s.title}</h2>
			{#if s.message}
				<p class="text-fg-dim mt-2 text-sm leading-relaxed whitespace-pre-line">{s.message}</p>
			{/if}
		</div>
		<div class="border-line/70 flex items-center justify-end gap-3 border-t px-6 py-4">
			{#if !s.notice}
				<button
					onclick={() => confirmer.cancel()}
					class="text-fg-dim hover:text-fg rounded-[var(--radius)] px-4 py-2 text-sm font-600 transition-colors"
					>{s.cancelLabel ?? 'Cancel'}</button
				>
			{/if}
			<button
				onclick={() => confirmer.confirm()}
				class="rounded-[var(--radius)] border px-4 py-2 text-sm font-600 transition-colors {s.danger
					? 'border-coral/40 text-coral hover:bg-coral/15'
					: 'border-signal/40 bg-signal/15 text-signal hover:bg-signal/25'}"
				>{s.confirmLabel ?? (s.notice ? 'OK' : 'Confirm')}</button
			>
		</div>
	</Modal>
{/if}
