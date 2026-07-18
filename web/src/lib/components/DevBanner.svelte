<script lang="ts">
	// Prominent "this is not the live site" strip, shown only when the server's
	// /api/health reports a non-production environment (see $lib/env). Rendered
	// in-flow at the very top of the desktop and mobile shells so it can never be
	// missed and never overlaps page content. Not dismissible — that's the point.
	import { env } from '$lib/env.svelte';
	import { SITE_NAME } from '$lib/site';
</script>

{#if env.isDev}
	<div
		class="bg-amber text-ink flex shrink-0 flex-wrap items-center justify-center gap-x-2 gap-y-0.5 px-3 py-1 text-center text-[0.72rem] leading-tight font-700"
		role="alert"
		aria-live="polite"
	>
		<span
			class="bg-ink/20 rounded px-1.5 py-0.5 text-[0.6rem] font-900 tracking-[0.15em] uppercase"
			>{env.label}</span
		>
		<span>Test environment — <strong class="font-900">not</strong> the live {SITE_NAME} site. Data
			here may be reset without notice.</span
		>
	</div>
{/if}
