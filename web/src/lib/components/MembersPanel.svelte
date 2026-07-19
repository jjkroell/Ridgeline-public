<script lang="ts">
	// Registered-member management — promote/demote admins, block/unblock, remove —
	// shared by the desktop (/admin) and mobile (/m/admin) consoles so the two stay
	// in parity. Self-loading: it fetches the member list itself once admin status
	// is confirmed, so host pages just drop it in. `compact` switches to the mobile
	// card chrome and tightens padding.
	import { auth } from '$lib/auth.svelte';
	import { adminUsers, type AuthUser } from '$lib/api';
	import { ago } from '$lib/format';

	let { compact = false }: { compact?: boolean } = $props();

	const shell = $derived(
		compact
			? 'border-line/60 bg-panel mb-3 rounded-2xl border overflow-hidden'
			: 'panel rise mt-6 overflow-hidden'
	);
	const head = $derived(compact ? 'px-4 py-3' : 'px-5 py-3.5');
	const row = $derived(compact ? 'px-4 py-3' : 'px-5 py-3');

	let members = $state<AuthUser[]>([]);
	let loadingMembers = $state(false);
	let membersError = $state('');
	let memberBusyId = $state<number | null>(null);
	let confirmDeleteId = $state<number | null>(null);

	async function loadMembers() {
		if (!auth.isAdmin) return;
		loadingMembers = true;
		membersError = '';
		try {
			members = await adminUsers.list();
		} catch (e) {
			membersError = String((e as Error).message ?? e);
		} finally {
			loadingMembers = false;
		}
	}

	async function setMemberAdmin(u: AuthUser, isAdmin: boolean) {
		memberBusyId = u.id;
		membersError = '';
		try {
			await adminUsers.setAdmin(auth.csrf, u.id, isAdmin);
			await loadMembers();
		} catch (e) {
			membersError = String((e as Error).message ?? e);
		} finally {
			memberBusyId = null;
		}
	}

	async function setMemberBlocked(u: AuthUser, blocked: boolean) {
		memberBusyId = u.id;
		membersError = '';
		try {
			await adminUsers.setBlocked(auth.csrf, u.id, blocked);
			await loadMembers();
		} catch (e) {
			membersError = String((e as Error).message ?? e);
		} finally {
			memberBusyId = null;
		}
	}

	async function removeMember(u: AuthUser) {
		memberBusyId = u.id;
		membersError = '';
		try {
			await adminUsers.remove(auth.csrf, u.id);
			confirmDeleteId = null;
			await loadMembers();
		} catch (e) {
			membersError = String((e as Error).message ?? e);
		} finally {
			memberBusyId = null;
		}
	}

	// Load exactly once, when admin status is confirmed (auth.isAdmin flips
	// false→true after the /me probe). The `loaded` guard is essential: gating on
	// members.length would loop forever with zero members, since loadMembers
	// reassigns the array and re-triggers the effect.
	let loaded = $state(false);
	$effect(() => {
		if (auth.isAdmin && !loaded) {
			loaded = true;
			loadMembers();
		}
	});
</script>

<section class={shell}>
	<div class="border-line/70 flex items-center gap-2.5 border-b {head}">
		<h2 class="font-display text-fg font-700 tracking-wide {compact ? 'text-xs' : 'text-sm'}">
			MEMBERS
		</h2>
		<span class="label normal-case text-fg-faint">{members.length} registered</span>
		<button
			onclick={loadMembers}
			class="label hover:text-signal ml-auto transition-colors"
			disabled={loadingMembers}>{loadingMembers ? 'Loading…' : 'Refresh'}</button
		>
	</div>
	{#if membersError}
		<div class="text-coral py-3 text-xs {compact ? 'px-4' : 'px-5'}">{membersError}</div>
	{/if}
	<div class="divide-line/60 divide-y">
		{#each members as m (m.id)}
			{@const self = m.id === auth.user?.id}
			<div
				class="flex flex-wrap items-center gap-3 {row} {m.blocked ? 'opacity-60' : ''}"
			>
				<div class="min-w-0 flex-1">
					<div class="flex items-center gap-2">
						<span class="text-fg truncate text-sm font-600">{m.displayName || m.email}</span>
						{#if m.isOwner}
							<span class="bg-signal/15 text-signal rounded-full px-2 py-0.5 text-[0.62rem] font-600"
								>Owner</span
							>
						{/if}
						{#if m.blocked}
							<span class="bg-coral/15 text-coral rounded-full px-2 py-0.5 text-[0.62rem] font-600"
								>Blocked</span
							>
						{/if}
					</div>
					<div class="text-fg-faint truncate text-xs">{m.email} · joined {ago(m.createdAt)}</div>
				</div>
				<label class="text-fg-dim flex items-center gap-1.5 text-xs">
					<input
						type="checkbox"
						checked={m.isAdmin}
						disabled={memberBusyId === m.id || self || m.isOwner}
						onchange={(e) => setMemberAdmin(m, e.currentTarget.checked)}
						class="accent-signal"
					/>
					Admin
				</label>
				<!-- Moderation: never available for the owner or your own account. -->
				{#if !m.isOwner && !self}
					<div class="flex items-center gap-3">
						<button
							onclick={() => setMemberBlocked(m, !m.blocked)}
							disabled={memberBusyId === m.id}
							class="text-xs font-600 transition-colors disabled:opacity-50 {m.blocked
								? 'text-signal hover:text-signal/80'
								: 'text-amber hover:text-amber/80'}"
						>
							{m.blocked ? 'Unblock' : 'Block'}
						</button>
						{#if confirmDeleteId === m.id}
							<button
								onclick={() => removeMember(m)}
								disabled={memberBusyId === m.id}
								class="text-coral text-xs font-700 disabled:opacity-50">Confirm</button
							>
							<button
								onclick={() => (confirmDeleteId = null)}
								class="text-fg-faint hover:text-fg-dim text-xs">Cancel</button
							>
						{:else}
							<button
								onclick={() => (confirmDeleteId = m.id)}
								class="text-coral/80 hover:text-coral text-xs font-600">Remove</button
							>
						{/if}
					</div>
				{/if}
			</div>
		{/each}
		{#if members.length === 0 && !loadingMembers}
			<div class="text-fg-faint py-6 text-center text-sm {compact ? 'px-4' : 'px-5'}">
				No registered members.
			</div>
		{/if}
	</div>
</section>
