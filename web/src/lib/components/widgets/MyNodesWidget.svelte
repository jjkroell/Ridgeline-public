<script lang="ts">
	import { onMount } from 'svelte';
	import WidgetShell from './WidgetShell.svelte';
	import { overview } from '$lib/overview.svelte';
	import { auth } from '$lib/auth.svelte';
	import { claims, shares, type ClaimWithNode, type SharedWithMe } from '$lib/api';
	import { shortKey } from '$lib/format';
	import RoleBadge from '$lib/components/RoleBadge.svelte';
	import OwnershipIcon from '$lib/components/OwnershipIcon.svelte';

	let { base = '' }: { base?: string } = $props();
	const m = overview.meta('mynodes')!;

	let mine = $state<ClaimWithNode[]>([]);
	let shared = $state<SharedWithMe[]>([]);

	async function load() {
		if (!auth.loggedIn) {
			mine = [];
			shared = [];
			return;
		}
		try {
			[mine, shared] = await Promise.all([claims.mine(), shares.mine()]);
		} catch {
			/* transient — keep last */
		}
	}
	// Reload whenever the session changes (login/logout).
	$effect(() => {
		void auth.user;
		load();
	});
	onMount(load);
</script>

<WidgetShell title={m.title} icon={m.icon} color="signal" href="{base}/account" linkLabel="Account →">
	{#if !auth.loggedIn}
		<div class="text-fg-faint px-5 py-8 text-center text-sm">
			<a href="{base}/account" class="text-signal hover:underline">Sign in</a> to see nodes you've claimed
			and locations shared with you.
		</div>
	{:else if mine.length === 0 && shared.length === 0}
		<div class="text-fg-faint px-5 py-8 text-center text-sm">
			You haven't claimed any nodes yet. Claim one from its node page.
		</div>
	{:else}
		<div class="divide-line/50 divide-y">
			{#each mine as c (c.nodePubkey)}
				<a href="{base}/nodes/{c.nodePubkey}" class="panel-hover flex items-center gap-3 px-5 py-2.5">
					<div class="min-w-0 flex-1">
						<div class="flex items-center gap-1.5">
							<span class="text-fg truncate text-sm font-medium">{c.nodeName || shortKey(c.nodePubkey)}</span>
							<OwnershipIcon kind={c.status === 'verified' ? 'owned' : 'pending'} />
						</div>
						<div class="font-mono text-fg-faint mt-0.5 text-[0.68rem]">
							{c.status === 'verified' ? 'owned' : 'claim pending'}
						</div>
					</div>
					<RoleBadge role={c.nodeRole} />
				</a>
			{/each}
			{#each shared as s (s.nodePubkey)}
				<a href="{base}/nodes/{s.nodePubkey}" class="panel-hover flex items-center gap-3 px-5 py-2.5">
					<div class="min-w-0 flex-1">
						<div class="flex items-center gap-1.5">
							<span class="text-fg truncate text-sm font-medium">{s.nodeName || shortKey(s.nodePubkey)}</span>
							<OwnershipIcon kind="shared" sharedBy={s.sharedByName} />
						</div>
						<div class="font-mono text-fg-faint mt-0.5 text-[0.68rem]">shared by {s.sharedByName}</div>
					</div>
					<RoleBadge role={s.nodeRole} />
				</a>
			{/each}
		</div>
	{/if}
</WidgetShell>
