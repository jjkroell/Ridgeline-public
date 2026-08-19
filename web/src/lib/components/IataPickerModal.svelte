<script lang="ts">
	// Searchable list of Canadian IATA codes, opened from the observer setup guide.
	//
	// The code an observer publishes under is just a region label, so what matters
	// is picking something recognisable and nearby rather than the technically
	// closest airstrip. Grouped by province and sorted with the major airports
	// first for that reason.
	import Modal from './Modal.svelte';
	import { CA_IATA, type IataEntry } from '$lib/iata-ca';

	let { onclose, onpick }: { onclose: () => void; onpick?: (code: string) => void } = $props();

	let q = $state('');
	let copied = $state<string | null>(null);

	const matches = $derived.by(() => {
		const needle = q.trim().toLowerCase();
		if (!needle) return CA_IATA;
		// Match on code, city, airport name or province, so "vancouver", "yvr" and
		// "british" all get somewhere useful.
		return CA_IATA.filter(
			(e) =>
				e.code.toLowerCase().includes(needle) ||
				e.city.toLowerCase().includes(needle) ||
				e.name.toLowerCase().includes(needle) ||
				e.region.toLowerCase().includes(needle)
		);
	});

	// Province headings, recomputed against the filtered set so groups that no
	// longer match disappear entirely rather than lingering as empty headers.
	const grouped = $derived.by(() => {
		const g: { region: string; entries: IataEntry[] }[] = [];
		for (const e of matches) {
			const last = g[g.length - 1];
			if (last && last.region === e.region) last.entries.push(e);
			else g.push({ region: e.region, entries: [e] });
		}
		return g;
	});

	async function choose(code: string) {
		onpick?.(code);
		try {
			await navigator.clipboard.writeText(code);
			copied = code;
			setTimeout(() => (copied === code ? (copied = null) : null), 1400);
		} catch {
			/* clipboard blocked — the code is still shown */
		}
	}
</script>

<!-- Fixed height: the list is the whole point of this modal and it shrinks hard
     as you type, so sizing to content makes the panel leap about under the
     pointer between one keystroke and the next.
     Mobile-first: the sheet keeps the full 80vh, where it reads as a normal
     bottom sheet and the screen is small anyway. Desktop halves it — a
     full-height panel there is mostly empty space once a search narrows it. -->
<Modal {onclose} size="2xl" height="h-[80vh] md:h-[40vh]">
	<div class="border-line/70 flex items-center gap-3 border-b px-5 py-4">
		<h2 class="font-display text-fg text-base font-700">Canadian IATA codes</h2>
		<button onclick={onclose} class="label hover:text-signal ml-auto transition-colors">Back</button
		>
	</div>

	<div class="border-line/70 space-y-2 border-b px-5 py-3">
		<!-- svelte-ignore a11y_autofocus -->
		<input
			bind:value={q}
			autofocus
			type="search"
			placeholder="Search city, airport, province or code…"
			class="border-line bg-ink text-fg placeholder:text-fg-faint focus:border-line-bright w-full rounded-[var(--radius)] border px-3 py-2 text-sm outline-none"
		/>
		<div class="text-fg-faint font-mono text-xs">
			{matches.length}
			{matches.length === 1 ? 'airport' : 'airports'}
			{#if q.trim()}matching “{q.trim()}”{/if}
		</div>
	</div>

	<!-- min-h-0 lets this flex child actually shrink so it scrolls, rather than
	     growing past the panel and pushing the footer off. -->
	<div class="min-h-0 flex-1 overflow-y-auto px-5 py-3">
		{#if matches.length === 0}
			<p class="text-fg-faint py-8 text-center text-sm">
				Nothing matches that. Try a city or province name.
			</p>
		{:else}
			{#each grouped as g (g.region)}
				<div class="mb-3">
					<div class="label bg-panel sticky top-0 py-1">{g.region}</div>
					<div class="flex flex-col">
						{#each g.entries as e (e.code)}
							<button
								onclick={() => choose(e.code)}
								class="hover:bg-line/30 flex items-baseline gap-3 rounded-[var(--radius)] px-2 py-1.5 text-left transition-colors"
							>
								<span
									class="font-mono text-sm {e.major ? 'text-signal' : 'text-fg-dim'} w-10 shrink-0"
									>{e.code}</span
								>
								<span class="text-fg min-w-0 flex-1 truncate text-sm">{e.city}</span>
								<span class="text-fg-faint shrink-0 text-xs"
									>{copied === e.code ? 'copied ✓' : e.name === e.city ? '' : e.name}</span
								>
							</button>
						{/each}
					</div>
				</div>
			{/each}
		{/if}
	</div>

	<div class="border-line/70 text-fg-faint border-t px-5 py-3 text-xs leading-relaxed">
		Pick the nearest recognisable airport — it's only a label for grouping your
		observer, so it doesn't have to be the closest airstrip. Selecting one copies
		the code.
	</div>
</Modal>
