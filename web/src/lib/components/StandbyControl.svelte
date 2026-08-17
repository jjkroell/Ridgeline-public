<!--
  Admin control to stand an observer down / return it to service. Shared by the
  desktop and mobile observer detail pages so the wording and the confirmation
  copy can't drift apart.

  Standby sits between the two actions that already existed: blocking treats a
  publisher as rogue and is permanent, retiring hides the receiver but keeps
  ingesting everything it reports. This one keeps the receiver visible and
  connected while throwing its packets away — for a receiver being moved,
  bench-tested, or running firmware under suspicion.

  Unlike Retire, this deliberately does NOT navigate away on success: the
  observer stays on this page, and staying here is how the operator sees the
  standby badge and the rising discard count confirming it took effect.
-->
<script lang="ts">
	import { auth } from '$lib/auth.svelte';
	import { confirmer } from '$lib/confirm.svelte';
	import { admin, type Observer } from '$lib/api';
	import Tooltip from '$lib/components/Tooltip.svelte';

	let {
		observer,
		id,
		disabled = false,
		onchange
	}: {
		observer: Observer | null;
		id: string;
		disabled?: boolean;
		onchange?: () => void;
	} = $props();

	let busy = $state(false);
	const onStandby = $derived(!!observer?.standbySince);
	const label = $derived(observer?.name ?? id);

	async function toggle() {
		if (onStandby) {
			busy = true;
			try {
				await admin.resumeObserver(auth.csrf, id);
				onchange?.();
			} catch (e) {
				await confirmer.tell({ title: 'Could not return to service', message: (e as Error).message });
			} finally {
				busy = false;
			}
			return;
		}
		if (
			!(await confirmer.ask({
				title: `Put "${label}" on standby?`,
				message:
					'It stays connected and keeps reporting its own telemetry, but every packet it hears will be discarded instead of stored. Nothing already recorded is affected. Packets discarded while on standby are gone for good — they are not backfilled when you return it to service.',
				confirmLabel: 'Put on standby'
			}))
		)
			return;
		busy = true;
		try {
			await admin.standbyObserver(auth.csrf, id);
			onchange?.();
		} catch (e) {
			await confirmer.tell({ title: 'Could not put on standby', message: (e as Error).message });
		} finally {
			busy = false;
		}
	}
</script>

{#if auth.isAdmin}
	<Tooltip
		text={onStandby
			? 'Resume ingest from this observer. Packets discarded while it was on standby are not recovered.'
			: 'Keep this observer connected but discard every packet it reports, until you return it to service.'}
	>
		<button
			onclick={toggle}
			disabled={busy || disabled}
			class="rounded-[var(--radius)] border px-3 py-1 text-xs font-600 transition-colors disabled:opacity-50 {onStandby
				? 'border-amber/50 text-amber hover:bg-amber/15'
				: 'border-fg-faint/40 text-fg-dim hover:bg-fg-faint/10 hover:text-fg'}"
		>
			{#if busy}
				{onStandby ? 'Returning…' : 'Standing down…'}
			{:else}
				{onStandby ? 'Return to duty' : 'Put on standby'}
			{/if}
		</button>
	</Tooltip>
{/if}
