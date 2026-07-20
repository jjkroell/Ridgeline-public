<script lang="ts">
	import { goto } from '$app/navigation';
	import { auth } from '$lib/auth.svelte';
	import { confirmer } from '$lib/confirm.svelte';
	import { admin, type InjectionReport, type BlockEntry, type BridgeCandidate, type InjectorCandidate } from '$lib/api';
	import { purgeCascade, roleColor, skippedNote } from '$lib/format';
	import MembersPanel from '$lib/components/MembersPanel.svelte';
	import RetiredObserversPanel from '$lib/components/RetiredObserversPanel.svelte';

	// Gated by the signed-in account's is_admin flag (no static token). Bounce
	// non-admins once the /me probe resolves.
	$effect(() => {
		if (auth.ready && !auth.isAdmin) goto('/m/account');
	});
	const authed = $derived(auth.isAdmin);

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
	let busy = $state('');
	let msg = $state('');
	let expanded = $state<Record<string, boolean>>({});
	let scrubKey = $state('');
	let scrubbing = $state(false);

	// Load once when admin status is confirmed. The `loaded` guard is essential:
	// gating on blocks.length loops forever when the blocklist is empty (each
	// refreshBlocks reassigns []→ re-triggers the effect → 429).
	let loaded = $state(false);
	$effect(() => {
		if (auth.isAdmin && !loaded) {
			loaded = true;
			refreshBlocks();
		}
	});
	async function refreshBlocks() { try { blocks = await admin.blocklist(); } catch (e) { msg = `blocklist: ${(e as Error).message}`; } }
	async function runDetect() {
		detecting = true; msg = '';
		try { report = await admin.detect(windowSec); } catch (e) { msg = `detect: ${(e as Error).message}`; } finally { detecting = false; }
	}

	const isBlocked = (kind: string, key: string) => blocks.some((b) => b.kind === kind && b.key.toUpperCase() === key.toUpperCase());
	const isAllowed = (key: string) => blocks.some((b) => b.kind === 'allow' && b.key.toUpperCase() === key.toUpperCase());
	// Dismissed isn't a bridge; known is already decided. Both leave the candidate
	// list — known bridges keep their own section below.
	const visibleBridges = $derived(
		(report?.bridges ?? []).filter((b) => !isAllowed(b.nodeKey) && !b.known && !isKnown(b.nodeKey))
	);
	const isKnown = (key: string) => blocks.some((b) => b.kind === 'known' && b.key.toUpperCase() === key.toUpperCase());
	// A sanctioned bridge is not quarantined — it gets its own list.
	const knownEntries = $derived(blocks.filter((b) => b.kind === 'known'));
	const quarantineEntries = $derived(blocks.filter((b) => b.reason !== 'purged' && b.kind !== 'known'));
	const purgedEntries = $derived(blocks.filter((b) => b.reason === 'purged'));

	async function quarantineBridge(b: BridgeCandidate) {
		busy = b.nodeKey; msg = '';
		try {
			const captive = b.foreign.filter((f) => f.captive).map((f) => f.key);
			await admin.block(auth.csrf, { kind: 'bridge', key: b.nodeKey, name: b.name, reason: 'RF bridge (detected)', nodes: captive });
			await refreshBlocks();
			msg = `Quarantined ${b.name} + ${captive.length} captive nodes.`;
		} catch (e) { msg = `quarantine: ${(e as Error).message}`; } finally { busy = ''; }
	}
	async function markKnown(b: BridgeCandidate) {
		busy = b.nodeKey; msg = '';
		try { await admin.block(auth.csrf, { kind: 'known', key: b.nodeKey, name: b.name, reason: 'known bridge' }); await refreshBlocks(); msg = `${b.name} marked as a known bridge.`; }
		catch (e) { msg = `mark known: ${(e as Error).message}`; } finally { busy = ''; }
	}
	async function dismissBridge(b: BridgeCandidate) {
		busy = b.nodeKey; msg = '';
		try { await admin.block(auth.csrf, { kind: 'allow', key: b.nodeKey, name: b.name, reason: 'dismissed' }); await refreshBlocks(); msg = `Dismissed ${b.name}.`; }
		catch (e) { msg = `dismiss: ${(e as Error).message}`; } finally { busy = ''; }
	}
	async function purgeBridge(b: BridgeCandidate) {
		const captive = b.foreign.filter((f) => f.captive).map((f) => f.key);
		if (!(await confirmer.ask({ title: `Purge bridge ${b.name}?`, message: `Permanently deletes ${captive.length} captive node${captive.length === 1 ? '' : 's'} and blocks the bridge. Cannot be undone.`, confirmLabel: 'Purge', danger: true }))) return;
		busy = b.nodeKey; msg = '';
		try { const r = await admin.purge(auth.csrf, { bridges: [b.nodeKey], nodes: captive }); await refreshBlocks(); report = null; msg = `Purged ${b.name}: ${r.observations} obs, ${r.nodes} nodes.` + skippedNote(r); }
		catch (e) { msg = `purge: ${(e as Error).message}`; } finally { busy = ''; }
	}
	async function quarantineInjector(i: InjectorCandidate) {
		busy = i.observer; msg = '';
		try { await admin.block(auth.csrf, { kind: 'observer', key: i.observer, name: i.observer, reason: 'MQTT injector (detected)' }); await refreshBlocks(); msg = `Quarantined ${i.observer}.`; }
		catch (e) { msg = `quarantine: ${(e as Error).message}`; } finally { busy = ''; }
	}
	async function purgeInjector(i: InjectorCandidate) {
		if (!(await confirmer.ask({ title: `Purge injector ${i.observer}?`, message: `Permanently deletes packets from ${i.observer} and its ${i.exclusiveCount} node${i.exclusiveCount === 1 ? '' : 's'}. Cannot be undone.`, confirmLabel: 'Purge', danger: true }))) return;
		busy = i.observer; msg = '';
		try { const r = await admin.purge(auth.csrf, { observers: [i.observer], nodes: i.exclusive.map((f) => f.key) }); await refreshBlocks(); report = null; msg = `Purged ${i.observer}: ${r.observations} obs.` + skippedNote(r); }
		catch (e) { msg = `purge: ${(e as Error).message}`; } finally { busy = ''; }
	}
	async function removeBlock(b: BlockEntry) {
		busy = b.kind + b.key; msg = '';
		try { await admin.unblock(auth.csrf, b.kind, b.key); await refreshBlocks(); } catch (e) { msg = `unblock: ${(e as Error).message}`; } finally { busy = ''; }
	}
	async function deletePurged(b: BlockEntry) {
		if (!(await confirmer.ask({ title: `Delete ${b.name || b.key}?`, message: 'Removes it from the list and sweeps any remaining data. Cannot be undone.', confirmLabel: 'Delete', danger: true }))) return;
		busy = b.kind + b.key; msg = '';
		try { await admin.deleteNodes(auth.csrf, [b.key]); await admin.unblock(auth.csrf, b.kind, b.key); await refreshBlocks(); } catch (e) { msg = `delete: ${(e as Error).message}`; } finally { busy = ''; }
	}

	// Scrub a node + all its data points by public key. Irreversible.
	async function scrubNode() {
		const key = scrubKey.trim().toUpperCase();
		if (!key) return;
		if (!/^[0-9A-F]{6,64}$/.test(key)) { msg = 'scrub: enter a hex public key (full key for an exact match).'; return; }
		if (!(await confirmer.ask({ title: 'Scrub this node?', message: 'Permanently deletes the node, all of its data points, and any ownership claim, notes, private location and location shares attached to it. This cannot be undone.', code: key, confirmLabel: 'Scrub', danger: true }))) return;
		scrubbing = true; msg = '';
		try {
			const res = await admin.deleteNodes(auth.csrf, [key]);
			const cascaded = purgeCascade(res);
			// No node row but leftover user data = a ghost from a pre-cascade scrub.
			msg = res.nodes > 0 ? `Scrubbed ${key}: ${res.nodes} node row + ${res.observations} data points${cascaded ? ` + ${cascaded}` : ''}.` : cascaded ? `No node matched ${key}, but cleaned up ${cascaded} left over from an earlier scrub.` : `No node matched ${key} (removed ${res.observations} data points). Use the full key.`;
			scrubKey = '';
			await refreshBlocks();
		} catch (e) { msg = `scrub: ${(e as Error).message}`; } finally { scrubbing = false; }
	}

	const kindColor: Record<string, string> = { bridge: 'var(--color-coral)', observer: 'var(--color-amber)', node: 'var(--color-fg-dim)', allow: 'var(--color-signal)' };
	const kindLabel = (k: string) => (k === 'allow' ? 'dismissed' : k);
</script>

<div class="px-4 py-4">
	{#if !auth.ready}
		<div class="text-fg-faint py-16 text-center text-sm">Checking…</div>
	{:else if !authed}
		<div class="text-fg-faint py-16 text-center text-sm">Admin access only. Redirecting…</div>
	{:else}
		<div class="mb-3 flex items-center gap-2">
			<button onclick={runDetect} disabled={detecting} class="border-signal/40 bg-signal/15 text-signal rounded-xl border px-4 py-2 text-sm font-600 disabled:opacity-50">{detecting ? 'Scanning…' : 'Run detection'}</button>
		</div>
		<div class="-mx-4 mb-3 flex gap-2 overflow-x-auto px-4" style="scrollbar-width:none">
			{#each windows as w (w.sec)}
				<button onclick={() => (windowSec = w.sec)} class="shrink-0 rounded-full border px-3 py-1 text-xs font-600 {windowSec === w.sec ? 'border-signal/50 bg-signal/15 text-signal' : 'border-line text-fg-dim'}">{w.label}</button>
			{/each}
			<span class="text-fg-faint ml-auto shrink-0 self-center text-[0.6rem]">short window = fresh bridges</span>
		</div>

		{#if msg}<div class="border-signal/40 text-fg-dim mb-3 rounded-xl border px-3 py-2 text-xs">{msg}</div>{/if}

		<!-- Scrub node by key -->
		<div class="border-line/60 bg-panel mb-3 rounded-2xl border p-4">
			<div class="label normal-case text-fg-faint mb-1">Scrub node by key</div>
			<p class="text-fg-faint mb-3 text-xs">Permanently delete a node + all its data points, plus any ownership claim, notes, private location and location shares attached to it. Paste the full public key. Irreversible.</p>
			<form class="flex flex-col gap-2" onsubmit={(e) => { e.preventDefault(); scrubNode(); }}>
				<input bind:value={scrubKey} placeholder="public key (hex)" spellcheck="false" autocomplete="off" class="border-line bg-ink-2 text-fg focus:border-coral w-full rounded-xl border px-3 py-3 font-mono text-xs outline-none" />
				<button type="submit" disabled={scrubbing || !scrubKey.trim()} class="border-coral/40 bg-coral/15 text-coral rounded-xl border px-4 py-3 text-sm font-600 disabled:opacity-50">{scrubbing ? 'Scrubbing…' : 'Scrub node'}</button>
			</form>
		</div>

		{#if report}
			<div class="border-line/60 bg-panel mb-3 rounded-2xl border px-4 py-3">
				<div class="label normal-case text-fg-faint mb-1">Scan</div>
				<div class="font-mono text-fg-dim text-[0.68rem]">
					{report.packetsScanned.toLocaleString()} packets · {report.advertsScanned.toLocaleString()} adverts / {report.windowHours.toFixed(0)}h
					{#if report.advertsRejected > 0}
						· <span class="text-amber">{report.advertsRejected.toLocaleString()} rejected (bad signature)</span>
					{:else}
						· all signatures verified
					{/if}
				</div>
			</div>

			<!-- bridges -->
			{#if visibleBridges.length > 0}
			<h2 class="font-display text-fg mb-2 px-1 text-xs font-700 tracking-wide">RF BRIDGE CANDIDATES · {visibleBridges.length}</h2>
			<div class="flex flex-col gap-2">
				{#if true}
					{#each visibleBridges as b (b.nodeKey)}
						<div class="border-line/60 bg-panel rounded-2xl border p-3.5">
							<div class="flex items-center gap-2">
								<span class="h-2 w-2 shrink-0 rounded-full" style="background:var(--color-coral)"></span>
								<a href="/m/nodes/{b.nodeKey}" class="text-fg min-w-0 flex-1 truncate text-sm font-600">{b.name}</a>
							</div>
							<div class="text-fg-faint mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1 font-mono text-[0.62rem]">
								{#each b.signals as sig (sig)}
										<span class={sig === 'wired' ? 'text-amber' : 'text-coral'}>{sig}</span>
									{/each}
									{#if b.foreignThrough > 0}
										<span class="text-coral">{b.captiveCount}/{b.foreignThrough} captive</span>
									{/if}
									{#if b.pathVolume > 0}
										<span class={b.nextHops === 1 && b.pathVolume >= 200 ? 'text-amber' : ''}
											>{b.nextHops} next hop{b.nextHops === 1 ? '' : 's'}</span
										>
										{#if b.terminalShare === 0}
											<span class="text-amber">never terminal</span>
										{/if}
									{/if}
								<span>{(b.captiveFraction * 100).toFixed(0)}% of foreign</span>
								{#if b.foreignKm > 5}<span>{b.foreignKm.toFixed(0)} km</span>{/if}
								<button onclick={() => (expanded[b.nodeKey] = !expanded[b.nodeKey])} class="active:text-signal underline">{expanded[b.nodeKey] ? 'hide' : 'show'} nodes</button>
							</div>
							{#if expanded[b.nodeKey]}
								<div class="mt-2 flex flex-wrap gap-1.5">
									{#each b.foreign as f (f.key)}
										<span class="rounded-md border px-1.5 py-0.5 text-[0.6rem] {f.captive ? 'border-coral/40 text-fg-dim' : 'border-line/40 text-fg-faint'}">
											<span class="h-1.5 w-1.5 rounded-full" style="background:{roleColor(f.role ?? '')}"></span>
											{f.name} <span class="tnum {f.captive ? 'text-coral' : ''}">{(f.transitPct ?? 0).toFixed(0)}%</span>
										</span>
									{/each}
								</div>
							{/if}
							<div class="mt-3 flex gap-2">
								{#if isBlocked('bridge', b.nodeKey)}
									<span class="text-amber self-center text-xs">quarantined</span>
								{:else}
									<button onclick={() => markKnown(b)} disabled={busy === b.nodeKey} class="border-signal/40 text-signal flex-1 rounded-xl border py-2 text-xs font-600 disabled:opacity-50">Known</button>
									<button onclick={() => dismissBridge(b)} disabled={busy === b.nodeKey} class="border-line text-fg-dim flex-1 rounded-xl border py-2 text-xs font-600 disabled:opacity-50">Dismiss</button>
									<button onclick={() => quarantineBridge(b)} disabled={busy === b.nodeKey} class="border-amber/40 text-amber flex-1 rounded-xl border py-2 text-xs font-600 disabled:opacity-50">Quarantine</button>
								{/if}
								<button onclick={() => purgeBridge(b)} disabled={busy === b.nodeKey} class="border-coral/40 text-coral flex-1 rounded-xl border py-2 text-xs font-600 disabled:opacity-50">Purge</button>
							</div>
						</div>
					{/each}
				{/if}
			</div>
			{/if}

			

			<!-- injectors -->
			{#if (report.injectors?.length ?? 0) > 0}
			<h2 class="font-display text-fg mt-5 mb-2 px-1 text-xs font-700 tracking-wide">MQTT INJECTORS · {report.injectors?.length ?? 0}</h2>
			<div class="flex flex-col gap-2">
				{#if true}
					{#each report.injectors as i (i.observer)}
						<div class="border-line/60 bg-panel rounded-2xl border p-3.5">
							<div class="flex items-center gap-2">
								<span class="h-2 w-2 shrink-0 rounded-full" style="background:var(--color-amber)"></span>
								<span class="text-fg min-w-0 flex-1 truncate text-sm font-600">{i.observer}</span>
								<span class="text-amber font-mono text-[0.62rem]">{i.exclusiveCount} excl.</span>
							</div>
							<div class="mt-3 flex gap-2">
								{#if isBlocked('observer', i.observer)}
									<span class="text-amber self-center text-xs">quarantined</span>
								{:else}
									<button onclick={() => quarantineInjector(i)} disabled={busy === i.observer} class="border-amber/40 text-amber flex-1 rounded-xl border py-2 text-xs font-600 disabled:opacity-50">Quarantine</button>
								{/if}
								<button onclick={() => purgeInjector(i)} disabled={busy === i.observer} class="border-coral/40 text-coral flex-1 rounded-xl border py-2 text-xs font-600 disabled:opacity-50">Purge</button>
							</div>
						</div>
					{/each}
				{/if}
			</div>
		{/if}

		
		{/if}

		<!-- quarantine list -->
		{#if report && report.migrations.length > 0}
			<h2 class="font-display text-fg mt-5 mb-2 px-1 text-xs font-700 tracking-wide">MOVED BEHIND A BRIDGE · {report.migrations.length}</h2>
			<div class="border-line/60 bg-panel divide-line/50 divide-y overflow-hidden rounded-2xl border">
				{#each report.migrations as m (m.key)}
					<div class="px-4 py-2.5">
						<a href="/m/nodes/{m.key}" class="text-fg text-sm font-600">{m.name}</a>
						{#if m.viaBridge}
							<span class="text-amber ml-1 text-[0.62rem]">now behind {m.viaBridge}</span>
						{/if}
						<div class="text-fg-faint font-mono text-[0.62rem]">{m.relayedAfter} relayed since it stopped being heard directly</div>
					</div>
				{/each}
			</div>
			{/if}

		
		{/if}

		{#if knownEntries.length > 0}
			<h2 class="font-display text-fg mt-5 mb-2 px-1 text-xs font-700 tracking-wide">KNOWN BRIDGES · {knownEntries.length}</h2>
			<div class="border-line/60 bg-panel divide-line/50 divide-y overflow-hidden rounded-2xl border">
				{#each knownEntries as b (b.kind + b.key)}
					<div class="flex items-center gap-3 px-4 py-2.5 text-sm">
						<span class="label text-signal !text-[0.55rem]">known</span>
						<a href="/m/nodes/{b.key}" class="text-fg min-w-0 flex-1 truncate">{b.name || b.key.slice(0, 14)}</a>
						<button onclick={() => removeBlock(b)} disabled={busy === b.kind + b.key} class="text-fg-faint active:text-signal text-xs disabled:opacity-50">unmark</button>
					</div>
				{/each}
			</div>
		{/if}

		{#if quarantineEntries.length > 0}
		<h2 class="font-display text-fg mt-5 mb-2 px-1 text-xs font-700 tracking-wide">QUARANTINE LIST · {quarantineEntries.length}</h2>
		<div class="border-line/60 bg-panel divide-line/50 divide-y overflow-hidden rounded-2xl border">
			{#if true}
				{#each quarantineEntries as b (b.kind + b.key)}
					<div class="flex items-center gap-3 px-4 py-2.5 text-sm">
						<span class="label !text-[0.55rem]" style="color:{kindColor[b.kind]}">{kindLabel(b.kind)}</span>
						<span class="text-fg min-w-0 flex-1 truncate">{b.name || b.key.slice(0, 14)}</span>
						<button onclick={() => removeBlock(b)} disabled={busy === b.kind + b.key} class="text-fg-faint active:text-signal text-xs disabled:opacity-50">release</button>
					</div>
				{/each}
			{/if}
		</div>

		<!-- purged -->
		{#if purgedEntries.length > 0}
			<h2 class="font-display text-fg-dim mt-5 mb-2 px-1 text-xs font-700 tracking-wide">PURGED · {purgedEntries.length}</h2>
			<div class="border-line/60 bg-panel divide-line/50 divide-y overflow-hidden rounded-2xl border opacity-80">
				{#each purgedEntries as b (b.kind + b.key)}
					<div class="flex items-center gap-3 px-4 py-2.5 text-sm">
						<span class="label !text-[0.55rem]" style="color:{kindColor[b.kind]}">{kindLabel(b.kind)}</span>
						<span class="text-fg-dim min-w-0 flex-1 truncate">{b.name || b.key.slice(0, 14)}</span>
						<button onclick={() => removeBlock(b)} disabled={busy === b.kind + b.key} class="text-fg-faint active:text-signal text-xs disabled:opacity-50">unblock</button>
						<button onclick={() => deletePurged(b)} disabled={busy === b.kind + b.key} class="text-fg-faint active:text-coral text-xs disabled:opacity-50">delete</button>
					</div>
				{/each}
			</div>
		{/if}

		<div class="mt-5">
			<MembersPanel compact />
			<RetiredObserversPanel compact />
		</div>
	{/if}
</div>
