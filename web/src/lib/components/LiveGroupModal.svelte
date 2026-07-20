<script lang="ts">
	import type { LiveGroup } from '$lib/live.svelte';
	import { channels } from '$lib/channels.svelte';
	import { api, type Node, type LiveEvent } from '$lib/api';
	import { ago, shortKey, fmtSnr, snrColor, roleColor, roleLabel, fmtCoord } from '$lib/format';
	import { buildPacketFields, parseTrace, type ByteRange } from '$lib/packet-fields';
	import PayloadTag from './PayloadTag.svelte';
	import HopChips from './HopChips.svelte';
	import Tooltip from './Tooltip.svelte';

	interface Props {
		group: LiveGroup | null;
		onclose: () => void;
		// Deep-link straight to one observation's packet detail: obs + receivedAt
		// uniquely identify a repeat within the group (see the shareable copy-link).
		initialObs?: string | null;
		initialAt?: string | null;
	}
	let { group, onclose, initialObs = null, initialAt = null }: Props = $props();

	let nodes = $state<Node[]>([]);
	let copied = $state('');
	// The repeat (single observation) drilled into; null = show the repeats list.
	let repeat = $state<LiveEvent | null>(null);

	// Pull the node list once (for resolving path-hop prefixes to names).
	$effect(() => {
		if (group && nodes.length === 0) api.nodes().then((n) => (nodes = n)).catch(() => {});
	});
	// When a packet group opens, start on the repeats list — unless a specific
	// observation was deep-linked (obs+receivedAt), in which case open its detail.
	$effect(() => {
		void group;
		repeat =
			initialObs && group
				? (group.events.find((e) => e.observerId === initialObs && e.receivedAt === initialAt) ?? null)
				: null;
	});

	// Observations sorted by arrival (first heard first).
	const events = $derived(
		group ? [...group.events].sort((a, b) => +new Date(a.receivedAt) - +new Date(b.receivedAt)) : []
	);
	const firstAt = $derived(events.length ? +new Date(events[0].receivedAt) : 0);
	const span = $derived(
		events.length > 1 ? (+new Date(events[events.length - 1].receivedAt) - firstAt) / 1000 : 0
	);
	const distinctPaths = $derived(new Set(events.map((e) => (e.path ?? []).join('>'))).size);
	// Decrypt against the user's configured channels (client-side). First repeat
	// that decodes provides the banner; null when no configured key matches.
	const decoded = $derived.by(() => {
		for (const e of events) {
			const d = channels.decrypt(e.payloadRaw);
			if (d) return d;
		}
		return null;
	});
	// Decryption for the currently drilled-into repeat.
	const repeatMsg = $derived(repeat ? channels.decrypt(repeat.payloadRaw) : null);

	const title = $derived(
		group?.node ? group.node.name || shortKey(group.node.publicKey, 8, 4) : (group?.messageHash ?? '')
	);

	async function copy(text: string, label: string) {
		await navigator.clipboard.writeText(text);
		copied = label;
		setTimeout(() => (copied = ''), 1200);
	}

	// Shareable deep link to whatever's on screen: the packet's repeats list, or —
	// when drilled into one observation — that exact packet-detail sub-view.
	function copyLink() {
		if (!group) return;
		let url = `${location.origin}/live?p=${encodeURIComponent(group.messageHash)}`;
		if (repeat) {
			url +=
				`&obs=${encodeURIComponent(repeat.observerId ?? '')}` +
				`&at=${encodeURIComponent(repeat.receivedAt)}`;
		}
		copy(url, 'link');
	}

	function advTime(ts?: number): string {
		if (!ts) return '—';
		return new Date(ts * 1000).toISOString().replace('T', ' ').slice(0, 19) + 'Z';
	}
	const absTime = (iso: string) => new Date(iso).toISOString().slice(11, 23) + 'Z';
	const rel = (iso: string) => `+${((+new Date(iso) - firstAt) / 1000).toFixed(1)}s`;
	// Plain-language version of rel() for the packet-detail header: how long after
	// the first observer this one heard the same transmission.
	const relWords = (iso: string) => {
		const d = (+new Date(iso) - firstAt) / 1000;
		return d < 0.05 ? 'first to hear it' : `${d.toFixed(1)}s after first`;
	};

	// Byte-level breakdown of the drilled-into packet's raw hex.
	const breakdown = $derived(repeat ? buildPacketFields(repeat) : { fields: [], ranges: [] });
	const obsIndex = $derived(repeat ? events.indexOf(repeat) : -1);

	// First→last propagation summary for this transmission.
	const propagation = $derived(
		events.length > 1
			? `${span.toFixed(1)}s · ${group?.count ?? events.length} obs × ${group?.observers.length ?? 0} observers`
			: null
	);

	// sender → channel for group text (the only type whose endpoints we decode).
	const srcDst = $derived.by(() => {
		if (!repeat || repeat.payloadType !== 'GroupText') return null;
		return {
			src: repeatMsg?.sender || '?',
			dst: repeatMsg?.channel || (repeat.channelHash ? 'Ch ' + repeat.channelHash : '#?')
		};
	});

	// Transmitter location: the advertised node, or a node matching a decoded sender name.
	const senderLocation = $derived.by(() => {
		if (!repeat) return null;
		const n = repeat.node;
		if (n && n.latitude != null && n.longitude != null)
			return { name: n.name, lat: n.latitude, lon: n.longitude, key: n.publicKey };
		const sender = repeatMsg?.sender;
		if (sender) {
			const m = nodes.find((nn) => nn.name === sender);
			if (m && m.latitude != null && m.longitude != null)
				return { name: m.name, lat: m.latitude, lon: m.longitude, key: m.publicKey };
		}
		return null;
	});

	const summary = $derived(repeat ? eventSummary(repeat) : '');

	function eventSummary(ev: LiveEvent): string {
		const raw = ev.payloadRaw ?? '';
		const route = raw.length >= 4 ? `${raw.slice(2, 4)}→${raw.slice(0, 2)}` : '';
		switch (ev.payloadType) {
			case 'Advert':
				return ev.node?.name || 'advert';
			case 'GroupText':
				return repeatMsg
					? (repeatMsg.sender ? `${repeatMsg.sender}: ` : '') + repeatMsg.text
					: `Ch ${ev.channelHash ?? '?'} (encrypted)`;
			case 'TextMessage':
				return route ? `direct message ${route} (encrypted)` : 'direct message (encrypted)';
			case 'Ack':
				return 'acknowledgement';
			case 'Trace':
				return `trace #${ev.messageHash}`;
			case 'Path':
				return route ? `return path ${route} (encrypted)` : 'path update';
			case 'Request':
				return route ? `request ${route} (encrypted)` : 'request (encrypted)';
			case 'Response':
				return route ? `response ${route} (encrypted)` : 'response (encrypted)';
			case 'Control': {
				const sub = raw.length >= 2 ? parseInt(raw.slice(0, 2), 16) & 0xf0 : 0;
				return sub === 0x90 ? 'node discover response' : sub === 0x80 ? 'node discover request' : 'control';
			}
			case 'AnonRequest':
				return raw.length >= 2 ? `anonymous request →${raw.slice(0, 2)}` : 'anonymous request';
			default:
				return ev.payloadType.toLowerCase();
		}
	}

	// --- Byte-dump colouring (keyed by field type) ---
	const FIELD_COLORS: Record<string, string> = {
		header: 'var(--color-signal)',
		transport: 'var(--color-violet)',
		pathlen: 'var(--color-amber)',
		path: 'var(--color-sky)',
		pubkey: 'var(--color-coral)',
		timestamp: 'var(--color-amber)',
		signature: 'var(--color-violet)',
		flags: 'var(--color-signal)',
		location: 'var(--color-sky)',
		name: 'var(--color-violet)',
		channel: 'var(--color-signal)',
		mac: 'var(--color-amber)',
		encrypted: 'var(--color-coral)',
		payload: 'var(--color-coral)',
		hash: 'var(--color-sky)',
		checksum: 'var(--color-violet)',
		tag: 'var(--color-signal)',
		snr: 'var(--color-amber)'
	};
	const KEY_LABEL: Record<string, string> = {
		header: 'Header', transport: 'Transport', pathlen: 'Path Length', path: 'Path',
		pubkey: 'Public Key', timestamp: 'Timestamp', signature: 'Signature', flags: 'Flags',
		location: 'Location', name: 'Name', channel: 'Channel', mac: 'MAC',
		encrypted: 'Encrypted', payload: 'Payload', hash: 'Node Hash', checksum: 'Checksum',
		tag: 'Tag', snr: 'SNR'
	};
	const fieldColor = (k: string) => FIELD_COLORS[k] ?? 'var(--color-fg-faint)';

	function legendItems(ranges: ByteRange[]): { label: string; color: string }[] {
		const seen = new Set<string>();
		const out: { label: string; color: string }[] = [];
		for (const r of ranges) {
			if (seen.has(r.key)) continue;
			seen.add(r.key);
			out.push({ label: KEY_LABEL[r.key] ?? r.label, color: fieldColor(r.key) });
		}
		return out;
	}

	function dumpBytes(hex: string, ranges: ByteRange[]): { b: string; color: string }[] {
		const clean = (hex ?? '').replace(/\s+/g, '');
		const n = Math.floor(clean.length / 2);
		const cls = new Array<string>(n).fill('');
		for (const r of ranges)
			for (let i = r.start; i <= Math.min(r.end, n - 1); i++) cls[i] = r.key;
		const out: { b: string; color: string }[] = [];
		for (let i = 0; i < n; i++)
			out.push({
				b: clean.slice(i * 2, i * 2 + 2),
				color: cls[i] ? fieldColor(cls[i]) : 'var(--color-fg-faint)'
			});
		return out;
	}
</script>

<!-- Top-right header actions, identical in the repeats-list and packet-detail views:
     a shareable copy-link (to whichever view is open) + the close button. -->
{#snippet headerActions()}
	<div class="flex shrink-0 items-center gap-2.5">
		<Tooltip text="Copy a shareable link to this packet">
			<button
				onclick={copyLink}
				class="inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-[0.68rem] font-600 transition-colors {copied ===
				'link'
					? 'border-signal/50 bg-signal/15 text-signal'
					: 'border-line text-fg-dim hover:border-signal/50 hover:text-signal'}"
			>
				<svg viewBox="0 0 24 24" class="h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
					<path d="M10 13a5 5 0 0 0 7 0l3-3a5 5 0 0 0-7-7l-1 1M14 11a5 5 0 0 0-7 0l-3 3a5 5 0 0 0 7 7l1-1" />
				</svg>
				{copied === 'link' ? 'Link copied' : 'Copy link'}
			</button>
		</Tooltip>
		<button onclick={onclose} class="text-fg-faint hover:text-fg text-xl leading-none" aria-label="Close">✕</button>
	</div>
{/snippet}

<svelte:window onkeydown={(e) => e.key === 'Escape' && group && (repeat ? (repeat = null) : onclose())} />

{#if group}
	<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-50 flex items-end justify-center bg-black/60 p-0 backdrop-blur-sm md:items-center md:p-6"
		onclick={onclose}
		role="dialog"
		aria-modal="true"
		tabindex="-1"
	>
		<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
		<div
			class="panel rise flex max-h-[88vh] w-full flex-col md:max-w-2xl"
			style="animation-duration:.25s"
			onclick={(e) => e.stopPropagation()}
		>
			{#if !repeat}
				<!-- ============================== REPEATS LIST ============================== -->
				<div class="border-line/70 flex items-start gap-3 border-b px-5 py-4">
					<div class="min-w-0 flex-1">
						<div class="mb-2 flex flex-wrap items-center gap-2">
							<PayloadTag type={group.payloadType} tip={false} />
							<span class="label">{group.routeType}</span>
							<span class="label">· {group.kind === 'node' ? 'all from node' : 'one transmission'}</span>
						</div>
						<h2 class="font-display text-fg truncate text-lg font-700">{title}</h2>
						<div class="font-mono text-fg-faint mt-1 text-[0.68rem]">{group.messageHash}</div>
					</div>
					{@render headerActions()}
				</div>

				<!-- Summary -->
				<div class="border-line/50 grid grid-cols-4 gap-3 border-b px-5 py-3">
					{#each [{ k: 'Repeats', v: String(group.count) }, { k: 'Observers', v: String(group.observers.length) }, { k: 'Best SNR', v: fmtSnr(group.bestSnr) }, { k: 'Spread', v: span > 0 ? span.toFixed(1) + 's' : '—' }] as s (s.k)}
						<div>
							<div class="label">{s.k}</div>
							<div class="font-display tnum text-fg mt-1 text-xl font-700">{s.v}</div>
						</div>
					{/each}
				</div>

				<div class="min-h-0 flex-1 space-y-5 overflow-y-auto px-5 py-4">
					<!-- Decrypted channel message (if available) -->
					{#if decoded?.text}
						<section>
							<div class="label mb-2 flex items-center justify-between">
								<span>Message</span>
								{#if decoded.channel}<span class="text-signal normal-case">{decoded.channel} channel</span>{/if}
							</div>
							<div class="border-line/50 rounded-[var(--radius)] border px-3 py-2.5">
								{#if decoded.sender}<div class="font-display text-fg mb-1 text-sm font-700">{decoded.sender}</div>{/if}
								<div class="text-fg-dim text-sm break-words whitespace-pre-wrap">{decoded.text}</div>
							</div>
						</section>
					{/if}

					<!-- Repeats list -->
					<section>
						<div class="label mb-2">Repeats · select one for packet detail</div>
						<div class="overflow-hidden rounded-[var(--radius)] border border-line/50">
							<div class="label grid grid-cols-[52px_1fr_44px_52px_44px_16px] gap-2 border-b border-line/50 px-3 py-2">
								<span>Δt</span><span>Observer</span><span class="text-right">Hops</span><span class="text-right">SNR</span><span class="text-right">RSSI</span><span></span>
							</div>
							<div class="divide-line/30 divide-y">
								{#each events as e (e.observerId + e.receivedAt)}
									<button
										onclick={() => (repeat = e)}
										class="panel-hover group grid w-full grid-cols-[52px_1fr_44px_52px_44px_16px] items-center gap-2 px-3 py-2 text-left text-xs"
									>
										<span class="font-mono text-fg-faint tnum">{rel(e.receivedAt)}</span>
										<span class="font-mono text-fg-dim truncate">{e.observerName ?? e.observerId ?? '—'}</span>
										<span class="font-mono text-fg-dim text-right tnum">{e.path?.length ?? 0}</span>
										<span class="font-mono text-right tnum" style="color:{snrColor(e.snr)}">{fmtSnr(e.snr)}</span>
										<span class="font-mono text-fg-dim text-right tnum">{e.rssi ?? '—'}</span>
										<span class="text-fg-faint group-hover:text-signal text-right transition-colors">›</span>
									</button>
								{/each}
							</div>
						</div>
					</section>
				</div>
			{:else}
				<!-- ============================== REPEAT DETAIL ============================== -->
				{@const ev = repeat}
				{@const node = ev.node}
				<div class="border-line/70 flex items-start gap-3 border-b px-5 py-4">
					<div class="min-w-0 flex-1">
						<button
							onclick={() => (repeat = null)}
							class="font-mono border-line text-fg-dim hover:border-signal/50 hover:text-signal mb-3 inline-flex items-center gap-1.5 rounded-[var(--radius)] border px-2.5 py-1 text-xs transition-colors"
							>← All {group.count} repeats</button
						>
						<div class="mb-2 flex flex-wrap items-center gap-2">
							<PayloadTag type={ev.payloadType} tip={false} />
							<span class="label">{ev.routeType}</span>
							<span class="label">v{ev.payloadVersion ?? 0}</span>
							{#if events.length > 1}<span class="label text-fg-faint">obs {obsIndex + 1}/{events.length}</span>{/if}
						</div>
						{#if summary}<div class="text-fg-dim mb-1 truncate text-sm">{summary}</div>{/if}
						{#if srcDst}<div class="font-mono text-fg-faint mb-1 text-xs">{srcDst.src} <span class="text-fg-faint">→</span> {srcDst.dst}</div>{/if}
						<h2 class="font-display text-fg truncate text-lg font-700">{ev.observerName ?? ev.observerId ?? '—'}</h2>
						<div class="font-mono text-fg-dim text-xs">heard {ago(ev.receivedAt)} ago{#if events.length > 1} · {relWords(ev.receivedAt)}{/if}</div>
					</div>
					{@render headerActions()}
				</div>

				<!-- Reception stats for this repeat -->
				<div class="border-line/50 grid grid-cols-4 gap-3 border-b px-5 py-3">
					{#each [{ k: 'SNR', v: fmtSnr(ev.snr), u: 'dB' }, { k: 'RSSI', v: ev.rssi != null ? String(ev.rssi) : '—', u: 'dBm' }, { k: 'Hops', v: String(ev.path?.length ?? ev.pathHops ?? 0), u: '' }, { k: 'Heard', v: absTime(ev.receivedAt), u: '' }] as s (s.k)}
						<div>
							<div class="label">{s.k}</div>
							<div class="font-display tnum text-fg mt-1 text-lg font-700">{s.v}{#if s.u && s.v !== '—'}<span class="text-fg-faint ml-1 text-xs font-400">{s.u}</span>{/if}</div>
						</div>
					{/each}
				</div>

				<div class="min-h-0 flex-1 space-y-5 overflow-y-auto px-5 py-4">
					<!-- Channel message (decrypted group text) -->
					{#if ev.payloadType === 'GroupText'}
						<section>
							<div class="label mb-2">Channel Message</div>
							{#if repeatMsg}
								<div class="border-line/50 rounded-[var(--radius)] border px-3 py-2.5">
									<div class="mb-1 flex items-center justify-between">
										{#if repeatMsg.sender}<span class="font-display text-fg text-sm font-700">{repeatMsg.sender}</span>{/if}
										<span class="label text-signal">{repeatMsg.channel}</span>
									</div>
									<div class="text-fg-dim text-sm break-words whitespace-pre-wrap">{repeatMsg.text}</div>
								</div>
							{:else}
								<div class="text-fg-faint text-sm">Channel <span class="font-mono">{ev.channelHash}</span> · encrypted (no key)</div>
							{/if}
						</section>
					{/if}

					<!-- Packet facts -->
					<section>
						<div class="label mb-2">Packet</div>
						<div class="divide-line/40 border-line/50 divide-y rounded-[var(--radius)] border text-sm">
							{#each [{ k: 'Message Hash', v: ev.messageHash, mono: true }, { k: 'Route Type', v: ev.routeType }, { k: 'Payload Type', v: ev.payloadType }, { k: 'Payload Version', v: String(ev.payloadVersion ?? 0) }, { k: 'Hop Count', v: String(ev.pathHops ?? 0) }, ...(ev.hashSize ? [{ k: 'Hash Size', v: `${ev.hashSize}-byte` }] : []), ...(ev.transportCodes ? [{ k: 'Transport Codes', v: ev.transportCodes.map((c) => '0x' + c.toString(16).toUpperCase()).join(', '), mono: true }] : []), { k: 'Observer', v: ev.observerName ?? ev.observerId ?? '—' }, ...(ev.region ? [{ k: 'Scope', v: ev.region }] : []), ...(propagation ? [{ k: 'Propagation', v: propagation }] : [])] as f (f.k)}
							<div class="flex items-center justify-between gap-3 px-3 py-2">
								<span class="label normal-case">{f.k}</span>
								<span class="text-fg text-right {f.mono ? 'font-mono text-xs' : 'text-sm'} tnum">{f.v}</span>
							</div>
						{/each}
						{#if senderLocation}
							<div class="flex items-center justify-between gap-3 px-3 py-2">
								<span class="label normal-case">Location</span>
								<a href="/nodes/{senderLocation.key}" onclick={onclose} class="text-fg hover:text-signal text-right text-sm transition-colors">
									{#if senderLocation.name}<span class="text-fg-dim">{senderLocation.name} — </span>{/if}<span class="font-mono text-xs tnum">{senderLocation.lat.toFixed(5)}, {senderLocation.lon.toFixed(5)}</span>
								</a>
							</div>
						{/if}
						</div>
					</section>

					<!-- Trace: traced route (resolved nodes) + per-hop SNR -->
					{#if ev.payloadType === 'Trace'}
						{@const tr = parseTrace(ev)}
						{#if tr}
							<section>
								<div class="label mb-2">Traced Route · {tr.routeHashes.length} {tr.routeHashes.length === 1 ? 'node' : 'nodes'}</div>
								{#if tr.routeHashes.length}
									<HopChips hops={tr.routeHashes} {nodes} onnavigate={onclose} />
								{:else}
									<div class="text-fg-faint text-sm">No route hashes in payload.</div>
								{/if}
								{#if tr.hopSnr.length}
									<div class="label mt-3 mb-2">Hop SNR · signal at each flood hop</div>
									<div class="flex flex-wrap items-center gap-1.5">
										{#each tr.hopSnr as snr, i (i)}
											{#if i > 0}<span class="text-fg-faint">→</span>{/if}
											<span
												class="border-line/60 font-mono tnum rounded-[var(--radius)] border px-2 py-1 text-xs"
												style="color:{snrColor(snr)}">{snr.toFixed(1)} dB</span
											>
										{/each}
									</div>
								{/if}
							</section>
						{/if}
					{/if}

					<!-- Path (as this observer saw it) — for a trace the header path is
					     SNR data, surfaced in the Trace section above instead. -->
					{#if (ev.path ?? []).length > 0 && ev.payloadType !== 'Trace'}
						<section>
							<div class="label mb-2 flex items-center justify-between">
								<span>Path · {ev.path?.length} hops</span>
								{#if distinctPaths > 1}<span class="text-amber normal-case">{distinctPaths} variants across repeats</span>{/if}
							</div>
							<HopChips hops={ev.path ?? []} {nodes} onnavigate={onclose} />
						</section>
					{/if}

					<!-- Payload -->
					<section>
						<div class="label mb-2">Payload</div>
						{#if node}
							<div class="divide-line/40 border-line/50 divide-y rounded-[var(--radius)] border text-sm">
								<div class="flex items-center justify-between px-3 py-2">
									<span class="label normal-case">Role</span>
									<span class="text-sm" style="color:{roleColor(node.role)}">{roleLabel(node.role)}</span>
								</div>
								<div class="flex items-center justify-between px-3 py-2">
									<span class="label normal-case">Location</span>
									<span class="font-mono text-fg text-xs tnum">{fmtCoord(node.latitude, node.longitude)}</span>
								</div>
								<div class="flex items-center justify-between px-3 py-2">
									<span class="label normal-case">Advertised Time</span>
									<span class="font-mono text-fg text-xs tnum">{advTime(node.timestamp)}</span>
								</div>
							</div>
						{:else if ev.payloadRaw}
							<button
								onclick={() => copy(ev.payloadRaw ?? '', 'payload')}
								class="panel-hover w-full rounded-[var(--radius)] border border-line/50 px-3 py-2 text-left"
							>
								<div class="label mb-1 flex justify-between">
									<span class="normal-case">{ev.payloadRaw.length / 2} bytes{ev.payloadType === 'GroupText' && !repeatMsg ? ' (encrypted)' : ''}</span>
									<span class={copied === 'payload' ? '!text-signal' : ''}>{copied === 'payload' ? 'copied' : 'copy'}</span>
								</div>
								<div class="font-mono text-fg-dim break-all text-[0.68rem] leading-relaxed">{ev.payloadRaw}</div>
							</button>
						{:else}
							<div class="text-fg-faint text-sm">No payload.</div>
						{/if}
					</section>

					<!-- Byte-level breakdown -->
					{#if ev.raw}
						<section>
							<div class="label mb-2 flex items-center justify-between">
								<span>Byte Breakdown · {ev.raw.length / 2} bytes</span>
								<button onclick={() => copy(ev.raw ?? '', 'raw')} class="hover:text-signal transition-colors {copied === 'raw' ? '!text-signal' : ''}">{copied === 'raw' ? 'copied' : 'copy'}</button>
							</div>
							<!-- legend -->
							<div class="mb-2 flex flex-wrap gap-x-3 gap-y-1">
								{#each legendItems(breakdown.ranges) as l (l.label)}
									<span class="label normal-case flex items-center gap-1.5">
										<span class="inline-block h-2 w-2 rounded-full" style="background:{l.color}"></span>
										{l.label}
									</span>
								{/each}
							</div>
							<!-- colour-coded hex dump -->
							<div class="rounded-[var(--radius)] border border-line/50 px-3 py-2.5 font-mono text-[0.7rem] leading-relaxed break-all">
								{#each dumpBytes(ev.raw, breakdown.ranges) as d, i (i)}<span style="color:{d.color}">{d.b}</span> {/each}
							</div>
							<!-- field table -->
							{#if breakdown.fields.length}
								<div class="divide-line/40 border-line/50 mt-3 divide-y overflow-hidden rounded-[var(--radius)] border">
									{#each breakdown.fields as f, i (i)}
										<div class="grid grid-cols-[2.4rem_1fr_auto] items-baseline gap-2 px-3 py-1.5">
											<span class="font-mono text-fg-faint text-[0.62rem] tnum">{f.off != null ? f.off : ''}</span>
											<span class="min-w-0">
												<span class="flex items-center gap-1.5">
													<span class="inline-block h-1.5 w-1.5 shrink-0 rounded-full" style="background:{fieldColor(f.key)}"></span>
													<span class="text-fg-dim text-xs">{f.label}</span>
												</span>
												{#if f.desc}<span class="text-fg-faint mt-0.5 ml-3 block text-[0.62rem]">{f.desc}</span>{/if}
											</span>
											<span class="font-mono text-fg text-right text-xs break-all tnum">{f.value}</span>
										</div>
									{/each}
								</div>
							{/if}
						</section>
					{/if}
				</div>
			{/if}
		</div>
	</div>
{/if}
