<script lang="ts">
	// A compact SVG line sparkline. Renders the trend of a numeric series; null/
	// undefined entries are skipped (gaps bridged). Auto-scales to the value range
	// with a little padding. Width is fluid (100%); height is fixed via the prop.
	let {
		values,
		color = 'var(--color-signal)',
		height = 40,
		fill = true,
		strokeWidth = 1.5
	}: {
		values: (number | null | undefined)[];
		color?: string;
		height?: number;
		fill?: boolean;
		strokeWidth?: number;
	} = $props();

	// Use a fixed viewBox width and let SVG scale it to the container.
	const W = 200;

	const pts = $derived(
		values
			.map((v, i) => ({ v, i }))
			.filter((p): p is { v: number; i: number } => p.v != null && Number.isFinite(p.v))
	);

	const range = $derived.by(() => {
		if (pts.length === 0) return { min: 0, max: 1 };
		let min = Infinity;
		let max = -Infinity;
		for (const p of pts) {
			if (p.v < min) min = p.v;
			if (p.v > max) max = p.v;
		}
		if (min === max) {
			min -= 1;
			max += 1;
		}
		return { min, max };
	});

	// Map a sample to SVG coords. x by its index across the full series; y inverted
	// (SVG origin top-left), padded 10% top/bottom.
	const n = $derived(Math.max(1, values.length - 1));
	function x(i: number): number {
		return (i / n) * W;
	}
	function y(v: number): number {
		const { min, max } = range;
		const t = (v - min) / (max - min); // 0..1
		const pad = height * 0.1;
		return height - pad - t * (height - 2 * pad);
	}

	const line = $derived(pts.map((p) => `${x(p.i).toFixed(2)},${y(p.v).toFixed(2)}`).join(' '));
	const area = $derived(
		pts.length > 0
			? `${x(pts[0].i).toFixed(2)},${height} ${line} ${x(pts[pts.length - 1].i).toFixed(2)},${height}`
			: ''
	);
	const gid = `spark-${Math.random().toString(36).slice(2, 9)}`;
</script>

{#if pts.length >= 2}
	<svg
		viewBox="0 0 {W} {height}"
		preserveAspectRatio="none"
		class="block w-full"
		style="height:{height}px"
		role="img"
	>
		{#if fill}
			<defs>
				<linearGradient id={gid} x1="0" y1="0" x2="0" y2="1">
					<stop offset="0%" stop-color={color} stop-opacity="0.22" />
					<stop offset="100%" stop-color={color} stop-opacity="0" />
				</linearGradient>
			</defs>
			<polygon points={area} fill="url(#{gid})" />
		{/if}
		<polyline
			points={line}
			fill="none"
			stroke={color}
			stroke-width={strokeWidth}
			stroke-linejoin="round"
			stroke-linecap="round"
			vector-effect="non-scaling-stroke"
		/>
	</svg>
{:else}
	<div class="text-fg-faint flex items-center text-[0.62rem]" style="height:{height}px">
		collecting…
	</div>
{/if}
