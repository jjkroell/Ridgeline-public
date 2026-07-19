<script lang="ts">
	import { goto } from '$app/navigation';
	import { auth } from '$lib/auth.svelte';
	import { confirmer } from '$lib/confirm.svelte';
	import {
		admin,
		type InjectionReport,
		type BlockEntry,
		type BridgeCandidate,
		type InjectorCandidate
	} from '$lib/api';
	import { ago, purgeCascade, roleColor, roleLabel, skippedNote } from '$lib/format';
	import MembersPanel from '$lib/components/MembersPanel.svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import WindowToggle from '$lib/components/WindowToggle.svelte';
	import Tooltip from '$lib/components/Tooltip.svelte';

	// The admin console is gated by the signed-in account's is_admin flag (no more
	// static token). Bounce anyone who isn't an admin once the /me probe resolves.
	$effect(() => {
		if (auth.ready && !auth.isAdmin) goto('/');
	});
	const authed = $derived(auth.isAdmin);

	// Shorter windows catch freshly-set-up bridges: a node just moved to the other
	// mesh still has recent zero-hop adverts on its old frequency, which read as
	// "local" until they age out — so a long window hides a new bridge.
	let windowSec = $state(21600);
	const windows = [
		{ label: '1h', sec: 3600 },
		{ label: '6h', sec: 21600 },
		{ label: '24h', sec: 86400 },
		{ label: '3d', sec: 259200 }
	];

	let report = $state<InjectionReport | null>(null);
	let blocks = $state<BlockEntry[]>([]);
	let detecting = $state(false);
	let busy = $state(''); // key currently acting on
	let msg = $state('');
	let expanded = $state<Record<string, boolean>>({});

	// Manual node scrub (paste a public key → delete the node + all its data).
	let scrubKey = $state('');
	let scrubbing = $state(false);

	// Kick off the initial load exactly once, when admin status is confirmed
	// (auth.isAdmin flips false→true after the /me probe). The `loaded` guard is
	// essential: gating on blocks.length would loop forever when the blocklist is
	// empty (refreshBlocks reassigns []→ re-triggers the effect → 429).
	let loaded = $state(false);
	$effect(() => {
		if (auth.isAdmin && !loaded) {
			loaded = true;
			refreshBlocks();
		}
	});

	async function refreshBlocks() {
		try {
			blocks = await admin.blocklist();
		} catch (e) {
			msg = `blocklist: ${(e as Error).message}`;
		}
	}

	async function runDetect() {
		detecting = true;
		msg = '';
		try {
			report = await admin.detect(windowSec);
		} catch (e) {
			msg = `detect: ${(e as Error).message}`;
		} finally {
			detecting = false;
		}
	}

	const isBlocked = (kind: string, key: string) =>
		blocks.some((b) => b.kind === kind && b.key.toUpperCase() === key.toUpperCase());
	const isAllowed = (key: string) =>
		blocks.some((b) => b.kind === 'allow' && b.key.toUpperCase() === key.toUpperCase());

	// Bridge candidates minus any the admin has dismissed as known-good.
	const isKnown = (key: string) =>
		blocks.some((b) => b.kind === 'known' && b.key.toUpperCase() === key.toUpperCase());
	// Candidates are things to decide about. A dismissed node isn't a bridge and a
	// known one is already decided — both belong out of this list (known bridges
	// keep their own section, so they stay visible).
	const visibleBridges = $derived(
		(report?.bridges ?? []).filter(
			(b) => !isAllowed(b.nodeKey) && !b.known && !isKnown(b.nodeKey)
		)
	);

	async function quarantineBridge(b: BridgeCandidate) {
		busy = b.nodeKey;
		msg = '';
		try {
			// Block the bridge AND its CAPTIVE foreign nodes (the ones with no
			// alternative route) so the injected set disappears from maps/lists.
			// Non-captive nodes are left alone — they reach the mesh other ways too.
			const captive = b.foreign.filter((f) => f.captive).map((f) => f.key);
			await admin.block(auth.csrf, {
				kind: 'bridge',
				key: b.nodeKey,
				name: b.name,
				reason: 'RF bridge (detected)',
				nodes: captive
			});
			await refreshBlocks();
			msg = `Quarantined ${b.name} + ${captive.length} captive nodes — hidden from maps/feed and dropped at ingest.`;
		} catch (e) {
			msg = `quarantine: ${(e as Error).message}`;
		} finally {
			busy = '';
		}
	}

	// Mark a bridge as sanctioned. Unlike Dismiss — which asserts a candidate is
	// NOT a bridge and hides it — this asserts it is one and is wanted, so it
	// stays visible, labelled, and sorted below anything unexpected.
	async function markKnown(b: BridgeCandidate) {
		busy = b.nodeKey;
		msg = '';
		try {
			await admin.block(auth.csrf, { kind: 'known', key: b.nodeKey, name: b.name, reason: 'known bridge' });
			await refreshBlocks();
			msg = `${b.name} marked as a known bridge — it stays listed but won't read as a new finding.`;
		} catch (e) {
			msg = `mark known: ${(e as Error).message}`;
		} finally {
			busy = '';
		}
	}

	async function dismissBridge(b: BridgeCandidate) {
		busy = b.nodeKey;
		msg = '';
		try {
			await admin.block(auth.csrf, { kind: 'allow', key: b.nodeKey, name: b.name, reason: 'dismissed — not a bridge' });
			await refreshBlocks();
			msg = `Dismissed ${b.name} — it won't be flagged as a candidate again.`;
		} catch (e) {
			msg = `dismiss: ${(e as Error).message}`;
		} finally {
			busy = '';
		}
	}

	async function purgeBridge(b: BridgeCandidate) {
		const captive = b.foreign.filter((f) => f.captive).map((f) => f.key);
		if (
			!(await confirmer.ask({
				title: `Purge bridge ${b.name}?`,
				message: `Permanently deletes ${captive.length} captive node${captive.length === 1 ? '' : 's'} and blocks the bridge so it can't re-inject (it stays on the Purged list). This cannot be undone.`,
				confirmLabel: 'Purge bridge',
				danger: true
			}))
		)
			return;
		busy = b.nodeKey;
		msg = '';
		try {
			const res = await admin.purge(auth.csrf, { bridges: [b.nodeKey], nodes: captive });
			await refreshBlocks();
			report = null;
			msg =
				`Purged ${b.name}: deleted ${res.observations} observations and ${res.nodes} node rows.` +
				skippedNote(res);
		} catch (e) {
			msg = `purge: ${(e as Error).message}`;
		} finally {
			busy = '';
		}
	}

	async function quarantineInjector(i: InjectorCandidate) {
		busy = i.observer;
		msg = '';
		try {
			await admin.block(auth.csrf, { kind: 'observer', key: i.observer, name: i.observer, reason: 'MQTT injector (detected)' });
			await refreshBlocks();
			msg = `Quarantined observer ${i.observer} — its published packets will now be dropped.`;
		} catch (e) {
			msg = `quarantine: ${(e as Error).message}`;
		} finally {
			busy = '';
		}
	}

	async function purgeInjector(i: InjectorCandidate) {
		if (
			!(await confirmer.ask({
				title: `Purge injector ${i.observer}?`,
				message: `Permanently deletes all packets published by ${i.observer} and its ${i.exclusiveCount} exclusively-sourced node${i.exclusiveCount === 1 ? '' : 's'}. This cannot be undone.`,
				confirmLabel: 'Purge injector',
				danger: true
			}))
		)
			return;
		busy = i.observer;
		msg = '';
		try {
			const res = await admin.purge(auth.csrf, { observers: [i.observer], nodes: i.exclusive.map((f) => f.key) });
			await refreshBlocks();
			report = null;
			msg =
				`Purged ${i.observer}: deleted ${res.observations} observations and ${res.nodes} node rows.` +
				skippedNote(res);
		} catch (e) {
			msg = `purge: ${(e as Error).message}`;
		} finally {
			busy = '';
		}
	}

	async function removeBlock(b: BlockEntry) {
		busy = b.kind + b.key;
		msg = '';
		try {
			await admin.unblock(auth.csrf, b.kind, b.key);
			await refreshBlocks();
		} catch (e) {
			msg = `unblock: ${(e as Error).message}`;
		} finally {
			busy = '';
		}
	}

	// Permanently delete a purged entry: sweep any stored data and drop the
	// blocklist row, so it disappears from the list entirely.
	async function deletePurged(b: BlockEntry) {
		if (
			!(await confirmer.ask({
				title: `Delete ${b.name || b.key}?`,
				message: 'Removes it from the list. Any remaining stored data is swept and the block is lifted. This cannot be undone.',
				confirmLabel: 'Delete',
				danger: true
			}))
		)
			return;
		busy = b.kind + b.key;
		msg = '';
		try {
			await admin.deleteNodes(auth.csrf, [b.key]);
			await admin.unblock(auth.csrf, b.kind, b.key);
			await refreshBlocks();
		} catch (e) {
			msg = `delete: ${(e as Error).message}`;
		} finally {
			busy = '';
		}
	}

	// Scrub a node from the database by public key: deletes its adverts + node row
	// (no blocklist entry — it re-appears if it transmits again). Irreversible.
	async function scrubNode() {
		const key = scrubKey.trim().toUpperCase();
		if (!key) return;
		if (!/^[0-9A-F]{6,64}$/.test(key)) {
			msg = 'scrub: enter a hex public key (paste the full key for an exact match).';
			return;
		}
		if (
			!(await confirmer.ask({
				title: 'Scrub this node?',
				message:
					'Permanently deletes the node, all of its stored data points, and any ownership claim, notes, private location and location shares attached to it. This cannot be undone.',
				code: key,
				confirmLabel: 'Scrub node',
				danger: true
			}))
		)
			return;
		scrubbing = true;
		msg = '';
		try {
			const res = await admin.deleteNodes(auth.csrf, [key]);
			const cascaded = purgeCascade(res);
			// No node row but leftover user data = a ghost from a pre-cascade scrub,
			// which is a successful cleanup, not a mistyped key.
			msg =
				res.nodes > 0
					? `Scrubbed ${key}: removed ${res.nodes} node row + ${res.observations} data points${cascaded ? ` + ${cascaded}` : ''}.`
					: cascaded
						? `No node row matched ${key}, but cleaned up ${cascaded} left over from an earlier scrub.`
						: `No node row matched ${key} (removed ${res.observations} data points). Check the key — deletion needs the full public key.`;
			scrubKey = '';
			await refreshBlocks();
		} catch (e) {
			msg = `scrub: ${(e as Error).message}`;
		} finally {
			scrubbing = false;
		}
	}

	const kindColor: Record<string, string> = {
		bridge: 'var(--color-coral)',
		observer: 'var(--color-amber)',
		node: 'var(--color-fg-dim)',
		allow: 'var(--color-signal)'
	};
	const kindLabel = (k: string) => (k === 'allow' ? 'dismissed' : k);

	// Purged entries are deleted (not "quarantined") — they stay blocked so they
	// can't re-ingest, but they belong in their own section, not the quarantine list.
	// A sanctioned bridge is not quarantined — nothing is blocked or hidden — so
	// it gets its own list rather than sitting under "blocked/dismissed".
	const knownEntries = $derived(blocks.filter((b) => b.kind === 'known'));
	const quarantineEntries = $derived(
		blocks.filter((b) => b.reason !== 'purged' && b.kind !== 'known')
	);
	const purgedEntries = $derived(blocks.filter((b) => b.reason === 'purged'));
</script>

<PageHeader eyebrow="Restricted" title="Admin — Site Control" />

<div class="px-6 py-6 md:px-10">
	{#if !auth.ready}
		<div class="panel text-fg-faint px-5 py-12 text-center text-sm">Checking…</div>
	{:else if !authed}
		<div class="panel text-fg-faint px-5 py-12 text-center text-sm">
			Admin access only. Redirecting…
		</div>
	{:else}
		<!-- Controls -->
		<div class="flex flex-wrap items-center gap-3">
			<button
				onclick={runDetect}
				disabled={detecting}
				class="bg-signal/15 text-signal border-signal/40 hover:bg-signal/25 rounded-[var(--radius)] border px-4 py-2 text-sm font-600 transition-colors disabled:opacity-50"
			>
				{detecting ? 'Scanning…' : 'Run detection'}
			</button>
			<WindowToggle options={windows} bind:value={windowSec} />
			<span class="text-fg-faint text-xs">Scans adverts for ingress points. Use a short window (1–6h) to catch a freshly-moved bridge.</span>
		</div>

		{#if msg}
			<div class="panel border-signal/40 text-fg-dim mt-4 px-4 py-2.5 text-sm">{msg}</div>
		{/if}

		<!-- Manual node scrub: delete a node + all its data by public key -->
		<div class="panel mt-4 px-5 py-4">
			<div class="label normal-case text-fg-faint mb-1">Scrub node by key</div>
			<p class="text-fg-faint mb-3 text-xs">
				Permanently delete a node and all of its stored data points (its adverts + node row), along
				with any ownership claim, notes, private location and location shares attached to it. Paste
				the full public key. Irreversible; the node re-appears if it transmits again.
			</p>
			<form
				class="flex flex-wrap items-center gap-2"
				onsubmit={(e) => {
					e.preventDefault();
					scrubNode();
				}}
			>
				<input
					bind:value={scrubKey}
					placeholder="public key (hex)"
					spellcheck="false"
					autocomplete="off"
					class="border-line bg-ink-2 text-fg focus:border-coral min-w-0 flex-1 rounded-[var(--radius)] border px-3 py-2 font-mono text-xs outline-none"
				/>
				<button
					type="submit"
					disabled={scrubbing || !scrubKey.trim()}
					class="bg-coral/15 text-coral border-coral/40 hover:bg-coral/25 rounded-[var(--radius)] border px-4 py-2 text-sm font-600 transition-colors disabled:opacity-50"
				>
					{scrubbing ? 'Scrubbing…' : 'Scrub node'}
				</button>
			</form>
		</div>

		<!-- Detection results -->
		{#if report}
			<!-- Scan summary: says how much evidence the verdict rests on, so an
			     empty result reads as "clean window" rather than "scan broken". -->
			<div class="panel rise mt-6 flex flex-wrap items-center gap-x-6 gap-y-1 px-5 py-3">
				<span class="label normal-case text-fg-faint">Scanned</span>
				<span class="font-mono text-fg-dim text-xs tnum"
					>{report.packetsScanned.toLocaleString()} packets · {report.pathsScanned.toLocaleString()} with
					paths · {report.advertsScanned.toLocaleString()} adverts, over {report.windowHours.toFixed(0)}h</span
				>
				{#if report.unresolvedHops > 0}
					<Tooltip
						text="Hops whose hash prefix matched no single node — ambiguous 1-byte hops are common. These count as unknown rather than being silently dropped, so a candidate resting mostly on them deserves less weight."
					>
						<span class="text-fg-faint font-mono text-xs tnum"
							>{report.unresolvedHops.toLocaleString()} hops unresolved</span
						>
					</Tooltip>
				{/if}
				{#if report.advertsRejected > 0}
					<Tooltip
						text="Adverts whose Ed25519 signature didn't verify. A corrupt public key invents a node that never existed, so these are dropped before scoring — they used to surface as injector candidates."
					>
						<span class="text-amber font-mono text-xs tnum"
							>{report.advertsRejected.toLocaleString()} rejected (bad signature)</span
						>
					</Tooltip>
				{:else}
					<span class="text-fg-faint font-mono text-xs">all signatures verified</span>
				{/if}
			</div>

			<!-- RF bridges -->
			{#if visibleBridges.length > 0}
			<section class="panel rise mt-6">
				<div class="border-line/70 flex items-center gap-2.5 border-b px-5 py-3.5">
					<h2 class="font-display text-fg text-sm font-700 tracking-wide">RF BRIDGE CANDIDATES</h2>
					<span class="label normal-case text-fg-faint">nodes funnelling never-heard-direct traffic in</span>
					<span class="label ml-auto tnum">{visibleBridges.length}</span>
				</div>
				{#if true}
					<div class="divide-line/40 divide-y">
						{#each visibleBridges as b (b.nodeKey)}
							<div class="px-5 py-3">
								<div class="flex flex-wrap items-center gap-3">
									<span class="h-2 w-2 shrink-0 rounded-full" style="background:var(--color-coral)"></span>
									<a href="/nodes/{b.nodeKey}" class="text-fg hover:text-signal font-600">{b.name}</a>
									<span class="font-mono text-fg-faint text-[0.62rem]">{b.nodeKey.slice(0, 12)}…</span>
									{#each b.signals as sig (sig)}
										<Tooltip
											text={sig === 'wired'
												? 'Every packet this relay handed on went to the same node. RF is broadcast, so a radiating relay picks up alternative next hops as traffic grows — one that never does is handing off over a cable. Finds a bridge however few nodes sit behind it.'
												: 'A population of nodes with no alternative route into the mesh — ≥95% of their traffic transits this node. Finds a bridge with a large far side.'}
										>
											<span
												class="rounded-full px-2 py-0.5 text-[0.62rem] font-600 {sig === 'wired'
													? 'bg-amber/15 text-amber'
													: 'bg-coral/15 text-coral'}">{sig}</span
											>
										</Tooltip>
									{/each}
									{#if b.foreignThrough > 0}
										<Tooltip text="foreign nodes with no alternative route — ≥95% of their traffic transits this node">
											<span class="label normal-case tnum text-coral">{b.captiveCount}/{b.foreignThrough} captive</span>
										</Tooltip>
									{/if}
									{#if b.foreignThrough > 0}
										<span class="label normal-case tnum text-fg-faint">{(b.captiveFraction * 100).toFixed(0)}% of foreign</span>
									{/if}
									{#if b.foreignKm > 5}
										<Tooltip text="distance of the captive cluster from the mesh — a hint only, not used for ranking">
											<span class="label normal-case tnum text-fg-faint">{b.foreignKm.toFixed(0)} km</span>
										</Tooltip>
									{/if}
									<!-- Physical behaviour, from all payload types. One next hop over many
									     packets means the egress is a wire; several means it radiates. -->
									{#if b.pathVolume > 0 && b.terminalShare === 0}
										<Tooltip
											text="No observer has ever received this relay's own transmission — it is never the last hop, across {b.pathVolume.toLocaleString()} packets. A relay transmitting on a frequency nobody monitors can't be, however much it carries. Corroborating evidence only; a relay simply out of everyone's range looks the same."
										>
											<span class="label normal-case tnum text-amber">never terminal</span>
										</Tooltip>
									{:else if b.pathVolume > 0}
										<Tooltip text="Share of carried packets where an observer received this relay's own transmission.">
											<span class="label normal-case tnum text-fg-faint"
												>terminal {(b.terminalShare * 100).toFixed(0)}%</span
											>
										</Tooltip>
									{/if}
									{#if b.pathVolume > 0}
										<Tooltip
											text="Distinct nodes this relay was seen handing off to, across {b.pathVolume.toLocaleString()} packets. RF is broadcast, so the next hop varies — a typical relay here has ~13 at ~44%. Exactly one, over many packets, is the signature of a wired link rather than a radio."
										>
											<span
												class="label normal-case tnum {b.nextHops === 1 && b.pathVolume >= 200
													? 'text-amber'
													: 'text-fg-faint'}"
												>{b.nextHops} next hop{b.nextHops === 1 ? '' : 's'} · {(
													b.nextHopTopShare * 100
												).toFixed(0)}%</span
											>
										</Tooltip>
									{/if}
									<div class="ml-auto flex items-center gap-2">
										<button onclick={() => (expanded[b.nodeKey] = !expanded[b.nodeKey])} class="label hover:text-signal">
											{expanded[b.nodeKey] ? 'hide' : 'show'} nodes
										</button>
										{#if isBlocked('bridge', b.nodeKey)}
											<span class="label text-amber">quarantined</span>
										{:else}
											<Tooltip text="A real bridge you run on purpose. It moves to the Known bridges list and stops appearing as a candidate. Nothing is blocked or hidden.">
												<button
													onclick={() => markKnown(b)}
													disabled={busy === b.nodeKey}
													class="border-signal/40 text-signal hover:bg-signal/15 ml-2 rounded-[var(--radius)] border px-3 py-1 text-xs font-600 transition-colors disabled:opacity-50"
												>Known</button>
											</Tooltip>
											<Tooltip text="Not a bridge — stop flagging this node">
												<button
													onclick={() => dismissBridge(b)}
													disabled={busy === b.nodeKey}
													class="border-line/60 text-fg-faint hover:text-fg hover:bg-line/30 rounded-[var(--radius)] border px-3 py-1 text-xs font-600 transition-colors disabled:opacity-50"
												>Dismiss</button>
											</Tooltip>
											<button
												onclick={() => quarantineBridge(b)}
												disabled={busy === b.nodeKey}
												class="border-amber/40 text-amber hover:bg-amber/15 rounded-[var(--radius)] border px-3 py-1 text-xs font-600 transition-colors disabled:opacity-50"
											>Quarantine</button>
										{/if}
										<button
											onclick={() => purgeBridge(b)}
											disabled={busy === b.nodeKey}
											class="border-coral/40 text-coral hover:bg-coral/15 rounded-[var(--radius)] border px-3 py-1 text-xs font-600 transition-colors disabled:opacity-50"
										>Purge</button>
									</div>
								</div>
								{#if expanded[b.nodeKey]}
									<div class="mt-2 flex flex-wrap gap-1.5 pl-5">
										{#each b.foreign as f (f.key)}
											<span
												class="flex items-center gap-1.5 rounded-[var(--radius)] border px-2 py-0.5 text-[0.68rem] {f.captive ? 'border-coral/40 text-fg-dim' : 'border-line/40 text-fg-faint'}"
											>
												<span class="h-1.5 w-1.5 rounded-full" style="background:{roleColor(f.role ?? '')}"></span>
												{f.name}
												<span class="tnum {f.captive ? 'text-coral' : 'text-fg-faint'}">{(f.transitPct ?? 0).toFixed(0)}%</span>
											</span>
										{/each}
									</div>
									<p class="text-fg-faint mt-1.5 pl-5 text-[0.62rem]">
										% = share of that node's traffic transiting this candidate; <span class="text-coral">coral</span> = captive (≥95%, no alternative route). Quarantine/Purge act on captive nodes only.
									</p>
								{/if}
							</div>
						{/each}
					</div>
				{/if}
			</section>
			{/if}

			

			<!-- Nodes that stopped being heard directly -->
			{#if report.migrations.length > 0}
				<section class="panel rise mt-6">
					<div class="border-line/70 flex items-center gap-2.5 border-b px-5 py-3.5">
						<h2 class="font-display text-fg text-sm font-700 tracking-wide">MOVED BEHIND A BRIDGE</h2>
						<span class="label normal-case text-fg-faint"
							>nodes that stopped being heard directly and now arrive through a bridge</span
						>
						<span class="label ml-auto tnum">{report.migrations.length}</span>
					</div>
					<div class="divide-line/60 divide-y">
						{#each report.migrations as m (m.key)}
							<div class="flex flex-wrap items-center gap-x-3 gap-y-1 px-5 py-3">
								<a href="/nodes/{m.key}" class="text-fg hover:text-signal text-sm font-600">{m.name}</a>
								<span class="font-mono text-fg-faint text-[0.62rem]">{m.key.slice(0, 12)}…</span>
								{#if m.viaBridge}
									<Tooltip
										text="A bridge candidate carries this node's traffic since it went quiet — it moved to the far side rather than simply out of range."
									>
										<span class="bg-amber/15 text-amber rounded-full px-2 py-0.5 text-[0.62rem] font-600"
											>now behind {m.viaBridge}</span
										>
									</Tooltip>
								{/if}
								<span class="label normal-case tnum text-fg-faint"
									>last heard directly {ago(m.lastDirectAt)} · {m.relayedAfter} relayed since</span
								>
							</div>
						{/each}
					</div>
				</section>
			{/if}

			<!-- MQTT injectors -->
			{#if (report.injectors?.length ?? 0) > 0}
			<section class="panel rise mt-6">
				<div class="border-line/70 flex items-center gap-2.5 border-b px-5 py-3.5">
					<h2 class="font-display text-fg text-sm font-700 tracking-wide">MQTT INJECTOR CANDIDATES</h2>
					<span class="label normal-case text-fg-faint">observers that are the sole source of nodes</span>
					<span class="label ml-auto tnum">{report.injectors?.length ?? 0}</span>
				</div>
				{#if true}
					<div class="divide-line/40 divide-y">
						{#each report.injectors as i (i.observer)}
							<div class="px-5 py-3">
								<div class="flex flex-wrap items-center gap-3">
									<span class="h-2 w-2 shrink-0 rounded-full" style="background:var(--color-amber)"></span>
									<span class="text-fg font-600">{i.observer}</span>
									<span class="label normal-case tnum text-amber">{i.exclusiveCount} exclusive nodes</span>
									<div class="ml-auto flex items-center gap-2">
										<button onclick={() => (expanded[i.observer] = !expanded[i.observer])} class="label hover:text-signal">
											{expanded[i.observer] ? 'hide' : 'show'} nodes
										</button>
										{#if isBlocked('observer', i.observer)}
											<span class="label text-amber">quarantined</span>
										{:else}
											<button
												onclick={() => quarantineInjector(i)}
												disabled={busy === i.observer}
												class="border-amber/40 text-amber hover:bg-amber/15 rounded-[var(--radius)] border px-3 py-1 text-xs font-600 transition-colors disabled:opacity-50"
											>Quarantine</button>
										{/if}
										<button
											onclick={() => purgeInjector(i)}
											disabled={busy === i.observer}
											class="border-coral/40 text-coral hover:bg-coral/15 rounded-[var(--radius)] border px-3 py-1 text-xs font-600 transition-colors disabled:opacity-50"
										>Purge</button>
									</div>
								</div>
								{#if expanded[i.observer]}
									<div class="mt-2 flex flex-wrap gap-1.5 pl-5">
										{#each i.exclusive as f (f.key)}
											<span class="border-line/60 text-fg-dim rounded-[var(--radius)] border px-2 py-0.5 text-[0.68rem]">{f.name}</span>
										{/each}
									</div>
								{/if}
							</div>
						{/each}
					</div>
				{/if}
			</section>
			{/if}

		
		{/if}

		<!-- Known bridges: real, sanctioned, and deliberately kept -->
		{#if knownEntries.length > 0}
			<section class="panel rise mt-6">
				<div class="border-line/70 flex items-center gap-2.5 border-b px-5 py-3.5">
					<h2 class="font-display text-fg text-sm font-700 tracking-wide">KNOWN BRIDGES</h2>
					<span class="label normal-case text-fg-faint"
						>bridges you run on purpose — still detected and listed, nothing blocked</span
					>
					<span class="label ml-auto tnum">{knownEntries.length}</span>
				</div>
				<div class="divide-line/40 divide-y">
					{#each knownEntries as b (b.kind + b.key)}
						<div class="flex items-center gap-3 px-5 py-2.5 text-sm">
							<span class="label text-signal !text-[0.58rem]">known</span>
							<a href="/nodes/{b.key}" class="text-fg hover:text-signal min-w-0 flex-1 truncate">{b.name || b.key}</a>
							{#if b.reason}<span class="text-fg-faint text-xs">{b.reason}</span>{/if}
							<button
								onclick={() => removeBlock(b)}
								disabled={busy === b.kind + b.key}
								class="label hover:text-signal disabled:opacity-50">unmark</button
							>
						</div>
					{/each}
				</div>
			</section>
		{/if}

		<!-- Active quarantines -->
		{#if quarantineEntries.length > 0}
		<section class="panel rise mt-6">
			<div class="border-line/70 flex items-center gap-2.5 border-b px-5 py-3.5">
				<h2 class="font-display text-fg text-sm font-700 tracking-wide">QUARANTINE LIST</h2>
				<span class="label normal-case text-fg-faint">blocked = dropped at ingest + hidden · dismissed = excluded from detection</span>
				<span class="label ml-auto tnum">{quarantineEntries.length}</span>
			</div>
			<div class="divide-line/40 divide-y">
					{#each quarantineEntries as b (b.kind + b.key)}
						<div class="flex items-center gap-3 px-5 py-2.5 text-sm">
							<span class="label !text-[0.58rem]" style="color:{kindColor[b.kind] ?? 'var(--color-fg-dim)'}">{kindLabel(b.kind)}</span>
							<span class="text-fg min-w-0 flex-1 truncate">{b.name || b.key}</span>
							{#if b.reason}<span class="text-fg-faint text-xs">{b.reason}</span>{/if}
							<button
								onclick={() => removeBlock(b)}
								disabled={busy === b.kind + b.key}
								class="label hover:text-signal disabled:opacity-50"
							>release</button>
						</div>
					{/each}
				</div>
		</section>
		{/if}

		<!-- Purged (deleted + still blocked so they can't re-ingest) -->
		{#if purgedEntries.length > 0}
			<section class="panel rise mt-6">
				<div class="border-line/70 flex items-center gap-2.5 border-b px-5 py-3.5">
					<h2 class="font-display text-fg-dim text-sm font-700 tracking-wide">PURGED</h2>
					<span class="label normal-case text-fg-faint">deleted · still blocked so they can't re-ingest</span>
					<span class="label ml-auto tnum">{purgedEntries.length}</span>
				</div>
				<div class="divide-line/40 divide-y">
					{#each purgedEntries as b (b.kind + b.key)}
						<div class="flex items-center gap-3 px-5 py-2.5 text-sm opacity-70">
							<span class="label !text-[0.58rem]" style="color:{kindColor[b.kind] ?? 'var(--color-fg-dim)'}">{kindLabel(b.kind)}</span>
							<span class="text-fg-dim min-w-0 flex-1 truncate">{b.name || b.key}</span>
							<Tooltip text="Lift the block (data is already deleted; the node could re-ingest on its next advert)">
								<button
									onclick={() => removeBlock(b)}
									disabled={busy === b.kind + b.key}
									class="label hover:text-signal disabled:opacity-50"
								>unblock</button>
							</Tooltip>
							<Tooltip text="Permanently delete and remove from this list">
								<button
									onclick={() => deletePurged(b)}
									disabled={busy === b.kind + b.key}
									class="label hover:text-coral disabled:opacity-50"
								>delete</button>
							</Tooltip>
						</div>
					{/each}
				</div>
			</section>
		{/if}

		<MembersPanel />
	{/if}
</div>
