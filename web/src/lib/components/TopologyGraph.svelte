<script lang="ts">
	// Fullscreen, interactive force-directed view of the mesh relay backbone.
	// Pan (drag empty space), zoom (wheel), reposition a node (drag it), hover to
	// highlight a node and its neighbours, click a node to open it. Rendered on a
	// canvas with a hand-rolled force simulation so it stays smooth at scale.
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { roleColor, shortKey } from '$lib/format';
	import type { TopologyNode, TopologyEdge } from '$lib/api';

	// nodePath lets the mobile PWA (/m/nodes) reuse this with its own detail route.
	let {
		nodes,
		edges,
		nodePath = '/nodes'
	}: { nodes: TopologyNode[]; edges: TopologyEdge[]; nodePath?: string } = $props();

	let canvas: HTMLCanvasElement;
	let raf = 0;
	let resetView: () => void = () => {};

	onMount(() => {
		// only nodes that participate in an edge (isolated nodes carry no topology)
		const connected = new Set<string>();
		for (const e of edges) {
			connected.add(e.a);
			connected.add(e.b);
		}
		const gNodes = nodes.filter((n) => connected.has(n.publicKey));

		const dpr = window.devicePixelRatio || 1;
		const ctx = canvas.getContext('2d')!;

		// Canvas can't parse `var(--…)`, so resolve the role palette to concrete
		// hex once from the computed stylesheet (theme-aware). roleColor() returns
		// CSS custom properties — fine for HTML, useless for a 2D context.
		const cs = getComputedStyle(document.documentElement);
		const cvar = (name: string) => cs.getPropertyValue(name).trim();
		const roleHex: Record<string, string> = {
			Repeater: cvar('--color-role-repeater'),
			ChatNode: cvar('--color-role-companion'),
			RoomServer: cvar('--color-role-room'),
			Sensor: cvar('--color-role-sensor'),
			Observer: cvar('--color-role-observer')
		};
		const fallbackHex = cvar('--color-fg-faint') || '#8b9bad';
		const fillFor = (role: string) => roleHex[role] || fallbackHex;
		const fit = () => {
			const W = canvas.clientWidth,
				H = canvas.clientHeight;
			canvas.width = W * dpr;
			canvas.height = H * dpr;
			return { W, H };
		};
		let { W, H } = fit();

		const idx = new Map(gNodes.map((n, i) => [n.publicKey, i]));
		type PN = { n: TopologyNode; x: number; y: number; vx: number; vy: number; fixed: boolean; deg: number };
		const P: PN[] = gNodes.map((n, i) => ({
			n,
			x: W / 2 + Math.cos(i * 2.4) * (60 + (i % 180)),
			y: H / 2 + Math.sin(i * 2.4) * (60 + (i % 180)),
			vx: 0,
			vy: 0,
			fixed: false,
			deg: 0
		}));
		const E = edges
			.filter((e) => idx.has(e.a) && idx.has(e.b))
			.map((e) => ({ a: idx.get(e.a)!, b: idx.get(e.b)!, w: e.weight }));
		const adj: Set<number>[] = P.map(() => new Set());
		for (const e of E) {
			adj[e.a].add(e.b);
			adj[e.b].add(e.a);
			P[e.a].deg++;
			P[e.b].deg++;
		}
		const maxRelayed = Math.max(1, ...gNodes.map((n) => n.relayed));

		// Only the busiest ~12 nodes ("major hubs") carry a persistent label so the
		// view isn't a wall of names; everything else is revealed on hover. Cutoff
		// is the 12th-highest degree, floored so tiny graphs still label a few.
		const degsSorted = P.map((p) => p.deg).sort((a, b) => b - a);
		const hubCutoff = Math.max(4, degsSorted[Math.min(degsSorted.length - 1, 11)] ?? 4);

		let scale = 1,
			tx = 0,
			ty = 0,
			alpha = 1;
		let hover = -1,
			dragNode = -1,
			panning = false,
			moved = false;
		let lastX = 0,
			lastY = 0,
			downX = 0,
			downY = 0;
		// Active touch/mouse pointers → two of them = pinch-zoom (mobile).
		const pointers = new Map<number, { x: number; y: number }>();
		let pinchDist = 0;
		const pinch = () => {
			const p = [...pointers.values()];
			const dx = p[0].x - p[1].x,
				dy = p[0].y - p[1].y;
			return { dist: Math.hypot(dx, dy), cx: (p[0].x + p[1].x) / 2, cy: (p[0].y + p[1].y) / 2 };
		};
		const wx = (sx: number) => (sx - tx) / scale;
		const wy = (sy: number) => (sy - ty) / scale;
		const rad = (p: PN) => 3 + Math.sqrt(p.n.relayed / maxRelayed) * 9;

		function nodeAt(sx: number, sy: number) {
			const x = wx(sx),
				y = wy(sy);
			let best = -1,
				bd = 1e9;
			for (let i = 0; i < P.length; i++) {
				const d = (P[i].x - x) ** 2 + (P[i].y - y) ** 2;
				if (d < bd) {
					bd = d;
					best = i;
				}
			}
			if (best < 0) return -1;
			const r = rad(P[best]) + 6;
			return bd <= r * r ? best : -1;
		}
		function physics() {
			if (alpha < 0.005) return;
			for (let i = 0; i < P.length; i++)
				for (let j = i + 1; j < P.length; j++) {
					const dx = P[i].x - P[j].x,
						dy = P[i].y - P[j].y,
						d2 = dx * dx + dy * dy + 0.01,
						d = Math.sqrt(d2),
						f = 900 / d2;
					const fx = (dx / d) * f,
						fy = (dy / d) * f;
					P[i].vx += fx;
					P[i].vy += fy;
					P[j].vx -= fx;
					P[j].vy -= fy;
				}
			for (const e of E) {
				const a = P[e.a],
					b = P[e.b],
					dx = b.x - a.x,
					dy = b.y - a.y,
					d = Math.sqrt(dx * dx + dy * dy) + 0.01,
					f = (d - 80) * 0.02;
				const fx = (dx / d) * f,
					fy = (dy / d) * f;
				a.vx += fx;
				a.vy += fy;
				b.vx -= fx;
				b.vy -= fy;
			}
			for (const p of P) {
				if (p.fixed) {
					p.vx = 0;
					p.vy = 0;
					continue;
				}
				p.vx += (W / 2 - p.x) * 0.002;
				p.vy += (H / 2 - p.y) * 0.002;
				p.x += p.vx * alpha;
				p.y += p.vy * alpha;
				p.vx *= 0.85;
				p.vy *= 0.85;
			}
			alpha *= 0.99;
		}
		function draw() {
			ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
			ctx.clearRect(0, 0, W, H);
			ctx.translate(tx, ty);
			ctx.scale(scale, scale);
			const hl = hover >= 0;
			for (const e of E) {
				const on = hl && (e.a === hover || e.b === hover);
				ctx.strokeStyle = on
					? 'rgba(45,212,167,.85)'
					: hl
						? 'rgba(139,155,173,.06)'
						: 'rgba(139,155,173,.22)';
				ctx.lineWidth = (on ? 2 : 1) / scale;
				ctx.beginPath();
				ctx.moveTo(P[e.a].x, P[e.a].y);
				ctx.lineTo(P[e.b].x, P[e.b].y);
				ctx.stroke();
			}
			for (let i = 0; i < P.length; i++) {
				const p = P[i],
					r = rad(p),
					dim = hl && i !== hover && !adj[hover].has(i);
				ctx.globalAlpha = dim ? 0.2 : 1;
				ctx.beginPath();
				ctx.arc(p.x, p.y, r, 0, 7);
				ctx.fillStyle = fillFor(p.n.role);
				ctx.fill();
				if (i === hover) {
					ctx.lineWidth = 2 / scale;
					ctx.strokeStyle = '#e6edf3';
					ctx.stroke();
				}
				const isHub = p.deg >= hubCutoff;
				if (i === hover || (hl && adj[hover].has(i)) || (!hl && isHub)) {
					ctx.globalAlpha = 1;
					// major hubs read brighter + a touch larger than the spoke names
					// that surface on hover
					const hubLabel = isHub && !hl;
					ctx.fillStyle = hubLabel ? '#e6edf3' : '#c7d2de';
					ctx.font = `${(hubLabel ? 12 : 11) / scale}px ui-sans-serif, system-ui`;
					ctx.fillText((p.n.name || shortKey(p.n.publicKey)).slice(0, 22), p.x + r + 2 / scale, p.y + 3 / scale);
				}
			}
			ctx.globalAlpha = 1;
		}
		function loop() {
			physics();
			draw();
			raf = requestAnimationFrame(loop);
		}

		const onWheel = (ev: WheelEvent) => {
			ev.preventDefault();
			const rect = canvas.getBoundingClientRect(),
				mx = ev.clientX - rect.left,
				my = ev.clientY - rect.top;
			const ns = Math.max(0.15, Math.min(6, scale * (ev.deltaY < 0 ? 1.12 : 1 / 1.12)));
			tx = mx - (mx - tx) * (ns / scale);
			ty = my - (my - ty) * (ns / scale);
			scale = ns;
		};
		const onDown = (ev: PointerEvent) => {
			canvas.setPointerCapture(ev.pointerId);
			const rect = canvas.getBoundingClientRect();
			const mx = ev.clientX - rect.left,
				my = ev.clientY - rect.top;
			pointers.set(ev.pointerId, { x: mx, y: my });
			if (pointers.size === 2) {
				// second finger down → start pinch, abandon any single-touch gesture
				if (dragNode >= 0) {
					P[dragNode].fixed = false;
					dragNode = -1;
				}
				panning = false;
				pinchDist = pinch().dist;
				return;
			}
			lastX = downX = mx;
			lastY = downY = my;
			moved = false;
			const hit = nodeAt(mx, my);
			if (hit >= 0) {
				dragNode = hit;
				P[hit].fixed = true;
			} else {
				panning = true;
				canvas.style.cursor = 'grabbing';
			}
		};
		const onMove = (ev: PointerEvent) => {
			const rect = canvas.getBoundingClientRect(),
				mx = ev.clientX - rect.left,
				my = ev.clientY - rect.top;
			if (pointers.has(ev.pointerId)) pointers.set(ev.pointerId, { x: mx, y: my });
			if (pointers.size === 2) {
				const { dist, cx, cy } = pinch();
				if (pinchDist > 0 && dist > 0) {
					const ns = Math.max(0.15, Math.min(6, scale * (dist / pinchDist)));
					tx = cx - (cx - tx) * (ns / scale);
					ty = cy - (cy - ty) * (ns / scale);
					scale = ns;
				}
				pinchDist = dist;
				return;
			}
			if (Math.abs(mx - downX) + Math.abs(my - downY) > 3) moved = true;
			if (dragNode >= 0) {
				P[dragNode].x = wx(mx);
				P[dragNode].y = wy(my);
				P[dragNode].vx = 0;
				P[dragNode].vy = 0;
				alpha = Math.max(alpha, 0.3);
			} else if (panning) {
				tx += mx - lastX;
				ty += my - lastY;
			} else {
				hover = nodeAt(mx, my);
				canvas.style.cursor = hover >= 0 ? 'pointer' : 'grab';
			}
			lastX = mx;
			lastY = my;
		};
		const onUp = (ev: PointerEvent) => {
			try {
				canvas.releasePointerCapture(ev.pointerId);
			} catch {
				/* ignore */
			}
			pointers.delete(ev.pointerId);
			if (pointers.size < 2) pinchDist = 0;
			if (dragNode >= 0) {
				P[dragNode].fixed = false;
				if (!moved) goto(`${nodePath}/${P[dragNode].n.publicKey}`);
				dragNode = -1;
			}
			panning = false;
			canvas.style.cursor = 'grab';
		};
		const onLeave = () => {
			hover = -1;
		};

		canvas.addEventListener('wheel', onWheel, { passive: false });
		canvas.addEventListener('pointerdown', onDown);
		canvas.addEventListener('pointermove', onMove);
		canvas.addEventListener('pointerup', onUp);
		canvas.addEventListener('pointercancel', onUp);
		canvas.addEventListener('pointerleave', onLeave);
		const ro = new ResizeObserver(() => {
			const d = fit();
			W = d.W;
			H = d.H;
		});
		ro.observe(canvas);
		resetView = () => {
			scale = 1;
			tx = 0;
			ty = 0;
			alpha = 1;
			for (const p of P) p.fixed = false;
		};
		loop();

		return () => {
			cancelAnimationFrame(raf);
			ro.disconnect();
			canvas.removeEventListener('wheel', onWheel);
			canvas.removeEventListener('pointerdown', onDown);
			canvas.removeEventListener('pointermove', onMove);
			canvas.removeEventListener('pointerup', onUp);
			canvas.removeEventListener('pointercancel', onUp);
			canvas.removeEventListener('pointerleave', onLeave);
		};
	});

	const legend = [
		{ role: 'Repeater', label: 'Repeater' },
		{ role: 'ChatNode', label: 'Companion' },
		{ role: 'RoomServer', label: 'Room' },
		{ role: 'Sensor', label: 'Sensor' }
	];
</script>

<div class="relative h-full w-full overflow-hidden">
	<canvas bind:this={canvas} class="h-full w-full" style="cursor:grab;touch-action:none"></canvas>
	<button
		onclick={() => resetView()}
		class="border-line bg-panel/85 text-fg-dim hover:border-signal/50 hover:text-signal absolute top-3 right-3 rounded-[var(--radius)] border px-2.5 py-1 font-mono text-[0.68rem] font-600 backdrop-blur transition-colors"
		>⟲ Reset view</button
	>
	<div
		class="border-line/70 bg-panel/85 text-fg-dim absolute bottom-3 left-3 flex max-w-[calc(100%-1.5rem)] flex-wrap items-center gap-x-4 gap-y-1 rounded-[var(--radius)] border px-3 py-2 font-mono text-[0.62rem] backdrop-blur"
	>
		{#each legend as l (l.role)}
			<span class="flex items-center gap-1.5"
				><span class="inline-block h-2.5 w-2.5 rounded-full" style="background:{roleColor(l.role)}"></span>{l.label}</span
			>
		{/each}
		<span class="text-fg-faint w-full">scroll / pinch = zoom · drag = pan · drag a node = move it · tap a node = open it</span>
	</div>
</div>
