<script lang="ts">
	import { roleColor, roleLabel } from '$lib/format';
	let { role }: { role: string } = $props();
	const color = $derived(roleColor(role));
</script>

<span
	class="font-mono inline-flex items-center gap-1.5 rounded-[var(--radius)] px-2 py-0.5 text-[0.7rem] tracking-wide"
	style="color:{color}; background:color-mix(in srgb, {color} 12%, transparent); border:1px solid color-mix(in srgb, {color} 30%, transparent)"
>
	<span class="inline-block h-1.5 w-1.5 rounded-full" style="background:{color}"></span>
	{roleLabel(role)}
</span>
