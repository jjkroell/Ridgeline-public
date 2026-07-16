<script lang="ts">
	import { onMount, tick } from 'svelte';
	import Seo from '$lib/components/Seo.svelte';
	import { channels, type Channel } from '$lib/channels.svelte';
	import { decryptGroupText } from '$lib/channel-crypto';
	import { live } from '$lib/live.svelte';
	import { api, type LiveEvent } from '$lib/api';
	import { ago } from '$lib/format';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import Tooltip from '$lib/components/Tooltip.svelte';

	// How far back the reader pulls history (server caps channel history at 24h).
	const HISTORY_SEC = 86400;

	let history = $state<LiveEvent[]>([]);
	let loadingHistory = $state(true);
	let selectedId = $state<string | null>(null);
	// On mobile only one pane shows at a time; desktop shows both.
	let mobilePane = $state<'list' | 'chat'>('list');

	onMount(async () => {
		channels.init();
		if (!selectedId && channels.list.length) selectedId = channels.list[0].id;
		try {
			history = await api.channelHistory(HISTORY_SEC);
		} finally {
			loadingHistory = false;
		}
	});

	interface ChatMsg {
		hash: string;
		sender: string;
		text: string;
		ts: number; // sender's clock (decrypted)
		receivedAt: string; // earliest observation
		t: number; // earliest observation epoch ms (sort/display)
		observers: number; // how many observers heard this transmission
	}

	// One decode pass over history + the live buffer, bucketed per channel and
	// deduped by message hash (one bubble per transmission, not per observer).
	const byChannel = $derived.by(() => {
		type Acc = Omit<ChatMsg, 'observers'> & { obs: Set<string> };
		const buckets = new Map<string, Map<string, Acc>>();
		for (const ev of [...history, ...live.events]) {
			if (ev.payloadType !== 'GroupText' || !ev.payloadRaw) continue;
			const hb = ev.payloadRaw.slice(0, 2).toUpperCase();
			for (const c of channels.list) {
				if (c.hashByte !== hb) continue;
				const d = decryptGroupText(ev.payloadRaw, c.keyHex);
				if (!d) continue;
				let m = buckets.get(c.id);
				if (!m) buckets.set(c.id, (m = new Map()));
				const t = +new Date(ev.receivedAt);
				const ex = m.get(ev.messageHash);
				if (!ex) {
					m.set(ev.messageHash, {
						hash: ev.messageHash,
						sender: d.sender,
						text: d.text,
						ts: d.ts,
						receivedAt: ev.receivedAt,
						t,
						obs: new Set(ev.observerId ? [ev.observerId] : [])
					});
				} else {
					if (t < ex.t) {
						ex.t = t;
						ex.receivedAt = ev.receivedAt;
					}
					if (ev.observerId) ex.obs.add(ev.observerId);
				}
				break; // a valid HMAC means this payload belongs to this channel
			}
		}
		const out = new Map<string, ChatMsg[]>();
		for (const [id, m] of buckets) {
			const arr = [...m.values()]
				.map(({ obs, ...rest }) => ({ ...rest, observers: obs.size }))
				.sort((a, b) => a.t - b.t);
			out.set(id, arr);
		}
		return out;
	});

	const selected = $derived(channels.list.find((c) => c.id === selectedId) ?? null);
	const messages = $derived(selectedId ? (byChannel.get(selectedId) ?? []) : []);
	// Per-channel sort (persisted): 'asc' = newest at bottom, 'desc' = newest at top.
	const sortDir = $derived(channels.getSort(selectedId));
	const displayMessages = $derived(sortDir === 'desc' ? [...messages].reverse() : messages);
	const lastOf = (id: string): ChatMsg | undefined => {
		const a = byChannel.get(id);
		return a && a.length ? a[a.length - 1] : undefined;
	};

	function selectChannel(id: string) {
		selectedId = id;
		mobilePane = 'chat';
		pinned = true;
	}

	// --- Auto-scroll: stay pinned to whichever edge holds the newest message ---
	let scroller = $state<HTMLDivElement>();
	let pinned = $state(true);
	function onScroll() {
		if (!scroller) return;
		pinned =
			sortDir === 'desc'
				? scroller.scrollTop < 80
				: scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight < 80;
	}
	$effect(() => {
		// re-run when the channel, its message count, or the sort direction changes
		void selectedId;
		void displayMessages.length;
		void sortDir;
		if (pinned && scroller)
			tick().then(() => {
				if (scroller) scroller.scrollTop = sortDir === 'desc' ? 0 : scroller.scrollHeight;
			});
	});

	function toggleSort() {
		if (!selected) return;
		channels.toggleSort(selected.id);
		pinned = true; // jump to the newest edge after flipping
	}

	// --- Add-channel forms (collapsible) ---
	let adding = $state(false);
	let hashtagName = $state('');
	let hashtagErr = $state<string | null>(null);
	let privateName = $state('');
	let privateKey = $state('');
	let privateErr = $state<string | null>(null);

	function addHashtag() {
		hashtagErr = channels.addHashtag(hashtagName);
		if (!hashtagErr) {
			const added = channels.list[channels.list.length - 1];
			hashtagName = '';
			selectChannel(added.id);
			adding = false;
		}
	}
	function addPrivate() {
		privateErr = channels.addPrivate(privateName, privateKey);
		if (!privateErr) {
			const added = channels.list[channels.list.length - 1];
			privateName = '';
			privateKey = '';
			selectChannel(added.id);
			adding = false;
		}
	}

	function removeChannel(id: string) {
		channels.remove(id);
		if (selectedId === id) selectedId = channels.list[0]?.id ?? null;
		mobilePane = 'list';
	}

	// --- Key reveal / copy in the channel settings disclosure ---
	let showKey = $state(false);
	let copied = $state(false);
	async function copyKey(c: Channel) {
		try {
			await navigator.clipboard.writeText(c.keyHex);
			copied = true;
			setTimeout(() => (copied = false), 1200);
		} catch {
			/* clipboard unavailable */
		}
	}
	let settingsOpen = $state(false);
	$effect(() => {
		// collapse the settings disclosure when switching channels
		void selectedId;
		settingsOpen = false;
		showKey = false;
	});

	const typeColor: Record<Channel['type'], string> = {
		public: 'var(--color-signal)',
		hashtag: 'var(--color-amber)',
		private: 'var(--color-coral)'
	};

	// Deterministic, theme-friendly color per sender for chat readability.
	const palette = [
		'var(--color-signal)',
		'var(--color-amber)',
		'#7aa2f7',
		'#bb9af7',
		'#9ece6a',
		'var(--color-coral)'
	];
	function senderColor(name: string): string {
		let h = 0;
		for (let i = 0; i < name.length; i++) h = (h * 31 + name.charCodeAt(i)) >>> 0;
		return palette[h % palette.length];
	}
	function clockTime(iso: string): string {
		return new Date(iso).toLocaleString();
	}

	const inputCls =
		'w-full rounded-[var(--radius)] border border-line bg-panel-2/50 px-3 py-2 font-mono text-sm text-fg placeholder:text-fg-faint focus:border-signal/60 focus:outline-none';
	const btnCls =
		'rounded-[var(--radius)] border border-signal/50 bg-signal/10 px-3 py-2 text-sm font-medium text-signal transition-colors hover:bg-signal/20 disabled:opacity-40';
</script>

<Seo
	title="MeshCore Channels"
	description="Public channel activity on the MeshCore LoRa mesh."
	path="/channels"
/>

<div class="flex h-[calc(100dvh-3.25rem)] flex-col md:h-screen">
	<PageHeader eyebrow="Decrypted Traffic" title="Channels">
		<div class="font-mono text-fg-dim text-xs">
			<span class="text-signal tnum">{channels.list.length}</span>
			<span class="text-fg-faint">configured · stored in this browser</span>
		</div>
	</PageHeader>

	<div class="border-line/80 flex min-h-0 flex-1 border-t">
		<!-- ============================ CHANNEL LIST ============================ -->
		<aside
			class="border-line/80 w-full shrink-0 flex-col border-r md:flex md:w-[18rem] {mobilePane ===
			'chat'
				? 'hidden'
				: 'flex'}"
		>
			<div class="border-line/70 flex items-center justify-between border-b px-4 py-3">
				<div class="label">Your channels</div>
				<button
					onclick={() => (adding = !adding)}
					class="text-fg-dim hover:border-signal/50 hover:text-signal border-line rounded-[var(--radius)] border px-2 py-1 text-xs transition-colors"
				>
					{adding ? 'Close' : '+ Add'}
				</button>
			</div>

			{#if adding}
				<div class="border-line/70 space-y-4 border-b px-4 py-4">
					<form class="space-y-2" onsubmit={(e) => { e.preventDefault(); addHashtag(); }}>
						<div class="label">Hashtag channel</div>
						<div class="flex items-center gap-1.5">
							<span class="text-fg-faint font-mono text-sm">#</span>
							<input bind:value={hashtagName} placeholder="name" class={inputCls} oninput={() => (hashtagErr = null)} />
							<button type="submit" class={btnCls} disabled={!hashtagName.trim()}>Add</button>
						</div>
						{#if hashtagErr}<div class="text-coral text-xs">{hashtagErr}</div>{/if}
					</form>
					<form class="space-y-2" onsubmit={(e) => { e.preventDefault(); addPrivate(); }}>
						<div class="label">Private channel</div>
						<input bind:value={privateName} placeholder="name" class={inputCls} oninput={() => (privateErr = null)} />
						<div class="flex items-center gap-1.5">
							<input bind:value={privateKey} placeholder="32 hex key" spellcheck="false" autocomplete="off" class={inputCls} oninput={() => (privateErr = null)} />
							<button type="submit" class={btnCls} disabled={!privateName.trim() || !privateKey.trim()}>Add</button>
						</div>
						{#if privateErr}<div class="text-coral text-xs">{privateErr}</div>{/if}
					</form>
				</div>
			{/if}

			<div class="min-h-0 flex-1 overflow-y-auto py-2">
				{#if channels.list.length === 0}
					<div class="text-fg-faint px-4 py-8 text-center text-xs">No channels configured.</div>
				{:else}
					{#each channels.list as c (c.id)}
						{@const last = lastOf(c.id)}
						{@const on = c.id === selectedId}
						<button
							onclick={() => selectChannel(c.id)}
							class="relative flex w-full flex-col gap-0.5 px-4 py-2.5 text-left transition-colors {on
								? 'bg-panel-2/60'
								: 'hover:bg-panel-2/30'}"
						>
							{#if on}<span class="bg-signal absolute top-1/2 left-0 h-7 w-[2px] -translate-y-1/2 rounded-full"></span>{/if}
							<div class="flex items-center gap-2">
								<span class="h-2 w-2 shrink-0 rounded-full" style="background:{typeColor[c.type]}"></span>
								<span class="font-display truncate text-sm font-700 {on ? 'text-fg' : 'text-fg-dim'}">{c.name}</span>
								{#if last}<span class="text-fg-faint ml-auto shrink-0 font-mono text-[0.62rem]">{ago(last.receivedAt)}</span>{/if}
							</div>
							<div class="text-fg-faint truncate pl-4 text-xs">
								{#if last}{last.sender ? `${last.sender}: ` : ''}{last.text}{:else}<span class="italic">no recent messages</span>{/if}
							</div>
						</button>
					{/each}
				{/if}

				{#if !channels.hasPublic}
					<button
						onclick={() => channels.restorePublic()}
						class="text-fg-faint hover:text-signal mx-4 mt-2 text-xs"
					>
						+ Restore Public channel
					</button>
				{/if}
			</div>
		</aside>

		<!-- ============================= CHAT READER ============================= -->
		<section
			class="min-w-0 flex-1 flex-col md:flex {mobilePane === 'list' ? 'hidden' : 'flex'}"
		>
			{#if !selected}
				<div class="text-fg-faint flex flex-1 items-center justify-center px-6 text-center text-sm">
					{channels.list.length ? 'Select a channel to read its messages.' : 'Add a channel to get started.'}
				</div>
			{:else}
				<!-- Header + settings -->
				<header class="border-line/70 border-b px-4 py-3.5 md:px-6">
					<div class="flex items-center gap-3">
						<button
							onclick={() => (mobilePane = 'list')}
							aria-label="Back to channels"
							class="text-fg-faint hover:text-fg -ml-1 shrink-0 md:hidden"
						>
							<svg viewBox="0 0 24 24" class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M15 18l-6-6 6-6" /></svg>
						</button>
						<span class="h-2.5 w-2.5 shrink-0 rounded-full" style="background:{typeColor[selected.type]}"></span>
						<h2 class="font-display text-fg truncate text-xl font-700">{selected.name}</h2>
						<Tooltip
							text={sortDir === 'desc' ? 'Newest first — tap for oldest first' : 'Oldest first — tap for newest first'}
							class="ml-auto shrink-0"
						>
							<button
								onclick={toggleSort}
								class="text-fg-faint hover:text-fg flex items-center gap-1 font-mono text-[0.62rem] transition-colors"
							>
								<svg viewBox="0 0 24 24" class="h-3.5 w-3.5 transition-transform {sortDir === 'desc' ? 'rotate-180' : ''}" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M12 5v14M6 13l6 6 6-6" /></svg>
								<span class="hidden sm:inline">{sortDir === 'desc' ? 'Newest first' : 'Oldest first'}</span>
							</button>
						</Tooltip>
						<span class="text-fg-faint font-mono text-xs"><span class="text-signal tnum">{messages.length}</span> msgs</span>
						<button
							onclick={() => (settingsOpen = !settingsOpen)}
							aria-label="Channel settings"
							class="text-fg-faint hover:text-fg shrink-0 transition-colors"
						>
							<svg viewBox="0 0 24 24" class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3" /><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" /></svg>
						</button>
					</div>

					{#if settingsOpen}
						<div class="border-line/60 mt-3 flex flex-wrap items-center gap-2 border-t pt-3">
							<span class="label shrink-0">Hash</span>
							<span class="font-mono text-fg-dim mr-2 text-xs">{selected.hashByte}</span>
							<span class="label shrink-0">Key</span>
							<code class="font-mono text-fg-dim text-xs break-all">{showKey ? selected.keyHex : '•'.repeat(32)}</code>
							<button onclick={() => (showKey = !showKey)} class="text-fg-faint hover:text-fg text-xs">{showKey ? 'hide' : 'show'}</button>
							<button onclick={() => selected && copyKey(selected)} class="text-xs {copied ? 'text-signal' : 'text-fg-faint hover:text-fg'}">{copied ? 'copied' : 'copy'}</button>
							<button onclick={() => selected && removeChannel(selected.id)} class="text-fg-faint hover:text-coral hover:border-coral/50 border-line ml-auto rounded-[var(--radius)] border px-2 py-1 text-xs transition-colors">Remove channel</button>
						</div>
					{/if}
				</header>

				<!-- Messages -->
				<div bind:this={scroller} onscroll={onScroll} class="min-h-0 flex-1 space-y-3 overflow-y-auto px-4 py-5 md:px-6">
					{#if loadingHistory && messages.length === 0}
						<div class="text-fg-faint py-12 text-center text-sm">Loading messages…</div>
					{:else if messages.length === 0}
						<div class="text-fg-faint py-12 text-center text-sm">
							No messages on this channel in the last 6 hours.
						</div>
					{:else}
						{#each displayMessages as m (m.hash)}
							<div class="panel max-w-[42rem] px-4 py-2.5">
								<div class="mb-0.5 flex items-baseline gap-2">
									{#if m.sender}
										<span class="font-display text-sm font-700" style="color:{senderColor(m.sender)}">{m.sender}</span>
									{:else}
										<span class="font-display text-fg-faint text-sm font-700 italic">anon</span>
									{/if}
									<Tooltip text={clockTime(m.receivedAt)}><span class="text-fg-faint font-mono text-[0.62rem]">{ago(m.receivedAt)} ago</span></Tooltip>
									{#if m.observers > 1}<span class="text-fg-faint ml-auto font-mono text-[0.62rem]">heard ×{m.observers}</span>{/if}
								</div>
								<div class="text-fg text-sm break-words whitespace-pre-wrap">{m.text}</div>
							</div>
						{/each}
					{/if}
				</div>
			{/if}
		</section>
	</div>
</div>
