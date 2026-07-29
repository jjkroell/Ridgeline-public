// Named colour themes, persisted to localStorage. The initial attribute/class is
// applied by an inline script in app.html (before paint); this store mirrors it
// for the picker UI and handles changes.
//
// Two separate signals live on <html>, and they do different jobs:
//   data-theme="<id>"    selects the token block in app.css (mutually exclusive,
//                        so there are no specificity games between themes)
//   class="theme-light"  marks the theme as having a LIGHT base — read by
//                        map-util.isLight() to pick basemap/hillshade variants,
//                        and by color-scheme. Any number of themes may set it.

export type ThemeId = 'dark' | 'light' | 'slate' | 'graphite' | 'mist';

export interface ThemeDef {
	id: ThemeId;
	label: string;
	/** Light-based themes get the `theme-light` class (basemaps, color-scheme). */
	light: boolean;
	/** Picker swatch colours: [ground, panel, accent]. */
	swatch: [string, string, string];
	/** Browser UI colour (PWA address bar / status bar). */
	themeColor: string;
}

export const THEMES: ThemeDef[] = [
	{
		id: 'dark',
		// PUBLIC-ONLY: the private build labels this "Ridgeline". Self-hosters set
		// their own SITE_NAME, so the default theme is named for its accent
		// instead of for the site.
		label: 'Signal',
		light: false,
		swatch: ['#0e151b', '#161f28', '#34e3c4'],
		themeColor: '#070a0e'
	},
	{
		id: 'slate',
		label: 'Slate',
		light: false,
		swatch: ['#0b1220', '#142032', '#5cc8ff'],
		themeColor: '#0b1220'
	},
	{
		id: 'graphite',
		label: 'Graphite',
		light: false,
		swatch: ['#14100c', '#221b14', '#e0a63f'],
		themeColor: '#14100c'
	},
	{
		id: 'light',
		label: 'Paper',
		light: true,
		swatch: ['#e1e5e0', '#fbfcfa', '#0f9e8a'],
		themeColor: '#e1e5e0'
	},
	{
		id: 'mist',
		label: 'Mist',
		light: true,
		swatch: ['#dbe3ec', '#f7fafd', '#2b5fa8'],
		themeColor: '#dbe3ec'
	}
];

const BY_ID = new Map(THEMES.map((t) => [t.id, t]));
const KEY = 'ridgeline-theme';
const DEFAULT: ThemeId = 'dark';

/** Accepts anything from storage; falls back to the default. Pre-existing
 *  installs hold 'dark' or 'light', both of which are still valid ids. */
function coerce(raw: string | null): ThemeId {
	return raw && BY_ID.has(raw as ThemeId) ? (raw as ThemeId) : DEFAULT;
}

class Theme {
	id = $state<ThemeId>(DEFAULT);

	get def(): ThemeDef {
		return BY_ID.get(this.id) ?? THEMES[0];
	}

	/** True when the active theme has a light base. */
	get isLight(): boolean {
		return this.def.light;
	}

	init() {
		let saved: string | null = null;
		try {
			saved = localStorage.getItem(KEY);
		} catch {
			/* storage unavailable */
		}
		this.id = coerce(saved);
		this.#apply();
	}

	set(id: ThemeId) {
		if (!BY_ID.has(id) || id === this.id) return;
		this.id = id;
		try {
			localStorage.setItem(KEY, id);
		} catch {
			/* storage unavailable */
		}
		this.#apply();
	}

	/** Next theme in list order — a one-tap affordance where a full picker
	 *  doesn't fit. */
	cycle() {
		const i = THEMES.findIndex((t) => t.id === this.id);
		this.set(THEMES[(i + 1) % THEMES.length].id);
	}

	#apply() {
		const def = this.def;
		const root = document.documentElement;
		root.dataset.theme = def.id;
		root.classList.toggle('theme-light', def.light);
		document.querySelector('meta[name="theme-color"]')?.setAttribute('content', def.themeColor);
	}
}

export const theme = new Theme();
