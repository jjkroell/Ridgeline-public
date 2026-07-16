<script lang="ts">
	import NodeDetail from './NodeDetail.svelte';
	import Modal from './Modal.svelte';

	interface Props {
		pubkey: string | null;
		onclose: () => void;
	}
	let { pubkey, onclose }: Props = $props();
</script>

{#if pubkey}
	<Modal {onclose} size="2xl">
		<div class="border-line/70 flex items-center gap-3 border-b px-5 py-4">
			<div class="label flex items-center gap-2">
				<span class="bg-signal/70 inline-block h-px w-6"></span> Node Detail
			</div>
			<button onclick={onclose} class="text-fg-faint hover:text-fg ml-auto shrink-0 text-xl leading-none" aria-label="Close">✕</button>
		</div>
		<div class="min-h-0 flex-1 overflow-y-auto px-5 py-5">
			<NodeDetail {pubkey} heading compact />
		</div>
	</Modal>
{/if}
