// Announcement / "what's new" modal state. Auto-shows once per release the first
// time a visitor lands (tracked in localStorage by version), and can be reopened
// on demand from the overview page. Bump CURRENT when there's something new to
// announce and every visitor sees it once again.
const STORAGE_KEY = 'ridgeline-announce-seen';
export const CURRENT = '2026-07-dashboard';

class Announce {
	open = $state(false);

	/** Called once on app load: opens the modal if this release hasn't been seen. */
	init() {
		let seen = '';
		try {
			seen = localStorage.getItem(STORAGE_KEY) ?? '';
		} catch {
			return; // storage blocked — skip the auto-show rather than nag every load
		}
		if (seen !== CURRENT) this.open = true;
	}

	/** Reopen on demand (e.g. the overview "What's new" button). */
	show() {
		this.open = true;
	}

	/** Close and remember this release as seen so it won't auto-open again. */
	close() {
		this.open = false;
		try {
			localStorage.setItem(STORAGE_KEY, CURRENT);
		} catch {
			/* ignore */
		}
	}
}

export const announce = new Announce();
