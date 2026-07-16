// Light/dark theme, persisted to localStorage. The initial class is applied
// by an inline script in app.html (before paint); this store mirrors it for
// the toggle UI and handles changes.
type Mode = 'dark' | 'light';

const KEY = 'ridgeline-theme';

class Theme {
	mode = $state<Mode>('dark');

	init() {
		let saved: string | null = null;
		try {
			saved = localStorage.getItem(KEY);
		} catch {
			/* storage unavailable */
		}
		this.mode = saved === 'light' ? 'light' : 'dark';
		this.#apply();
	}

	toggle() {
		this.mode = this.mode === 'dark' ? 'light' : 'dark';
		try {
			localStorage.setItem(KEY, this.mode);
		} catch {
			/* storage unavailable */
		}
		this.#apply();
	}

	#apply() {
		document.documentElement.classList.toggle('theme-light', this.mode === 'light');
	}
}

export const theme = new Theme();
