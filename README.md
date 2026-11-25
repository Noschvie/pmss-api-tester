# PMSS API Tester

A Go-based, read-only API explorer and conformance tester for the MQTT Security Service API.

## Run

### Complete test stack

[`compose.yaml`](compose.yaml) starts everything the tester needs in one Compose project:

| Service | Source | Host port | Role |
|---|---|---|---|
| `keycloak` | Keycloak 26.8.2 with realm `pmss-test`, from the submodule | 8180, 8190 (health) | Token issuer |
| `mosquitto` | Mosquitto 2.1.2 with Dynamic Security and test PKI, from the submodule | 127.0.0.1:18883, 127.0.0.1:18884 | Broker |
| `pmss` | Image `ghcr.io/noschvie/phylax-mqtt-security-service:v0.1.0` | 127.0.0.1:18080 | Service under test |
| `pmss-api-tester` | Built from this repository | 127.0.0.1:8080 | Tester |

Keycloak and Mosquitto are included from the integration environment of the Security Service.
The submodule [`stack/pmss`](https://github.com/Noschvie/phylax-mqtt-security-service) pins it to
tag `v0.1.0`, the same release as the service image. The tester keeps no copy of the realm or the
broker configuration.

```sh
git submodule update --init
stack/pmss/test/integration/broker/make-certs.sh stack/pmss/test/integration/broker/run
docker compose up -d --build --wait
```

`make-certs.sh` creates the test PKI and the Dynamic Security state. Pass the directory explicitly:
without an argument the script writes to `./run` relative to the current directory, and the broker
would not find its certificates. The keys are valid for two days; run the script again before the
next start. The generated files are ignored inside the submodule.

Open <http://127.0.0.1:8080> and sign in with a test user of the realm, `alice` or `bob`
(password = user name, see the [realm README](https://github.com/Noschvie/phylax-mqtt-security-service/blob/v0.1.0/test/integration/keycloak/README.md)).
`alice` has `security-admin` and must set up an OTP at the first sign-in. Use `127.0.0.1`, not
`localhost`: by default the Keycloak issuer and the redirect URI of the `pmss-api-tester` client are
`127.0.0.1`. Stop and remove the stack, including the broker data, with:

```sh
docker compose down -v
```

#### Browser on another machine

If the stack runs on a machine without a browser, set the address under which the browser reaches
that machine, in an untracked `.env` file next to `compose.yaml` or in the environment:

```env
STACK_HOST=hadersdorf      # host name or IP of the stack machine, as seen by the browser
STACK_BIND=0.0.0.0         # publish the tester port on all interfaces, not only 127.0.0.1
```

| Variable | Default | Used for |
|---|---|---|
| `STACK_HOST` | `127.0.0.1` | `KC_HOSTNAME` (issuer of all tokens), redirect URIs of client `pmss-api-tester`, issuer of the service and the tester, `OIDC_CALLBACK_URL` |
| `STACK_BIND` | `127.0.0.1` | Host interface of the tester port 8080 |

Then start the stack as above and open `http://<STACK_HOST>:8080` on the other machine, with exactly
that host name (the callback and the session cookie are bound to it). The browser needs ports 8080
(tester) and 8180 (Keycloak sign-in) of the stack machine; Keycloak is published on all interfaces
by the Security Service's Compose file. Mosquitto and the service stay bound to `127.0.0.1`.
`STACK_HOST` only has to resolve on the browser's machine: the tester and the service reach Keycloak
inside the Compose network as `keycloak:8080`.

[`compose.keycloak.yaml`](compose.keycloak.yaml) sets `KC_HOSTNAME` and
`PMSS_API_TESTER_URL` of the included Keycloak service; the Security Service's files stay unchanged.
The redirect URIs are resolved during the realm import, so recreate Keycloak after changing
`STACK_HOST` (`docker compose up -d` does this when the variable changed).

Keycloak serves plain HTTP here. The realm keeps Keycloak's default `sslRequired = external`, which
allows HTTP only from private addresses (for example `192.168.x.x` or `10.x.x.x`). Use the stack only
in an isolated test network: tokens, passwords and OTP codes cross the network unencrypted.

To update to a newer PMSS release, move the submodule and the image tag together:

```sh
git -C stack/pmss fetch --depth 1 origin tag vX.Y.Z
git -C stack/pmss checkout vX.Y.Z
# change the default of PMSS_IMAGE in compose.yaml to :vX.Y.Z
```

`PMSS_IMAGE` overrides the image for a single run, for example to test `:edge`; the Keycloak and
broker configuration still comes from the submodule.

### Tester only, against another target

[`compose.tester.yaml`](compose.tester.yaml) starts only the tester, for example against a PMSS
installation with HTTPS and its own Keycloak. All settings come from an untracked `.env` file; copy
[`.env.example`](.env.example) and set the target, the issuer and the client secret (see
[Keycloak login](#keycloak-login-phase-2) and [API Explorer](#api-explorer-phase-3)):

```sh
cp .env.example .env
docker compose -f compose.tester.yaml up -d --build
```

Stop and remove the container with `docker compose -f compose.tester.yaml down`. To use a different
host port, change the first port in the `ports` mapping in `compose.tester.yaml` (for example,
`8081:8080`). `compose.yaml` and `compose.tester.yaml` use separate Compose projects
(`pmss-api-tester-stack`, `pmss-api-tester`) but publish the same port 8080; stop one before starting
the other.

Alternatively, build and run the image directly with Docker:

```sh
docker build -t pmss-api-tester .
docker run --rm -p 8080:8080 pmss-api-tester
```

Open <http://localhost:8080> (or the configured host port). The application listens on `:8080`
inside the container by default; set `HTTP_ADDR` to change the listen address and adjust the
container port mapping accordingly. `GET /healthz` returns the application's health status.

At startup, the embedded `api/openapi.yaml` is parsed and validated as OpenAPI 3.2.1. Invalid
specifications cause the application to exit instead of serving a partial application.

The UI follows TasmoFleet's design direction: Geist typography, a compact neutral interface, blue
accents, softly rounded surfaces, and light and dark themes. HTMX 2.0.11 and the licensed Geist font
files are served locally from embedded application assets. Version 1 remains strictly read-only.

## Tests

With Go 1.26 or newer, run the Go tests from the repository root:

```sh
go test ./...
```

The suite includes table-driven unit tests and authenticated HTTP integration tests with local mock
OIDC and PMSS servers, including checks that every non-GET OpenAPI operation is blocked.

### Browser tests

With Go 1.26 or newer and Node.js 20 or newer installed, run:

```sh
npm ci
npx playwright install chromium
npm run test:e2e
```

Playwright starts the real application and local OIDC and PMSS test doubles on loopback ports
4173 and 4174. Both ports must be free; existing services are never reused. The app's endpoint,
OIDC, cookie and audience settings are explicitly overwritten for the test process, so no
production Keycloak or PMSS is contacted. The OIDC mock provides a login form, signed ID tokens,
PKCE verification, token exchange and logout, but is not a real Keycloak instance. The tests cover
sign-in, an authenticated GET, checks, logout, a token without `read`, and blocked write operations;
they also assert which requests reached the PMSS mock. The mock users and credentials are only
test fixtures. These tests do not replace testing against a real isolated Keycloak and PMSS.

### Visual regression

The three viewport snapshots use the same local fixtures, Chromium, a 1440 × 900 viewport,
device scale 1, light theme, UTC and English locale. Animations are disabled, fonts are loaded,
and the session expiry, request duration and response headers are masked. Generate and compare
baselines only in the same pinned Linux/amd64 container, not with a locally installed browser.
Docker must be able to access the image registries and Go module proxy when building/running:

```sh
docker build --platform linux/amd64 -f e2e/Dockerfile -t pmss-e2e-visual .
docker run --rm --platform linux/amd64 --ipc=host -v "${PWD}:/work" pmss-e2e-visual
```

To create or deliberately update the reference images, use the same image and command with
`test --project=visual --update-snapshots` after the image name:

```sh
docker run --rm --platform linux/amd64 --ipc=host -v "${PWD}:/work" pmss-e2e-visual node /opt/e2e/node_modules/@playwright/test/cli.js test --project=visual --update-snapshots
```

Review the resulting PNG diffs in `e2e/visual.spec.js-snapshots` before checking them in. A missing
or changed baseline fails the comparison; never update snapshots automatically in CI. Rebuild and
review all references deliberately when changing the pinned Playwright image, browser or fonts.
The initial reference images still need to be generated and reviewed in that container.

## Keycloak login (Phase 2)

The login is optional until OIDC is configured. The test realm `pmss-test` of the Security Service
(`stack/pmss/test/integration/keycloak/realm/pmss-test-realm.json`, pinned by the submodule) defines
a dedicated confidential `pmss-api-tester` client with Standard Flow, PKCE `S256`, Standard Token
Exchange and the optional `read` scope. The scope's audience mapper targets
`phylax-mqtt-security-service`; the tester client cannot request `write`. `config/realm_test.go`
checks these settings in the pinned realm and is skipped when the submodule is not checked out. The
configuration requires Keycloak 26.2 or newer with Standard Token Exchange enabled.

The complete test stack (see [Run](#complete-test-stack)) imports this realm and configures the
tester with the fixed test secret `pmss-api-tester-secret`; nothing else is needed. For another
Keycloak instance, import the realm into a **new test realm**, or create the client with the same
settings in an existing realm. Keycloak's startup realm import skips realms that already exist;
changing the JSON alone does not update a running realm. Do not overwrite an existing realm just to
add this client.

With a client created in another realm, obtain its generated secret under **Clients →
pmss-api-tester → Credentials** in the Keycloak admin console and place it in an untracked `.env`
file. Do not put the secret in source control. The test realm contains test users and credentials
and is intended only for an isolated test instance.

The redirect URI of the realm's client is `${PMSS_API_TESTER_URL}/auth/callback` and its post-logout
redirect URI is `${PMSS_API_TESTER_URL}/`, resolved during the realm import (default
`http://127.0.0.1:8080`). If the tester is accessed at another origin, set `PMSS_API_TESTER_URL` for
Keycloak before the first import and set `OIDC_CALLBACK_URL` accordingly.

For [`compose.tester.yaml`](compose.tester.yaml), set these variables in the environment used to
start Compose (or in an untracked `.env` file); the complete test stack sets them itself:

```env
OIDC_ISSUER_URL=http://127.0.0.1:8180/realms/pmss-test
OIDC_INTERNAL_URL=
OIDC_CLIENT_ID=pmss-api-tester
OIDC_CLIENT_SECRET=<client-secret>
OIDC_CALLBACK_URL=http://127.0.0.1:8080/auth/callback
OIDC_COOKIE_SECURE=false
PMSS_AUDIENCE=phylax-mqtt-security-service
```

`OIDC_ISSUER_URL` must exactly match the `issuer` advertised by Keycloak; the application discovers
the authorization and token endpoints from its OpenID Connect metadata. Leave `OIDC_INTERNAL_URL`
empty when the issuer URL is reachable from the tester container. If Docker needs a different
internal address, set it to the equivalent Keycloak realm URL; only backend requests are rewritten,
while discovery and ID-token issuer validation continue to use the canonical issuer. Open the tester
at the same origin as `OIDC_CALLBACK_URL` so the callback and session cookie use the same host. Set
`OIDC_COOKIE_SECURE=true` when serving the tester over HTTPS.

The complete test stack uses this rewrite: the issuer is `http://<STACK_HOST>:8180/realms/pmss-test`,
and `OIDC_INTERNAL_URL` points to `keycloak:8080` inside its Compose network.

After login, the backend validates the OIDC ID token and exchanges the user's access token using
RFC 8693, requesting only `read` and the configured audience. Access tokens and session data are
kept in memory on the server and are never sent to the browser. The session ends when the login
access token expires or after eight hours, whichever comes first; refreshing requires logging in
again. Logout is a CSRF-protected POST and also redirects through Keycloak's end-session endpoint
when the provider advertises one, with `client_id` and, while the tester still knows the session, the
ID token as `id_token_hint`. If the session already ended on the server (with the access token, or
because the tester was restarted), the logout still clears the cookie and redirects to Keycloak
without `id_token_hint`; Keycloak then asks the user to confirm the logout, so a forged request cannot
end the Keycloak session. Only an active session with a wrong CSRF token is rejected with `403`.
Keycloak's own session outlives the tester's: without the logout, the next sign-in reuses it without
asking for a password. A missing `read` scope is shown as `BLOCKED`, not as a successful
authorization.

To check the integration, sign in with each configured test user and confirm that the tester shows
the user and `read` access, then execute a GET against a configured PMSS endpoint and log out. Both
`alice` (with `security-admin`) and `bob` (without it) should have read access; neither should gain
`write` through this client. The target PMSS service must trust the same issuer and audience.

## API Explorer (Phase 3)

For [`compose.tester.yaml`](compose.tester.yaml), set the target's HTTPS base URL in the environment
or `.env` file; the complete test stack uses `http://pmss:8080` inside its Compose network:

```env
PMSS_API_BASE_URL=https://pmss.example.org
PMSS_ALLOW_INSECURE_HTTP=false
```

The URL may include a base path, but must not include user information, a query, or a fragment.
HTTPS is required by default, with normal certificate verification. For an isolated integration
test environment only, explicitly enable unencrypted HTTP:

```env
PMSS_API_BASE_URL=http://192.0.2.10:18080
PMSS_ALLOW_INSECURE_HTTP=true
```

In this example, `192.0.2.10:18080` stands for the published port of an isolated test PMSS; the
tester container must be able to reach that address. HTTP sends the user's bearer token and
responses unencrypted and does not authenticate the target. Anyone with network access to the
traffic may read, modify, or replay the token; never enable this for production or untrusted
networks. The application logs a warning at startup and displays one in the explorer when using
HTTP. The flag does not disable TLS certificate verification for HTTPS, affect OIDC transport, or
change the read-only restriction. Recreate the container after changing `.env`:

```sh
docker compose -f compose.tester.yaml up --build -d
```

Leave `PMSS_API_BASE_URL` empty to browse the embedded specification without executing calls. Open **API Explorer**
from the navigation to see Core API and PMSS Extra operations separately. GET parameters and schemas
are generated from the embedded OpenAPI
specification. Calls run server-side with the logged-in user's exchanged token and require the
`read` scope. Other HTTP methods are rejected server-side as `BLOCKED`; response cookies and access
tokens are never displayed. JSON responses are checked against the matching OpenAPI response schema;
the result view shows validation status, HTTP status, headers, body, and duration. Response bodies are
limited to 1 MiB in the explorer.

## Automated checks (Phase 4)

Open **Prüfungen** from the navigation to run read-only Core checks against the configured target.
Reachability (`GET /healthz`), authentication with missing and invalid tokens,
the logged-in user's `read` scope, and the authenticated API information/version response are
checked on each run where their prerequisites are met; otherwise the protected checks are marked
`BLOCKED`. Select additional Core GET endpoints and supply any required path or query parameters;
PMSS Extra and all write operations are excluded.

Results are reported as `PASS`, `FAIL`, `NOT TESTED`, or `BLOCKED` and remain in memory only for
the current response. A missing `read` scope blocks protected requests. Response schemas, the
`Cache-Control: no-store` header on `/api/v1/info` and `/api/v1/export`, and strong ETags on
individual client, group, and role responses are checked. Path encoding is verified when a
supplied path value contains characters that need escaping; otherwise that sub-check is
`NOT TESTED`. RFC 9457 details are validated for authentication failures and broker errors.
`503 broker_unavailable` and `504 broker_timeout` are distinct failures; requests are never
retried automatically.

## Acknowledgements

Thanks to [TasmoFleet](https://github.com/hellwand/TasmoFleet) for the design inspiration.

This project is an API explorer and conformance tester for the [Phylax MQTT Security Service](https://github.com/Noschvie/phylax-mqtt-security-service).

## License

[GNU Affero General Public License v3.0 or later](LICENSE) (`AGPL-3.0-or-later`). For modified
versions offered for remote network interaction, see section 13 of the license for the
corresponding-source requirement.
