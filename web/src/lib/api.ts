// Typed client for the ridgelined REST API.

export interface Stats {
	nodes: number;
	observers: number;
	observations: number;
	lastPacketAt?: string;
}

export interface Node {
	publicKey: string;
	name: string;
	role: string;
	latitude?: number;
	longitude?: number;
	hasLocation: boolean;
	firstSeen: string;
	lastSeen: string;
	lastAdvert?: string;
	advertCount: number;
	/** Actual advert transmissions (re-flood / multi-observer copies of one
	 *  broadcast collapsed by a ~90s gap) — vs advertCount which counts every
	 *  observation. */
	advertTxCount: number;
	/** Path-hash length in bytes (1, 2, or 3), from the node's advert; 0 = unknown. */
	hashSize: number;
	/** Coordinates are a statistical outlier — likely corrupt GPS. */
	gpsSuspect?: boolean;
	/** "freq,bw,sf,cr" config, inherited from the observer that heard it. */
	radio?: string;
	/** Most recent time this node relayed a packet (within the analytics window). */
	lastRelayed?: string;
	/** Packets this node relayed in the last hour. */
	relayCount1h?: number;
	/** A registered user has verified ownership of this node ("claimed" badge). */
	claimed?: boolean;
}

export interface ObserverStatus {
	state?: string; // online | offline
	radio?: string; // raw "freq,bw,sf,cr"
	freqMhz?: number;
	bandwidthKhz?: number;
	spreadingFactor?: number;
	codingRate?: number;
	model?: string;
	firmware?: string;
	clientVersion?: string;
	batteryMv?: number;
	uptimeSecs?: number;
	noiseFloor?: number;
	txAirSecs?: number;
	rxAirSecs?: number;
	recvErrors?: number;
	queueLen?: number;
}

export interface Observer {
	/** Stable identity — the observer's public key. Use `name` for display. */
	id: string;
	/** Operator-chosen label. Falls back to `id` when absent. */
	name?: string;
	region: string;
	firstSeen: string;
	lastSeen: string;
	packetCount: number;
	/** Latest self-reported device telemetry from the observer's /status message. */
	status?: ObserverStatus;
	lastStatusAt?: string;
	/** Set once retired (decommissioned). Retired observers are omitted from the
	 *  observers list; their packets stay attributed to them. */
	retiredAt?: string;
}

export interface Observation {
	messageHash: string;
	routeType: string;
	payloadType: string;
	pathHops: number;
	observerId?: string;
	/** Friendly label for observerId, resolved server-side. */
	observerName?: string;
	region?: string;
	snr?: number;
	rssi?: number;
	receivedAt: string;
}

export interface LiveNode {
	publicKey: string;
	name: string;
	role: string;
	latitude?: number;
	longitude?: number;
	timestamp?: number;
}

export interface LiveEvent extends Observation {
	node?: LiveNode;
	/** Per-hop relay key prefixes the packet accumulated as it flooded. */
	path?: string[];
	payloadVersion?: number;
	hashSize?: number;
	transportCodes?: [number, number];
	payloadRaw?: string;
	raw?: string;
	/** GroupText channel fields. channelHash is always set; the rest only when decrypted. */
	channelHash?: string;
	channel?: string;
	sender?: string;
	text?: string;
}

// --- Per-node analytics (GET /api/nodes/{pubkey}) ---
export interface NodeObserverStat {
	/** Stable identity — the observer's public key. Use `name` for display. */
	id: string;
	/** Operator-chosen label. Falls back to `id` when absent. */
	name?: string;
	region?: string;
	count: number;
	avgSnr?: number;
	avgRssi?: number;
}
export interface NodeNeighbor {
	publicKey: string;
	name: string;
	role: string;
	count: number;
}
export interface NodePacketRef {
	messageHash: string;
	payloadType: string;
	receivedAt: string;
	observerId?: string;
	/** Friendly label for observerId, resolved server-side. */
	observerName?: string;
	snr?: number;
	rssi?: number;
	pathHops: number;
}
export interface NodeRelay {
	lastRelayed?: string;
	count1h: number;
	count24h: number;
	active: boolean;
}
export interface NodeAnalytics {
	publicKey: string;
	windowHours: number;
	totalPackets: number; // advert transmissions in window
	totalObservations: number;
	packetsToday: number;
	avgSnr?: number;
	avgHops?: number;
	firstHeard?: string;
	lastHeard?: string;
	observers: NodeObserverStat[];
	recentPackets: NodePacketRef[];
	neighbors: NodeNeighbor[];
	relay: NodeRelay;
	trafficShare: number;
	bridge: number;
	/** Median seconds between the node's advert transmissions (heartbeat cadence). */
	advertIntervalSec?: number;
	/** Per-hour advert counts over the window, oldest bucket first. */
	activity: number[];
}
export interface NodeDetailResponse {
	node: Node | null;
	detail: NodeAnalytics | null;
	generatedAt?: string;
	/** Set when the node is quarantined as suspected injected traffic. */
	quarantined?: boolean;
	block?: BlockEntry;
}

// One stored observation attributable to a node (GET /api/nodes/{pubkey}/history).
export interface NodeHistoryEntry {
	messageHash: string;
	payloadType: string;
	routeType: string;
	kind: 'advert' | 'relay';
	receivedAt: string;
	observerId?: string;
	/** Friendly label for observerId, resolved server-side. */
	observerName?: string;
	region?: string;
	snr?: number;
	rssi?: number;
	pathHops: number;
	hopIndex: number;
}

// --- Mesh-wide analytics (GET /api/mesh-analytics) ---
export interface RadioParams {
	SpreadingFactor: number;
	BandwidthHz: number;
	CodingRate: number;
	PreambleSymbols: number;
}
export interface MeshKPIs {
	activeNodes: number;
	transmissions: number;
	observations: number;
	avgLinkScore?: number;
	floodRedundancy?: number;
	channelUtilPct: number;
	congestionTier: string;
}
export interface NameCount {
	label: string;
	count: number;
}
export interface HistogramBin {
	label: string;
	count: number;
}
export interface AirtimeBucket {
	timestamp: string;
	airtimeMs: number;
	utilPct: number;
	transmissions: number;
	/** Transmissions in the slice that were relayed (≥1 hop). */
	relayTx: number;
	/** Mean per-reception link score in the slice (relay-health trend). */
	avgLinkScore?: number;
}
export interface TopologyNode {
	publicKey: string;
	name: string;
	role: string;
	relayed: number;
}
export interface TopologyEdge {
	a: string;
	b: string;
	weight: number;
}
export interface Topology {
	nodes: TopologyNode[];
	edges: TopologyEdge[];
}
export interface RelayRank {
	publicKey: string;
	name: string;
	role: string;
	relayed: number;
	airtimeMs: number;
}
export interface ObserverCoverage {
	id: string;
	name?: string;
	region?: string;
	observations: number;
	distinctNodes: number;
	directNodes: number;
	/** Median receive-time deviation from consensus (ms) — clock-drift signal. */
	clockSkewMs?: number;
}
export interface DirectLink {
	observer: string;
	observerName?: string;
	nodeKey: string;
	nodeName: string;
	role: string;
	count: number;
}
export interface MeshAnalytics {
	generatedAt: string;
	windowHours: number;
	radio: RadioParams;
	kpis: MeshKPIs;
	payloadTypes: NameCount[];
	routeTypes: NameCount[];
	linkScoreHist: HistogramBin[];
	snrHist: HistogramBin[];
	airtime: AirtimeBucket[];
	topRelays: RelayRank[];
	observers: ObserverCoverage[];
	directLinks: DirectLink[];
	directReach: HistogramBin[];
	hashSizes: NameCount[];
	topology: Topology;
}

export interface NodeActivity {
	grid: number[][]; // [weekday 0=Sun][hour 0-23]
	max: number;
	total: number;
	days: number;
}

export interface ObserverAnalytics {
	id: string;
	region?: string;
	windowHours: number;
	totalPackets: number;
	packetsPerHour: number;
	activity: number[]; // per-hour receptions, oldest bucket first
	payloadTypes: NameCount[];
	snrHist: HistogramBin[];
	avgSnr?: number;
	distinctNodes: number;
	directNodes: number;
	clockSkewMs?: number;
	neighbors: DirectLink[];
}

export interface TelemetryPoint {
	recordedAt: string;
	batteryMv?: number;
	uptimeSecs?: number;
	noiseFloor?: number;
	txAirSecs?: number;
	rxAirSecs?: number;
	recvErrors?: number;
	queueLen?: number;
}

export interface TelemetrySummary {
	samples: number;
	spanHours: number;
	batteryMv?: number;
	batteryTrendMvHr?: number;
	batteryDir?: string; // charging | discharging | stable
	reboots: number;
	noiseFloor?: number;
	noiseTrendDbHr?: number;
	noiseMin?: number;
	noiseMax?: number;
	noiseAvg?: number;
}

export interface ObserverTelemetry {
	id: string;
	points: TelemetryPoint[];
	summary: TelemetrySummary;
}

async function get<T>(path: string): Promise<T> {
	const res = await fetch(path, { headers: { accept: 'application/json' } });
	if (!res.ok) throw new Error(`${path}: ${res.status}`);
	return res.json() as Promise<T>;
}

// ---- Admin (auth-gated injection detection + quarantine/purge) ----

export interface ForeignNode {
	key: string;
	name: string;
	role?: string;
	latitude?: number;
	longitude?: number;
	transitPct?: number; // % of this node's observed paths through the candidate
	captive?: boolean; // transitPct >= 95% (no alternative route)
}
export interface BridgeCandidate {
	/** How the relay behaves physically, from all payload types. RF is broadcast so
	 *  the next hop varies (median relay: 13 distinct, 44% top share); a relay whose
	 *  egress is a wire has exactly one, forever. */
	pathVolume: number;
	nextHops: number;
	nextHopTopShare: number;
	/** Share of carried packets where this relay was the LAST hop — where an
	 *  observer received its own transmission. Zero over real volume means it
	 *  transmits where nothing is listening. Shown, not ranked on. */
	terminalShare: number;
	/** Which rule produced this candidate: "captivity" (a population with no
	 *  alternative route in), "wired" (an egress that never varies), or both. */
	signals: string[];
	/** Operator has sanctioned this bridge: still reported, but not a finding. */
	known?: boolean;
	nodeKey: string;
	name: string;
	captiveCount: number; // foreign nodes ≥95% captive to this node
	foreignThrough: number; // foreign nodes routed through it at all
	captiveFraction: number; // captiveCount / foreignThrough
	foreignKm: number; // geographic displacement — shown as a hint, not ranked
	foreign: ForeignNode[];
}
export interface InjectorCandidate {
	observer: string;
	exclusiveCount: number;
	exclusive: ForeignNode[];
}
export interface InjectionReport {
	windowHours: number;
	/** Adverts decoded in the window, and how many were dropped because their
	 *  Ed25519 signature didn't verify (a corrupt key invents a node that never
	 *  existed). Shown so a quiet result reads as "clean data", not "broken scan". */
	advertsScanned: number;
	advertsRejected: number;
	/** Every decoded packet, those carrying at least one hop, and hops whose hash
	 *  prefix matched no single node. Path evidence comes from all payload types,
	 *  not just adverts. */
	packetsScanned: number;
	pathsScanned: number;
	unresolvedHops: number;
	bridges: BridgeCandidate[];
	injectors: InjectorCandidate[];
	migrations: MigrationEvent[];
}

/** A node that stopped being heard directly while its traffic kept arriving
 *  relayed. The pubkey is unchanged, so nothing else notices it moved. */
export interface MigrationEvent {
	key: string;
	name: string;
	role?: string;
	lastDirectAt: string;
	lastRelayAt: string;
	relayedAfter: number;
	/** Set when a bridge carries its traffic — the difference between "moved
	 *  behind a bridge" and "drifted out of earshot". */
	viaBridge?: string;
}
export interface BlockEntry {
	kind: string; // observer | bridge | node
	key: string;
	name?: string;
	reason?: string;
	createdAt: string;
}
export interface PurgeResult {
	observations: number;
	nodes: number;
	/** User-authored data cascaded with a purged node (see store.PurgeTargets). */
	claims: number;
	notes: number;
	locations: number;
	shares: number;
	/** Keys held back from a purge because a user has claimed them — evidence the
	 *  detector misfired, so they're blocked but not deleted. */
	skippedClaimed?: string[];
}

// The admin console is gated by the is_admin account (session auth). Reads use
// the shared session cookie; mutations additionally send the CSRF token.
export const admin = {
	detect: (sinceSec = 86400) =>
		get<InjectionReport>(`/api/admin/detect?since=${sinceSec}`),
	blocklist: () => get<BlockEntry[]>('/api/admin/blocklist'),
	/** Quarantine (reversible): drop at ingest + hide; does not delete stored rows.
	 *  `nodes` optionally blocks extra node pubkeys (a bridge's foreign cluster).
	 *  kind "allow" dismisses a detection candidate without blocking it. */
	block: (
		csrf: string,
		body: { kind: string; key: string; name?: string; reason?: string; nodes?: string[] }
	) => mutate<{ ok: boolean }>('/api/admin/block', 'POST', csrf, body),
	unblock: (csrf: string, kind: string, key: string) =>
		mutate<{ ok: boolean }>(
			`/api/admin/block?kind=${encodeURIComponent(kind)}&key=${encodeURIComponent(key)}`,
			'DELETE',
			csrf
		),
	/** Purge: delete stored data; blocks the INGRESS points (bridges/observers)
	 *  but deletes `nodes` permanently with no block. */
	purge: (csrf: string, body: { observers?: string[]; bridges?: string[]; nodes?: string[] }) =>
		mutate<PurgeResult>('/api/admin/purge', 'POST', csrf, body),
	/** Permanently delete nodes (adverts + rows) with no blocklist entry. */
	deleteNodes: (csrf: string, nodes: string[]) =>
		mutate<PurgeResult>('/api/admin/delete', 'POST', csrf, { nodes }),
	/** Permanently delete observers, every packet they reported, and their device
	 *  telemetry, with no block. Destructive — prefer `retireObserver` for a
	 *  receiver that has simply left the network, which keeps its history. */
	deleteObservers: (csrf: string, observers: string[]) =>
		mutate<PurgeResult>('/api/admin/delete', 'POST', csrf, { observers }),
	/** Observers withdrawn from the observers page but whose packets are kept. */
	retiredObservers: () => get<Observer[]>('/api/admin/observers/retired'),
	/** Retire a decommissioned observer: hides it from the observers page and
	 *  keeps every packet it reported. Survives the broker replaying its retained
	 *  /status, which is what used to bring deleted observers back. Reversible. */
	retireObserver: (csrf: string, observer: string) =>
		mutate<{ observer: string; retired: boolean }>('/api/admin/observers/retire', 'POST', csrf, {
			observer
		}),
	/** Return a retired observer to the observers page. */
	unretireObserver: (csrf: string, observer: string) =>
		mutate<{ observer: string; retired: boolean }>(
			'/api/admin/observers/unretire',
			'POST',
			csrf,
			{ observer }
		)
};

export const api = {
	stats: () => get<Stats>('/api/stats'),
	nodes: () => get<Node[]>('/api/nodes'),
	/** One node's row plus its computed analytics snapshot. */
	nodeDetail: (pubkey: string) => get<NodeDetailResponse>(`/api/nodes/${encodeURIComponent(pubkey)}`),
	/** A node's stored observations (own adverts + relayed packets) over the last sinceSec seconds, newest first. */
	nodeHistory: (pubkey: string, sinceSec = 86400, limit = 300) =>
		get<NodeHistoryEntry[]>(`/api/nodes/${encodeURIComponent(pubkey)}/history?since=${sinceSec}&limit=${limit}`),
	/** Per-observer reception of a node's adverts over the last sinceSec seconds (on demand, so the range can span the node's advert cadence). */
	nodeObservers: (pubkey: string, sinceSec = 3 * 86400) =>
		get<NodeObserverStat[]>(`/api/nodes/${encodeURIComponent(pubkey)}/observers?since=${sinceSec}`),
	/** A node's weekday×hour activity heatmap over the last `days` days. */
	nodeHeatmap: (pubkey: string, days = 7) =>
		get<NodeActivity>(`/api/nodes/${encodeURIComponent(pubkey)}/heatmap?days=${days}`),
	observers: () => get<Observer[]>('/api/observers'),
	/** One observer's feed metrics over the last sinceSec seconds (default 24h, max 7d). */
	observerAnalytics: (id: string, sinceSec = 86400) =>
		get<ObserverAnalytics>(`/api/observers/${encodeURIComponent(id)}/analytics?since=${sinceSec}`),
	/** One observer's device-telemetry time series + derived health summary (default 24h, max 7d). */
	observerTelemetry: (id: string, sinceSec = 86400) =>
		get<ObserverTelemetry>(`/api/observers/${encodeURIComponent(id)}/telemetry?since=${sinceSec}`),
	observations: (limit = 100) => get<Observation[]>(`/api/observations?limit=${limit}`),
	/** Recent history (default last hour) in the live-event shape, newest first. */
	recent: (sinceSec = 3600) => get<LiveEvent[]>(`/api/recent?since=${sinceSec}`),
	/** All observations of one transmission (by message hash) — backs the shareable
	 *  per-packet deep link. Empty if the hash is unknown or has aged out. */
	packet: (hash: string) => get<LiveEvent[]>(`/api/packets/${encodeURIComponent(hash)}`),
	/**
	 * Channel (GroupText) message history, newest first, one row per distinct
	 * message. Default & max 24h — for the channel chat reader.
	 */
	channelHistory: (sinceSec = 86400) =>
		get<LiveEvent[]>(`/api/channels/recent?since=${sinceSec}`),
	/** Mesh-wide analytics over the last sinceSec seconds (default 6h, max 24h). */
	meshAnalytics: (sinceSec = 21600, bucketMin = 10) =>
		get<MeshAnalytics>(`/api/mesh-analytics?since=${sinceSec}&bucket=${bucketMin}`)
};

// ---- Accounts / auth ----

export interface AuthUser {
	id: number;
	email: string;
	displayName: string;
	/** Site administrator: manages members and (later) moderation. */
	isAdmin: boolean;
	/** Admin-granted gate for claiming nodes and storing private locations. */
	canClaim: boolean;
	/** Suspended: cannot log in; existing sessions are void. */
	blocked: boolean;
	/** The protected initial admin — cannot be demoted, blocked, or removed. */
	isOwner: boolean;
	/** Whether the account's email address has been confirmed. */
	emailVerified: boolean;
	createdAt: string;
	lastLogin?: string;
}

/** Response from register/login/me: the user (null when signed out) plus the
 *  session's CSRF token, echoed on authenticated mutations via X-CSRF-Token. */
export interface AuthResponse {
	user: AuthUser | null;
	csrfToken?: string;
	/** Count of nodes newly shared with the user (not yet seen) — account badge. */
	unseenShares?: number;
	/** Set by register when a verification email was sent instead of logging in. */
	verificationSent?: boolean;
	/** Echoed address for the "check your email" screen / resend. */
	email?: string;
}

/** Error thrown by auth requests; carries the HTTP status and the unverified flag
 *  so the login screen can offer to resend the confirmation email. */
export class AuthError extends Error {
	status: number;
	unverified: boolean;
	constructor(message: string, status: number, unverified = false) {
		super(message);
		this.name = 'AuthError';
		this.status = status;
		this.unverified = unverified;
	}
}

// Session cookies are HttpOnly and set by the server; same-origin fetches send
// them automatically, so the client never handles the session token directly.
async function authReq(path: string, body?: unknown): Promise<AuthResponse> {
	const res = await fetch(path, {
		method: 'POST',
		headers: { accept: 'application/json', ...(body ? { 'content-type': 'application/json' } : {}) },
		body: body ? JSON.stringify(body) : undefined
	});
	const data = (await res.json().catch(() => ({}))) as AuthResponse & {
		error?: string;
		unverified?: boolean;
	};
	if (!res.ok) throw new AuthError(data.error ?? `${res.status}`, res.status, !!data.unverified);
	return data;
}

export const authApi = {
	me: () => get<AuthResponse>('/api/auth/me'),
	register: (email: string, password: string, displayName: string) =>
		authReq('/api/auth/register', { email, password, displayName }),
	login: (email: string, password: string) => authReq('/api/auth/login', { email, password }),
	logout: () => authReq('/api/auth/logout'),
	/** Confirm an emailed verification token; on success the server logs the user in. */
	verifyEmail: (token: string) => authReq('/api/auth/verify', { token }),
	/** Ask for a fresh verification email (always resolves; never reveals account state). */
	resendVerification: (email: string) =>
		fetch('/api/auth/resend-verification', {
			method: 'POST',
			headers: { 'content-type': 'application/json' },
			body: JSON.stringify({ email })
		}).then(() => undefined),
	/** Request a password-reset email (always resolves; never reveals account state). */
	forgotPassword: (email: string) =>
		fetch('/api/auth/forgot', {
			method: 'POST',
			headers: { 'content-type': 'application/json' },
			body: JSON.stringify({ email })
		}).then(() => undefined),
	/** Set a new password from an emailed reset token; on success the server logs the user in. */
	resetPassword: (token: string, password: string) => authReq('/api/auth/reset', { token, password })
};

/** Self-service account editing (authenticated + CSRF). */
export const account = {
	/** Change display name; returns the updated account. */
	updateProfile: (csrf: string, displayName: string) =>
		mutate<AuthUser>('/api/account/profile', 'PUT', csrf, { displayName }),
	/** Change password after re-authenticating with the current one. */
	changePassword: (csrf: string, currentPassword: string, newPassword: string) =>
		mutate<{ ok: boolean }>('/api/account/password', 'POST', csrf, {
			currentPassword,
			newPassword
		}),
	/** Change email (re-auth required); the new address must be re-verified. Returns
	 *  the updated account (emailVerified will be false until confirmed). */
	changeEmail: (csrf: string, currentPassword: string, newEmail: string) =>
		mutate<AuthUser>('/api/account/email', 'POST', csrf, { currentPassword, newEmail }),
	/** Permanently delete the caller's own account (re-auth with password). Every
	 *  node they owned is released and marked "previously owned by …". */
	deleteAccount: (csrf: string, password: string) =>
		mutate<{ ok: boolean }>('/api/account/delete', 'POST', csrf, { password })
};

// mutate is the shared helper for authenticated, CSRF-protected state changes
// (used by the account features). It relies on the same-origin session cookie
// and sends the session's CSRF token in the header (double-submit).
export async function mutate<T>(
	path: string,
	method: string,
	csrf: string,
	body?: unknown
): Promise<T> {
	const res = await fetch(path, {
		method,
		headers: {
			accept: 'application/json',
			'x-csrf-token': csrf,
			...(body ? { 'content-type': 'application/json' } : {})
		},
		body: body ? JSON.stringify(body) : undefined
	});
	if (!res.ok) {
		let msg = `${res.status}`;
		try {
			msg = (await res.json()).error ?? msg;
		} catch {
			/* ignore */
		}
		throw new Error(msg);
	}
	return res.json() as Promise<T>;
}

// ---- Node ownership claims ----

export interface Claim {
	id: number;
	nodePubkey: string;
	userId: number;
	/** Verification code (present only on your own pending claim). */
	code?: string;
	status: 'pending' | 'verified';
	createdAt: string;
	expiresAt?: string;
	verifiedAt?: string;
}

export interface ClaimStatus {
	/** The verified owner (public), if any. */
	owner?: { userId: number; displayName: string };
	/** Display name of the node's last owner, kept after they deleted their
	 *  account. Only set when the node currently has no owner. */
	previousOwner?: string;
	ownedByMe: boolean;
	/** The requesting user's own claim on this node, if any. */
	mine?: Claim;
	loggedIn: boolean;
	/** Whether the requester is allowed to start a claim on this node. */
	canClaim: boolean;
	/** True when you own the node but its advertised name still contains the
	 *  verification code — restore the real name and re-advert to clear it. */
	nameNeedsReset?: boolean;
}

export interface ClaimWithNode extends Claim {
	nodeName: string;
	nodeRole: string;
	/** False when the claimed node isn't currently in the mesh — the retention
	 *  sweep prunes silent nodes but the owner keeps the claim, so these render
	 *  as dormant rather than linking to a node page that would 404. */
	nodePresent: boolean;
}

export const claims = {
	/** Public: ownership + the caller's own claim status for a node. */
	status: (pubkey: string) => get<ClaimStatus>(`/api/nodes/${encodeURIComponent(pubkey)}/claim`),
	/** Open or refresh a pending claim; returns the code to embed in the advert name. */
	create: (csrf: string, pubkey: string) => mutate<Claim>('/api/claims', 'POST', csrf, { pubkey }),
	/** Cancel a pending claim or release ownership. */
	release: (csrf: string, pubkey: string) =>
		mutate<{ ok: boolean }>(`/api/claims/${encodeURIComponent(pubkey)}`, 'DELETE', csrf),
	/** The caller's own claims (pending + owned) with node display info. */
	mine: () => get<ClaimWithNode[]>('/api/claims/mine'),
	/** Private-key proof: request a challenge to sign with the node's private key. */
	keyChallenge: (csrf: string, pubkey: string) =>
		mutate<{ challenge: string }>(
			`/api/nodes/${encodeURIComponent(pubkey)}/claim/key-challenge`,
			'POST',
			csrf
		),
	/** Private-key proof: submit the signature over the challenge to verify ownership. */
	keyVerify: (csrf: string, pubkey: string, signature: string) =>
		mutate<Claim>(`/api/nodes/${encodeURIComponent(pubkey)}/claim/key-verify`, 'POST', csrf, {
			signature
		})
};

// ---- Node notes ----

export type NoteVisibility = 'public' | 'private' | 'team';

export interface Note {
	id: number;
	nodePubkey: string;
	userId: number;
	authorName: string;
	visibility: NoteVisibility;
	body: string;
	createdAt: string;
	updatedAt: string;
	/** The requester may edit/delete this note (author, or owner/admin for delete). */
	mine: boolean;
}

/** Notes list plus the caller's posting rights (drives the note-type options). */
export interface NotesResult {
	notes: Note[];
	/** Caller may post "team" notes (node owner or a shared-with user). */
	canTeam: boolean;
	/** Caller is signed in (may post at all). */
	loggedIn: boolean;
}

export const notes = {
	/** Notes visible to the caller (public + own + team-if-in-circle) + their rights. */
	list: (pubkey: string) => get<NotesResult>(`/api/nodes/${encodeURIComponent(pubkey)}/notes`),
	create: (csrf: string, pubkey: string, body: string, visibility: NoteVisibility) =>
		mutate<Note>(`/api/nodes/${encodeURIComponent(pubkey)}/notes`, 'POST', csrf, { body, visibility }),
	update: (csrf: string, id: number, body: string, visibility: NoteVisibility) =>
		mutate<Note>(`/api/notes/${id}`, 'PATCH', csrf, { body, visibility }),
	remove: (csrf: string, id: number) => mutate<{ ok: boolean }>(`/api/notes/${id}`, 'DELETE', csrf)
};

/** Minimal public identity for the share autocomplete. */
export interface UserBrief {
	id: number;
	displayName: string;
}

/** Autocomplete registered users by display name (signed-in only). */
export const userSearch = (q: string) =>
	get<UserBrief[]>(`/api/users/search?q=${encodeURIComponent(q)}`);

// ---- Node private exact location (owner-only) ----

export interface PrivateLocation {
	nodePubkey: string;
	userId: number;
	latitude: number;
	longitude: number;
	label: string;
	updatedAt: string;
}

/** GET response: `set` says whether a location exists; `location` is present when it does. */
export interface PrivateLocationResult {
	set: boolean;
	location?: PrivateLocation;
	/** True only for the node's owner (shared-with viewers get read-only). */
	canEdit?: boolean;
	/** Present for a shared-with viewer: who shared the location with them. */
	sharedBy?: { userId: number; displayName: string };
}

/** One user a node's private location is shared with. */
export interface LocationShare {
	nodePubkey: string;
	granteeUserId: number;
	displayName: string;
	email: string;
	createdAt: string;
}

export const privateLocation = {
	/** Fetch a node's private exact location. 200 for the owner or a shared-with
	 *  user (403 otherwise, so it never confirms a location exists to others). */
	get: (pubkey: string) =>
		get<PrivateLocationResult>(`/api/nodes/${encodeURIComponent(pubkey)}/private-location`),
	/** Owner-only: store/replace the private exact location. */
	set: (csrf: string, pubkey: string, latitude: number, longitude: number, label: string) =>
		mutate<PrivateLocationResult>(
			`/api/nodes/${encodeURIComponent(pubkey)}/private-location`,
			'PUT',
			csrf,
			{ latitude, longitude, label }
		),
	/** Owner-only: clear the private exact location. */
	remove: (csrf: string, pubkey: string) =>
		mutate<{ ok: boolean }>(
			`/api/nodes/${encodeURIComponent(pubkey)}/private-location`,
			'DELETE',
			csrf
		)
};

/** A node whose private location has been shared WITH the current user. */
export interface SharedWithMe {
	nodePubkey: string;
	nodeName: string;
	nodeRole: string;
	sharedById: number;
	sharedByName: string;
	createdAt: string;
	seen: boolean;
	/** False when the shared node isn't currently in the mesh — same dormant
	 *  treatment as ClaimWithNode.nodePresent. */
	nodePresent: boolean;
}

/** Grantee-facing: the nodes shared with me + clearing the "new" badge. */
export const shares = {
	mine: () => get<SharedWithMe[]>('/api/shares/mine'),
	markSeen: (csrf: string) => mutate<{ ok: boolean }>('/api/shares/mark-seen', 'POST', csrf)
};

/** Owner-only management of who a node's private location is shared with. */
export const locationShares = {
	list: (pubkey: string) =>
		get<LocationShare[]>(`/api/nodes/${encodeURIComponent(pubkey)}/location-shares`),
	/** Grant read access to a registered user (by id from the picker, or email);
	 *  returns the updated list. */
	grant: (csrf: string, pubkey: string, grantee: { userId: number } | { email: string }) =>
		mutate<LocationShare[]>(
			`/api/nodes/${encodeURIComponent(pubkey)}/location-shares`,
			'POST',
			csrf,
			grantee
		),
	/** Revoke a grantee's access. */
	revoke: (csrf: string, pubkey: string, granteeUserId: number) =>
		mutate<{ ok: boolean }>(
			`/api/nodes/${encodeURIComponent(pubkey)}/location-shares/${granteeUserId}`,
			'DELETE',
			csrf
		)
};

/** Admin member management (session-admin gated). */
export const adminUsers = {
	list: () => get<AuthUser[]>('/api/admin/users'),
	/** Grant/revoke admin. (Claiming is universal, so there's no can-claim flag.) */
	setAdmin: (csrf: string, id: number, isAdmin: boolean) =>
		mutate<{ ok: boolean }>('/api/admin/users/flags', 'POST', csrf, { id, isAdmin }),
	/** Suspend (blocked=true) or restore (blocked=false) an account. */
	setBlocked: (csrf: string, id: number, blocked: boolean) =>
		mutate<{ ok: boolean }>('/api/admin/users/block', 'POST', csrf, { id, blocked }),
	/** Permanently delete an account. */
	remove: (csrf: string, id: number) =>
		mutate<{ ok: boolean }>('/api/admin/users/delete', 'POST', csrf, { id })
};
