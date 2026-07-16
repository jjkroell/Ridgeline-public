<!--
  Live-map audio controls (sound toggle + volume / chord / ring), bound to the
  shared `chime` singleton. Designed to sit inside the map's "Map Control" panel
  (MapRoleFilter's children slot). Used by the WebGL-free live map to reach parity
  with the MapLibre live map's audio controls; because it drives the same singleton
  (same localStorage keys), tuning carries across the WebGL boundary.
-->
<script lang="ts">
	import { onMount } from 'svelte';
	import { chime, CHORDS, RINGS } from '$lib/live-audio.svelte';
	import Tooltip from '$lib/components/Tooltip.svelte';

	onMount(() => chime.load());

	function toggle() {
		chime.ensure(); // within this click gesture so the browser allows audio
		chime.toggle();
	}
</script>

<div class="label mb-1">Audio</div>
<Tooltip
	text={chime.on ? 'Mute node chimes' : 'Play a soft wind chime as pulses reach nodes'}
	class="block w-full"
>
<button
	onclick={toggle}
	aria-pressed={chime.on}
	class="flex w-full items-center justify-center gap-1.5 rounded-[var(--radius)] border px-2 py-1 text-[0.68rem] font-medium transition-colors {chime.on
		? 'border-signal/50 text-signal'
		: 'border-line text-fg-dim hover:text-fg'}"
>
	{#if chime.on}
		<svg viewBox="0 0 24 24" class="h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M11 5 6 9H2v6h4l5 4z" /><path d="M15.5 8.5a5 5 0 0 1 0 7M19 5a9 9 0 0 1 0 14" /></svg>
		<span>Sound on</span>
	{:else}
		<svg viewBox="0 0 24 24" class="h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M11 5 6 9H2v6h4l5 4z" /><path d="M22 9l-6 6M16 9l6 6" /></svg>
		<span>Muted</span>
	{/if}
</button>
</Tooltip>

{#if chime.on}
	<div class="border-line/60 mt-2 space-y-2 border-t pt-2">
		<div>
			<div class="label mb-1 flex items-center justify-between">
				<span>Volume</span><span class="text-fg-faint tnum">{Math.round((chime.volume / 0.8) * 100)}%</span>
			</div>
			<input
				type="range"
				min="0"
				max="0.8"
				step="0.05"
				value={chime.volume}
				oninput={(e) => chime.setVolume(+e.currentTarget.value)}
				onchange={() => chime.preview()}
				class="accent-signal h-1 w-full cursor-pointer"
			/>
		</div>
		<div>
			<div class="label mb-1">Chord</div>
			<div class="grid grid-cols-2 gap-1">
				{#each CHORDS as c (c.id)}
					<button
						onclick={() => {
							chime.setChord(c.id);
							chime.preview();
						}}
						class="rounded-[var(--radius)] border px-1.5 py-1 text-[0.66rem] font-medium transition-colors {chime.chordId ===
						c.id
							? 'border-signal/50 text-signal'
							: 'border-line text-fg-dim hover:text-fg'}">{c.label}</button
					>
				{/each}
			</div>
		</div>
		<div>
			<div class="label mb-1">Ring</div>
			<div class="grid grid-cols-3 gap-1">
				{#each RINGS as r (r.id)}
					<button
						onclick={() => {
							chime.setRing(r.id);
							chime.preview();
						}}
						class="rounded-[var(--radius)] border px-0.5 py-1 text-center text-[0.66rem] font-medium transition-colors {chime.ringId ===
						r.id
							? 'border-signal/50 text-signal'
							: 'border-line text-fg-dim hover:text-fg'}">{r.label}</button
					>
				{/each}
			</div>
		</div>
	</div>
{/if}
