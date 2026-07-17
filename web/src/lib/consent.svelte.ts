// Cookie / storage consent state (GDPR + PIPEDA).
//
// The app only ever sets strictly-necessary cookies (session, CSRF) and stores
// UI preferences in localStorage — both exempt from prior consent but disclosed
// on /privacy. The one consentable category is analytics, offered only when this
// build ships it (see ANALYTICS_ENABLED). The visitor's choice is recorded in
// localStorage so the banner doesn't reappear; bump CONSENT_VERSION to re-prompt
// everyone if the set of cookies/purposes materially changes.
import { ANALYTICS_ENABLED } from './analytics';

const KEY = 'ridgeline-consent';

/** Increment when the disclosed cookies/purposes change, to re-ask everyone. */
export const CONSENT_VERSION = 1;

interface Stored {
	v: number;
	analytics: boolean;
	at: string; // ISO timestamp of the decision
}

class Consent {
	/** Has the visitor made a choice under the current CONSENT_VERSION? */
	decided = $state(false);
	/** Analytics opt-in. Always false when this build ships no analytics. */
	analytics = $state(false);
	/** Whether the banner / preferences panel is visible. */
	open = $state(false);

	/** Load any saved choice; show the banner if none (or the version changed). */
	init(): void {
		try {
			const raw = localStorage.getItem(KEY);
			if (raw) {
				const s = JSON.parse(raw) as Stored;
				if (s && s.v === CONSENT_VERSION) {
					this.analytics = ANALYTICS_ENABLED && !!s.analytics;
					this.decided = true;
				}
			}
		} catch {
			/* storage blocked — treat as undecided and show the notice */
		}
		this.open = !this.decided;
	}

	private persist(): void {
		try {
			const s: Stored = { v: CONSENT_VERSION, analytics: this.analytics, at: new Date().toISOString() };
			localStorage.setItem(KEY, JSON.stringify(s));
		} catch {
			/* ignore — the choice still applies for this session */
		}
	}

	private finish(): void {
		this.decided = true;
		this.open = false;
		this.persist();
	}

	/** Accept everything on offer (analytics too, when available). */
	acceptAll(): void {
		this.analytics = ANALYTICS_ENABLED;
		this.finish();
	}

	/** Decline every non-essential category. */
	rejectNonEssential(): void {
		this.analytics = false;
		this.finish();
	}

	/** Save a granular choice from the "customize" view. */
	save(analytics: boolean): void {
		this.analytics = ANALYTICS_ENABLED && analytics;
		this.finish();
	}

	/** Re-open the banner so the visitor can change their mind (from /privacy). */
	reopen(): void {
		this.open = true;
	}
}

export const consent = new Consent();
