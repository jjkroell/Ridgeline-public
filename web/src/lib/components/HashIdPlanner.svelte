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

	// How much live traffic actually travels at each width. A width's ambiguity
	// only matters in proportion to the traffic using it, so show the measurement
	// rather than leaving the reader to assume it is hypothetical.
	type WidthMix = { 1: number; 2: number; 3: number; total: number };
	let widthMix = $state<WidthMix | null>(null);
	const observedShare = $derived(
		widthMix && widthMix.total ? (100 * widthMix[byteLen]) / widthMix.total : null
	);

	onMount(async () => {
		const len = parseInt(page.url.searchParams.get('len') ?? '', 10);
		if (len === 1 || len === 2 || len === 3) byteLen = len;
		highlightPrefix = (page.url.searchParams.get('id') ?? '').toUpperCase();
		try {
			nodes = await api.nodes();
			// Best-effort: the planner is still useful without it.
			try {
				const evts = await api.recent(24 * 3600);
				const mix: WidthMix = { 1: 0, 2: 0, 3: 0, total: 0 };
				for (const e of evts) {
					const h = e.hashSize;
					if (h === 1 || h === 2 || h === 3) {
						mix[h]++;
						mix.total++;
					}
				}
				if (mix.total > 0) widthMix = mix;
			} catch {
				/* leave widthMix null; the share line just doesn't render */
			}
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
	// Usable, not raw: MeshCore rejects keys whose FIRST byte is 00 or FF
	// (Identity.cpp), so each extra byte multiplies 254, not 256.
	// 254 / 65,024 / 16,646,144 — same convention as the /hash-ids guide.
	const space = $derived(254 * 256 ** (byteLen - 1));
	const status = $derived(prefixStatus(nodes, byteLen, prefix));
	const want = $derived(byteLen * 2);
	// Every path-participating node is exposed at whatever width the sender picks.
	const pathNodeCount = $derived(nodes.filter(isPathNode).length);
	// Nodes inside a collision group whose OWN adverts use the selected width.
	// These are not innocent bystanders: they originate narrow paths themselves,
	// so their operator has a change to make. Everyone else in the group appears
	// only because somebody else's packet was narrow.
	const selfNarrow = $derived(
		groups.flatMap((g) => g.nodes).filter((n) => n.hashSize === byteLen)
	);

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
		<div class="mb-3 flex items-center justify-between gap-3">
			<div class="label">Hash ID length</div>
			<a
				href="{compact ? '/m' : ''}/hash-ids"
				class="text-fg-faint hover:text-signal shrink-0 text-xs transition-colors"
				>How to change this →</a
			>
		</div>
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
			This is the width of the <em>packet</em>, not a per-node setting. A relay writes its prefix at
			whatever width the sender chose, so at {byteLen} byte{byteLen > 1 ? 's' : ''}
			<span class="text-fg-dim">every</span> routing node is identified by its first {byteLen}
			byte{byteLen > 1 ? 's' : ''} — whatever its own adverts use.
			{#if !loading}
				<span class="text-fg-dim tnum">{used.size.toLocaleString()} of {space.toLocaleString()}</span
				>
				IDs are taken across all {pathNodeCount.toLocaleString()}
				routing node{pathNodeCount === 1 ? '' : 's'}.
				<span class="text-fg-dim"
					>Adverts today: {cohorts[1]} at 1 byte, {cohorts[2]} at 2, {cohorts[3]} at 3{#if cohorts.unknown}, {cohorts.unknown}
						not yet advertised{/if}.</span
				>
				Companions are excluded — they don't repeat packets, so they never appear in a path.
			{/if}
		</div>
	</div>

	<!-- Genuine collisions -->
	<div class="panel rise px-5 py-4" style="animation-delay:40ms">
		<div class="mb-1 flex items-center justify-between gap-3">
			<div class="label">Ambiguous in a {byteLen}-byte path</div>
			<span class="text-fg-faint font-mono text-xs tnum">
				{groups.length} group{groups.length === 1 ? '' : 's'}
			</span>
		</div>
		<div class="text-fg-faint mb-3 text-xs">
			These nodes are indistinguishable <em>from each other in the path of a {byteLen}-byte
			packet</em> — they are not misconfigured, and their own adverts may well be unambiguous. Any
			routing node can appear in a {byteLen}-byte path, because a relay writes its prefix at the
			width the <span class="text-fg-dim">sender</span> chose, not its own.
			{#if observedShare !== null}
				<span class="text-fg-dim"
					>Over the last 24h, <span class="tnum">{observedShare.toFixed(1)}%</span> of observed
					traffic used this width{observedShare < 1
						? ' — so this is largely theoretical today'
						: ''}.</span
				>
			{/if}
			Records with corrupted keys are filtered out below.
		</div>
		{#if loading}
			<div class="text-fg-faint py-4 text-sm">Loading nodes…</div>
		{:else if groups.length === 0}
			<div class="text-fg-dim py-2 text-sm">
				No two routing nodes share a prefix at {byteLen} byte{byteLen > 1 ? 's' : ''} — every hop in
				a {byteLen}-byte path is attributable to exactly one node.
			</div>
		{:else}
			<div
				class="border-signal/40 bg-signal/5 mb-3 rounded-[var(--radius)] border px-3 py-2.5 text-xs"
			>
				<span class="text-signal">Who fixes this:</span>
				<span class="text-fg-dim">
					{#if selfNarrow.length}
						<span class="text-fg"
							>{selfNarrow.length} of the nodes below advertise at {byteLen} byte{byteLen > 1
								? 's'
								: ''} themselves</span
						>
						(marked <span class="text-amber">{byteLen}B</span>) — they originate narrow paths, so
						their operators can fix that directly with
						<code class="text-fg-dim font-mono text-[0.7rem]">set path.hash.mode</code>.
						The rest appear here only because something <em>else</em> sent a {byteLen}-byte packet
						through them; that traffic comes mostly from companions, apps and bots.
						<a href="{compact ? '/m' : ''}/hash-ids#companions" class="text-signal hover:underline"
							>How to change a sender →</a
						>
					{:else}
						none of the nodes below — every one of them already advertises wider than {byteLen}
						byte{byteLen > 1 ? 's' : ''}. They appear here only because something <em>sent</em> a
						{byteLen}-byte packet through them, and that traffic comes mostly from companions, apps
						and bots.
						<a href="{compact ? '/m' : ''}/hash-ids#companions" class="text-signal hover:underline"
							>How to change a sender →</a
						>
					{/if}
				</span>
			</div>
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
								<!-- On a phone the badges (role + advert width + no-GPS) are all
								     shrink-0 and consume ~230 of ~290px, leaving the name about 60px —
								     i.e. unreadable. Let the row wrap and give the name its own full
								     line; the badges follow underneath. -->
								<li class="flex items-center gap-2 text-sm {compact ? 'flex-wrap' : ''}">
									<span class="text-fg min-w-0 truncate {compact ? 'w-full' : ''}"
										>{n.name || '(unnamed)'}</span
									>
									<span class="shrink-0"><RoleBadge role={n.role} /></span>
									{#if n.hashSize === 1 || n.hashSize === 2 || n.hashSize === 3}
										<Tooltip
											text={n.hashSize === byteLen
												? `This node originates ${byteLen}-byte paths itself — its operator can widen it with 'set path.hash.mode'.`
												: `This node's own adverts use ${n.hashSize}-byte IDs. It appears here only because something else sent a ${byteLen}-byte packet through it.`}
											class="shrink-0"
										>
											<span
												class="rounded-[var(--radius)] border px-1.5 py-0.5 text-[0.6rem] tracking-wide {n.hashSize ===
												byteLen
													? 'border-amber/50 text-amber'
													: 'border-line text-fg-faint'}">adverts {n.hashSize}B</span
											>
										</Tooltip>
									{/if}
									{#if !n.hasLocation}
										<Tooltip text="No GPS location broadcast" class="shrink-0">
											<span
												class="text-fg-faint border-line rounded-[var(--radius)] border px-1.5 py-0.5 text-[0.6rem] tracking-wide uppercase"
												>no GPS</span
											>
										</Tooltip>
									{/if}
									<!-- The key is dropped on narrow screens. Everything else in this row
									     is shrink-0, so the name is the only thing that can give — and on a
									     phone it gave all of it, leaving the row unreadable. The key is
									     still one tap away on the node itself. -->
									{#if !compact}
										<button
											onclick={() => copy(n.publicKey, n.publicKey)}
											class="text-fg-faint hover:text-signal ml-auto shrink-0 font-mono text-xs"
										>
											<Tooltip text="Copy full public key"
												>{copied === n.publicKey ? 'copied' : shortKey(n.publicKey, 6, 4)}</Tooltip
											>
										</button>
									{/if}
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

{#if !compact}
	<!-- Pick an unused hash ID + Generate key pair — side by side on desktop -->
	<!-- [&>*]:min-w-0 is load-bearing. Grid items default to min-width:auto, so a
	     child with a wide min-content (the key rows, the status line) pushes the
	     column past the viewport instead of shrinking. On /m that overflow is not
	     visible in document.scrollWidth — the shell is `fixed inset-0
	     overflow-hidden` and it is <main> that scrolls — so it shows up as the
	     page sliding left/right rather than as a document-level scrollbar. -->
	<div class="grid items-stretch gap-4 [&>*]:min-w-0 {compact ? '' : 'md:grid-cols-2'}">
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
					class="border-signal/30 bg-signal/5 flex flex-wrap items-center gap-2 rounded-[var(--radius)] border px-3 py-2"
				>
					<span class="live-dot shrink-0"></span>
					<span class="text-signal min-w-0 text-sm font-medium"
						>Found a key with hash ID <span class="font-mono font-700">{result.publicKey.slice(0, want)}</span></span
					>
					<span class="text-fg-faint ml-auto shrink-0 font-mono text-xs tnum"
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
{:else}
	<!-- Picking an ID and generating a vanity key pair are desktop-only.
	     A 3-byte vanity search is a brute-force loop over millions of keypairs —
	     minutes of sustained CPU, which on a phone means heat and battery for a
	     task you only do once. The panels also need the width. The collision
	     analysis above is the part that is genuinely useful on a phone, so that
	     stays. -->
	<div class="panel rise px-5 py-4" style="animation-delay:80ms">
		<div class="label mb-2">Picking an ID &amp; generating a key</div>
		<p class="text-fg-dim text-sm leading-relaxed">
			These are desktop-only. Searching for a key with a chosen hash ID is a
			brute-force loop — at 3 bytes it can run for minutes of solid CPU, which is a
			poor trade on a phone for something you do once per node.
		</p>
		<p class="text-fg-faint mt-2 text-xs leading-relaxed">
			Open Ridgeline on a desktop browser and go to <span class="text-fg-dim">Identity</span>.
			The collision list above works the same on either.
		</p>
	</div>
{/if}
</div>
