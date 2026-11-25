# PMSS API Tester

A Go-based, read-only API explorer and conformance tester for the MQTT Security Service API.

## Run

Build and start the application with Docker Compose:

```sh
# Start the pmss-it integration stack first so its external Docker network exists.
docker compose up --build -d
```

The Compose setup joins the existing `pmss-it_default` network automatically. Stop and remove the
container with `docker compose down`. To use a different host port, change the first port in the
`ports` mapping in `compose.yaml` (for example, `8081:8080`).

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
OIDC and PMSS servers, including checks that every non-GET OpenAPI operation is blocked. It does
not replace a test run against a real Keycloak and PMSS instance, browser or visual regression
tests, or Docker runtime verification.

## Keycloak login (Phase 2)

The login is optional until OIDC is configured. `config/pmss-test-realm.json` defines a dedicated
confidential `pmss-api-tester` client for the test realm, with Standard Flow, PKCE `S256`, Standard
Token Exchange and the optional `read` scope. The scope's audience mapper targets
`phylax-mqtt-security-service`; the tester client cannot request `write`. The configuration requires
Keycloak 26.2 or newer with Standard Token Exchange enabled.

Import the realm into a **new test realm**, or create the client with the same settings in an existing
realm. Keycloak's startup realm import skips realms that already exist; changing the JSON alone does
not update a running realm. Do not overwrite an existing realm just to add this client.

After creating the client, obtain its generated secret under **Clients → pmss-api-tester →
Credentials** in the Keycloak admin console and place it in an untracked `.env` file. Do not put the
secret in source control. The supplied realm contains test users and credentials and is intended
only for an isolated test instance.

The supplied realm's redirect URI is `http://192.168.7.39:8080/auth/callback` and its post-logout
redirect URI is `http://192.168.7.39:8080/`. If the tester is accessed at another origin, update both URIs on the
Keycloak client and set `OIDC_CALLBACK_URL` accordingly.

Set these variables in the environment used to start Compose (or in an untracked `.env` file):

```env
OIDC_ISSUER_URL=http://192.168.7.39:8180/realms/pmss-test
OIDC_INTERNAL_URL=
OIDC_CLIENT_ID=pmss-api-tester
OIDC_CLIENT_SECRET=<client-secret>
OIDC_CALLBACK_URL=http://192.168.7.39:8080/auth/callback
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

When using the integration-stack hostname `keycloak:8080` for `OIDC_INTERNAL_URL`, start the
upstream integration stack before starting this Compose project. The tester then joins its
`pmss-it_default` network automatically, including after the container is recreated.

After login, the backend validates the OIDC ID token and exchanges the user's access token using
RFC 8693, requesting only `read` and the configured audience. Access tokens and session data are
kept in memory on the server and are never sent to the browser. The session ends when the login
access token expires or after eight hours, whichever comes first; refreshing requires logging in
again. Logout is a CSRF-protected POST and also redirects through Keycloak's end-session endpoint
when the provider advertises one. A missing `read` scope is shown as `BLOCKED`, not as a successful
authorization.

To check the integration, sign in with each configured test user and confirm that the tester shows
the user and `read` access, then execute a GET against a configured PMSS endpoint and log out. Both
`alice` (with `security-admin`) and `bob` (without it) should have read access; neither should gain
`write` through this client. The target PMSS service must trust the same issuer and audience.

## API Explorer (Phase 3)

Set the target's HTTPS base URL in the environment or `.env` file:

```env
PMSS_API_BASE_URL=https://pmss.example.org
PMSS_ALLOW_INSECURE_HTTP=false
```

The URL may include a base path, but must not include user information, a query, or a fragment.
HTTPS is required by default, with normal certificate verification. For an isolated integration
test environment only, explicitly enable unencrypted HTTP:

```env
PMSS_API_BASE_URL=http://192.168.7.39:18080
PMSS_ALLOW_INSECURE_HTTP=true
```

In this example, port `18080` is the integration stack's published PMSS port on the Docker host;
the tester container must be able to reach that address. HTTP sends the user's bearer token and
responses unencrypted and does not authenticate the target. Anyone with network access to the
traffic may read, modify, or replay the token; never enable this for production or untrusted
networks. The application logs a warning at startup and displays one in the explorer when using
HTTP. The flag does not disable TLS certificate verification for HTTPS, affect OIDC transport, or
change the read-only restriction. Recreate the container after changing `.env`:

```sh
docker compose up --build -d
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
