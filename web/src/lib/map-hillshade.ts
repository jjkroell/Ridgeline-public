// Subtle terrain relief drawn on top of the flat OpenFreeMap basemap. Elevation
// comes from Tilezen's "Joerd" terrarium-encoded DEM tiles, hosted on the AWS
// Open Data registry (elevation-tiles-prod, no API key, CORS-open). We render a
// single MapLibre `hillshade` layer from it, slipped beneath the basemap's own
// label/symbol layers so place names stay crisp on top of the shaded hills.
import type maplibregl from 'maplibre-gl';

const DEM_SOURCE = 'joerd-dem';
const HILLSHADE_LAYER = 'joerd-hillshade';

const DEM_TILES = ['https://s3.amazonaws.com/elevation-tiles-prod/terrarium/{z}/{x}/{y}.png'];
// Joerd is a composite of public DEMs; this is the attribution AWS asks for.
const DEM_ATTRIBUTION =
	'Elevation: <a href="https://github.com/tilezen/joerd" target="_blank" rel="noopener">Tilezen Joerd</a>';

// Find the lowest symbol (label) layer so the hillshade sits under the basemap
// text but over its fills — keeps roads/place names legible above the relief.
function firstSymbolLayerId(map: maplibregl.Map): string | undefined {
	for (const layer of map.getStyle().layers ?? []) {
		if (layer.type === 'symbol') return layer.id;
	}
	return undefined;
}

// Idempotently add the DEM source + hillshade layer to the current style. Safe
// to call on every `styledata` — setStyle() wipes both, so we re-add them.
export function ensureHillshade(map: maplibregl.Map, light: boolean): void {
	if (!map.isStyleLoaded()) return;

	if (!map.getSource(DEM_SOURCE)) {
		map.addSource(DEM_SOURCE, {
			type: 'raster-dem',
			tiles: DEM_TILES,
			encoding: 'terrarium',
			tileSize: 256,
			maxzoom: 15,
			attribution: DEM_ATTRIBUTION
		});
	}

	if (!map.getLayer(HILLSHADE_LAYER)) {
		map.addLayer(
			{
				id: HILLSHADE_LAYER,
				type: 'hillshade',
				source: DEM_SOURCE,
				paint: {
					'hillshade-exaggeration': light ? 0.35 : 0.5,
					'hillshade-shadow-color': light ? '#5a4a36' : '#020807',
					'hillshade-highlight-color': light ? '#fffaf0' : '#2c5048',
					'hillshade-accent-color': light ? '#8a7a5e' : '#04140f',
					'hillshade-illumination-direction': 315
				}
			},
			firstSymbolLayerId(map)
		);
	} else {
		// Theme swap may keep the layer but needs new shadow/highlight tints.
		map.setPaintProperty(HILLSHADE_LAYER, 'hillshade-exaggeration', light ? 0.35 : 0.5);
		map.setPaintProperty(HILLSHADE_LAYER, 'hillshade-shadow-color', light ? '#5a4a36' : '#020807');
		map.setPaintProperty(HILLSHADE_LAYER, 'hillshade-highlight-color', light ? '#fffaf0' : '#2c5048');
		map.setPaintProperty(HILLSHADE_LAYER, 'hillshade-accent-color', light ? '#8a7a5e' : '#04140f');
	}
}
