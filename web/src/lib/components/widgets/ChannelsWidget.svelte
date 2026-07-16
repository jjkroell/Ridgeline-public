<script lang="ts">
	import { onMount } from 'svelte';
	import WidgetShell from './WidgetShell.svelte';
	import { overview } from '$lib/overview.svelte';
	import { api, type LiveEvent } from '$lib/api';
	import { live } from '$lib/live.svelte';
	import { channels } from '$lib/channels.svelte';
	import { decryptGroupText } from '$lib/channel-crypto';
	import { ago } from '$lib/format';

	let { base = '' }: { base?: string } = $props();
	const m = overview.meta('channels')!;

	let history = $state<LiveEvent[]>([]);
	async function load() {
		try {
			history = await api.channelHistory(86400);
		} catch {
			/* keep last */
		}
	}
	onMount(load);

	interface Msg {
		hash: string;
		channel: string;
		sender: string;
		text: string;
		receivedAt: string;
	}
	// Decode GroupText from history + the live buffer against the user's saved
	// channel keys, dedupe by message hash, newest first.
	const recent = $derived.by(() => {
		const seen = new Map<string, Msg>();
		for (const ev of [...history, ...live.events]) {
			if (ev.payloadType !== 'GroupText' || !ev.payloadRaw) continue;
			const hb = ev.payloadRaw.slice(0, 2).toUpperCase();
			for (const c of channels.list) {
				if (c.hashByte !== hb) continue;
				const d = decryptGroupText(ev.payloadRaw, c.keyHex);
				if (!d) continue;
				if (!seen.has(ev.messageHash)) {
					seen.set(ev.messageHash, {
						hash: ev.messageHash,
						channel: c.name,
						sender: d.sender,
						text: d.text,
						receivedAt: ev.receivedAt
					});
				}
			}
		}
		return [...seen.values()]
			.sort((a, b) => (b.receivedAt ?? '').localeCompare(a.receivedAt ?? ''))
			.slice(0, 6);
	});
</script>

<WidgetShell title={m.title} icon={m.icon} href="{base}/channels" linkLabel="Open →">
	{#if recent.length === 0}
		<div class="text-fg-faint px-5 py-10 text-center text-sm">No recent channel messages.</div>
	{:else}
		<div class="divide-line/50 divide-y">
			{#each recent as msg (msg.hash)}
				<a href="{base}/channels" class="panel-hover flex flex-col gap-0.5 px-5 py-2.5">
					<div class="flex items-center gap-2 text-xs">
						<span class="text-signal font-mono shrink-0">{msg.channel}</span>
						{#if msg.sender}<span class="text-fg-dim truncate font-medium">{msg.sender}</span>{/if}
						<span class="text-fg-faint ml-auto shrink-0 font-mono">{ago(msg.receivedAt)}</span>
					</div>
					<div class="text-fg truncate text-sm">{msg.text}</div>
				</a>
			{/each}
		</div>
	{/if}
</WidgetShell>
