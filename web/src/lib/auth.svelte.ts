// Current-user session state, backed by the server's HttpOnly session cookie.
// The cookie is authoritative; this store mirrors it for the UI and holds the
// CSRF token that authenticated mutations must echo. Initialised once from
// /api/auth/me on app start (auth.init()).
import { authApi, account, shares, claims, type AuthUser } from './api';

class Auth {
	user = $state<AuthUser | null>(null);
	/** Session CSRF token, sent as X-CSRF-Token on authenticated mutations. */
	csrf = $state('');
	/** True once the initial /me probe has resolved (so UI can avoid flicker). */
	ready = $state(false);
	/** Nodes newly shared with the user, not yet seen — drives the account badge. */
	unseenShares = $state(0);
	/** Uppercase pubkeys of nodes the current user has verified ownership of — lets
	 *  the claimed badge show YOUR nodes in a distinct colour from others'. */
	myClaims = $state<Set<string>>(new Set());

	async init() {
		try {
			const r = await authApi.me();
			this.user = r.user;
			this.csrf = r.csrfToken ?? '';
			this.unseenShares = r.unseenShares ?? 0;
		} catch {
			// Offline or server error — treat as signed out; UI stays usable.
		}
		this.ready = true;
		this.refreshClaims();
	}

	/** Load the current user's owned-node pubkeys (empty when signed out). */
	async refreshClaims() {
		if (!this.user) {
			this.myClaims = new Set();
			return;
		}
		try {
			const list = await claims.mine();
			this.myClaims = new Set(
				list.filter((c) => c.status === 'verified').map((c) => c.nodePubkey.toUpperCase())
			);
		} catch {
			/* keep whatever we had */
		}
	}

	/** Whether the signed-in user is the verified owner of this node. */
	ownsNode(pubkey: string): boolean {
		return this.myClaims.has((pubkey ?? '').toUpperCase());
	}

	/** Reflect a single node's verified-ownership state locally, so the claimed
	 *  badge (auth.ownsNode) flips the moment a claim verifies or a node is
	 *  released — without waiting for a page reload to re-run refreshClaims(). */
	setOwnership(pubkey: string, owned: boolean) {
		const key = (pubkey ?? '').toUpperCase();
		if (!key || !this.user || owned === this.myClaims.has(key)) return;
		const next = new Set(this.myClaims);
		if (owned) next.add(key);
		else next.delete(key);
		this.myClaims = next;
	}

	/** Clear the "new shares" badge once the user has seen their list. */
	async markSharesSeen() {
		if (this.unseenShares === 0 || !this.csrf) return;
		try {
			await shares.markSeen(this.csrf);
			this.unseenShares = 0;
		} catch {
			/* leave the badge; it clears on next successful attempt */
		}
	}

	/** Register. Returns the response so the caller can show a "check your email"
	 *  screen when verification was sent (user is null in that case). */
	async register(email: string, password: string, displayName: string) {
		const r = await authApi.register(email, password, displayName);
		if (r.user) this.#adopt(r);
		return r;
	}

	async login(email: string, password: string) {
		const r = await authApi.login(email, password);
		this.#adopt(r);
	}

	/** Confirm an emailed verification token; logs the user in on success. */
	async verifyEmail(token: string) {
		const r = await authApi.verifyEmail(token);
		this.#adopt(r);
		return r;
	}

	/** Change display name; updates the cached user on success. */
	async updateDisplayName(displayName: string) {
		this.user = await account.updateProfile(this.csrf, displayName);
	}

	/** Change password (re-auth with the current one). Session stays valid. */
	async changePassword(currentPassword: string, newPassword: string) {
		await account.changePassword(this.csrf, currentPassword, newPassword);
	}

	/** Change email (re-auth required). Updates the cached user; emailVerified
	 *  flips false until the new address is confirmed. */
	async changeEmail(currentPassword: string, newEmail: string) {
		this.user = await account.changeEmail(this.csrf, currentPassword, newEmail);
	}

	async logout() {
		try {
			await authApi.logout();
		} finally {
			this.user = null;
			this.csrf = '';
			this.unseenShares = 0;
			this.myClaims = new Set();
		}
	}

	/** Permanently delete the current account (re-auth with password). The server
	 *  releases the user's nodes and clears the session; we drop local state so the
	 *  UI reflects a signed-out user immediately. */
	async deleteAccount(password: string) {
		await account.deleteAccount(this.csrf, password);
		this.user = null;
		this.csrf = '';
		this.unseenShares = 0;
		this.myClaims = new Set();
	}

	#adopt(r: { user: AuthUser | null; csrfToken?: string; unseenShares?: number }) {
		this.user = r.user;
		this.csrf = r.csrfToken ?? '';
		this.unseenShares = r.unseenShares ?? 0;
		this.refreshClaims();
	}

	get loggedIn() {
		return this.user !== null;
	}
	get isAdmin() {
		return this.user?.isAdmin ?? false;
	}
	get canClaim() {
		return this.user?.canClaim ?? false;
	}
}

export const auth = new Auth();
