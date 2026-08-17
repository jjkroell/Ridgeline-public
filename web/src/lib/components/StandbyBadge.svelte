<!--
  The "on standby" marker for an observer that is still connected but is having
  every packet it publishes discarded at ingest.

  Amber on purpose: this is an operator-chosen state that needs to be noticed
  (a stand-down left on by accident silently loses data), but it is not an error
  — coral is reserved for destructive/failed states like a blocked or deleted
  observer. Renders nothing when the observer is in service, so call sites can
  drop it in unconditionally.
-->
<script lang="ts">
	import type { Observer } from '$lib/api';
	import { ago, fmtNum } from '$lib/format';
	import Tooltip from '$lib/components/Tooltip.svelte';

	let { observer, compact = false }: { observer: Observer | null; compact?: boolean } = $props();

	const dropped = $derived(observer?.standbyDropped ?? 0);
	const tip = $derived(
		`Connected, but every packet it reports is being discarded — on standby for ${ago(observer!.standbySince!)}.` +
			(dropped > 0 ? ` ${fmtNum(dropped)} discarded since the daemon started.` : '')
	);
</script>

{#if observer?.standbySince}
	<Tooltip text={tip}>
		<span
			class="border-amber/50 bg-amber/10 text-amber inline-flex shrink-0 items-center gap-1 rounded-[var(--radius)] border px-1.5 py-0.5 font-mono text-[0.6rem] font-600 tracking-wide uppercase"
		>
			<span class="bg-amber inline-block h-1.5 w-1.5 rounded-full"></span>
			Standby{#if !compact && dropped > 0}<span class="tnum opacity-70">· {fmtNum(dropped)} dropped</span>{/if}
		</span>
	</Tooltip>
{/if}
