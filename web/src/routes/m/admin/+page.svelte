<script lang="ts">
	import { goto } from '$app/navigation';
	import { auth } from '$lib/auth.svelte';
	import { confirmer } from '$lib/confirm.svelte';
	import { admin, type InjectionReport, type BlockEntry, type BridgeCandidate, type InjectorCandidate } from '$lib/api';
	import { roleColor } from '$lib/format';

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
	const visibleBridges = $derived((report?.bridges ?? []).filter((b) => !isAllowed(b.nodeKey)));
	const quarantineEntries = $derived(blocks.filter((b) => b.reason !== 'purged'));
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
	async function dismissBridge(b: BridgeCandidate) {
		busy = b.nodeKey; msg = '';
		try { await admin.block(auth.csrf, { kind: 'allow', key: b.nodeKey, name: b.name, reason: 'dismissed' }); await refreshBlocks(); msg = `Dismissed ${b.name}.`; }
		catch (e) { msg = `dismiss: ${(e as Error).message}`; } finally { busy = ''; }
	}
	async function purgeBridge(b: BridgeCandidate) {
		const captive = b.foreign.filter((f) => f.captive).map((f) => f.key);
		if (!(await confirmer.ask({ title: `Purge bridge ${b.name}?`, message: `Permanently deletes ${captive.length} captive node${captive.length === 1 ? '' : 's'} and blocks the bridge. Cannot be undone.`, confirmLabel: 'Purge', danger: true }))) return;
		busy = b.nodeKey; msg = '';
		try { const r = await admin.purge(auth.csrf, { bridges: [b.nodeKey], nodes: captive }); await refreshBlocks(); report = null; msg = `Purged ${b.name}: ${r.observations} obs, ${r.nodes} nodes.`; }
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
		try { const r = await admin.purge(auth.csrf, { observers: [i.observer], nodes: i.exclusive.map((f) => f.key) }); await refreshBlocks(); report = null; msg = `Purged ${i.observer}: ${r.observations} obs.`; }
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
		if (!(await confirmer.ask({ title: 'Scrub this node?', message: 'Permanently deletes the node and all of its data points. This cannot be undone.', code: key, confirmLabel: 'Scrub', danger: true }))) return;
		scrubbing = true; msg = '';
		try {
			const res = await admin.deleteNodes(auth.csrf, [key]);
			msg = res.nodes > 0 ? `Scrubbed ${key}: ${res.nodes} node row + ${res.observations} data points.` : `No node matched ${key} (removed ${res.observations} data points). Use the full key.`;
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
			<p class="text-fg-faint mb-3 text-xs">Permanently delete a node + all its data points. Paste the full public key. Irreversible.</p>
			<form class="flex flex-col gap-2" onsubmit={(e) => { e.preventDefault(); scrubNode(); }}>
				<input bind:value={scrubKey} placeholder="public key (hex)" spellcheck="false" autocomplete="off" class="border-line bg-ink-2 text-fg focus:border-coral w-full rounded-xl border px-3 py-3 font-mono text-xs outline-none" />
				<button type="submit" disabled={scrubbing || !scrubKey.trim()} class="border-coral/40 bg-coral/15 text-coral rounded-xl border px-4 py-3 text-sm font-600 disabled:opacity-50">{scrubbing ? 'Scrubbing…' : 'Scrub node'}</button>
			</form>
		</div>

		{#if report}
			<!-- bridges -->
			<h2 class="font-display text-fg mb-2 px-1 text-xs font-700 tracking-wide">RF BRIDGE CANDIDATES · {visibleBridges.length}</h2>
			<div class="flex flex-col gap-2">
				{#if visibleBridges.length === 0}
					<div class="border-line/60 bg-panel text-fg-faint rounded-2xl border px-4 py-6 text-center text-sm">No RF bridge signature in this window.</div>
				{:else}
					{#each visibleBridges as b (b.nodeKey)}
						<div class="border-line/60 bg-panel rounded-2xl border p-3.5">
							<div class="flex items-center gap-2">
								<span class="h-2 w-2 shrink-0 rounded-full" style="background:var(--color-coral)"></span>
								<a href="/m/nodes/{b.nodeKey}" class="text-fg min-w-0 flex-1 truncate text-sm font-600">{b.name}</a>
							</div>
							<div class="text-fg-faint mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1 font-mono text-[0.62rem]">
								<span class="text-coral">{b.captiveCount}/{b.foreignThrough} captive</span>
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
									<button onclick={() => dismissBridge(b)} disabled={busy === b.nodeKey} class="border-line text-fg-dim flex-1 rounded-xl border py-2 text-xs font-600 disabled:opacity-50">Dismiss</button>
									<button onclick={() => quarantineBridge(b)} disabled={busy === b.nodeKey} class="border-amber/40 text-amber flex-1 rounded-xl border py-2 text-xs font-600 disabled:opacity-50">Quarantine</button>
								{/if}
								<button onclick={() => purgeBridge(b)} disabled={busy === b.nodeKey} class="border-coral/40 text-coral flex-1 rounded-xl border py-2 text-xs font-600 disabled:opacity-50">Purge</button>
							</div>
						</div>
					{/each}
				{/if}
			</div>

			<!-- injectors -->
			<h2 class="font-display text-fg mt-5 mb-2 px-1 text-xs font-700 tracking-wide">MQTT INJECTORS · {report.injectors?.length ?? 0}</h2>
			<div class="flex flex-col gap-2">
				{#if (report.injectors?.length ?? 0) === 0}
					<div class="border-line/60 bg-panel text-fg-faint rounded-2xl border px-4 py-6 text-center text-sm">No rogue publisher in this window.</div>
				{:else}
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

		<!-- quarantine list -->
		<h2 class="font-display text-fg mt-5 mb-2 px-1 text-xs font-700 tracking-wide">QUARANTINE LIST · {quarantineEntries.length}</h2>
		<div class="border-line/60 bg-panel divide-line/50 divide-y overflow-hidden rounded-2xl border">
			{#if quarantineEntries.length === 0}
				<div class="text-fg-faint px-4 py-6 text-center text-sm">Nothing quarantined.</div>
			{:else}
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
	{/if}
</div>
