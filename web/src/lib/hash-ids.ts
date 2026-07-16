// Pure helpers for the hash-ID planner: analysing which short pubkey prefixes
// ("hash IDs") are already in use across the known mesh, and finding free ones.
//
// A MeshCore node is identified in packet paths by the first 1, 2, or 3 bytes of
// its public key. Two nodes sharing that prefix at the same length "collide".
import type { Node } from './api';

export type HashByteLen = 1 | 2 | 3;

export interface CollisionGroup {
	/** Shared prefix, uppercase hex (byteLen*2 chars). */
	prefix: string;
	/** Nodes sharing this prefix, the genuine (located/named) ones first. */
	nodes: Node[];
}

/** Uppercase N-byte (N*2-hex) prefix of a node's public key. */
export function nodePrefix(node: Pick<Node, 'publicKey'>, byteLen: HashByteLen): string {
	return (node.publicKey ?? '').slice(0, byteLen * 2).toUpperCase();
}

/** Reserved hash IDs that MeshCore never assigns. */
export function isReserved(prefix: string): boolean {
	const b0 = prefix.slice(0, 2).toUpperCase();
	return b0 === '00' || b0 === 'FF';
}

export function isHex(s: string): boolean {
	return /^[0-9a-fA-F]*$/.test(s);
}

// Companions (ChatNode role) don't repeat packets, so their hash ID never shows
// up in a routing path — they can't cause a path-hash collision. All collision
// analysis considers only path-participating nodes (repeaters, room servers).
export function isPathNode(n: Pick<Node, 'role'>): boolean {
	return n.role !== 'ChatNode';
}

// A node only collides with other nodes that share its *configured* hash-ID
// length: a node set to 3-byte IDs is identified by 3 bytes and can't be confused
// with a 1-byte node, and vice-versa. So all collision analysis is scoped to the
// cohort of nodes with the same `hashSize`. (hashSize 0 = the node hasn't
// advertised its length yet, so we can't place it in a cohort.)
function inCohort(n: Node, byteLen: HashByteLen): boolean {
	return n.hashSize === byteLen;
}

/** How many path-participating nodes are configured at each hash-ID length. */
export function cohortCounts(nodes: Node[]): { 1: number; 2: number; 3: number; unknown: number } {
	const c = { 1: 0, 2: 0, 3: 0, unknown: 0 };
	for (const n of nodes) {
		if (!isPathNode(n)) continue;
		if (n.hashSize === 1 || n.hashSize === 2 || n.hashSize === 3) c[n.hashSize]++;
		else c.unknown++;
	}
	return c;
}

/** Every prefix occupied by a path node *configured at this length*. */
export function usedPrefixes(nodes: Node[], byteLen: HashByteLen): Set<string> {
	const set = new Set<string>();
	for (const n of nodes) {
		if (!isPathNode(n) || !inCohort(n, byteLen)) continue;
		const p = nodePrefix(n, byteLen);
		if (p.length === byteLen * 2) set.add(p);
	}
	return set;
}

/** Groups of two-or-more same-length path nodes that share their hash-ID prefix. */
export function collisionGroups(nodes: Node[], byteLen: HashByteLen): CollisionGroup[] {
	const buckets = new Map<string, Node[]>();
	for (const n of nodes) {
		if (!isPathNode(n) || !inCohort(n, byteLen)) continue;
		const p = nodePrefix(n, byteLen);
		if (p.length !== byteLen * 2) continue;
		const arr = buckets.get(p);
		if (arr) arr.push(n);
		else buckets.set(p, [n]);
	}
	const groups: CollisionGroup[] = [];
	for (const [prefix, members] of buckets) {
		if (members.length < 2) continue;
		members.sort(nodeQualityCmp);
		groups.push({ prefix, nodes: members });
	}
	// Biggest collisions first, then by prefix for stable display.
	groups.sort((a, b) => b.nodes.length - a.nodes.length || a.prefix.localeCompare(b.prefix));
	return groups;
}

// Sort "real" nodes (has a location and/or a sensible name) ahead of phantom /
// corrupt records, so a collision group leads with the legitimate node.
function nodeQualityCmp(a: Node, b: Node): number {
	const score = (n: Node) => (n.hasLocation ? 2 : 0) + (n.name && n.name.trim().length > 1 ? 1 : 0);
	return score(b) - score(a) || (a.name ?? '').localeCompare(b.name ?? '');
}

/** Is this exact prefix already used by a node? (case-insensitive) */
export function isPrefixUsed(nodes: Node[], byteLen: HashByteLen, prefix: string): boolean {
	return usedPrefixes(nodes, byteLen).has(prefix.toUpperCase());
}

export type PrefixStatus = 'empty' | 'incomplete' | 'invalid' | 'reserved' | 'used' | 'free';

/** Classify a typed prefix for the picker UI. */
export function prefixStatus(nodes: Node[], byteLen: HashByteLen, prefix: string): PrefixStatus {
	const want = byteLen * 2;
	if (!prefix) return 'empty';
	if (!isHex(prefix)) return 'invalid';
	if (prefix.length < want) return 'incomplete';
	if (prefix.length > want) return 'invalid';
	if (isReserved(prefix)) return 'reserved';
	return isPrefixUsed(nodes, byteLen, prefix) ? 'used' : 'free';
}

// ── Corruption-artifact classifier ──────────────────────────────────────────
//
// Packet corruption can flip bytes inside an advert, including the public key.
// That produces a *phantom* node record: a key that shares a prefix with a real
// node (so it shows up as a hash-ID "collision") but isn't a distinct node at
// all. These false positives are what this classifier filters out, using the
// signals the operator identified:
//
//   • Name match  — if a phantom's name equals a real node's name, it's the same
//     node with a corrupted key (the name field survived). Strong signal.
//   • Advert count — a real node adverts repeatedly; a corruption blip is heard
//     once or twice. Within a collision the *most-adverted* key is the true node.
//   • Shared prefix — two independent keys sharing more than the hash-ID length
//     of leading bytes is astronomically unlikely, so extra shared bytes point to
//     corruption/duplication rather than a genuine collision.

export type Confidence = 'high' | 'medium';

// Two distinct keys sharing this many leading bytes is statistically impossible
// by chance, so it marks a corrupted duplicate rather than a real collision.
const SHARED_DEFINITE = 4;

export interface ArtifactFinding {
	/** The phantom/corrupt record. */
	node: Node;
	/** The real node it's a corrupted duplicate of (highest advert count in the group). */
	canonical: Node;
	reason: string;
	confidence: Confidence;
	/** Leading whole bytes the two public keys share. */
	sharedBytes: number;
}

export interface GenuineCollision {
	prefix: string;
	/** Two-or-more genuinely distinct nodes that share this prefix. */
	nodes: Node[];
}

export interface CollisionAnalysis {
	/** Real collisions, after corruption artifacts are removed. */
	genuine: GenuineCollision[];
	/** Records judged to be corruption/duplication of a real node. */
	artifacts: ArtifactFinding[];
}

function normName(name: string | undefined): string {
	return (name ?? '').trim().toLowerCase().replace(/\s+/g, ' ');
}

/** A name that carries no real identity — empty, a single char, or no letters/digits. */
function isGarbageName(name: string | undefined): boolean {
	const t = (name ?? '').trim();
	return t.length <= 1 || !/[a-z0-9]/i.test(t);
}

/** Leading whole bytes (pairs of hex chars) that two public keys share. */
export function sharedPrefixBytes(a: string, b: string): number {
	const x = (a ?? '').toUpperCase();
	const y = (b ?? '').toUpperCase();
	const max = Math.min(x.length, y.length) >> 1;
	let n = 0;
	for (; n < max; n++) {
		if (x.slice(n * 2, n * 2 + 2) !== y.slice(n * 2, n * 2 + 2)) break;
	}
	return n;
}

// Canonical ordering of a colliding set: most adverts wins (the true node is
// heard most), then a located record, then most-recently seen, then by key for
// stability. "Best" (most canonical) sorts first.
function canonicalCmp(a: Node, b: Node): number {
	return (
		(b.advertCount ?? 0) - (a.advertCount ?? 0) ||
		(b.hasLocation ? 1 : 0) - (a.hasLocation ? 1 : 0) ||
		(b.lastSeen ?? '').localeCompare(a.lastSeen ?? '') ||
		a.publicKey.localeCompare(b.publicKey)
	);
}

// Of the busier candidate sources, the most plausible parent of `m` is the one
// whose key it most resembles (an exact name match breaks ties — a corrupted key
// often keeps the original name).
function bestParent(m: Node, candidates: Node[]): Node {
	return candidates.reduce((best, c) => {
		const cs = sharedPrefixBytes(m.publicKey, c.publicKey);
		const bs = sharedPrefixBytes(m.publicKey, best.publicKey);
		if (cs !== bs) return cs > bs ? c : best;
		const cn = normName(c.name) === normName(m.name) ? 1 : 0;
		const bn = normName(best.name) === normName(m.name) ? 1 : 0;
		return cn > bn ? c : best;
	});
}

// Classify one colliding set: walk members best-first, checking each against the
// busier members above it. Members flagged as corruption become artifacts; the
// rest are genuinely distinct nodes.
function classifyGroup(
	members: Node[],
	byteLen: HashByteLen
): { real: Node[]; artifacts: ArtifactFinding[] } {
	const ordered = [...members].sort(canonicalCmp);
	const real: Node[] = [];
	const artifacts: ArtifactFinding[] = [];
	for (let i = 0; i < ordered.length; i++) {
		if (i === 0) {
			real.push(ordered[i]); // busiest member is always canonical
			continue;
		}
		const finding = classifyMember(ordered[i], bestParent(ordered[i], ordered.slice(0, i)), byteLen);
		if (finding) artifacts.push(finding);
		else real.push(ordered[i]);
	}
	return { real, artifacts };
}

/**
 * Decide whether `m` is a corruption/duplication artifact of canonical node `a`,
 * given the hash-ID length the two collided at. Returns null if `m` looks like a
 * genuinely distinct node.
 */
function classifyMember(m: Node, a: Node, byteLen: HashByteLen): ArtifactFinding | null {
	if (m.publicKey === a.publicKey) return null;
	const shared = sharedPrefixBytes(m.publicKey, a.publicKey);
	const mAdv = m.advertCount ?? 0;
	const aAdv = a.advertCount ?? 0;
	const base = { node: m, canonical: a, sharedBytes: shared };

	// 1. Structural near-identity. Two independent 32-byte keys virtually never
	//    share 4+ leading bytes (~1-in-4-billion per pair, ~5e-6 across the whole
	//    mesh), so this can only be the same node whose key got bytes flipped in
	//    transit. The lower-advert record is the corrupted copy. This is the main
	//    catch — it works even when the corrupted name still reads sensibly and
	//    the advert carried a (real) location.
	if (shared >= SHARED_DEFINITE) {
		return {
			...base,
			confidence: 'high',
			reason: `key matches ${a.name || 'a real node'} in ${shared} of 32 bytes — corrupted copy`
		};
	}

	// 2. Same name → same node with a corrupted key (high confidence).
	if (normName(m.name) && normName(m.name) === normName(a.name) && !isGarbageName(m.name)) {
		return { ...base, confidence: 'high', reason: `identical name to ${a.name}` };
	}

	// 3. Unreadable name on a record the mesh barely heard, next to a real node it
	//    was heard far less than → a corrupted advert (high confidence).
	if (isGarbageName(m.name) && aAdv >= 5 && aAdv >= mAdv * 4) {
		return {
			...base,
			confidence: 'high',
			reason: `unreadable name, heard ${mAdv}× vs ${a.name || 'anchor'}'s ${aAdv}×`
		};
	}

	// 4. Residual: rarely heard, shares more than the hash-ID length of bytes with
	//    a strongly dominant node. Catches corruption that flipped a byte inside
	//    the first four (so rule 1 misses) but still left an outsized shared run.
	if (mAdv <= 2 && shared > byteLen && aAdv >= 10 && aAdv >= mAdv * 8) {
		return {
			...base,
			confidence: 'medium',
			reason: `rarely heard (${mAdv}×), shares ${shared} bytes with ${a.name || 'a busier node'}`
		};
	}

	return null;
}

/**
 * Split the hash-ID collisions at `byteLen` into genuine collisions (≥2 truly
 * distinct nodes) and corruption artifacts (phantom keys that duplicate a real
 * node). This is the safety net that keeps packet corruption from showing up as
 * false collisions.
 */
export function analyzeCollisions(nodes: Node[], byteLen: HashByteLen): CollisionAnalysis {
	const genuine: GenuineCollision[] = [];
	const artifacts: ArtifactFinding[] = [];

	for (const group of collisionGroups(nodes, byteLen)) {
		const { real, artifacts: groupArtifacts } = classifyGroup(group.nodes, byteLen);
		artifacts.push(...groupArtifacts);
		if (real.length >= 2) genuine.push({ prefix: group.prefix, nodes: real });
	}

	// Most-shared-byte (most obviously corrupt) artifacts first.
	artifacts.sort((a, b) => b.sharedBytes - a.sharedBytes);
	return { genuine, artifacts };
}

/**
 * For a single node, return its genuine collision peers (excluding corruption
 * artifacts) and — if the node itself is a corruption artifact — what real node
 * it duplicates. Uses the node's own hash-ID length. Used by the node detail
 * view so its collision count stops counting phantom records.
 */
export function nodeCollisionInfo(
	nodes: Node[],
	node: Node
): { genuinePeers: Node[]; artifactOf: Node | null; reason?: string } {
	const hs = node.hashSize;
	// Companions never route, so their hash ID can't collide.
	if (!isPathNode(node) || (hs !== 1 && hs !== 2 && hs !== 3))
		return { genuinePeers: [], artifactOf: null };
	const byteLen = hs as HashByteLen;
	const prefix = nodePrefix(node, byteLen);
	// Only path nodes configured at the same length can collide with this one.
	const members = nodes.filter(
		(n) => isPathNode(n) && inCohort(n, byteLen) && nodePrefix(n, byteLen) === prefix
	);
	if (members.length < 2) return { genuinePeers: [], artifactOf: null };

	const { real, artifacts } = classifyGroup(members, byteLen);

	// Is this node itself a corruption artifact?
	const self = artifacts.find((a) => a.node.publicKey === node.publicKey);
	if (self) return { genuinePeers: [], artifactOf: self.canonical, reason: self.reason };

	// Otherwise its genuine peers are the other real (non-artifact) members.
	return {
		genuinePeers: real.filter((m) => m.publicKey !== node.publicKey),
		artifactOf: null
	};
}

/**
 * Hash-ID display info for a node's detail view: its prefix at its configured
 * length, the count of genuine (non-artifact) colliding peers, and — if this
 * record is itself a corruption artifact — the real node it duplicates. Null
 * when the node's length isn't known yet. Shared by the desktop and mobile
 * node-detail views.
 */
export function nodeHashId(
	nodes: Node[],
	node: Node | null,
	pubkey: string
): { bytes: number; hex: string; shared: number; artifactOf: Node | null; reason?: string } | null {
	const hs = node?.hashSize ?? 0;
	if (!hs || !node) return null;
	const info = nodeCollisionInfo(nodes, node);
	return {
		bytes: hs,
		hex: pubkey.slice(0, hs * 2).toUpperCase(),
		shared: info.genuinePeers.length,
		artifactOf: info.artifactOf,
		reason: info.reason
	};
}

/**
 * Suggest a random free (and non-reserved) prefix at this length. For 1-byte the
 * space is tiny (256, minus 2 reserved) and often crowded, so we scan it
 * exhaustively; for 2/3-byte the space is huge so random sampling finds one fast.
 * Returns null only if every usable slot is taken (effectively just 1-byte).
 */
export function suggestFreePrefix(nodes: Node[], byteLen: HashByteLen): string | null {
	const used = usedPrefixes(nodes, byteLen);
	const total = 1 << (8 * byteLen);
	const toHex = (v: number) => v.toString(16).toUpperCase().padStart(byteLen * 2, '0');

	if (byteLen === 1) {
		const free: string[] = [];
		for (let v = 0; v < total; v++) {
			const p = toHex(v);
			if (!used.has(p) && !isReserved(p)) free.push(p);
		}
		return free.length ? free[Math.floor(Math.random() * free.length)] : null;
	}

	for (let tries = 0; tries < 10000; tries++) {
		const v = Math.floor(Math.random() * total);
		const p = toHex(v);
		if (!used.has(p) && !isReserved(p)) return p;
	}
	return null;
}
