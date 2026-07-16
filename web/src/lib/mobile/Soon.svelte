<script lang="ts">
	let { name, desc = '' }: { name: string; desc?: string } = $props();
</script>

<div class="flex min-h-[60vh] flex-col items-center justify-center px-8 text-center">
	<svg viewBox="0 0 32 32" class="mb-5 h-12 w-12 opacity-80" aria-hidden="true">
		<path d="M3 23 11 11 16 17 22 7 29 23" fill="none" stroke="var(--color-signal)" stroke-width="2.2" stroke-linejoin="round" stroke-linecap="round" />
		<path d="M3 27 11 17 16 22 22 14 29 27" fill="none" stroke="var(--color-amber)" stroke-width="1.4" stroke-linejoin="round" stroke-linecap="round" opacity="0.6" />
	</svg>
	<h2 class="font-display text-fg text-lg font-700">{name}</h2>
	<p class="text-fg-faint mt-2 max-w-xs text-sm">{desc || 'This screen is being built for mobile — coming in the next pass.'}</p>
</div>
