// Basemaps for the map views. The default ("topo") is the OpenFreeMap flat
// vector style (theme-aware dark/positron) with a Joerd hillshade on top — the
// look the maps have always had. The selector lets a user switch to other free,
// key-less basemaps; the choice is persisted in basemap.svelte.ts.
import type { StyleSpecification, Map as MlMap } from 'maplibre-gl';

// Theme-aware flat vector style — also the topo/minimal base and the inset maps.
export function basemapStyleUrl(light: boolean): string {
	return light
		? 'https://tiles.openfreemap.org/styles/positron'
		: 'https://tiles.openfreemap.org/styles/dark';
}

// One selectable basemap. `themed` flags the styles that follow the UI's
// light/dark theme (the rest are fixed imagery/colour regardless of theme).
export interface BasemapOption {
	id: string;
	label: string;
	desc: string;
	themed: boolean;
}

export const BASEMAPS: BasemapOption[] = [
	{ id: 'topo', label: 'Hillshade', desc: 'Shaded terrain relief over a clean base', themed: true },
	{ id: 'minimal', label: 'Minimal', desc: 'Flat, label-light base', themed: true },
	{ id: 'street', label: 'Street', desc: 'Detailed streets & places', themed: false },
	{ id: 'satellite', label: 'Satellite', desc: 'Aerial imagery', themed: false },
	{ id: 'terrain', label: 'Topographic', desc: 'Contour lines & relief', themed: false }
];

export const BASEMAP_IDS = new Set(BASEMAPS.map((b) => b.id));
export const DEFAULT_BASEMAP = 'topo';

// Minimal raster style. Glyphs are pulled from OpenFreeMap so symbol layers we
// add on top (e.g. the cluster counts) still find their font on a raster base.
function raster(tiles: string[], attribution: string, maxzoom: number): StyleSpecification {
	return {
		version: 8,
		glyphs: 'https://tiles.openfreemap.org/fonts/{fontstack}/{range}.pbf',
		sources: { base: { type: 'raster', tiles, tileSize: 256, attribution, maxzoom } },
		layers: [{ id: 'base', type: 'raster', source: 'base' }]
	} as StyleSpecification;
}

// basemapStyle resolves a basemap id (+ current theme) to a MapLibre style — a
// URL for the vector styles, or an inline raster style spec for imagery/terrain.
export function basemapStyle(id: string, light: boolean): string | StyleSpecification {
	switch (id) {
		case 'street':
			return 'https://tiles.openfreemap.org/styles/liberty';
		case 'satellite':
			return raster(
				['https://server.arcgisonline.com/ArcGIS/rest/services/World_Imagery/MapServer/tile/{z}/{y}/{x}'],
				'Imagery © <a href="https://www.esri.com" target="_blank" rel="noopener">Esri</a>, Maxar, Earthstar Geographics',
				19
			);
		case 'terrain':
			return raster(
				[
					'https://a.tile.opentopomap.org/{z}/{x}/{y}.png',
					'https://b.tile.opentopomap.org/{z}/{x}/{y}.png',
					'https://c.tile.opentopomap.org/{z}/{x}/{y}.png'
				],
				'Map data: © <a href="https://www.openstreetmap.org/copyright" target="_blank" rel="noopener">OpenStreetMap</a> contributors, SRTM | © <a href="https://opentopomap.org" target="_blank" rel="noopener">OpenTopoMap</a> (CC-BY-SA)',
				17
			);
		case 'minimal':
		case 'topo':
		default:
			return basemapStyleUrl(light);
	}
}

// The Joerd hillshade overlay only makes sense on the flat vector "topo" base;
// "minimal" is intentionally flat and the raster terrain/satellite styles carry
// their own relief.
export function basemapHasHillshade(id: string): boolean {
	return id === 'topo';
}

// Self-hosted terrain support was removed from the public build (it depended on a
// private tile server). These stubs keep the map call sites simple; a self-hosted
// terrain option could be reintroduced here behind a configurable tile URL.
export function basemapHasLocalTerrain(_id: string): boolean {
	return false;
}
export function ensureLocalTerrain(_map: MlMap, _light: boolean): void {
	/* no-op: no self-hosted terrain layer in the public build */
}

// MapLibre's compact AttributionControl renders expanded by default. Collapse it
// to the ⓘ button so the credits stay out of the way (still expandable on click).
// Safe to call once the control exists; it stays collapsed afterwards because
// _updateCompact only re-opens when the 'maplibregl-compact' class is absent.
export function collapseAttribution(map: { getContainer(): HTMLElement }): void {
	const el = map.getContainer().querySelector('.maplibregl-ctrl-attrib');
	el?.classList.remove('maplibregl-compact-show');
	el?.removeAttribute('open');
}
