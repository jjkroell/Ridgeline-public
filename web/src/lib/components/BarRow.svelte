<script lang="ts">
	// One labelled horizontal bar in a distribution/leaderboard list.
	import { fmtNum } from '$lib/format';
	let { label, count, max, color }: { label: string; count: number; max: number; color: string } = $props();
</script>

<div class="flex items-center gap-3 px-5 py-1.5">
	<div class="text-fg-dim w-28 shrink-0 truncate text-xs">{label}</div>
	<div class="bg-line/40 relative h-3 flex-1 overflow-hidden rounded-[var(--radius)]">
		<div class="h-full rounded-[var(--radius)]" style="width:{(count / max) * 100}%;background:{color}"></div>
	</div>
	<div class="font-mono tnum text-fg-dim w-12 shrink-0 text-right text-xs">{fmtNum(count)}</div>
</div>
