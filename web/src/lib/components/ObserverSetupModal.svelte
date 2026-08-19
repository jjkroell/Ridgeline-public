<script lang="ts">
	// How to add an observer to this mesh, as a modal off the Observers page.
	//
	// Every value here comes from site-content.ts so a self-hoster's own broker,
	// radio settings and region appear instead of anyone else's — setup.sh writes
	// them from the install answers. The whole guide hides itself when MQTT.broker
	// is empty, since without a broker there is nothing to point an observer at.
	//
	// The audience must equal the broker's hostname exactly or tokens are refused
	// as minted for another broker. The CLI shown is the observer-firmware slot
	// syntax (mqttN.*), not the older compile-time build — see
	// observer.gessaman.com/docs.
	import Modal from './Modal.svelte';
	import IataPickerModal from './IataPickerModal.svelte';
	import { MQTT, RADIO_CLI } from '$lib/site-content';
	import { SITE_NAME } from '$lib/site';

	let { onclose }: { onclose: () => void } = $props();

	const BROKER = MQTT.broker;
	const AUDIENCE = MQTT.audience;

	let copied = $state<string | null>(null);
	// The picker stacks on top of this modal; while it's open this one must stop
	// answering Escape, or one press would close both.
	let showIata = $state(false);
	// Once a code is chosen it replaces the placeholder in the command blocks, so
	// the copy buttons hand over something ready to paste.
	let iata = $state<string | null>(null);
	const IATA_PLACEHOLDER = MQTT.defaultRegion || 'YOUR_IATA';
	const iataToken = $derived(iata ?? IATA_PLACEHOLDER);
	async function copy(label: string, text: string) {
		try {
			await navigator.clipboard.writeText(text);
			copied = label;
			setTimeout(() => (copied === label ? (copied = null) : null), 1400);
		} catch {
			/* clipboard blocked */
		}
	}

	// Slot 1 is the uplink; slot 2 ships preset to the Let's Mesh EU analyzer, so
	// it is explicitly disabled rather than left to publish elsewhere by default.
	const uplink = $derived(`set mqtt1.preset custom
set mqtt1.server ${BROKER}
${AUDIENCE ? `set mqtt1.audience ${AUDIENCE}\n` : ''}set mqtt2.preset none
set mqtt.iata ${iataToken}`);

	const radio = RADIO_CLI ? `set radio ${RADIO_CLI}\nset tx 22` : '';

	const wifi = `set wifi.ssid YOUR_NETWORK
set wifi.pwd YOUR_PASSWORD`;

	// The meshcoretomqtt script is the other common route locally. Same two values
	// as the firmware CLI — server and audience — and it authenticates identically
	// (v1_{PUBKEY} with an Ed25519 JWT), so there is still nothing to hand out.
	// No [topics] block: the script already builds the meshcore/{IATA}/{key}/…
	// layout from its own IATA setting, so repeating it here is just another
	// place for the two to disagree.
	const mc2mqtt = `[[broker]]
name = "${SITE_NAME.toLowerCase().replace(/[^a-z0-9]+/g, '-')}"
enabled = true
server = "${AUDIENCE}"
port = 443
transport = "websockets"

[broker.tls]
enabled = true

[broker.auth]
method = "token"
audience = "${AUDIENCE}"`;

	const mc2restart = 'sudo systemctl restart mctomqtt';

	const verify = `get mqtt.status
get mqtt1.preset
get wifi.status`;

	const everything = $derived(`${radio ? radio + '\n' : ''}set name YOUR_OBSERVER_NAME
${wifi}
${uplink}
reboot`);
</script>

<Modal {onclose} size="2xl" closeOnEscape={!showIata}>
	<div class="border-line/70 flex items-center gap-3 border-b px-5 py-4">
		<h2 class="font-display text-fg text-base font-700">Add an observer</h2>
		<button onclick={onclose} class="label hover:text-signal ml-auto transition-colors"
			>Close</button
		>
	</div>

	<div class="space-y-5 overflow-y-auto px-5 py-4">
		<p class="text-fg-dim text-sm leading-relaxed">
			An observer is a receive-only MeshCore node that reports the packets it hears
			to Ridgeline over the internet. It doesn't extend the mesh or carry anyone's
			traffic — it just adds a vantage point, and more vantage points mean a truer
			picture of which links actually work.
		</p>

		<section class="space-y-2">
			<div class="label">1 · Choose how to run it</div>
			<p class="text-fg-dim text-sm leading-relaxed">
				There are several ways to run an observer, so start by picking the one
				that fits the hardware you already have. You may not need to flash
				anything: a computer can read a companion radio over USB or TCP, OpenHop
				runs repeater, room server and observer as one Python process, and both
				the MeshCore Bot and the Home Assistant integration can observe alongside
				what they already do.
			</p>
			<div class="flex flex-wrap gap-2">
				<a
					href="https://observer.gessaman.com/observer-options"
					target="_blank"
					rel="noopener noreferrer"
					class="border-line text-fg-dim hover:border-line-bright hover:text-fg rounded-[var(--radius)] border px-3 py-1.5 text-sm transition-colors"
					>Compare the ways to run one ↗</a
				>
			</div>
			<p class="text-fg-faint text-xs leading-relaxed">
				Whichever you choose, it needs the same four things: the broker, the
				audience, a region and WiFi. Each project names them its own way — the
				values are in step 4.
			</p>

			<div class="border-line/70 mt-3 space-y-2 border-t pt-3">
				<div class="label">Or use a dedicated board</div>
				<p class="text-fg-dim text-sm leading-relaxed">
					The simplest route is a board of its own running observer firmware. Any
					WiFi-capable ESP32 MeshCore board works — Heltec V3/V4, LilyGo T3S3,
					T-Beam, Station G2, Xiao S3 WIO — and Adam Gessaman's flasher installs
					the build straight from the browser (Chrome or Edge), so there is
					nothing to compile.
				</p>
				<div class="flex flex-wrap gap-2">
					<a
						href="https://observer.gessaman.com/"
						target="_blank"
						rel="noopener noreferrer"
						class="border-line text-fg-dim hover:border-line-bright hover:text-fg rounded-[var(--radius)] border px-3 py-1.5 text-sm transition-colors"
						>Observer flasher ↗</a
					>
					<a
						href="https://observer.gessaman.com/docs"
						target="_blank"
						rel="noopener noreferrer"
						class="border-line text-fg-dim hover:border-line-bright hover:text-fg rounded-[var(--radius)] border px-3 py-1.5 text-sm transition-colors"
						>Firmware docs ↗</a
					>
				</div>
				<p class="text-fg-faint text-xs leading-relaxed">
					Took that route? Connect over serial at 115200 baud and follow the steps
					below.
				</p>
			</div>
		</section>

		{#if radio}
		<section class="space-y-2">
			<div class="label">2 · Match the mesh's radio settings</div>
			<p class="text-fg-dim text-sm leading-relaxed">
				A radio on the wrong settings hears nothing at all — this is the most
				common reason a new observer looks dead.
			</p>
			<div class="border-line bg-ink flex items-start gap-2 rounded-[var(--radius)] border px-3 py-2">
				<pre class="text-fg flex-1 overflow-x-auto font-mono text-xs leading-relaxed">{radio}</pre>
				<button
					onclick={() => copy('radio', radio)}
					class="text-fg-faint hover:text-fg shrink-0 text-xs">{copied === 'radio' ? '✓' : 'copy'}</button
				>
			</div>
		</section>
		{/if}

		<section class="space-y-2">
			<div class="label">3 · WiFi</div>
			<div class="border-line bg-ink flex items-start gap-2 rounded-[var(--radius)] border px-3 py-2">
				<pre class="text-fg flex-1 overflow-x-auto font-mono text-xs leading-relaxed">{wifi}</pre>
				<button
					onclick={() => copy('wifi', wifi)}
					class="text-fg-faint hover:text-fg shrink-0 text-xs">{copied === 'wifi' ? '✓' : 'copy'}</button
				>
			</div>
			<p class="text-fg-faint text-xs leading-relaxed">
				The value is the rest of the line — don't wrap it in quotes.
			</p>
		</section>

		<section class="space-y-2">
			<div class="label">4 · Point it at {SITE_NAME}</div>
			<p class="text-fg-dim text-sm leading-relaxed">
				{SITE_NAME}'s broker authenticates observers: your node signs a token with
				its own key, so it can only publish under its own identity and nobody can
				report traffic as you. Setting the audience is what turns that on — there
				is no password to obtain and nothing to register.
			</p>
			<div class="text-fg-faint text-xs">Observer firmware, over serial:</div>
			<div class="border-line bg-ink flex items-start gap-2 rounded-[var(--radius)] border px-3 py-2">
				<pre class="text-fg flex-1 overflow-x-auto font-mono text-xs leading-relaxed">{uplink}</pre>
				<button
					onclick={() => copy('uplink', uplink)}
					class="text-fg-faint hover:text-fg shrink-0 text-xs"
					>{copied === 'uplink' ? '✓' : 'copy'}</button
				>
			</div>

			<div class="text-fg-faint pt-1 text-xs">
				Or, if you're running the <span class="text-fg-dim font-mono">meshcoretomqtt</span>
				script, in
				<span class="text-fg-dim font-mono">/etc/mctomqtt/config.d/00-user.toml</span>:
			</div>
			<div class="border-line bg-ink flex items-start gap-2 rounded-[var(--radius)] border px-3 py-2">
				<pre class="text-fg flex-1 overflow-x-auto font-mono text-xs leading-relaxed">{mc2mqtt}</pre>
				<button
					onclick={() => copy('mc2mqtt', mc2mqtt)}
					class="text-fg-faint hover:text-fg shrink-0 text-xs"
					>{copied === 'mc2mqtt' ? '✓' : 'copy'}</button
				>
			</div>
			<div class="text-fg-faint text-xs">Then restart it:</div>
			<div class="border-line bg-ink flex items-start gap-2 rounded-[var(--radius)] border px-3 py-2">
				<pre class="text-fg flex-1 overflow-x-auto font-mono text-xs leading-relaxed">{mc2restart}</pre>
				<button
					onclick={() => copy('mc2restart', mc2restart)}
					class="text-fg-faint hover:text-fg shrink-0 text-xs"
					>{copied === 'mc2restart' ? '✓' : 'copy'}</button
				>
			</div>
			{#if MQTT.showIataPicker}
			<div class="flex flex-wrap items-center gap-2">
				<!-- Accented: it's the one control in this guide that does something
				     rather than linking away, and it's easy to miss between two code
				     blocks. -->
				<button
					onclick={() => (showIata = true)}
					class="border-signal/50 bg-signal/10 text-signal hover:bg-signal/20 rounded-[var(--radius)] border px-3 py-1.5 text-sm font-600 transition-colors"
					>{iata ? `Region: ${iata} — change` : 'Find your IATA code'}</button
				>
				{#if iata}
					<button
						onclick={() => (iata = null)}
						class="text-fg-faint hover:text-fg text-xs transition-colors">reset</button
					>
				{/if}
			</div>
			{/if}
			<p class="text-fg-faint text-xs leading-relaxed">
				<code class="text-fg-dim font-mono">{iataToken}</code> is the nearest airport's
				IATA code — it becomes the region your observer is grouped under. If you'd
				rather keep feeding the Let's Mesh analyzer as well, leave slots 1 and 2
				alone and put the settings above on
				<code class="text-fg-dim font-mono">mqtt3</code> instead.
			</p>
		</section>

		<section class="space-y-2">
			<div class="label">5 · Reboot and check</div>
			<div class="border-line bg-ink flex items-start gap-2 rounded-[var(--radius)] border px-3 py-2">
				<pre class="text-fg flex-1 overflow-x-auto font-mono text-xs leading-relaxed">{verify}</pre>
				<button
					onclick={() => copy('verify', verify)}
					class="text-fg-faint hover:text-fg shrink-0 text-xs"
					>{copied === 'verify' ? '✓' : 'copy'}</button
				>
			</div>
			<p class="text-fg-faint text-xs leading-relaxed">
				Your station should appear on this page within a few minutes of hearing its
				first packet.
			</p>
		</section>

		<section class="space-y-2 border-t border-line/70 pt-4">
			<div class="label">Everything at once</div>
			<div class="border-line bg-ink flex items-start gap-2 rounded-[var(--radius)] border px-3 py-2">
				<pre
					class="text-fg flex-1 overflow-x-auto font-mono text-xs leading-relaxed">{everything}</pre>
				<button
					onclick={() => copy('all', everything)}
					class="text-fg-faint hover:text-fg shrink-0 text-xs">{copied === 'all' ? '✓' : 'copy'}</button
				>
			</div>
			<p class="text-fg-faint text-xs leading-relaxed">
				Replace the three placeholders first. Paste a line at a time if your serial
				console drops characters.
			</p>
		</section>
	</div>
</Modal>

<!-- Rendered AFTER the modal above, not before: both overlays are z-50, so DOM
     order is what decides which one paints on top. Moved earlier in the file and
     the picker opens invisibly behind the guide, which looks like a dead link. -->
{#if showIata && MQTT.showIataPicker}
	<IataPickerModal
		onclose={() => (showIata = false)}
		onpick={(code) => {
			iata = code;
			showIata = false;
		}}
	/>
{/if}
