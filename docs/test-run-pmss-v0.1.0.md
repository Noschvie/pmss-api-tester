# Test Run – PMSS v0.1.0 on hadersdorf

**Document status:** Test record for Phase 5 of the roadmap
**Date:** 26 November 2026
**Result:** ✅ Passed

Manual test run of the PMSS API Tester against the PMSS reference implementation in the complete
test stack (see README, [Complete test stack](../README.md#complete-test-stack)). This closes the
roadmap item "Documented test run against a PMSS reference implementation"
(`docs/pmss-api-tester-roadmap-v0.2.md`, Phase 5).

## 1. Environment

| Item | Value |
|---|---|
| Stack machine | `hadersdorf` (no browser) |
| Browser | on another VM in the test network |
| Tester | `main` at commit `f7587f2` |
| PMSS | `ghcr.io/noschvie/phylax-mqtt-security-service:v0.1.0`, submodule `stack/pmss` at tag `v0.1.0` |
| Keycloak, Mosquitto | integration environment of PMSS from `stack/pmss` (tag `v0.1.0`), realm `pmss-test` |
| Stack settings | `STACK_HOST=hadersdorf`, `STACK_BIND=0.0.0.0` |
| Test users | `bob` (password only), `alice` (realm role `security-admin`, password and OTP) |

## 2. Procedure and Results

| # | Step | Expected | Result |
|---|---|---|---|
| 1 | Start the stack as described in the README | All services running | ✅ |
| 2 | Open `http://hadersdorf:8080` in the browser on the other VM | Tester overview loads | ✅ |
| 3 | Sign in as `bob` through Keycloak | Login succeeds, session with scope `read` | ✅ |
| 4 | Run all checks under **Checks** as `bob` | All checks OK, no `FAIL` | ✅ |
| 5 | Log out `bob` | Session ended, signed out | ✅ |
| 6 | Sign in as `alice`; set up the OTP, code generated with `scripts/totp.sh` | Login with second factor succeeds, session with scope `read` | ✅ |
| 7 | Run all checks under **Checks** as `alice` | All checks OK, no `FAIL` | ✅ |
| 8 | Log out `alice` | Session ended, signed out | ✅ |
| 9 | Stop the stack | All containers stopped | ✅ |

## 3. Checks Covered

The **Checks** page ran the read-only Core checks of Phase 4 for both users:

- Availability: `GET /healthz`
- Authentication with a missing and an invalid token
- `read` permission of the signed-in user
- `GET /api/v1/info` and API version
- The selected Core GET endpoints with status, headers, JSON structure, `Cache-Control` and ETags

Write operations and PMSS Extra are not part of version 1 and were not tested.

## 4. Findings

None. No deviations from the API specification were reported.

## 5. Not Covered

- authentik as identity provider (no authentik instance available)
- Broker errors `503 broker_unavailable` and `504 broker_timeout` (no fault injection in the stack)
- Write conformance tests (outside version 1)
