// Splits message text into plain runs and linkable URLs so chat bubbles can
// render anchors WITHOUT {@html} — channel text is decrypted from the air and
// is entirely untrusted, so it must never reach the DOM as markup.

export interface TextSegment {
	text: string;
	/** Present when this run should render as a link. Always http(s). */
	href?: string;
}

// Deliberately greedy: grab a whole non-whitespace run, then trim the trailing
// punctuation that sentences (rather than URLs) tend to end with.
const URL_RE = /(?:https?:\/\/|www\.)\S+/gi;

const CLOSERS: Record<string, string> = { ')': '(', ']': '[', '}': '{' };

// Trailing characters a sentence can leave stuck to a URL. Closing brackets are
// kept only when the URL opened them itself (wikipedia-style paths).
function trimTrailing(url: string): string {
	let end = url.length;
	for (; end > 0; end--) {
		const ch = url[end - 1];
		if ('.,;:!?\'"«»„“”‘’'.includes(ch)) continue;
		if (ch in CLOSERS) {
			const body = url.slice(0, end);
			const opens = body.split(CLOSERS[ch]).length - 1;
			const closes = body.split(ch).length - 1;
			if (closes > opens) continue; // unbalanced — punctuation, not path
		}
		break;
	}
	return url.slice(0, end);
}

// A bare "http://" or "www." with no dotted host isn't a link worth making.
function looksLikeUrl(url: string): boolean {
	const rest = url.replace(/^https?:\/\//i, '');
	const host = rest.split(/[/?#]/, 1)[0];
	return host.includes('.') && !host.startsWith('.') && !host.endsWith('.');
}

export function linkSegments(text: string): TextSegment[] {
	const out: TextSegment[] = [];
	let last = 0;
	URL_RE.lastIndex = 0;
	for (let m = URL_RE.exec(text); m; m = URL_RE.exec(text)) {
		const url = trimTrailing(m[0]);
		if (!url || !looksLikeUrl(url)) continue;
		if (m.index > last) out.push({ text: text.slice(last, m.index) });
		out.push({
			text: url,
			href: /^https?:\/\//i.test(url) ? url : `https://${url}`
		});
		last = m.index + url.length;
	}
	if (last < text.length) out.push({ text: text.slice(last) });
	return out;
}
