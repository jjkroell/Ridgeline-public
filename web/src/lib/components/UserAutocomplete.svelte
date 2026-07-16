<!--
  Type-ahead picker for a registered Ridgeline user by display name. Calls
  onselect with the chosen {id, displayName}. Debounced; keyboard-navigable.
-->
<script lang="ts">
	import { userSearch, type UserBrief } from '$lib/api';

	interface Props {
		onselect: (user: UserBrief) => void;
		placeholder?: string;
		/** User ids to hide from results (e.g. already-shared users). */
		exclude?: number[];
	}
	let { onselect, placeholder = 'Search by username…', exclude = [] }: Props = $props();

	let query = $state('');
	let results = $state<UserBrief[]>([]);
	let open = $state(false);
	let active = $state(-1);
	let loading = $state(false);
	let timer: ReturnType<typeof setTimeout> | null = null;

	function onInput() {
		if (timer) clearTimeout(timer);
		active = -1;
		const q = query.trim();
		if (q.length < 2) {
			results = [];
			open = false;
			return;
		}
		timer = setTimeout(search, 180);
	}

	async function search() {
		loading = true;
		try {
			const res = await userSearch(query.trim());
			results = res.filter((u) => !exclude.includes(u.id));
			open = true;
		} catch {
			results = [];
		} finally {
			loading = false;
		}
	}

	function pick(u: UserBrief) {
		onselect(u);
		query = '';
		results = [];
		open = false;
		active = -1;
	}

	function onKey(e: KeyboardEvent) {
		if (!open || results.length === 0) return;
		if (e.key === 'ArrowDown') {
			e.preventDefault();
			active = (active + 1) % results.length;
		} else if (e.key === 'ArrowUp') {
			e.preventDefault();
			active = (active - 1 + results.length) % results.length;
		} else if (e.key === 'Enter' && active >= 0) {
			e.preventDefault();
			pick(results[active]);
		} else if (e.key === 'Escape') {
			open = false;
		}
	}
</script>

<div class="relative">
	<input
		type="text"
		bind:value={query}
		oninput={onInput}
		onkeydown={onKey}
		onfocus={() => query.trim().length >= 2 && (open = true)}
		onblur={() => setTimeout(() => (open = false), 150)}
		{placeholder}
		autocomplete="off"
		class="bg-ink-2 text-fg focus:border-signal w-full rounded-[var(--radius)] border border-transparent px-2.5 py-1.5 text-sm outline-none"
	/>
	{#if open}
		<div
			class="border-line bg-ink-2/97 absolute z-20 mt-1 max-h-52 w-full overflow-auto rounded-[var(--radius)] border backdrop-blur-md"
		>
			{#if loading && results.length === 0}
				<p class="text-fg-faint px-3 py-2 text-xs">Searching…</p>
			{:else if results.length === 0}
				<p class="text-fg-faint px-3 py-2 text-xs">No registered users match “{query.trim()}”.</p>
			{:else}
				{#each results as u, i (u.id)}
					<button
						type="button"
						onmousedown={(e) => {
							e.preventDefault();
							pick(u);
						}}
						onmouseenter={() => (active = i)}
						class="flex w-full items-center gap-2 px-3 py-2 text-left transition-colors {i === active
							? 'bg-signal/10'
							: ''}"
					>
						<span
							class="bg-signal/15 text-signal grid h-6 w-6 shrink-0 place-items-center rounded-full text-[0.6rem] font-700"
							>{u.displayName.slice(0, 2).toUpperCase()}</span
						>
						<span class="text-fg truncate text-sm font-600">{u.displayName}</span>
					</button>
				{/each}
			{/if}
		</div>
	{/if}
</div>
