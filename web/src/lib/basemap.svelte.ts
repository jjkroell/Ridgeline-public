// Selected basemap, persisted to localStorage. Shared by every map view so the
// user's choice (topo / minimal / street / satellite / terrain) sticks across
// pages and reloads. Defaults to the original topographic look.
import { BASEMAP_IDS, DEFAULT_BASEMAP } from './map-basemap';

const KEY = 'ridgeline-basemap';

class Basemap {
	id = $state<string>(DEFAULT_BASEMAP);

	init() {
		try {
			const saved = localStorage.getItem(KEY);
			if (saved && BASEMAP_IDS.has(saved)) this.id = saved;
		} catch {
			/* storage unavailable */
		}
	}

	set(id: string) {
		if (!BASEMAP_IDS.has(id) || id === this.id) return;
		this.id = id;
		try {
			localStorage.setItem(KEY, id);
		} catch {
			/* storage unavailable */
		}
	}

	/** Apply a basemap for this visit WITHOUT persisting it. Used when a shared
	 *  link specifies one: the sender's choice should frame what the recipient
	 *  sees, but must not overwrite the preference they chose for themselves. */
	preview(id: string) {
		if (!BASEMAP_IDS.has(id)) return;
		this.id = id;
	}
}

export const basemap = new Basemap();
