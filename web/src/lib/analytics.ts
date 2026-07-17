// Analytics is disabled in the public build — no tracking of any kind is loaded,
// and the site phones home to nobody.
//
// The cookie-consent framework still ships (consent.svelte.ts + CookieConsent),
// so adding privacy-respecting, opt-in analytics to your own deployment is easy
// and stays compliant by construction:
//   1. set ANALYTICS_ENABLED = true and fill in ANALYTICS_PROVIDER;
//   2. in syncAnalytics(), inject your provider's <script> ONLY when
//      `consent.analytics` is true (import { consent } from './consent.svelte');
//   3. (optional) implement trackWebGLOnce / custom events the same way.
// The consent banner then automatically shows a granular analytics opt-in.

/** Whether this build offers an analytics category in the consent banner. */
export const ANALYTICS_ENABLED = false;
/** Provider name shown in the banner + privacy policy when enabled. */
export const ANALYTICS_PROVIDER = '';

/** Whether the analytics script has been injected (always false here). */
export function analyticsInjected(): boolean {
	return false;
}

/** Inject the analytics script once, if consented. No-op in the public build. */
export function syncAnalytics(): void {
	/* no analytics shipped */
}

/** Fire the once-per-session WebGL event. No-op in the public build. */
export function trackWebGLOnce(_enabled: boolean): void {
	/* no analytics shipped */
}
