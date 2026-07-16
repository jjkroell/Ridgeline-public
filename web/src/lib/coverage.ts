// Terrain-based RF coverage prediction, computed client-side from the AWS Open
// Data "terrarium" elevation tiles (the same DEM used for the map hillshade).
//
// v2 model: a true 2-D terrain VIEWSHED. We march rays out from the transmitter
// (ground + antenna height), maintaining a monotonically-rising horizon angle
// corrected for earth curvature (standard 4/3 effective-earth radius), and stamp
// the *visibility of every cell crossed* into a raster grid — not just the outer
// horizon distance. This is what makes terrain SHADOWS appear: a ridge or island
// peak raises the horizon, so lower ground behind it is marked not-visible, even
// when farther terrain pokes back over the top (a disconnected far patch). The
// older v1 produced one radius per azimuth (a star polygon) which structurally
// could not represent shadows, interior holes, or disconnected coverage.
//
// A future upgrade swaps the boolean visibility test for an ITM/Longley-Rice
// path-loss model (public-domain NTIA core) without changing this interface.
const TILE = 256;
const DEM_URL = (z: number, x: number, y: number) =>
	`https://s3.amazonaws.com/elevation-tiles-prod/terrarium/${z}/${x}/${y}.png`;
const EARTH_R = 6371008.8; // mean earth radius (m)
const EFF_EARTH_R = (4 / 3) * EARTH_R; // 4/3-earth for standard atmospheric refraction
const RAD = Math.PI / 180;

function lonToTileX(lon: number, z: number): number {
	return ((lon + 180) / 360) * 2 ** z;
}
function latToTileY(lat: number, z: number): number {
	const r = (lat * Math.PI) / 180;
	return ((1 - Math.log(Math.tan(r) + 1 / Math.cos(r)) / Math.PI) / 2) * 2 ** z;
}

/** Loads + caches terrarium DEM tiles and samples elevation (metres) at lon/lat. */
export class DemSampler {
	readonly z: number;
	private tiles = new Map<string, Float32Array | null>();
	private pending = new Map<string, Promise<void>>();

	constructor(z = 12) {
		this.z = z;
	}

	private key(x: number, y: number) {
		return `${x}/${y}`;
	}

	private async loadTile(x: number, y: number): Promise<void> {
		const k = this.key(x, y);
		if (this.tiles.has(k)) return;
		if (this.pending.has(k)) return this.pending.get(k);
		const p = new Promise<void>((resolve) => {
			const img = new Image();
			img.crossOrigin = 'anonymous';
			img.onload = () => {
				try {
					const c = document.createElement('canvas');
					c.width = c.height = TILE;
					const ctx = c.getContext('2d', { willReadFrequently: true })!;
					ctx.drawImage(img, 0, 0);
					const d = ctx.getImageData(0, 0, TILE, TILE).data;
					const elev = new Float32Array(TILE * TILE);
					for (let i = 0; i < TILE * TILE; i++) {
						const r = d[i * 4],
							g = d[i * 4 + 1],
							b = d[i * 4 + 2];
						elev[i] = r * 256 + g + b / 256 - 32768;
					}
					this.tiles.set(k, elev);
				} catch {
					this.tiles.set(k, null); // tainted/decoded failure → treat as no data
				}
				resolve();
			};
			img.onerror = () => {
				this.tiles.set(k, null);
				resolve();
			};
			img.src = DEM_URL(this.z, x, y);
		});
		this.pending.set(k, p);
		return p;
	}

	/** Pre-fetch every tile covering the given bbox [minLon, minLat, maxLon, maxLat]. */
	async ensure(minLon: number, minLat: number, maxLon: number, maxLat: number): Promise<void> {
		const x0 = Math.floor(lonToTileX(minLon, this.z));
		const x1 = Math.floor(lonToTileX(maxLon, this.z));
		const y0 = Math.floor(latToTileY(maxLat, this.z)); // note: y grows southward
		const y1 = Math.floor(latToTileY(minLat, this.z));
		const jobs: Promise<void>[] = [];
		for (let x = x0; x <= x1; x++) for (let y = y0; y <= y1; y++) jobs.push(this.loadTile(x, y));
		await Promise.all(jobs);
	}

	/** Elevation in metres at lon/lat (bilinear within the containing tile), or NaN. */
	elev(lon: number, lat: number): number {
		const fx = lonToTileX(lon, this.z);
		const fy = latToTileY(lat, this.z);
		const tx = Math.floor(fx),
			ty = Math.floor(fy);
		const grid = this.tiles.get(this.key(tx, ty));
		if (!grid) return NaN;
		const px = (fx - tx) * TILE,
			py = (fy - ty) * TILE;
		const x0 = Math.min(TILE - 1, Math.max(0, Math.floor(px)));
		const y0 = Math.min(TILE - 1, Math.max(0, Math.floor(py)));
		const x1 = Math.min(TILE - 1, x0 + 1),
			y1 = Math.min(TILE - 1, y0 + 1);
		const dx = px - x0,
			dy = py - y0;
		const e = (xx: number, yy: number) => grid[yy * TILE + xx];
		return (
			e(x0, y0) * (1 - dx) * (1 - dy) +
			e(x1, y0) * dx * (1 - dy) +
			e(x0, y1) * (1 - dx) * dy +
			e(x1, y1) * dx * dy
		);
	}
}

export interface CoverageParams {
	lat: number;
	lon: number;
	txHeightM: number; // transmitter antenna height above ground
	rxHeightM: number; // assumed receiver antenna height above ground
	maxRangeKm: number;
}

// 4 image-source corners, [lon,lat], in MapLibre order: TL, TR, BR, BL.
export type ImageCorners = [
	[number, number],
	[number, number],
	[number, number],
	[number, number]
];

export interface CoverageResult {
	center: [number, number]; // [lon, lat]
	groundElevM: number;
	gridSize: number; // G — cells per side
	grid: Uint8Array; // G*G, row-major; row 0 = north edge, 1 = visible
	imageCoords: ImageCorners; // geo corners the grid maps onto
	dataUrl: string; // teal viewshed rendered to a PNG for an image source
	maxReachKm: number; // farthest visible range
	maxRangeKm: number; // requested cap
}

/** Compute a terrain viewshed (with shadows) for a transmitter. */
export async function computeCoverage(
	p: CoverageParams,
	onProgress?: (frac: number) => void
): Promise<CoverageResult> {
	const maxRange = p.maxRangeKm * 1000;
	const sampler = new DemSampler(12);

	// pad the bbox by ~maxRange around the pin
	const dLat = (maxRange / EARTH_R) * (180 / Math.PI);
	const dLon = dLat / Math.cos(p.lat * RAD);
	await sampler.ensure(p.lon - dLon, p.lat - dLat, p.lon + dLon, p.lat + dLat);

	const ground = sampler.elev(p.lon, p.lat);
	const obs = (Number.isFinite(ground) ? ground : 0) + p.txHeightM;

	// Local equirectangular frame centred on the pin (accurate to ~50 km): east/
	// north metres map linearly to lon/lat, so the raster aligns with a MapLibre
	// image source by its 4 corners.
	const mPerDegLat = 111320;
	const mPerDegLon = 111320 * Math.cos(p.lat * RAD);

	// DEM ground resolution at this latitude (~30–40 m at z12).
	const demStep = Math.max(20, (Math.cos(p.lat * RAD) * 40075016.7) / (2 ** sampler.z * TILE));
	// March in half-DEM steps for a smoother visibility boundary (the DEM is
	// bilinearly interpolated, so sub-post samples are valid) and build a denser
	// output grid so the overlay stays crisp when zoomed in. Capped for memory.
	const step = demStep / 2;
	// Denser output grid (≈1500 cells/side) + less feather below => a more defined,
	// less blurry viewshed boundary, still bounded for memory/compute.
	const cell = Math.max(step, (2 * maxRange) / 1500);
	const G = Math.max(64, Math.min(1700, Math.round((2 * maxRange) / cell)));
	const grid = new Uint8Array(G * G);

	const toCell = (east: number, north: number): number => {
		const col = Math.floor(((east + maxRange) / (2 * maxRange)) * G);
		const row = Math.floor(((maxRange - north) / (2 * maxRange)) * G); // north → top
		if (col < 0 || col >= G || row < 0 || row >= G) return -1;
		return row * G + col;
	};

	// Enough rays that adjacent rays are ≤ one cell apart at the rim (no gaps).
	const nRays = Math.max(720, Math.ceil((2 * Math.PI * maxRange) / cell));
	let maxReach = 0;

	for (let a = 0; a < nRays; a++) {
		const brng = (a * 2 * Math.PI) / nRays; // clockwise from north
		const sinB = Math.sin(brng),
			cosB = Math.cos(brng);
		let maxAng = -Infinity; // rising horizon angle along this ray
		for (let r = step; r <= maxRange; r += step) {
			const east = sinB * r,
				north = cosB * r;
			const terr = sampler.elev(p.lon + east / mPerDegLon, p.lat + north / mPerDegLat);
			if (!Number.isFinite(terr)) continue;
			const effTerr = terr - (r * r) / (2 * EFF_EARTH_R); // earth-curvature drop
			// A receiver here is visible if it clears the horizon raised by all
			// nearer terrain — this is exactly what shadows a valley behind a peak.
			const rxAng = Math.atan2(effTerr + p.rxHeightM - obs, r);
			const visible = rxAng >= maxAng;
			const terrAng = Math.atan2(effTerr - obs, r);
			if (terrAng > maxAng) maxAng = terrAng;
			if (visible) {
				const idx = toCell(east, north);
				if (idx >= 0) grid[idx] = 1;
				if (r > maxReach) maxReach = r;
			}
		}
		if (onProgress && a % 64 === 0) onProgress(a / nRays);
	}
	const c0 = toCell(0, 0);
	if (c0 >= 0) grid[c0] = 1; // the transmitter site itself

	// Render the visible cells to a translucent teal raster (transparent = shadow).
	const raw = document.createElement('canvas');
	raw.width = raw.height = G;
	const rctx = raw.getContext('2d')!;
	const img = rctx.createImageData(G, G);
	for (let i = 0; i < G * G; i++) {
		if (grid[i]) {
			img.data[i * 4] = 52; // #34e3c4 signal-teal
			img.data[i * 4 + 1] = 227;
			img.data[i * 4 + 2] = 196;
			img.data[i * 4 + 3] = 140; // ~0.55 alpha; layer raster-opacity tunes the rest
		}
	}
	rctx.putImageData(img, 0, 0);

	// A light feather just anti-aliases the cell edges without smearing the
	// boundary — the denser grid above keeps it sharp and well-defined.
	const canvas = document.createElement('canvas');
	canvas.width = canvas.height = G;
	const ctx = canvas.getContext('2d')!;
	ctx.filter = 'blur(0.6px)';
	ctx.drawImage(raw, 0, 0);

	const halfLat = maxRange / mPerDegLat;
	const halfLon = maxRange / mPerDegLon;
	const imageCoords: ImageCorners = [
		[p.lon - halfLon, p.lat + halfLat], // TL
		[p.lon + halfLon, p.lat + halfLat], // TR
		[p.lon + halfLon, p.lat - halfLat], // BR
		[p.lon - halfLon, p.lat - halfLat] // BL
	];

	return {
		center: [p.lon, p.lat],
		groundElevM: ground,
		gridSize: G,
		grid,
		imageCoords,
		dataUrl: canvas.toDataURL(),
		maxReachKm: maxReach / 1000,
		maxRangeKm: p.maxRangeKm
	};
}

/** Is the given point inside the visible (non-shadowed) coverage? Grid lookup. */
export function covered(cov: CoverageResult, lon: number, lat: number): boolean {
	const tl = cov.imageCoords[0];
	const br = cov.imageCoords[2];
	const fx = (lon - tl[0]) / (br[0] - tl[0]); // 0 (west) → 1 (east)
	const fy = (lat - tl[1]) / (br[1] - tl[1]); // 0 (north) → 1 (south)
	if (fx < 0 || fx >= 1 || fy < 0 || fy >= 1) return false;
	const col = Math.floor(fx * cov.gridSize);
	const row = Math.floor(fy * cov.gridSize);
	return cov.grid[row * cov.gridSize + col] === 1;
}

/** Great-circle distance in km. */
export function distKm(lat1: number, lon1: number, lat2: number, lon2: number): number {
	const dLat = (lat2 - lat1) * RAD,
		dLon = (lon2 - lon1) * RAD;
	const a =
		Math.sin(dLat / 2) ** 2 +
		Math.cos(lat1 * RAD) * Math.cos(lat2 * RAD) * Math.sin(dLon / 2) ** 2;
	return (EARTH_R * 2 * Math.atan2(Math.sqrt(a), Math.sqrt(1 - a))) / 1000;
}
