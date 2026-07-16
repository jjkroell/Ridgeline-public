<script lang="ts">
	import Modal from './Modal.svelte';
	import { announce } from '$lib/announce.svelte';

	// Each entry is one new capability, in user-facing terms.
	const items: { icon: string; title: string; body: string }[] = [
		{
			icon: 'M4 6h10M4 12h7M4 18h13M16 4v4M20 10v4M12 16v4',
			title: 'Build your own dashboard',
			body: 'The Overview is now yours to arrange — hit Customize to drag cards into any order, show or hide them, and add new ones like a mini map, claimed nodes, top relays, network activity and channels. Your layout is saved on your device.'
		},
		{
			icon: 'M4 6h16v12H4zM4 7l8 6 8-6',
			title: 'Accounts with email verification',
			body: 'Create an account, confirm your email, and sign in to unlock the features below.'
		},
		{
			icon: 'M21 2l-2 2m-7.6 7.6a5.5 5.5 0 1 1-7.8 7.8 5.5 5.5 0 0 1 7.8-7.8zm0 0L15.5 7.5m0 0l3 3L22 7l-3-3',
			title: 'Claim your nodes — two ways',
			body: 'Prove ownership by briefly renaming the node with a code, or instantly by signing with the node’s private key (it never leaves your browser).'
		},
		{
			icon: 'M4 6h16v12H4zM4 7l8 6 8-6',
			title: 'Get notified about your nodes',
			body: 'When someone leaves a note on a node you own, we’ll email you so you never miss it.'
		},
		{
			icon: 'M12 2v6m0 8v6M2 12h6m8 0h6M12 8a4 4 0 100 8 4 4 0 000-8z',
			title: 'Manage your account',
			body: 'Change your display name, email, and password anytime from your account page.'
		},
		{
			icon: 'M9 4 3 6v14l6-2 6 2 6-2V4l-6 2-6-2zM9 4v14M15 6v14',
			title: 'New About page',
			body: 'A quick primer on this MeshCore mesh network and how to get involved.'
		}
	];
</script>

{#if announce.open}
	<Modal onclose={() => announce.close()} size="2xl">
		<div class="border-line/70 flex items-center gap-3 border-b px-6 py-4">
			<span class="bg-signal/15 text-signal rounded-full px-2.5 py-1 text-xs font-700">New</span>
			<h2 class="font-display text-fg text-lg font-700">What's new on Ridgeline</h2>
			<button
				onclick={() => announce.close()}
				class="text-fg-faint hover:text-fg ml-auto text-xl leading-none"
				aria-label="Close">✕</button
			>
		</div>

		<div class="min-h-0 flex-1 overflow-y-auto px-6 py-5">
			<ul class="flex flex-col gap-4">
				{#each items as item (item.title)}
					<li class="flex items-start gap-3">
						<span
							class="bg-signal/10 text-signal mt-0.5 grid h-9 w-9 shrink-0 place-items-center rounded-full"
						>
							<svg
								viewBox="0 0 24 24"
								class="h-[18px] w-[18px]"
								fill="none"
								stroke="currentColor"
								stroke-width="1.6"
								stroke-linecap="round"
								stroke-linejoin="round"><path d={item.icon} /></svg
							>
						</span>
						<div class="min-w-0">
							<div class="text-fg text-sm font-600">{item.title}</div>
							<div class="text-fg-dim mt-0.5 text-sm leading-relaxed">{item.body}</div>
						</div>
					</li>
				{/each}
			</ul>
		</div>

		<div class="border-line/70 flex items-center justify-end gap-3 border-t px-6 py-4">
			<a
				href="/about"
				onclick={() => announce.close()}
				class="text-fg-dim hover:text-fg text-sm transition-colors">Learn more</a
			>
			<button
				onclick={() => announce.close()}
				class="bg-signal/15 text-signal border-signal/40 hover:bg-signal/25 rounded-[var(--radius)] border px-4 py-2 text-sm font-600 transition-colors"
				>Got it</button
			>
		</div>
	</Modal>
{/if}
