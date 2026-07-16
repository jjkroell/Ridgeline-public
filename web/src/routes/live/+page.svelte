<script lang="ts">
	import { onMount } from 'svelte';
	import Seo from '$lib/components/Seo.svelte';
	import { page } from '$app/state';
	import { replaceState } from '$app/navigation';
	import { live, groupLive, type LiveGroup } from '$lib/live.svelte';
	import { channels } from '$lib/channels.svelte';
	import { api, type Node } from '$lib/api';
	import { ago, shortKey, roleLabel } from '$lib/format';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import PayloadTag from '$lib/components/PayloadTag.svelte';
	import Tooltip from '$lib/components/Tooltip.svelte';
	import LiveGroupModal from '$lib/components/LiveGroupModal.svelte';
	import PathModal from '$lib/components/PathModal.svelte';

	let paused = $state(false);
	let frozen = $state<typeof live.events>([]);
	let selected = $state<LiveGroup | null>(null);
	// Set when the Path cell is clicked — opens the full-path modal instead.
	let pathFor = $state<LiveGroup | null>(null);

	// Custom cursor-following hover hint (matches Tooltip.svelte styling).
	let tip = $state<{ text: string; x: number; y: number } | null>(null);
	const showTip = (e: MouseEvent, text: string) => (tip = { text, x: e.clientX, y: e.clientY });
	const hideTip = () => (tip = null);

	// The feed shows the last hour of traffic by default.
	const WINDOW_MS = 60 * 60 * 1000;
	const inWindow = (events: typeof live.events) => {
		const cutoff = Date.now() - WINDOW_MS;
		return events.filter((e) => +new Date(e.receivedAt) >= cutoff);
	};
	const groups = $derived(groupLive(inWindow(paused ? frozen : live.events)));

	// Node list (loaded once) lets us resolve path-hop key prefixes to names.
	let nodes = $state<Node[]>([]);
	$effect(() => {
		if (nodes.length === 0) api.nodes().then((n) => (nodes = n)).catch(() => {});
	});
	const resolveHop = (hop: string): Node | undefined => nodes.find((n) => n.publicKey.startsWith(hop));

	// Toggle the Path column's hop labels between friendly names and their hash
	// IDs (the pubkey prefix at the node's configured hash length).
	let showIds = $state(false);
	const hashId = (n: Node) => n.publicKey.slice(0, Math.max(1, n.hashSize || 2) * 2);

	// --- Shareable per-packet deep link (?p=<messageHash>, optionally &obs=&at=) ---
	// Reflect the open packet in the URL so the address bar is directly shareable,
	// and re-open a packet from a pasted link on load (fetching it by hash from the
	// backend, so it works for anyone even after it scrolls out of the live buffer).
	// obs+at deep-link straight to one observation's packet-detail sub-view.
	let deepObs = $state<string | null>(null);
	let deepAt = $state<string | null>(null);

	function setParams(hash: string | null) {
		const url = new URL(page.url);
		if (hash) url.searchParams.set('p', hash);
		else url.searchParams.delete('p');
		// obs/at only ever ride along on an incoming deep link; drop them once the
		// user navigates so a copied address-bar URL doesn't carry stale detail.
		url.searchParams.delete('obs');
		url.searchParams.delete('at');
		replaceState(url, {});
	}
	function selectGroup(g: LiveGroup) {
		selected = g;
		deepObs = null;
		deepAt = null;
		setParams(g.messageHash);
		hideTip();
	}
	function closeModal() {
		selected = null;
		deepObs = null;
		deepAt = null;
		setParams(null);
	}
	async function openPacketByHash(hash: string) {
		try {
			const events = await api.packet(hash);
			const g = groupLive(events)[0];
			if (g) selected = g;
		} catch {
			/* unknown/aged-out hash — leave the feed as-is */
		}
	}
	onMount(() => {
		const h = page.url.searchParams.get('p');
		deepObs = page.url.searchParams.get('obs');
		deepAt = page.url.searchParams.get('at');
		if (h) openPacketByHash(h);
	});

	/** The observer that reported this transmission earliest. */
	function firstObserver(g: LiveGroup): string {
		let earliest = g.events[0];
		for (const e of g.events)
			if (+new Date(e.receivedAt) < +new Date(earliest.receivedAt)) earliest = e;
		return earliest.observerId ?? '—';
	}

	/** Cap a label at 15 chars with a trailing ellipsis. */
	const trunc15 = (s: string) => (s.length > 15 ? s.slice(0, 15).trimEnd() + '…' : s);

	/** A short, human description of what the packet carried, by payload type. */
	function packetSummary(g: LiveGroup): string {
		const ev = g.events.find((e) => e.payloadRaw) ?? g.events[0];
		const raw = ev?.payloadRaw ?? '';
		const ch = raw.slice(0, 2); // first payload byte = channel hash for group msgs
		const bytes = raw.length / 2;
		switch (g.payloadType) {
			case 'Advert':
				return g.node ? g.node.name || shortKey(g.node.publicKey) : 'advert';
			case 'GroupText': {
				// Decrypt client-side against the user's configured channels — their
				// channel list is authoritative (removing a channel hides its msgs).
				const dec = channels.decrypt(ev?.payloadRaw);
				if (dec) return dec.sender ? `${dec.sender}: ${dec.text}` : dec.text;
				const h = ev?.channelHash ?? ch;
				return h ? `ch ${h} (encrypted)` : 'group msg';
			}
			case 'GroupData':
				return ch ? `ch ${ch} data` : 'group data';
			case 'TextMessage':
				return raw.length >= 4 ? `DM ${raw.slice(2, 4)}→${raw.slice(0, 2)}` : 'DM (encrypted)';
			case 'Ack':
				return raw ? `ack ${raw.slice(0, 8)}` : 'ack';
			case 'Trace':
				return `trace #${g.messageHash}`;
			case 'Path':
				return raw.length >= 4 ? `path ${raw.slice(2, 4)}→${raw.slice(0, 2)}` : 'path update';
			case 'Request':
				return raw.length >= 4 ? `req ${raw.slice(2, 4)}→${raw.slice(0, 2)}` : 'request (encrypted)';
			case 'Response':
				return raw.length >= 4 ? `resp ${raw.slice(2, 4)}→${raw.slice(0, 2)}` : 'response (encrypted)';
			case 'Control': {
				const sub = raw.length >= 2 ? parseInt(raw.slice(0, 2), 16) & 0xf0 : 0;
				return sub === 0x90 ? 'discover resp' : sub === 0x80 ? 'discover req' : 'control';
			}
			case 'AnonRequest':
				return raw.length >= 2 ? `anon →${raw.slice(0, 2)}` : 'anon request';
			default:
				return bytes ? `${bytes} B payload` : g.payloadType.toLowerCase();
		}
	}

	/** The most complete (longest) path observed for this transmission. */
	function leadPath(g: LiveGroup): string[] {
		return [...g.events].sort((a, b) => (b.path?.length ?? 0) - (a.path?.length ?? 0))[0]?.path ?? [];
	}

	function togglePause() {
		if (!paused) frozen = [...live.events];
		paused = !paused;
	}
</script>

<Seo
	title="Live MeshCore Packet Feed"
	description="Watch MeshCore mesh packets stream in real time — adverts, messages and traces as they arrive."
	path="/live"
/>

<PageHeader eyebrow="Real-time Telemetry" title="Feed">
	<Tooltip text="Toggle the Path column between node names and hash IDs">
		<button
			onclick={() => (showIds = !showIds)}
			class="font-mono border-line text-fg-dim hover:border-line-bright hover:text-fg flex items-center gap-2 rounded-[var(--radius)] border px-3 py-1.5 text-xs transition-colors"
		>
			Path ⇄ {showIds ? 'IDs' : 'Names'}
		</button>
	</Tooltip>
	<button
		onclick={togglePause}
		class="font-mono flex items-center gap-2 rounded-[var(--radius)] border px-3 py-1.5 text-xs transition-colors
			{paused
			? 'border-amber/50 text-amber bg-amber/10'
			: 'border-line text-fg-dim hover:border-line-bright hover:text-fg'}"
	>
		{#if paused}▶ Resume{:else}❙❙ Pause{/if}
	</button>
	<div class="font-mono text-fg-dim flex items-center gap-2 text-xs">
		{#if live.connected}<span class="live-dot"></span>{/if}
		<span class="tnum text-signal">{live.total}</span>
	</div>
</PageHeader>

<div class="px-6 py-6 md:px-10">
	<div class="panel overflow-hidden">
		<div
			class="label border-line/70 grid grid-cols-[34px_64px_140px_64px_160px_minmax(0,1fr)] gap-x-3 border-b px-5 py-3 md:grid-cols-[34px_64px_140px_370px_64px_160px_minmax(0,1fr)]"
		>
			<span class="text-center">Time</span>
			<span class="text-center">Type</span>
			<span class="text-center">First Observer</span>
			<span class="hidden text-center md:block">Path</span>
			<span class="text-center">Repeats</span>
			<span class="text-center">Contents</span>
			<span></span>
		</div>

		{#if groups.length === 0}
			<div class="text-fg-faint px-5 py-16 text-center text-sm">
				{#if live.connected}Waiting for the first packet…{:else}Connecting to live feed…{/if}
			</div>
		{:else}
			<div class="divide-line/40 divide-y">
				{#each groups as g (g.key)}
					{@const path = leadPath(g)}
					{@const summary = packetSummary(g)}
					<button
						onclick={() => selectGroup(g)}
						onmousemove={(e) => showTip(e, 'Click for full packet details')}
						onmouseleave={hideTip}
						class="panel-hover group grid w-full grid-cols-[34px_64px_140px_64px_160px_minmax(0,1fr)] items-center gap-x-3 px-5 py-2.5 text-left text-sm md:grid-cols-[34px_64px_140px_370px_64px_160px_minmax(0,1fr)]"
					>
						<span class="font-mono text-fg-faint text-xs tnum">{ago(g.latest)}</span>
						<!-- svelte-ignore a11y_no_static_element_interactions -->
						<span onmousemove={(e) => { e.stopPropagation(); hideTip(); }}>
							<PayloadTag type={g.payloadType} />
						</span>
						<span class="text-fg truncate">{trunc15(firstObserver(g))}</span>
						<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
						<span
							onclick={(e) => {
								e.stopPropagation();
								pathFor = g;
								hideTip();
							}}
							onmousemove={(e) => {
								e.stopPropagation();
								showTip(e, 'Click for full path');
							}}
							class="hover:bg-panel-2/50 -mx-1 hidden max-w-[370px] min-w-0 cursor-pointer items-center gap-1 overflow-hidden rounded-[var(--radius)] px-1 font-mono text-xs whitespace-nowrap transition-colors md:flex"
						>
							{#if path.length === 0}
								<span class="text-fg-faint">—</span>
							{:else}
								{#each path.slice(0, 4) as hop, i (i)}
									{#if i > 0}<span class="text-fg-faint shrink-0">→</span>{/if}
									{@const n = resolveHop(hop)}
									{#if n}
										<span class="text-fg-dim truncate"
											>{showIds ? hashId(n) : n.name || shortKey(n.publicKey)}</span
										>
									{:else}
										<span class="text-fg-faint shrink-0">{hop}</span>
									{/if}
								{/each}
								{#if path.length > 4}
									<span class="text-fg-faint shrink-0">→</span>
									<span class="text-fg-faint shrink-0">+{path.length - 4}</span>
								{/if}
							{/if}
						</span>
						<span class="text-center">
							{#if g.count > 1}
								<span
									class="font-mono text-signal bg-signal/10 rounded-[var(--radius)] px-1.5 py-0.5 text-[0.62rem] tnum"
									>×{g.count}</span
								>
							{:else}
								<span class="font-mono text-fg-faint text-xs tnum">1</span>
							{/if}
						</span>
						<span
							class="text-fg-dim group-hover:text-fg max-w-[200px] truncate text-xs transition-colors"
							>{summary}</span
						>
						<span></span>
					</button>
				{/each}
			</div>
		{/if}
	</div>
</div>

<LiveGroupModal group={selected} onclose={closeModal} initialObs={deepObs} initialAt={deepAt} />
<PathModal group={pathFor} {nodes} {showIds} onclose={() => (pathFor = null)} />

{#if tip}
	<div
		class="border-line-bright bg-ink-2 text-fg-dim pointer-events-none fixed z-[60] max-w-[250px] rounded-[var(--radius)] border px-2.5 py-1.5 text-xs leading-snug shadow-xl"
		style="left:{tip.x}px;top:{tip.y}px;transform:translate(-50%,calc(-100% - 14px))"
	>
		{tip.text}
		<span
			class="bg-ink-2 border-line-bright absolute top-full left-1/2 -mt-[5px] h-2 w-2 -translate-x-1/2 rotate-45 border-r border-b"
		></span>
	</div>
{/if}
