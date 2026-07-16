// Raster-tile equivalents of the MapLibre basemaps, for the WebGL-free Leaflet
// maps. MapLibre's vector styles need WebGL, so each basemap id maps here to a
// free, key-less raster source (XYZ tiles). The ids + labels come from the shared
// BASEMAPS list (map-basemap.ts) so the selector UI and persistence are identical;
// only the rendering differs. `topo` (the default "Hillshade") layers an Esri
// shaded-relief overlay on the themed CARTO base to approximate the GL hillshade.

export interface TileSpec {
	url: string;
	attribution: string;
	subdomains?: string;
	maxZoom: number;
	/** Highest zoom with real tiles; beyond it Leaflet overzooms (stretches) the
	 * last level instead of going blank. Used by the self-hosted terrain (z14). */
	maxNativeZoom?: number;
	/** Overlay opacity (hillshade only). */
	opacity?: number;
}
export interface LeafletBasemap {
	base: TileSpec;
	/** Shaded-relief overlay drawn (multiply-blended) above the base. */
	hillshade?: TileSpec;
	/** Transparent road/place labels drawn above the base (crisp at any zoom). */
	labels?: TileSpec;
}

const CARTO_ATTR =
	'© <a href="https://www.openstreetmap.org/copyright" target="_blank" rel="noopener">OpenStreetMap</a> © <a href="https://carto.com/attributions" target="_blank" rel="noopener">CARTO</a>';
const ESRI_IMAGERY_ATTR =
	'Imagery © <a href="https://www.esri.com" target="_blank" rel="noopener">Esri</a>, Maxar, Earthstar Geographics';
const ESRI_HILLSHADE_ATTR =
	'Hillshade © <a href="https://www.esri.com" target="_blank" rel="noopener">Esri</a>';
const OTM_ATTR =
	'map data © <a href="https://www.openstreetmap.org/copyright" target="_blank" rel="noopener">OpenStreetMap</a> contributors, SRTM | © <a href="https://opentopomap.org" target="_blank" rel="noopener">OpenTopoMap</a>';

const carto = (style: string): TileSpec => ({
	url: `https://{s}.basemaps.cartocdn.com/${style}/{z}/{x}/{y}{r}.png`,
	subdomains: 'abcd',
	maxZoom: 20,
	attribution: CARTO_ATTR
});

// Opacity + blend mode are applied theme-aware in FallbackMap (screen on dark,
// multiply on light) so the relief stays visible on either base.
const ESRI_HILLSHADE: TileSpec = {
	url: 'https://server.arcgisonline.com/ArcGIS/rest/services/Elevation/World_Hillshade/MapServer/tile/{z}/{y}/{x}',
	maxZoom: 16,
	attribution: ESRI_HILLSHADE_ATTR
};

/** Tile config for the given basemap id, theme-aware where the GL base is. */
export function leafletBasemap(id: string, light: boolean): LeafletBasemap {
	switch (id) {
		case 'minimal':
			return { base: carto(light ? 'light_all' : 'dark_all') };
		case 'street':
			return { base: carto('rastertiles/voyager') };
		case 'satellite':
			return {
				base: {
					url: 'https://server.arcgisonline.com/ArcGIS/rest/services/World_Imagery/MapServer/tile/{z}/{y}/{x}',
					maxZoom: 19,
					attribution: ESRI_IMAGERY_ATTR
				}
			};
		case 'terrain':
			return {
				base: {
					url: 'https://{s}.tile.opentopomap.org/{z}/{x}/{y}.png',
					subdomains: 'abc',
					maxZoom: 17,
					attribution: OTM_ATTR
				}
			};
		case 'topo':
		default:
			// Shaded relief over the themed base — the closest raster match to the
			// MapLibre "Hillshade" default.
			return { base: carto(light ? 'light_all' : 'dark_all'), hillshade: ESRI_HILLSHADE };
	}
}
