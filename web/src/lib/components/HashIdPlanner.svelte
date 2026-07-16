<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/state';
	import { api, type Node } from '$lib/api';
	import { shortKey } from '$lib/format';
	import {
		type HashByteLen,
		analyzeCollisions,
		cohortCounts,
		usedPrefixes,
		prefixStatus,
		suggestFreePrefix,
		nodePrefix,
		isPathNode
	} from '$lib/hash-ids';
	import { keygen } from '$lib/keygen.svelte';
	import Tooltip from '$lib/components/Tooltip.svelte';
	import RoleBadge from '$lib/components/RoleBadge.svelte';

	let { compact = false }: { compact?: boolean } = $props();

	let nodes = $state<Node[]>([]);
	let loading = $state(true);
	let byteLen = $state<HashByteLen>(2);
	let prefix = $state(''); // uppercase hex typed by the user
	let copied = $state<string | null>(null);
	// Deep-link target: /identity?len=2&id=AB12 (e.g. from a node's "+N collision"
	// pill) preselects the cohort and highlights that hash ID's collision group.
	let highlightPrefix = $state('');

	onMount(async () => {
		const len = parseInt(page.url.searchParams.get('len') ?? '', 10);
		if (len === 1 || len === 2 || len === 3) byteLen = len;
		highlightPrefix = (page.url.searchParams.get('id') ?? '').toUpperCase();
		try {
			nodes = await api.nodes();
		} catch {
			/* keep empty; collisions just show nothing */
		} finally {
			loading = false;
		}
	});

	// Scroll the highlighted collision group into view once it has rendered.
	function highlightScroll(node: HTMLElement, active: boolean) {
		if (active) requestAnimationFrame(() => node.scrollIntoView({ block: 'center' }));
	}
	onDestroy(() => keygen.cancel());

	const cohorts = $derived(cohortCounts(nodes));
	const analysis = $derived(analyzeCollisions(nodes, byteLen));
	const groups = $derived(analysis.genuine);
	const artifacts = $derived(analysis.artifacts);
	const used = $derived(usedPrefixes(nodes, byteLen));
	const space = $derived(1 << (8 * byteLen)); // 256 / 65536 / 16,777,216
	const status = $derived(prefixStatus(nodes, byteLen, prefix));
	const want = $derived(byteLen * 2);

	// Same-length path nodes occupying the typed prefix (shown when it's in use).
	const occupants = $derived(
		status === 'used'
			? nodes.filter(
					(n) =>
						isPathNode(n) && n.hashSize === byteLen && nodePrefix(n, byteLen) === prefix.toUpperCase()
				)
			: []
	);

	// Average number of keys to try for a full vanity match at this length.
	const expectedKeys = $derived(space);

	// Only surface a finished key when it actually matches the current target.
	const result = $derived(
		keygen.result &&
			prefix.length === want &&
			keygen.result.publicKey.startsWith(prefix.toUpperCase())
			? keygen.result
			: null
	);

	function setLen(l: HashByteLen) {
		if (byteLen === l) return;
		byteLen = l;
		prefix = '';
		keygen.cancel();
	}

	function onPrefixInput(e: Event) {
		const raw = (e.target as HTMLInputElement).value;
		prefix = raw
			.replace(/[^0-9a-fA-F]/g, '')
			.slice(0, want)
			.toUpperCase();
		if (keygen.running) keygen.cancel();
	}

	function suggest() {
		const p = suggestFreePrefix(nodes, byteLen);
		if (p) {
			prefix = p;
			keygen.cancel();
		}
	}

	function generate() {
		if (status !== 'free') return;
		keygen.start(prefix);
	}

	async function copy(label: string, text: string) {
		try {
			await navigator.clipboard.writeText(text);
			copied = label;
			setTimeout(() => (copied === label ? (copied = null) : null), 1200);
		} catch {
			/* clipboard blocked */
		}
	}

	function download() {
		if (!result) return;
		const json = JSON.stringify(
			{ public_key: result.publicKey, private_key: result.privateKey },
			null,
			2
		);
		const blob = new Blob([json], { type: 'application/json' });
		const url = URL.createObjectURL(blob);
		const a = document.createElement('a');
		a.href = url;
		a.download = `meshcore_${result.publicKey.slice(0, want)}.json`;
		a.click();
		URL.revokeObjectURL(url);
	}

	function fmtInt(n: number): string {
		return Math.round(n).toLocaleString();
	}
	function eta(): string {
		const remaining = Math.max(0, expectedKeys - keygen.attempts);
		if (keygen.rate <= 0) return '—';
		const s = remaining / keygen.rate;
		if (s < 1) return '<1s';
		if (s < 90) return `~${Math.round(s)}s`;
		if (s < 5400) return `~${Math.round(s / 60)}m`;
		return `~${(s / 3600).toFixed(1)}h`;
	}

	const statusText = $derived<Record<string, string>>({
		empty: `Enter ${want} hex characters`,
		incomplete: `${want - prefix.length} more hex character${want - prefix.length === 1 ? '' : 's'}`,
		invalid: 'Not valid hex',
		reserved: 'Reserved by MeshCore (00 / FF)',
		used: `In use by another ${byteLen}-byte node`,
		free: 'Available'
	});
	const statusColor: Record<string, string> = {
		empty: 'text-fg-faint',
		incomplete: 'text-fg-dim',
		invalid: 'text-coral',
		reserved: 'text-amber',
		used: 'text-coral',
		free: 'text-signal'
	};
</script>

<div class="space-y-4 {compact ? 'px-4 py-4' : 'px-6 py-6 md:px-10'}">
	<!-- Length selector (= which ID-length cohort) -->
	<div class="panel rise px-5 py-4">
		<div class="label mb-3">Hash ID length</div>
		<div class="flex gap-2">
			{#each [1, 2, 3] as const as l (l)}
				<button
					onclick={() => setLen(l as HashByteLen)}
					class="flex-1 rounded-[var(--radius)] border px-3 py-2.5 text-sm font-medium transition-colors
						{byteLen === l
						? 'border-signal/60 bg-signal/10 text-signal'
						: 'border-line text-fg-dim hover:border-line-bright hover:text-fg'}"
				>
					{l}-byte
					<span class="ml-1 font-mono text-xs {byteLen === l ? 'text-signal/70' : 'text-fg-faint'}">
						{#if !loading}{cohorts[l]} node{cohorts[l] === 1 ? '' : 's'}{:else}{l * 2} hex{/if}
					</span>
				</button>
			{/each}
		</div>
		<div class="text-fg-faint mt-3 text-xs leading-relaxed">
			Each node is configured for a {byteLen}-byte ID (the first {byteLen}
			byte{byteLen > 1 ? 's' : ''} of its key) and can only collide with other {byteLen}-byte nodes.
			{#if !loading}
				<span class="text-fg-dim tnum">{used.size.toLocaleString()} of {space.toLocaleString()}</span
				>
				IDs are taken by the {cohorts[byteLen].toLocaleString()}
				routing node{cohorts[byteLen] === 1 ? '' : 's'} at this length.{#if cohorts.unknown}
					<span class="text-fg-dim"
						>{cohorts.unknown} node{cohorts.unknown === 1 ? '' : 's'} haven't advertised a length yet.</span
					>{/if}
				Companions are excluded — they don't repeat packets, so they never appear in a path.
			{/if}
		</div>
	</div>

	<!-- Genuine collisions -->
	<div class="panel rise px-5 py-4" style="animation-delay:40ms">
		<div class="mb-1 flex items-center justify-between gap-3">
			<div class="label">Collisions among {byteLen}-byte nodes</div>
			<span class="text-fg-faint font-mono text-xs tnum">
				{groups.length} group{groups.length === 1 ? '' : 's'}
			</span>
		</div>
		<div class="text-fg-faint mb-3 text-xs">
			Only nodes configured at {byteLen} byte{byteLen > 1 ? 's' : ''} are compared, and records with
			corrupted keys are filtered out below.
		</div>
		{#if loading}
			<div class="text-fg-faint py-4 text-sm">Loading nodes…</div>
		{:else if groups.length === 0}
			<div class="text-fg-dim py-2 text-sm">
				No two {byteLen}-byte nodes share an ID — every {byteLen}-byte hash ID in the mesh is unique.
			</div>
		{:else}
			<div class="space-y-2.5 {compact ? '' : 'max-h-[22rem] overflow-y-auto pr-1'}">
				{#each groups as g (g.prefix)}
					{@const isTarget = !!highlightPrefix && g.prefix === highlightPrefix}
					<div
						use:highlightScroll={isTarget}
						class="rounded-[var(--radius)] border px-3 py-2.5 transition-colors
							{isTarget ? 'border-signal/60 ring-signal/30 bg-signal/5 ring-1' : 'border-line'}"
					>
						<div class="mb-1.5 flex items-center gap-2">
							<span
								class="bg-coral/10 text-coral rounded-[var(--radius)] px-1.5 py-0.5 font-mono text-sm font-700 tracking-wider"
								>{g.prefix}</span
							>
							<span class="text-fg-faint text-xs">{g.nodes.length} nodes</span>
						</div>
						<ul class="space-y-1">
							{#each g.nodes as n (n.publicKey)}
								<li class="flex items-center gap-2 text-sm">
									<span class="text-fg min-w-0 truncate">{n.name || '(unnamed)'}</span>
									<span class="shrink-0"><RoleBadge role={n.role} /></span>
									{#if !n.hasLocation}
										<Tooltip text="No GPS location broadcast" class="shrink-0">
											<span
												class="text-fg-faint border-line rounded-[var(--radius)] border px-1.5 py-0.5 text-[0.6rem] tracking-wide uppercase"
												>no GPS</span
											>
										</Tooltip>
									{/if}
									<button
										onclick={() => copy(n.publicKey, n.publicKey)}
										class="text-fg-faint hover:text-signal ml-auto shrink-0 font-mono text-xs"
									>
										<Tooltip text="Copy full public key"
											>{copied === n.publicKey ? 'copied' : shortKey(n.publicKey, 6, 4)}</Tooltip
										>
									</button>
								</li>
							{/each}
						</ul>
					</div>
				{/each}
			</div>
		{/if}
	</div>

	<!-- Corruption artifacts (false positives filtered from collisions) -->
	{#if artifacts.length}
		<div class="panel rise px-5 py-4" style="animation-delay:60ms">
			<div class="mb-1 flex items-center justify-between gap-3">
				<div class="label !text-amber">Suspected corruption artifacts</div>
				<span class="text-fg-faint font-mono text-xs tnum">{artifacts.length}</span>
			</div>
			<div class="text-fg-faint mb-3 text-xs leading-relaxed">
				Phantom records from packet corruption — a real node's advert arrived with a damaged public
				key. These are <span class="text-fg-dim">not</span> real collisions. The
				<span class="text-fg-dim">true node</span> (more adverts / matching name) is shown for each.
			</div>
			<div class="space-y-2 {compact ? '' : 'max-h-[20rem] overflow-y-auto pr-1'}">
				{#each artifacts as a (a.node.publicKey)}
					<div class="border-line/70 rounded-[var(--radius)] border px-3 py-2.5">
						<div class="flex items-center gap-2">
							<span class="text-fg-dim truncate text-sm">{a.node.name || '(unnamed)'}</span>
							<span
								class="rounded-[var(--radius)] px-1.5 py-0.5 text-[0.6rem] font-700 tracking-wider
								{a.confidence === 'high' ? 'bg-amber/10 text-amber' : 'bg-fg-faint/10 text-fg-faint'}"
								>{a.confidence}</span
							>
							<button
								onclick={() => copy(a.node.publicKey, a.node.publicKey)}
								class="text-fg-faint hover:text-amber ml-auto shrink-0 font-mono text-xs"
							>
								<Tooltip text="Copy phantom public key"
									>{copied === a.node.publicKey
										? 'copied'
										: shortKey(a.node.publicKey, 6, 4)}</Tooltip
								>
							</button>
						</div>
						<div class="text-fg-faint mt-1 text-xs">
							corrupted copy of <span class="text-signal">{a.canonical.name || 'a real node'}</span>
							<span class="text-fg-dim">· {a.reason}</span>
						</div>
					</div>
				{/each}
			</div>
		</div>
	{/if}

	<!-- Pick an unused hash ID + Generate key pair — side by side on desktop -->
	<div class="grid items-stretch gap-4 {compact ? '' : 'md:grid-cols-2'}">
	<!-- Pick an unused hash ID -->
	<div class="panel rise px-5 py-4" style="animation-delay:80ms">
		<div class="label mb-3">Pick an unused hash ID</div>
		<div class="flex items-center gap-2">
			<input
				value={prefix}
				oninput={onPrefixInput}
				spellcheck="false"
				autocapitalize="characters"
				placeholder={'0'.repeat(want)}
				class="border-line bg-ink focus:border-signal/60 min-w-0 flex-1 rounded-[var(--radius)] border px-3 py-2.5 font-mono text-lg tracking-[0.2em] uppercase outline-none"
			/>
			<button
				onclick={suggest}
				class="border-line text-fg-dim hover:border-line-bright hover:text-fg shrink-0 rounded-[var(--radius)] border px-3 py-2.5 text-sm whitespace-nowrap"
			>
				Suggest free
			</button>
		</div>
		<div class="mt-2 flex items-center gap-2 text-sm">
			<span
				class="inline-block h-2 w-2 rounded-full
				{status === 'free' ? 'bg-signal' : status === 'used' ? 'bg-coral' : status === 'reserved' ? 'bg-amber' : 'bg-fg-faint'}"
			></span>
			<span class={statusColor[status]}>{statusText[status]}</span>
		</div>
		{#if occupants.length}
			<div class="text-fg-faint mt-1.5 text-xs">
				Used by {occupants.map((n) => n.name || shortKey(n.publicKey)).join(', ')}.
			</div>
		{/if}
	</div>

	<!-- Generate -->
	<div class="panel rise px-5 py-4" style="animation-delay:120ms">
		<div class="mb-3 flex items-center justify-between gap-3">
			<div class="label">Generate key pair</div>
			{#if byteLen === 3 && status === 'free'}
				<span class="text-amber text-xs">3-byte vanity can take several minutes</span>
			{/if}
		</div>

		{#if result}
			<!-- Result -->
			<div class="space-y-3">
				<div
					class="border-signal/30 bg-signal/5 flex items-center gap-2 rounded-[var(--radius)] border px-3 py-2"
				>
					<span class="live-dot"></span>
					<span class="text-signal text-sm font-medium"
						>Found a key with hash ID <span class="font-mono font-700">{result.publicKey.slice(0, want)}</span></span
					>
					<span class="text-fg-faint ml-auto font-mono text-xs tnum"
						>{fmtInt(keygen.attempts)} tries · {(keygen.elapsedMs / 1000).toFixed(1)}s</span
					>
				</div>

				{#each [{ label: 'Public key', field: 'public', value: result.publicKey }, { label: 'Private key', field: 'private', value: result.privateKey }] as row (row.field)}
					<div>
						<div class="mb-1 flex items-center justify-between">
							<span class="label">{row.label}</span>
							<button
								onclick={() => copy(row.field, row.value)}
								class="text-fg-faint hover:text-signal font-mono text-xs"
								>{copied === row.field ? 'copied ✓' : 'copy'}</button
							>
						</div>
						<div
							class="border-line bg-ink truncate rounded-[var(--radius)] border px-3 py-2 font-mono text-xs
							{row.field === 'private' ? 'text-amber' : 'text-fg'}"
						>
							{row.value}
						</div>
					</div>
				{/each}

				<div class="flex flex-wrap gap-2">
					<button
						onclick={download}
						class="bg-signal/15 text-signal border-signal/40 hover:bg-signal/25 rounded-[var(--radius)] border px-4 py-2 text-sm font-medium"
					>
						Download MeshCore JSON
					</button>
					<button
						onclick={() => keygen.reset()}
						class="border-line text-fg-dim hover:text-fg rounded-[var(--radius)] border px-4 py-2 text-sm"
					>
						Generate another
					</button>
				</div>

				<p class="text-fg-faint text-xs leading-relaxed">
					⚠ The private key is your node's identity — keep it secret. It was generated entirely in
					your browser and never sent anywhere. Import it via the MeshCore app → Settings → Manage
					Identity Key, or import the JSON file directly.
				</p>
			</div>
		{:else if keygen.running}
			<!-- In progress -->
			<div class="space-y-3">
				<div class="flex items-center gap-3">
					<span class="live-dot"></span>
					<span class="text-fg text-sm"
						>Searching for <span class="text-signal font-mono font-700">{keygen.prefixHex}</span>…</span
					>
					<button
						onclick={() => keygen.cancel()}
						class="border-line text-fg-dim hover:border-coral/50 hover:text-coral ml-auto rounded-[var(--radius)] border px-3 py-1.5 text-xs"
						>Cancel</button
					>
				</div>
				<div class="grid grid-cols-3 gap-3 text-center">
					<div>
						<div class="text-fg font-mono text-lg tnum">{fmtInt(keygen.attempts)}</div>
						<div class="label">tried</div>
					</div>
					<div>
						<div class="text-fg font-mono text-lg tnum">{fmtInt(keygen.rate)}</div>
						<div class="label">keys/sec</div>
					</div>
					<div>
						<div class="text-fg font-mono text-lg tnum">{eta()}</div>
						<div class="label">est. left</div>
					</div>
				</div>
				<div class="text-fg-faint text-xs">
					{keygen.workerCount} worker{keygen.workerCount === 1 ? '' : 's'} · ~{fmtInt(expectedKeys)}
					keys to try on average
				</div>
			</div>
		{:else}
			<!-- Idle -->
			<button
				onclick={generate}
				disabled={status !== 'free'}
				class="w-full rounded-[var(--radius)] px-4 py-3 text-sm font-medium transition-colors
					{status === 'free'
					? 'bg-signal/15 text-signal border-signal/40 hover:bg-signal/25 border'
					: 'border-line text-fg-faint cursor-not-allowed border'}"
			>
				{#if status === 'free'}
					Generate a key with hash ID <span class="font-mono font-700">{prefix}</span>
				{:else}
					Choose an available hash ID first
				{/if}
			</button>
			{#if status === 'free'}
				<div class="text-fg-faint mt-2 text-xs">
					~{fmtInt(expectedKeys)} keys to try on average · runs on all CPU cores
				</div>
			{/if}
			{#if keygen.error}
				<div class="text-coral mt-2 text-xs">{keygen.error}</div>
			{/if}
		{/if}
	</div>
	</div>
</div>
