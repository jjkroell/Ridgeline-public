// Editable site content for the public build.
//
// setup.sh overwrites this file from your install answers — the region name, the
// LoRa radio parameters your mesh uses, and the About page (including whether to
// have one at all). You can also just edit it by hand and rebuild; it's plain
// data, no framework knowledge required.

import { SITE_NAME } from './site';

/** One LoRa radio parameter shown in the About page's settings table. */
export interface RadioParam {
	k: string;
	v: string;
}

/** The LoRa parameters every radio on your mesh must match. Set to an empty
 *  array (or ABOUT.showRadio = false) to hide the settings table. */
export const RADIO_PARAMS: RadioParam[] = [
	{ k: 'Frequency', v: '915.0 MHz' },
	{ k: 'Bandwidth', v: '62.5 kHz' },
	{ k: 'Spreading factor', v: 'SF 7' },
	{ k: 'Coding rate', v: 'CR 5' }
];

/** One heading + body block on the About page. Each string in `paragraphs` is a
 *  paragraph; keep them plain text (they're rendered as-is, not HTML). */
export interface AboutSection {
	heading: string;
	paragraphs: string[];
}

export interface AboutContent {
	/** When false, the About page and its nav link are removed entirely. */
	enabled: boolean;
	/** Small kicker above the title. */
	kicker: string;
	/** Page H1. */
	title: string;
	/** Lead paragraph under the title. */
	intro: string;
	/** Body sections, in order. */
	sections: AboutSection[];
	/** Show the LoRa radio-settings table built from RADIO_PARAMS. */
	showRadio: boolean;
	/** Small footer line. */
	footer: string;
}

// Generic, geography-free default. setup.sh replaces this with your own words if
// you opt into a custom About page, or sets `enabled: false` if you don't want one.
export const ABOUT: AboutContent = {
	enabled: true,
	kicker: 'MeshCore Observatory',
	title: `${SITE_NAME} watches a MeshCore mesh`,
	intro:
		`${SITE_NAME} is a window onto a MeshCore radio mesh. Point it at the network ` +
		`and you can see which radios are awake, which repeaters are carrying traffic, ` +
		`and where a message can still get through today.`,
	sections: [
		{
			heading: `What ${SITE_NAME} does`,
			paragraphs: [
				'MeshCore is a protocol for cheap, low-power LoRa radios that pass messages hop to hop — no towers, no internet, no monthly bill. The catch is that a mesh is mostly invisible while it runs: the traffic is in the air, not on any screen.',
				`${SITE_NAME} gives it one. Receive-only stations called observers sit and listen, then pass what they hear back to be decoded. What comes out is a searchable node directory, a live packet feed, interactive coverage and signal maps, and analytics that describe the network as a whole.`
			]
		},
		{
			heading: 'Nodes, repeaters, and observers',
			paragraphs: [
				"A node is any MeshCore device on the network — a handheld you carry, a base station in a window, a solar-powered box bolted to a hill. A repeater is a node built to listen and rebroadcast, extending the mesh's reach with every hop. An observer only receives, and reports what it hears, which is why the maps have anything to show at all."
			]
		},
		{
			heading: 'Join the network',
			paragraphs: [
				'Anyone within range of a compatible MeshCore radio can join. Match the radio settings your mesh uses and the radio will begin hearing its neighbours and relaying their traffic. Then watch the live map and packet feed: the first time your own node announces itself, you will see it take its place among the others.'
			]
		}
	],
	showRadio: true,
	footer: `${SITE_NAME} is an independent, community-run MeshCore mesh observatory.`
};
