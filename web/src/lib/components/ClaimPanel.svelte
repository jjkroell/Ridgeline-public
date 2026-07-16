<script lang="ts">
	import { onDestroy } from 'svelte';
	import { claims, type ClaimStatus } from '$lib/api';
	import { auth } from '$lib/auth.svelte';
	import { signChallenge } from '$lib/sign';

	interface Props {
		pubkey: string;
		/** Called after ownership changes (claim/release) so a parent can refresh. */
		onchanged?: () => void;
	}
	let { pubkey, onchanged }: Props = $props();

	let status = $state<ClaimStatus | null>(null);
	let loading = $state(true);
	let busy = $state(false);
	let error = $state('');
	let copied = $state(false);
	let poll: ReturnType<typeof setInterval> | null = null;

	// Private-key proof (alternative to the advert-name method).
	let showKey = $state(false);
	let privKey = $state('');

	async function load() {
		try {
			status = await claims.status(pubkey);
			error = '';
			// Keep polling while there's something to detect live: a pending claim
			// awaiting its verifying advert, or a just-verified node whose name still
			// carries the code (waiting for the owner to restore it). Stop otherwise.
			const watching =
				status.mine?.status === 'pending' || (status.ownedByMe && status.nameNeedsReset);
			if (watching && !poll) {
				poll = setInterval(load, 15000);
			} else if (!watching && poll) {
				clearInterval(poll);
				poll = null;
			}
		} catch (e) {
			error = String((e as Error).message ?? e);
		} finally {
			loading = false;
		}
	}

	// (Re)load whenever the target node changes.
	let loadedFor = $state('');
	$effect(() => {
		if (pubkey && pubkey !== loadedFor) {
			loadedFor = pubkey;
			loading = true;
			load();
		}
	});
	onDestroy(() => poll && clearInterval(poll));

	async function claim() {
		busy = true;
		error = '';
		try {
			await claims.create(auth.csrf, pubkey);
			await load();
			onchanged?.();
		} catch (e) {
			error = String((e as Error).message ?? e);
		} finally {
			busy = false;
		}
	}

	// Prove ownership by signing a server challenge with the node's private key,
	// entirely in-browser. Only the signature is sent — never the key itself.
	async function proveWithKey() {
		busy = true;
		error = '';
		try {
			const { challenge } = await claims.keyChallenge(auth.csrf, pubkey);
			const signature = signChallenge(privKey, challenge);
			await claims.keyVerify(auth.csrf, pubkey, signature);
			privKey = '';
			showKey = false;
			await load();
			onchanged?.();
		} catch (e) {
			error = String((e as Error).message ?? e);
		} finally {
			busy = false;
		}
	}

	function cancelKey() {
		showKey = false;
		privKey = '';
		error = '';
	}

	async function release() {
		busy = true;
		error = '';
		try {
			await claims.release(auth.csrf, pubkey);
			await load();
			onchanged?.();
		} catch (e) {
			error = String((e as Error).message ?? e);
		} finally {
			busy = false;
		}
	}

	async function copyCode() {
		const code = status?.mine?.code;
		if (!code) return;
		try {
			await navigator.clipboard.writeText(code);
			copied = true;
			setTimeout(() => (copied = false), 1500);
		} catch {
			/* clipboard blocked — the code is visible to type manually */
		}
	}

	function expiresIn(iso?: string): string {
		if (!iso) return '';
		const ms = new Date(iso).getTime() - Date.now();
		if (ms <= 0) return 'expired';
		const m = Math.round(ms / 60000);
		return m >= 1 ? `~${m} min` : '<1 min';
	}
</script>

{#snippet keyProof()}
	{#if showKey}
		<div class="border-line mt-3 rounded-[var(--radius)] border p-3">
			<label class="text-fg-dim text-xs font-600" for="pk-{pubkey}">Node private key</label>
			<textarea
				id="pk-{pubkey}"
				bind:value={privKey}
				rows="2"
				placeholder="128-hex MeshCore private key"
				autocomplete="off"
				autocapitalize="off"
				spellcheck="false"
				class="border-line bg-ink-2 text-fg mt-1 w-full rounded-[var(--radius)] border px-2 py-1.5 font-mono text-xs break-all"
			></textarea>
			<p class="text-fg-faint mt-1.5 text-[0.7rem] leading-relaxed">
				<strong class="text-signal">Stays on your device.</strong> Your private key is used only in
				your browser to sign a one-time challenge — it is <strong>never sent</strong> to the server.
			</p>
			<div class="mt-2 flex items-center gap-2">
				<button
					onclick={proveWithKey}
					disabled={busy || !privKey.trim()}
					class="bg-signal/15 text-signal border-signal/40 hover:bg-signal/25 rounded-[var(--radius)] border px-3 py-1.5 text-xs font-600 transition-colors disabled:opacity-50"
					>{busy ? 'Verifying…' : 'Verify ownership'}</button
				>
				<button onclick={cancelKey} class="text-fg-faint hover:text-fg text-xs transition-colors"
					>Cancel</button
				>
			</div>
		</div>
	{:else}
		<button
			onclick={() => {
				showKey = true;
				error = '';
			}}
			class="text-signal/80 hover:text-signal mt-3 text-xs underline-offset-2 transition-colors hover:underline"
			>Have the node's private key? Verify instantly →</button
		>
	{/if}
{/snippet}

<div>
	{#if loading}
		<p class="text-fg-faint text-sm">Checking…</p>
	{:else if status}
		{#if status.ownedByMe}
			<!-- You own it -->
			<div class="flex flex-wrap items-center gap-3">
				<span class="bg-signal/15 text-signal rounded-full px-2.5 py-1 text-xs font-600"
					>You own this node</span
				>
				<button
					onclick={release}
					disabled={busy}
					class="text-fg-faint hover:text-coral ml-auto text-xs transition-colors disabled:opacity-50"
					>Release ownership</button
				>
			</div>
			{#if status.nameNeedsReset}
				<div class="border-amber/40 bg-amber/10 mt-3 rounded-[var(--radius)] border px-3 py-2.5">
					<p class="text-fg-dim text-xs leading-relaxed">
						<strong class="text-amber">Set the name back:</strong> this node's advertised name still
						contains the verification code. Restore its normal name and send another advert — this
						note clears once Ridgeline sees the change.
						<strong class="text-amber">Keep this window open until it does.</strong>
					</p>
				</div>
			{:else}
				<div class="border-signal/40 bg-signal/10 mt-3 rounded-[var(--radius)] border px-3 py-2.5">
					<p class="text-fg-dim text-xs leading-relaxed">
						<strong class="text-signal">Ownership confirmed.</strong> You may now close this window.
					</p>
				</div>
			{/if}
			<p class="text-fg-faint mt-3 text-xs leading-relaxed">
				Use the <span class="text-fg-dim font-600">Private location</span> and
				<span class="text-fg-dim font-600">Notes</span> options in Node Admin to set this node's exact
				location and manage notes.
			</p>
		{:else if status.owner}
			<!-- Owned by someone else -->
			<div class="flex items-center gap-2">
				<span class="bg-signal/15 text-signal rounded-full px-2.5 py-1 text-xs font-600">Claimed</span
				>
				<span class="text-fg-dim text-sm">by {status.owner.displayName}</span>
			</div>
		{:else if status.mine?.status === 'pending'}
			<!-- Your pending claim: show the code + instructions, poll for verification -->
			<div class="border-amber/50 bg-amber/10 mb-3 rounded-[var(--radius)] border px-3 py-2.5">
				<p class="text-amber text-xs font-700 leading-relaxed">
					Do not close this window until you are told to.
				</p>
				<p class="text-fg-dim mt-1 text-[0.7rem] leading-relaxed">
					Claiming takes a couple of steps — leave this open until it confirms you can close it.
				</p>
			</div>
			<p class="text-fg-dim text-sm leading-relaxed">
				To prove you control this node, set its advertised <strong class="text-fg">name</strong> to
				include this code, then send an advert:
			</p>
			<div class="border-line bg-ink-2 mt-3 flex items-center gap-3 rounded-[var(--radius)] border px-4 py-3">
				<code class="text-signal font-mono text-lg font-700 tracking-[0.2em]">{status.mine.code}</code>
				<button
					onclick={copyCode}
					class="border-line text-fg-dim hover:text-fg ml-auto rounded-[var(--radius)] border px-2.5 py-1 text-xs transition-colors"
					>{copied ? 'Copied' : 'Copy'}</button
				>
			</div>
			<ul class="text-fg-faint mt-3 space-y-1 text-xs leading-relaxed">
				<li>
					• Change the node name via the <strong class="text-fg-dim">MeshCore app</strong> (of your
					choice) or via <strong class="text-fg-dim">CLI</strong>, then trigger a flood advert.
				</li>
				<li>• We verify the advert's signature, so only your node can complete this.</li>
				<li>
					• Once it's verified, <strong class="text-fg-dim">change the name back</strong> and send another
					advert so Ridgeline shows the node's correct name.
				</li>
			</ul>
			<div class="mt-3 flex items-center gap-3">
				<span class="flex items-center gap-1.5 text-xs text-amber">
					<span class="inline-block h-2 w-2 animate-pulse rounded-full bg-amber"></span>
					Waiting for advert · code valid {expiresIn(status.mine.expiresAt)}
				</span>
				<button
					onclick={release}
					disabled={busy}
					class="text-fg-faint hover:text-coral ml-auto text-xs transition-colors disabled:opacity-50"
					>Cancel</button
				>
			</div>
			<div class="border-line/60 mt-3 border-t pt-1">
				<p class="text-fg-faint text-[0.7rem]">Prefer not to rename the node?</p>
				{@render keyProof()}
			</div>
		{:else if !status.loggedIn}
			{#if status.previousOwner}
				<p class="text-fg-faint mb-2 text-xs">Previously owned by {status.previousOwner}.</p>
			{/if}
			<p class="text-fg-dim text-sm">
				<a href="/login" class="text-signal hover:underline">Sign in</a> to claim your nodes.
			</p>
		{:else}
			{#if status.previousOwner}
				<p class="text-fg-faint mb-2 text-xs">Previously owned by {status.previousOwner}.</p>
			{/if}
			<div class="flex flex-wrap items-center gap-3">
				<p class="text-fg-dim text-sm">Is this your node?</p>
				<button
					onclick={claim}
					disabled={busy}
					class="bg-signal/15 text-signal border-signal/40 hover:bg-signal/25 ml-auto rounded-[var(--radius)] border px-4 py-2 text-sm font-600 transition-colors disabled:opacity-50"
					>{busy ? 'Starting…' : 'Claim this node'}</button
				>
			</div>
			{@render keyProof()}
		{/if}

		{#if error}
			<p class="text-coral mt-3 text-xs">{error}</p>
		{/if}
	{/if}
</div>
