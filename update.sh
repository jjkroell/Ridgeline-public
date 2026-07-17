#!/usr/bin/env bash
#
# Ridgeline updater: pull the latest code, rebuild, and restart the Docker stack —
# keeping your personalized config (web/.env, deploy/config.json, deploy/.env) and
# your database (deploy/data/) intact.
#
#   ./update.sh              update the stock stack (deploy/docker-compose.yml)
#   ./update.sh --external   update an external-broker install (docker-compose.external.yml)
#
# Safe to run any time. Your admin account and data persist across updates.

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

if [[ -t 1 ]]; then
	BOLD=$'\033[1m'; DIM=$'\033[2m'; GREEN=$'\033[32m'; CYAN=$'\033[36m'; YELLOW=$'\033[33m'; RED=$'\033[31m'; RESET=$'\033[0m'
else
	BOLD=''; DIM=''; GREEN=''; CYAN=''; YELLOW=''; RED=''; RESET=''
fi
say()  { printf '%s\n' "$*"; }
info() { printf '%s➤%s %s\n' "$CYAN" "$RESET" "$*"; }
ok()   { printf '%s✓%s %s\n' "$GREEN" "$RESET" "$*"; }
warn() { printf '%s!%s %s\n' "$YELLOW" "$RESET" "$*"; }
die()  { printf '%s✗ %s%s\n' "$RED" "$*" "$RESET" >&2; exit 1; }

# --- prerequisites ----------------------------------------------------------
[[ -d .git ]] || die "Not a git checkout. Update by re-cloning, or run ./setup.sh."
command -v git >/dev/null 2>&1 || die "git is required."
command -v docker >/dev/null 2>&1 || die "docker is required."
docker compose version >/dev/null 2>&1 || die "the docker compose plugin is required."

COMPOSE="docker-compose.yml"
[[ "${1:-}" == "--external" ]] && COMPOSE="docker-compose.external.yml"
[[ -f "deploy/$COMPOSE" ]] || die "deploy/$COMPOSE not found."

# --- pull -------------------------------------------------------------------
BRANCH="$(git rev-parse --abbrev-ref HEAD)"
BEFORE="$(git rev-parse --short HEAD)"
info "On branch ${BOLD}$BRANCH${RESET} at $BEFORE — fetching latest…"

if ! git pull --ff-only 2>/tmp/rl-update-pull.err; then
	warn "A plain pull didn't apply cleanly:"
	sed 's/^/    /' /tmp/rl-update-pull.err
	# The usual cause: setup.sh personalized some tracked files (site-content.ts,
	# app.html, Caddyfile…) and an update also changed one of them. Stash, pull,
	# and reapply.
	warn "Stashing your local changes and retrying…"
	STASHED=1
	git stash push -u -m "ridgeline-update-$(date +%s)" >/dev/null 2>&1 || die "Couldn't stash local changes — resolve manually with 'git status', then re-run."
	git pull --ff-only || die "Pull still failed (local commits diverged?). Resolve manually, then re-run."
	if git stash pop >/dev/null 2>&1; then
		ok "Reapplied your local changes."
	else
		warn "Your customizations conflicted with the update."
		warn "Simplest fix: re-run ${BOLD}./setup.sh${RESET} to regenerate your personalized"
		warn "files from your answers (your config + database are untouched). Then re-run this."
		die "Update paused on a merge conflict."
	fi
fi

AFTER="$(git rev-parse --short HEAD)"
if [[ "$BEFORE" == "$AFTER" ]]; then
	ok "Already up to date ($AFTER)."
else
	ok "Updated $BEFORE → $AFTER."
fi

# --- stamp the version so `-version` reflects the running commit ------------
VER="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
touch deploy/.env
grep -v '^RIDGELINE_VERSION=' deploy/.env > deploy/.env.tmp 2>/dev/null || true
echo "RIDGELINE_VERSION=$VER" >> deploy/.env.tmp
mv deploy/.env.tmp deploy/.env

# --- rebuild + restart ------------------------------------------------------
info "Rebuilding & restarting (${DIM}$COMPOSE${RESET})… first build can take a few minutes."
( cd deploy && docker compose -f "$COMPOSE" up -d --build )

# --- wait for health --------------------------------------------------------
info "Waiting for the daemon to come up…"
for _ in $(seq 1 20); do
	st="$(docker inspect --format '{{.State.Status}}' ridgeline-app 2>/dev/null || echo '?')"
	hs="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' ridgeline-app 2>/dev/null || echo '?')"
	if [[ "$hs" == healthy ]]; then
		ok "${BOLD}Healthy.${RESET} Running $VER."
		exit 0
	fi
	if [[ "$hs" == none && "$st" == running ]]; then
		ok "Running (image has no healthcheck). Version $VER."
		exit 0
	fi
	if [[ "$hs" == unhealthy ]]; then
		die "Container is unhealthy. Check: cd deploy && docker compose -f $COMPOSE logs ridgelined"
	fi
	sleep 3
done
warn "Didn't reach a healthy state in time. Check: ${DIM}cd deploy && docker compose -f $COMPOSE logs ridgelined${RESET}"
