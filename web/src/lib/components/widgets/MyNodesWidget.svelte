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
				<!-- A claim outlives its node: retention prunes silent nodes but ownership
				     survives, so dormant claims render un-linked instead of 404-ing. -->
				<svelte:element
					this={c.nodePresent ? 'a' : 'div'}
					href={c.nodePresent ? `${base}/nodes/${c.nodePubkey}` : undefined}
					class="flex items-center gap-3 px-5 py-2.5 {c.nodePresent ? 'panel-hover' : 'opacity-70'}"
				>
					<div class="min-w-0 flex-1">
						<div class="flex items-center gap-1.5">
							<span class="text-fg truncate text-sm font-medium">{c.nodeName || shortKey(c.nodePubkey)}</span>
							<OwnershipIcon kind={c.status === 'verified' ? 'owned' : 'pending'} />
						</div>
						<div class="font-mono text-fg-faint mt-0.5 text-[0.68rem]">
							{c.nodePresent
								? c.status === 'verified'
									? 'owned'
									: 'claim pending'
								: 'not currently in the mesh'}
						</div>
					</div>
					{#if c.nodePresent}<RoleBadge role={c.nodeRole} />{/if}
				</svelte:element>
			{/each}
			{#each shared as s (s.nodePubkey)}
				<!-- Shares outlive their node the same way claims do — see the mine loop. -->
				<svelte:element
					this={s.nodePresent ? 'a' : 'div'}
					href={s.nodePresent ? `${base}/nodes/${s.nodePubkey}` : undefined}
					class="flex items-center gap-3 px-5 py-2.5 {s.nodePresent ? 'panel-hover' : 'opacity-70'}"
				>
					<div class="min-w-0 flex-1">
						<div class="flex items-center gap-1.5">
							<span class="text-fg truncate text-sm font-medium">{s.nodeName || shortKey(s.nodePubkey)}</span>
							<OwnershipIcon kind="shared" sharedBy={s.sharedByName} />
						</div>
						<div class="font-mono text-fg-faint mt-0.5 text-[0.68rem]">
							{s.nodePresent ? `shared by ${s.sharedByName}` : 'not currently in the mesh'}
						</div>
					</div>
					{#if s.nodePresent}<RoleBadge role={s.nodeRole} />{/if}
				</svelte:element>
			{/each}
		</div>
	{/if}
</WidgetShell>
