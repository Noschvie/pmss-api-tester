#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (c) 2026 Noschvie
# PMSS API Tester - https://github.com/Noschvie/pmss-api-tester
#
# Smoke test of the tester in the complete test stack (compose.yaml):
# tester, PMSS release image and Keycloak with the test realm. Runs
# anonymously, without a browser; it does not sign in.
#
#   docker compose up -d --build --wait
#   scripts/stack-smoke.sh [TESTER_URL] [KEYCLOAK_URL]
#
# Defaults: http://127.0.0.1:8080 and http://127.0.0.1:8180 (STACK_HOST).
set -eu

TESTER=${1:-http://127.0.0.1:8080}
KEYCLOAK=${2:-http://127.0.0.1:8180}
ISSUER="$KEYCLOAK/realms/pmss-test"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

fail() { echo "FAIL: $*" >&2; exit 1; }
ok() { echo "ok:   $*"; }

# The tester has no healthcheck in compose.yaml; wait for it here.
i=0
until curl -sf "$TESTER/healthz" >/dev/null; do
	i=$((i + 1))
	[ "$i" -ge 60 ] && fail "tester not reachable at $TESTER/healthz"
	sleep 2
done
ok "tester /healthz"

curl -sf "$TESTER/" -o "$TMP/index.html" || fail "GET / failed"
grep -q "PMSS API Tester" "$TMP/index.html" || fail "GET / does not render the overview"
ok "tester overview page"

# Anonymous checks against the real PMSS: availability, missing and invalid
# token must pass; everything that needs a session is BLOCKED, nothing FAILs.
code=$(curl -s -o "$TMP/checks.html" -w '%{http_code}' -X POST "$TESTER/checks/run")
[ "$code" = 200 ] || fail "POST /checks/run returned HTTP $code"
# One line per result: the page renders all results on one line.
sed 's/<article class="check-result/\
&/g' "$TMP/checks.html" | grep '^<article class="check-result' >"$TMP/results" ||
	fail "POST /checks/run returned no check results"
for name in "Availability · GET /healthz" "Authentication · Missing token" "Authentication · Invalid token"; do
	grep -F "<strong>$name</strong>" "$TMP/results" | grep -q 'check-pass' ||
		{ grep -F "$name" "$TMP/results" >&2 || true; fail "check \"$name\" did not pass"; }
	ok "check $name"
done
if grep -q 'check-result check-fail' "$TMP/results"; then
	grep 'check-result check-fail' "$TMP/results" >&2
	fail "anonymous checks contain FAIL results"
fi
ok "no failed anonymous checks"

# Sign-in starts at the realm of the stack (OIDC discovery over the
# internal Keycloak URL, redirect to the public issuer).
location=$(curl -s -o /dev/null -w '%{redirect_url}' "$TESTER/auth/login")
case "$location" in
	"$ISSUER"/protocol/openid-connect/auth\?*) ok "sign-in redirects to $ISSUER" ;;
	*) fail "sign-in redirect is \"$location\", expected $ISSUER/protocol/openid-connect/auth?..." ;;
esac
case "$location" in
	*code_challenge=*) ok "sign-in uses PKCE" ;;
	*) fail "sign-in redirect has no code_challenge" ;;
esac

echo "Stack smoke test passed."
