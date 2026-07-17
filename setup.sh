#!/usr/bin/env bash
#
# Ridgeline interactive installer.
#
# Walks you through personalizing a fresh Ridgeline checkout for YOUR mesh:
# site name, region/map center, MQTT broker, email, domain/TLS, and an optional
# custom About page. It then writes every config file and can build + launch the
# Docker stack for you.
#
# Nothing here phones home. Every value is one you supply; defaults are generic.
# Re-run it any time to change your answers — it overwrites the same files.

set -euo pipefail

# --- locate the repo (this script's directory) -----------------------------
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# --- pretty output ----------------------------------------------------------
if [[ -t 1 ]]; then
	BOLD=$'\033[1m'; DIM=$'\033[2m'; GREEN=$'\033[32m'; CYAN=$'\033[36m'
	YELLOW=$'\033[33m'; RED=$'\033[31m'; RESET=$'\033[0m'
else
	BOLD=''; DIM=''; GREEN=''; CYAN=''; YELLOW=''; RED=''; RESET=''
fi
say()  { printf '%s\n' "$*"; }
info() { printf '%s➤%s %s\n' "$CYAN" "$RESET" "$*"; }
ok()   { printf '%s✓%s %s\n' "$GREEN" "$RESET" "$*"; }
warn() { printf '%s!%s %s\n' "$YELLOW" "$RESET" "$*"; }
die()  { printf '%s✗ %s%s\n' "$RED" "$*" "$RESET" >&2; exit 1; }
hr()   { printf '%s\n' "${DIM}────────────────────────────────────────────────────────${RESET}"; }

# --- prompt helpers ---------------------------------------------------------
# ask VAR "Prompt" "default"   -> reads a line, falls back to default on empty.
ask() {
	local __var="$1" __prompt="$2" __def="${3:-}" __in
	if [[ -n "$__def" ]]; then
		read -r -p "$(printf '%s %s[%s]%s: ' "$__prompt" "$DIM" "$__def" "$RESET")" __in || true
	else
		read -r -p "$(printf '%s: ' "$__prompt")" __in || true
	fi
	printf -v "$__var" '%s' "${__in:-$__def}"
}
# ask_secret VAR "Prompt"      -> reads without echo.
ask_secret() {
	local __var="$1" __prompt="$2" __in
	read -r -s -p "$(printf '%s: ' "$__prompt")" __in || true
	echo
	printf -v "$__var" '%s' "$__in"
}
# yesno "Prompt" default(y/n)  -> returns 0 for yes, 1 for no.
yesno() {
	local __prompt="$1" __def="${2:-n}" __in __hint
	[[ "$__def" == y ]] && __hint="Y/n" || __hint="y/N"
	read -r -p "$(printf '%s %s[%s]%s ' "$__prompt" "$DIM" "$__hint" "$RESET")" __in || true
	__in="${__in:-$__def}"
	[[ "$__in" =~ ^[Yy] ]]
}

command -v python3 >/dev/null 2>&1 || die "python3 is required (used to write JSON/TS config safely). Please install it and re-run."

clear 2>/dev/null || true
say "${BOLD}${CYAN}"
say "  ┌───────────────────────────────────────────────┐"
say "  │   Ridgeline — MeshCore mesh observatory setup  │"
say "  └───────────────────────────────────────────────┘"
say "${RESET}"
say "This will personalize your copy of Ridgeline. Press Enter to accept the"
say "${DIM}[default]${RESET} shown for any question."
echo

# ===========================================================================
# 1. Site identity
# ===========================================================================
hr; info "${BOLD}Your site${RESET}"
ask SITE_NAME    "Site name (shown in the header & tab title)" "Ridgeline"
ask SITE_TAGLINE "Short tagline under the name" "MeshCore Observatory"
say "${DIM}Your public URL is used for links in emails, the sitemap, and social previews.${RESET}"
ask SITE_URL     "Public site URL (e.g. https://mesh.example.com, blank to skip)" ""
SITE_URL="${SITE_URL%/}"   # strip trailing slash
ask SITE_DESC    "One-line description of your mesh (for search engines)" \
	"A live observatory for a MeshCore LoRa mesh network."
say "${DIM}Shown on the /privacy page as the data-controller contact (GDPR/PIPEDA).${RESET}"
ask PRIVACY_CONTACT "Privacy contact email (blank to omit)" ""

# ===========================================================================
# 2. Region / default map view
# ===========================================================================
echo; hr; info "${BOLD}Region${RESET}  ${DIM}(where the maps open by default)${RESET}"
say "Enter the approximate center of your mesh. Tip: right-click a spot in Google"
say "Maps to copy its latitude, longitude. Leave blank for a wide world view."
ask MAP_LAT  "Center latitude"  ""
ask MAP_LON  "Center longitude" ""
ask MAP_ZOOM "Default zoom (1 = world, 8 = regional, 12 = city)" "8"
if [[ -z "$MAP_LAT" || -z "$MAP_LON" ]]; then
	MAP_LAT="20"; MAP_LON="0"; MAP_ZOOM="3"
	warn "No center given — defaulting to a wide world view."
fi

# ===========================================================================
# 3. MQTT broker (where observer nodes publish)
# ===========================================================================
echo; hr; info "${BOLD}MQTT broker${RESET}  ${DIM}(where your observer nodes publish packets)${RESET}"
SELF_HOST_BROKER=false
if yesno "Host a broker for you as part of the stack (recommended)?" y; then
	SELF_HOST_BROKER=true
	# The daemon talks to the bundled mosquitto over the compose network.
	MQTT_BROKER="tcp://mosquitto:1883"
	MQTT_USER=""; MQTT_PASS=""
	ok "A mosquitto broker will run alongside Ridgeline (observers connect on :9001, WebSockets)."
else
	ask MQTT_BROKER "Existing broker URL (e.g. tcp://broker.example.com:1883)" "tcp://localhost:1883"
	ask MQTT_USER   "Broker username (blank if anonymous)" ""
	if [[ -n "$MQTT_USER" ]]; then ask_secret MQTT_PASS "Broker password"; else MQTT_PASS=""; fi
fi
ask MQTT_CLIENTID "MQTT client id (must be unique on the broker)" "ridgelined"
ask MQTT_TOPICS   "Topics to subscribe (comma-separated)" "meshcore/+/+/packets,meshcore/+/+/status"

# ===========================================================================
# 4. Email (optional — enables account verification & note notifications)
# ===========================================================================
echo; hr; info "${BOLD}Email${RESET}  ${DIM}(optional — powers account verification & owner note alerts)${RESET}"
EMAIL_ENABLED=false
EMAIL_HOST=""; EMAIL_PORT="587"; EMAIL_USER=""; EMAIL_PASS=""; EMAIL_FROM=""; EMAIL_FROMNAME="$SITE_NAME"; EMAIL_BASEURL="$SITE_URL"
if yesno "Configure outbound email (SMTP) now?" n; then
	say "${DIM}Works with any SMTP relay (Brevo, Postmark, SendGrid, Amazon SES, your own).${RESET}"
	ask EMAIL_HOST     "SMTP host (e.g. smtp-relay.brevo.com)" ""
	if [[ -n "$EMAIL_HOST" ]]; then
		ask EMAIL_PORT     "SMTP port" "587"
		ask EMAIL_USER     "SMTP username" ""
		ask_secret EMAIL_PASS "SMTP password / API key"
		ask EMAIL_FROM     "From address (must be on a domain you've authenticated)" "noreply@example.com"
		ask EMAIL_FROMNAME "From name" "$SITE_NAME"
		ask EMAIL_BASEURL  "Public base URL for links in emails" "${SITE_URL:-https://mesh.example.com}"
		EMAIL_ENABLED=true
		ok "Email enabled."
	else
		warn "No host entered — email stays disabled (registration will auto-verify accounts)."
	fi
else
	say "${DIM}Skipped. Accounts still work; registration just auto-verifies without email.${RESET}"
fi

# ===========================================================================
# 5. About page (opt-in, only the info you provide)
# ===========================================================================
echo; hr; info "${BOLD}About page${RESET}"
ABOUT_MODE="default"   # default | custom | none
ABOUT_KICKER="MeshCore Observatory"
ABOUT_TITLE="$SITE_NAME watches a MeshCore mesh"
ABOUT_INTRO=""
ABOUT_FOOTER="$SITE_NAME is an independent, community-run MeshCore mesh observatory."
SECTION_COUNT=0
say "Ridgeline can show an /about page describing your mesh."
say "  ${BOLD}1${RESET}) Keep the generic default page"
say "  ${BOLD}2${RESET}) Write my own (only the text I enter will appear)"
say "  ${BOLD}3${RESET}) No About page at all"
ask ABOUT_CHOICE "Choose 1, 2 or 3" "1"
case "$ABOUT_CHOICE" in
	2)
		ABOUT_MODE="custom"
		say "${DIM}Enter your own text. Blank answers are simply left out.${RESET}"
		ask ABOUT_KICKER "Small kicker above the title" "About $SITE_NAME"
		ask ABOUT_TITLE  "Page heading" "About $SITE_NAME"
		ask ABOUT_INTRO  "Intro paragraph" ""
		while true; do
			if [[ "$SECTION_COUNT" -eq 0 ]]; then
				yesno "Add a content section?" y || break
			else
				yesno "Add another section?" n || break
			fi
			ask __sh "  Section heading" ""
			ask __sb "  Section text" ""
			[[ -z "$__sh" && -z "$__sb" ]] && { warn "Empty section skipped."; continue; }
			printf -v "SECTION_${SECTION_COUNT}_HEADING" '%s' "$__sh"
			printf -v "SECTION_${SECTION_COUNT}_BODY" '%s' "$__sb"
			SECTION_COUNT=$((SECTION_COUNT + 1))
		done
		ask ABOUT_FOOTER "Footer line (blank to omit)" ""
		;;
	3)
		ABOUT_MODE="none"
		ok "The About page and its nav link will be removed."
		;;
	*)
		ABOUT_MODE="default"
		;;
esac

# Radio settings table (shown on the About page). Region-specific.
SHOW_RADIO=true
if [[ "$ABOUT_MODE" != "none" ]]; then
	if yesno "Show your mesh's LoRa radio settings on the About page?" y; then
		ask RADIO_FREQ "  Frequency (e.g. 915.0 MHz)" "915.0 MHz"
		ask RADIO_BW   "  Bandwidth (e.g. 62.5 kHz)"  "62.5 kHz"
		ask RADIO_SF   "  Spreading factor (e.g. SF 7)" "SF 7"
		ask RADIO_CR   "  Coding rate (e.g. CR 5)"      "CR 5"
	else
		SHOW_RADIO=false; RADIO_FREQ=""; RADIO_BW=""; RADIO_SF=""; RADIO_CR=""
	fi
else
	SHOW_RADIO=false; RADIO_FREQ=""; RADIO_BW=""; RADIO_SF=""; RADIO_CR=""
fi

# ===========================================================================
# 6. Domain / TLS (Caddy)
# ===========================================================================
echo; hr; info "${BOLD}Web serving${RESET}  ${DIM}(Caddy reverse proxy)${RESET}"
CADDY_ADDR=":80"
say "How is the site reached from the internet?"
say "  ${BOLD}1${RESET}) Caddy gets HTTPS automatically for a domain (points DNS straight at this host)"
say "  ${BOLD}2${RESET}) Behind Cloudflare Tunnel / another proxy that handles TLS (plain HTTP)"
ask SERVE_CHOICE "Choose 1 or 2" "2"
if [[ "$SERVE_CHOICE" == "1" ]]; then
	ask CADDY_DOMAIN "Domain for the certificate (e.g. mesh.example.com)" ""
	[[ -n "$CADDY_DOMAIN" ]] && CADDY_ADDR="$CADDY_DOMAIN" || warn "No domain — Caddy will serve plain HTTP on :80."
fi

# ===========================================================================
# 7. Admin token
# ===========================================================================
echo; hr; info "${BOLD}Admin access${RESET}"
GEN_TOKEN="$(head -c 32 /dev/urandom 2>/dev/null | od -An -tx1 | tr -d ' \n' || true)"
[[ -z "$GEN_TOKEN" ]] && GEN_TOKEN="$(python3 -c 'import secrets;print(secrets.token_hex(32))')"
ask ADMIN_TOKEN "Admin token (protects the /admin panel)" "$GEN_TOKEN"

LISTEN_ADDR="0.0.0.0:8080"
DB_PATH="/data/ridgeline.db"

# ===========================================================================
# Review
# ===========================================================================
echo; hr; info "${BOLD}Review${RESET}"
printf '  %-16s %s\n' "Site name:"  "$SITE_NAME"
printf '  %-16s %s\n' "URL:"        "${SITE_URL:-<none>}"
printf '  %-16s %s\n' "Map center:" "$MAP_LAT, $MAP_LON  (zoom $MAP_ZOOM)"
printf '  %-16s %s\n' "Broker:"     "$MQTT_BROKER $( $SELF_HOST_BROKER && echo '(bundled)')"
printf '  %-16s %s\n' "Email:"      "$( $EMAIL_ENABLED && echo "on ($EMAIL_HOST)" || echo off)"
printf '  %-16s %s\n' "About page:" "$ABOUT_MODE"
printf '  %-16s %s\n' "Caddy addr:" "$CADDY_ADDR"
echo
yesno "Write these settings?" y || die "Aborted — nothing was written."

# ===========================================================================
# Write files
# ===========================================================================
echo; info "Writing configuration…"

# Export everything the python writers below read from the environment.
export SITE_NAME SITE_TAGLINE SITE_URL SITE_DESC PRIVACY_CONTACT MAP_LAT MAP_LON MAP_ZOOM \
	ABOUT_MODE ABOUT_KICKER ABOUT_TITLE ABOUT_INTRO ABOUT_FOOTER SECTION_COUNT SHOW_RADIO \
	RADIO_FREQ RADIO_BW RADIO_SF RADIO_CR \
	MQTT_BROKER MQTT_CLIENTID MQTT_USER MQTT_PASS MQTT_TOPICS \
	EMAIL_ENABLED EMAIL_HOST EMAIL_PORT EMAIL_USER EMAIL_PASS EMAIL_FROM EMAIL_FROMNAME EMAIL_BASEURL \
	ADMIN_TOKEN LISTEN_ADDR DB_PATH CADDY_ADDR
# Section text vars are dynamically named (SECTION_0_HEADING, …); export by pattern.
for __i in $(seq 0 "$SECTION_COUNT"); do
	export "SECTION_${__i}_HEADING" 2>/dev/null || true
	export "SECTION_${__i}_BODY" 2>/dev/null || true
done

# 7a. Frontend build-time env (Vite inlines VITE_* at build).
python3 - <<'PY'
import os
def esc(s): return s.replace('\\', '\\\\').replace('"', '\\"')
vals = {
    'VITE_SITE_NAME':        os.environ['SITE_NAME'],
    'VITE_SITE_TAGLINE':     os.environ['SITE_TAGLINE'],
    'VITE_SITE_URL':         os.environ['SITE_URL'],
    'VITE_SITE_DESCRIPTION': os.environ['SITE_DESC'],
    'VITE_PRIVACY_CONTACT':  os.environ.get('PRIVACY_CONTACT', ''),
    'VITE_MAP_CENTER_LAT':   os.environ['MAP_LAT'],
    'VITE_MAP_CENTER_LON':   os.environ['MAP_LON'],
    'VITE_MAP_ZOOM':         os.environ['MAP_ZOOM'],
}
lines = ['# Generated by setup.sh — Vite inlines these into the built web app.',
         '# Safe to commit? No secrets here, but it is per-deploy; regenerate via ./setup.sh.']
for k, v in vals.items():
    lines.append(f'{k}="{esc(v)}"')
open('web/.env', 'w').write('\n'.join(lines) + '\n')
print('  web/.env')
PY

# 7b. Frontend content module (About page + radio table).
python3 - <<'PY'
import os, json
name = os.environ['SITE_NAME']
mode = os.environ['ABOUT_MODE']
show_radio = os.environ['SHOW_RADIO'] == 'true'

radio = []
for k, envk in (('Frequency','RADIO_FREQ'),('Bandwidth','RADIO_BW'),
                ('Spreading factor','RADIO_SF'),('Coding rate','RADIO_CR')):
    v = os.environ.get(envk, '').strip()
    if v: radio.append({'k': k, 'v': v})

if mode == 'custom':
    sections = []
    for i in range(int(os.environ.get('SECTION_COUNT', '0'))):
        h = os.environ.get(f'SECTION_{i}_HEADING', '').strip()
        b = os.environ.get(f'SECTION_{i}_BODY', '').strip()
        if h or b:
            sections.append({'heading': h, 'paragraphs': [b] if b else []})
    about = {
        'enabled': True,
        'kicker': os.environ.get('ABOUT_KICKER', '').strip(),
        'title':  os.environ.get('ABOUT_TITLE', '').strip() or f'About {name}',
        'intro':  os.environ.get('ABOUT_INTRO', '').strip(),
        'sections': sections,
        'showRadio': show_radio,
        'footer': os.environ.get('ABOUT_FOOTER', '').strip(),
    }
elif mode == 'none':
    about = {'enabled': False, 'kicker': '', 'title': '', 'intro': '',
             'sections': [], 'showRadio': False, 'footer': ''}
else:  # default — the generic, geography-free page
    about = {
        'enabled': True,
        'kicker': 'MeshCore Observatory',
        'title': f'{name} watches a MeshCore mesh',
        'intro': (f'{name} is a window onto a MeshCore radio mesh. Point it at the network '
                  'and you can see which radios are awake, which repeaters are carrying '
                  'traffic, and where a message can still get through today.'),
        'sections': [
            {'heading': f'What {name} does', 'paragraphs': [
                'MeshCore is a protocol for cheap, low-power LoRa radios that pass messages hop to hop — no towers, no internet, no monthly bill. The catch is that a mesh is mostly invisible while it runs: the traffic is in the air, not on any screen.',
                f'{name} gives it one. Receive-only stations called observers sit and listen, then pass what they hear back to be decoded. What comes out is a searchable node directory, a live packet feed, interactive coverage and signal maps, and analytics that describe the network as a whole.']},
            {'heading': 'Nodes, repeaters, and observers', 'paragraphs': [
                "A node is any MeshCore device on the network — a handheld you carry, a base station in a window, a solar-powered box bolted to a hill. A repeater is a node built to listen and rebroadcast, extending the mesh's reach with every hop. An observer only receives, and reports what it hears, which is why the maps have anything to show at all."]},
            {'heading': 'Join the network', 'paragraphs': [
                'Anyone within range of a compatible MeshCore radio can join. Match the radio settings your mesh uses and the radio will begin hearing its neighbours and relaying their traffic. Then watch the live map and packet feed: the first time your own node announces itself, you will see it take its place among the others.']},
        ],
        'showRadio': show_radio,
        'footer': f'{name} is an independent, community-run MeshCore mesh observatory.',
    }

def j(x): return json.dumps(x, ensure_ascii=False, indent='\t')

out = f'''// Editable site content for the public build.
//
// This file was generated by setup.sh. Edit it by hand any time and rebuild —
// it is plain data, no framework knowledge required.

import {{ SITE_NAME }} from './site';

export interface RadioParam {{
\tk: string;
\tv: string;
}}

export const RADIO_PARAMS: RadioParam[] = {j(radio)};

export interface AboutSection {{
\theading: string;
\tparagraphs: string[];
}}

export interface AboutContent {{
\tenabled: boolean;
\tkicker: string;
\ttitle: string;
\tintro: string;
\tsections: AboutSection[];
\tshowRadio: boolean;
\tfooter: string;
}}

export const ABOUT: AboutContent = {j(about)};

// Referenced so SITE_NAME stays imported even when the generated content above
// does not interpolate it (e.g. a fully custom About page).
void SITE_NAME;
'''
open('web/src/lib/site-content.ts', 'w').write(out)
print('  web/src/lib/site-content.ts')
PY

# 7c. Deploy config.json (secrets — gitignored).
python3 - <<'PY'
import os, json
topics = [t.strip() for t in os.environ['MQTT_TOPICS'].split(',') if t.strip()]
cfg = {
    'listenAddr': os.environ['LISTEN_ADDR'],
    'dbPath': os.environ['DB_PATH'],
    'webDir': '/app/web/build',
    'mqtt': {
        'broker': os.environ['MQTT_BROKER'],
        'clientID': os.environ['MQTT_CLIENTID'],
        'topics': topics,
    },
    'adminToken': os.environ['ADMIN_TOKEN'],
}
if os.environ['MQTT_USER']:
    cfg['mqtt']['username'] = os.environ['MQTT_USER']
    cfg['mqtt']['password'] = os.environ['MQTT_PASS']
if os.environ['EMAIL_ENABLED'] == 'true':
    cfg['email'] = {
        'host': os.environ['EMAIL_HOST'],
        'port': int(os.environ['EMAIL_PORT'] or '587'),
        'username': os.environ['EMAIL_USER'],
        'password': os.environ['EMAIL_PASS'],
        'from': os.environ['EMAIL_FROM'],
        'fromName': os.environ['EMAIL_FROMNAME'],
        'baseURL': os.environ['EMAIL_BASEURL'],
    }
os.makedirs('deploy', exist_ok=True)
open('deploy/config.json', 'w').write(json.dumps(cfg, indent=2) + '\n')
print('  deploy/config.json')
PY

# 7d. SEO surfaces + Caddy address (string substitutions).
python3 - <<'PY'
import os
name = os.environ['SITE_NAME']
url = os.environ['SITE_URL'].rstrip('/')
caddy_addr = os.environ['CADDY_ADDR']

def sub(path, old, new, count=0):
    try:
        s = open(path, encoding='utf-8').read()
    except FileNotFoundError:
        return
    s2 = s.replace(old, new) if count == 0 else s.replace(old, new, count)
    if s2 != s:
        open(path, 'w', encoding='utf-8').write(s2)
        print(f'  {path}')

# Brand name in the crawl-time SEO shell (capital-R only; leaves 'ridgeline-theme').
if name != 'Ridgeline':
    sub('web/src/app.html', 'Ridgeline', name)

# Absolute URLs for sitemap/robots when a site URL was given.
if url:
    sub('web/static/sitemap.xml', 'https://example.com', url)
    sub('web/static/robots.txt', 'Sitemap: /sitemap.xml', f'Sitemap: {url}/sitemap.xml')

# Caddy site address (first block only).
if caddy_addr and caddy_addr != ':80':
    sub('deploy/Caddyfile', ':80 {', f'{caddy_addr} {{', 1)
PY

# 7e. Deploy uid/gid so the bind-mounted DB dir is writable.
# Stamp the build with the current git version (tag/sha) so the daemon logs
# something more useful than "dev". Falls back to "dev" outside a git checkout.
RL_VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
printf 'RIDGELINE_UID=%s\nRIDGELINE_GID=%s\nRIDGELINE_VERSION=%s\n' \
	"$(id -u)" "$(id -g)" "$RL_VERSION" > deploy/.env
say "  deploy/.env"
mkdir -p deploy/data

ok "Configuration written."

if ! $SELF_HOST_BROKER; then
	echo
	warn "You chose an external broker. The bundled 'mosquitto' service in"
	warn "deploy/docker-compose.yml is harmless but unused — delete that service"
	warn "block if you don't want it running."
fi

# ===========================================================================
# Build & launch
# ===========================================================================
echo; hr; info "${BOLD}Build & launch${RESET}"
if command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
	if yesno "Build and start the Docker stack now?" y; then
		info "Building… (first run compiles the web app + Go daemon; a few minutes)"
		( cd deploy && docker compose up -d --build )
		echo
		ok "Ridgeline is up."
		say "  • Logs:   ${DIM}cd deploy && docker compose logs -f ridgelined${RESET}"
		say "  • Stop:   ${DIM}cd deploy && docker compose down${RESET}"
		if [[ "$CADDY_ADDR" == ":80" ]]; then
			say "  • Local:  ${DIM}http://localhost${RESET}"
		else
			say "  • Site:   ${DIM}https://$CADDY_ADDR${RESET} (once DNS points here)"
		fi
	else
		say "Skipped. When ready: ${DIM}cd deploy && docker compose up -d --build${RESET}"
	fi
else
	warn "Docker (with the compose plugin) wasn't found."
	say "Install Docker, then: ${DIM}cd deploy && docker compose up -d --build${RESET}"
	say "Or run without Docker — see ${DIM}deploy/README.md${RESET}."
fi

echo; ok "${BOLD}Done.${RESET} Re-run ./setup.sh any time to change these answers."
