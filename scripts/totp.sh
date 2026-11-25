#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (c) 2026 Noschvie
# PMSS API Tester - https://github.com/Noschvie/pmss-api-tester
#
# Prints the current TOTP code for an OTP secret of the test realm, for
# signing in without an authenticator app. TOTP as Keycloak configures it
# by default: SHA-1, six digits, 30 seconds. Test use only.
#
#   scripts/totp.sh              asks for the secret (not echoed, not in the history)
#   scripts/totp.sh -w           as above, prints a new code every 30 seconds until Ctrl+C
#   TOTP_SECRET=... scripts/totp.sh
#
# The secret is shown by Keycloak under "Unable to scan?" when setting up the
# OTP; spaces and lower case are accepted. Needs python3 (3.7 or newer) or,
# as a fallback, oathtool.

set -euo pipefail

usage() {
	sed -n '10,12p' "$0" | sed 's/^#   //'
	exit "${1:-0}"
}

watch=false
while getopts ":wh" opt; do
	case $opt in
	w) watch=true ;;
	h) usage 0 ;;
	*) usage 2 >&2 ;;
	esac
done
shift $((OPTIND - 1))
if [ $# -gt 0 ]; then
	echo "Pass the secret by prompt or TOTP_SECRET, not as an argument (shell history)." >&2
	exit 2
fi

secret=${TOTP_SECRET:-}
if [ -z "$secret" ]; then
	if [ ! -t 0 ]; then
		IFS= read -r secret || true
	else
		read -r -s -p "OTP secret: " secret
		echo >&2
	fi
fi
secret=$(printf '%s' "$secret" | tr -d '[:space:]' | tr '[:lower:]' '[:upper:]')
if [ -z "$secret" ]; then
	echo "No secret given." >&2
	exit 1
fi
if ! printf '%s' "$secret" | grep -Eq '^[A-Z2-7]+=*$'; then
	echo "The secret is not Base32 (letters A-Z and digits 2-7)." >&2
	exit 1
fi

code() {
	if command -v python3 >/dev/null 2>&1; then
		TOTP_SECRET=$secret python3 -c '
import base64, hmac, os, struct, time
s = os.environ["TOTP_SECRET"]
k = base64.b32decode(s + "=" * (-len(s) % 8))
h = hmac.digest(k, struct.pack(">Q", int(time.time()) // 30), "sha1")
o = h[-1] & 15
print("%06d" % ((int.from_bytes(h[o:o + 4], "big") & 0x7fffffff) % 10**6))'
	elif command -v oathtool >/dev/null 2>&1; then
		oathtool --totp -b "$secret"
	else
		echo "Neither python3 nor oathtool found; install one of them." >&2
		exit 1
	fi
}

show() {
	local current left
	current=$(code)
	left=$((30 - $(date +%s) % 30))
	printf '%s  (valid for %2d s)\n' "$current" "$left"
}

if ! $watch; then
	show
	exit 0
fi
while true; do
	show
	sleep $((30 - $(date +%s) % 30))
done
