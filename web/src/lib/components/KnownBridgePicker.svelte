<!--
  Marks a bridge candidate as sanctioned ("known") AND records which node sits on
  the far side of the link.

  A bridge is a link between two nodes, but detection only ever names the near
  end — the relay whose behaviour gave it away. Naming the far end is what turns
  a list of flagged nodes into a list of links you can read at a glance, and only
  the operator knows which one it is.

  The candidates offered are the node's observed neighbours (adjacent to it in
  packet paths), busiest first, because the far side of a real bridge is by
  definition something it passes traffic to. The operator can also mark it known
  without naming a peer — knowing a bridge is sanctioned is useful on its own.

  Shared by the desktop and mobile admin consoles so the wording and the
  neighbour ranking can't drift apart.
-->
<script lang="ts">
	import { api, type NodeNeighbor } from '$lib/api';
	import { fmtNum, roleColor, roleLabel } from '$lib/format';
	import Modal from '$lib/components/Modal.svelte';

	let {
		nodeKey,
		nodeName,
		compact = false,
		onconfirm,
		oncancel
	}: {
		nodeKey: string;
		nodeName: string;
		compact?: boolean;
		/** peer is '' when the operator marks it known without naming a far side. */
		onconfirm: (peer: string) => void | Promise<void>;
		oncancel: () => void;
	} = $props();

	let neighbors = $state<NodeNeighbor[]>([]);
	let loading = $state(true);
	let error = $state('');
	let selected = $state('');
	let saving = $state(false);

	$effect(() => {
		let cancelled = false;
		loading = true;
		error = '';
		api
			.nodeDetail(nodeKey)
			.then((d) => {
				if (cancelled) return;
				// Never offer the bridge itself as its own far side.
				neighbors = (d.detail?.neighbors ?? []).filter(
					(n) => n.publicKey.toUpperCase() !== nodeKey.toUpperCase()
				);
			})
			.catch((e) => {
				if (!cancelled) error = (e as Error).message;
			})
			.finally(() => {
				if (!cancelled) loading = false;
			});
		return () => {
			cancelled = true;
		};
	});

	async function confirm() {
		saving = true;
		try {
			await onconfirm(selected);
		} finally {
			saving = false;
		}
	}
</script>

<Modal onclose={oncancel} size={compact ? 'lg' : '2xl'}>
	<div class="border-line/70 border-b px-5 py-4">
		<div class="label flex items-center gap-2">
			<span class="bg-signal/70 inline-block h-px w-6"></span> Known bridge
		</div>
		<h2 class="text-fg mt-2 text-sm font-700">{nodeName || nodeKey.slice(0, 12)}</h2>
		<p class="text-fg-dim mt-1.5 text-xs">
			Which node is it bridged to? These are the nodes seen adjacent to it in packet
			paths, busiest first — the far side of a real bridge is something it passes
			traffic to.
		</p>
	</div>

	<div class="max-h-[50vh] min-h-0 flex-1 overflow-y-auto">
		{#if loading}
			<div class="text-fg-faint px-5 py-8 text-center text-sm">Loading neighbours…</div>
		{:else if error}
			<div class="text-coral px-5 py-6 text-xs">Couldn't load neighbours: {error}</div>
		{:else if neighbors.length === 0}
			<div class="text-fg-faint px-5 py-6 text-xs">
				No adjacent nodes observed for this one yet. You can still mark it as a known
				bridge and name the far side later.
			</div>
		{:else}
			<div class="divide-line/40 divide-y">
				{#each neighbors as n (n.publicKey)}
					<label
						class="hover:bg-panel-2/40 flex cursor-pointer items-center gap-3 px-5 py-2.5 text-sm"
					>
						<input type="radio" name="peer" value={n.publicKey} bind:group={selected} class="accent-[var(--color-signal)]" />
						<span class="min-w-0 flex-1 truncate">{n.name}</span>
						<span class="label shrink-0 !text-[0.58rem]" style="color:{roleColor(n.role)}"
							>{roleLabel(n.role)}</span
						>
						<span class="text-fg-faint tnum shrink-0 font-mono text-[0.62rem]"
							>{fmtNum(n.count)} shared</span
						>
					</label>
				{/each}
			</div>
		{/if}
	</div>

	<div class="border-line/70 flex items-center gap-3 border-t px-5 py-3.5">
		{#if selected}
			<button onclick={() => (selected = '')} class="label hover:text-signal transition-colors"
				>clear</button
			>
		{/if}
		<span class="text-fg-faint ml-auto text-xs">
			{selected ? 'Recorded as a link' : 'No far side named'}
		</span>
		<button onclick={oncancel} class="label hover:text-fg transition-colors">cancel</button>
		<button
			onclick={confirm}
			disabled={saving}
			class="bg-signal/15 text-signal border-signal/40 hover:bg-signal/25 rounded-[var(--radius)] border px-3 py-1 text-xs font-600 transition-colors disabled:opacity-50"
		>
			{saving ? 'Saving…' : 'Mark as known'}
		</button>
	</div>
</Modal>
