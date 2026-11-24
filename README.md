# PMSS API Tester

A Go-based, read-only API explorer and conformance tester for the MQTT Security Service API.

## Run

Build and start the application with Docker Compose:

```sh
docker compose up --build -d
```

Stop and remove the container with `docker compose down`. To use a different host port, change
the first port in the `ports` mapping in `compose.yaml` (for example, `8081:8080`).

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

## Keycloak login (Phase 2)

The login is optional until OIDC is configured. For the PMSS integration Keycloak on port 8180,
configure the existing `tasmofleet-test` client for **Standard Flow**, PKCE with `S256`, and the
exact redirect URI `http://localhost:8080/auth/callback` and post-logout redirect URI
`http://localhost:8080/`. Keep the client confidential and enable the token-exchange permission
for the `phylax-mqtt-security-service` audience. The client must be allowed to request the `read`
scope. Do not put its secret in source control.

Set these variables in the environment used to start Compose (or in an untracked `.env` file):

```env
OIDC_ISSUER_URL=http://127.0.0.1:8180/realms/pmss-test
OIDC_INTERNAL_URL=http://keycloak:8080/realms/pmss-test
OIDC_CLIENT_ID=tasmofleet-test
OIDC_CLIENT_SECRET=<client-secret>
OIDC_CALLBACK_URL=http://localhost:8080/auth/callback
OIDC_COOKIE_SECURE=true
PMSS_AUDIENCE=phylax-mqtt-security-service
```

`OIDC_ISSUER_URL` is the canonical issuer advertised by Keycloak. `OIDC_INTERNAL_URL` is only used
for backend-to-Keycloak traffic from Docker; discovery and ID-token issuer validation continue to
use the canonical issuer. Open the tester at `http://localhost:8080` so the callback and session
cookie use the same host. For local HTTP setups where the browser does not accept secure cookies,
set `OIDC_COOKIE_SECURE=false`; use secure cookies behind HTTPS.

The tester container must share the PMSS integration stack's Docker network so it can resolve
`keycloak:8080`. With the upstream integration Compose project name `pmss-it`, connect it after
starting the tester:

```sh
docker network connect pmss-it_default pmss-api-tester
```

Repeat this after recreating the tester container.

After login, the backend validates the OIDC ID token and exchanges the user's access token using
RFC 8693, requesting only `read` and the configured audience. Access tokens and session data are
kept in memory on the server and are never sent to the browser. The session ends when the login
access token expires or after eight hours, whichever comes first; refreshing requires logging in
again. Logout is a CSRF-protected POST and also redirects through Keycloak's end-session endpoint
when the provider advertises one. A missing `read` scope is shown as `BLOCKED`, not as a successful
authorization. PMSS API calls are implemented in Phase 3.

## Acknowledgements

Thanks to [TasmoFleet](https://github.com/Noschvie/TasmoFleet) for the design inspiration. This
project is an API explorer and conformance tester for the
[Phylax MQTT Security Service](https://github.com/Noschvie/phylax-mqtt-security-service).
