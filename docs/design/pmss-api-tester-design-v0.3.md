# PMSS API Tester – Design Document

**Version:** 0.3 (draft)
**Date:** 24 November 2026
**Status:** Architectural decisions for version 1; selected operational and Keycloak details remain open

---

## 1. Objective

The PMSS API Tester is a functional API and conformance tester for the MQTT Security Service (PMSS) REST API.

Objectives:

- Enable manual API calls against a PMSS target system.
- Technically validate and display API responses.
- Validate the interface against the integrated OpenAPI specification.
- Execute defined automated test cases.
- Present results in a traceable manner.

The architecture is intended to be generic. The first concrete target implementation is Phylax.

In the long term, the tester should also help external implementers. Version 1 focuses on developing and testing the first PMSS implementation.

### 1.1 Scope Exclusions

The tester is explicitly:

- not an MQTT client,
- not an independent ACL manager,
- not an implementation of PMSS,
- not a production administration interface.

---

## 2. Specification Basis

The PMSS specification consists of a normative Markdown specification and a machine-readable OpenAPI description.

Decisions:

- The Markdown specification is normative.
- The OpenAPI file describes the technical interface.
- In case of conflicts, the Markdown specification takes precedence.
- The OpenAPI file is integrated into the application and cannot be imported through the UI.
- The API version determines the associated OpenAPI version. The specification revision used is documented together with the API version.
- Core conformance and implementation extensions (`x-pmss-extra: true`) are reported separately.
- Functional test rules are maintained independently of the OpenAPI schemas.

The repository contains the normative Markdown specification `docs/mqtt-security-service-api-v1.0.md` (version 1.0, dated 23 November 2026) and the associated `api/openapi.yaml` (OpenAPI 3.2.1, API version 1.0). Both artifacts must be included in the implementation; normative behavior is checked against the Markdown specification.

### 2.1 Conformance Status

A test result uses one of the following statuses:

- `PASS`
- `FAIL`
- `NOT TESTED`
- `BLOCKED`

Cases that were not executed or cannot be meaningfully tested must not be reported as `PASS`.

---

## 3. Version 1 Scope

Version 1 includes:

- manual API calls,
- user-specific Bearer-token authentication through a server-side token exchange according to RFC 8693,
- display of HTTP status, response headers, and response body,
- OpenAPI schema checks,
- simple automated test cases defined in the application,
- display of test results.

Test results are kept in memory only for the duration of the active session. Persistent storage is not planned for version 1.

The following are not part of version 1:

- complex scenario engine,
- CI/CD integration,
- multi-user management in the tester,
- extensive reporting or historical analysis,
- dynamic import of OpenAPI specifications,
- parallel support for multiple API versions,
- automatic test-data management,
- broker-error simulation,
- concurrency and load tests.

---

## 4. Architecture

### 4.1 Core Architecture

The tester consists of a web interface and its own Go backend. The backend serves the UI and executes PMSS API calls.

Defined technology stack:

- Go with `net/http` and `ServeMux`,
- `templ` for HTML components,
- HTMX 2.x for dynamic interaction,
- `htmx-ext-sse` if required,
- CSS and static assets following the TasmoFleet GUI guidelines,
- Playwright for end-to-end tests,
- `pixelmatch` and `pngjs` for visual regression tests.

React and TypeScript are not used.

### 4.2 Product Family and UI

The interface should closely follow the look and feel of TasmoFleet and appear as another module in the same product family.

In particular, the following should be adopted:

- existing GUI guidelines and design tokens,
- existing UI components and design principles,
- light and dark mode,
- the established build and asset-embedding mechanism.

### 4.3 Deployment

- Docker exclusively.
- One Docker container for the application.
- Multi-stage build.
- The final image contains the Go binary and embedded static assets.
- No separate database.
- Runtime data and test results are kept in memory only.

---

## 5. Functional Areas

### 5.1 Configuration and Target System

The API base URL is provided through an environment variable. Overriding it in the UI is not yet defined for version 1; if enabled, it applies only to the active session and must be protected against unauthorized targets.

Planned environment variables:

```env
PMSS_API_BASE_URL=https://pmss.example.org
PMSS_HTTP_TIMEOUT=10s

OIDC_ISSUER_URL=https://keycloak.example.org/realms/Tasmofleet
OIDC_CLIENT_ID=mqtt-security-api-tester
OIDC_CLIENT_SECRET=<secret>

TZ=Europe/Vienna
```

Notes:

- `PMSS_API_BASE_URL` is the HTTPS base URL of the PMSS system under test.
- `PMSS_HTTP_TIMEOUT` configures the HTTP timeout. `10s` is a proposed value and not yet a confirmed limit.
- `OIDC_ISSUER_URL` points to the existing `Tasmofleet` Keycloak realm.
- `OIDC_CLIENT_ID` identifies the tester's own OIDC client.
- `OIDC_CLIENT_SECRET` is a confidential configuration value and must not appear in the UI or logs.
- `TZ` controls the local display of timestamps. UTC applies if it is not specified.

TLS certificate verification is enabled by default.

### 5.2 API Explorer

The API Explorer is generated automatically from the integrated OpenAPI specification.

Functions:

- group endpoints by HTTP method and resource,
- dynamically capture GET parameters and display request bodies based on the OpenAPI specification,
- execute API calls directly from the UI,
- execute only `GET` operations in version 1; treat other methods as `BLOCKED` on the server,
- display HTTP status, headers, and response body,
- validate request and response schemas.

### 5.3 Automated Test Cases

- Test cases are defined within the application.
- The UI displays the available test cases.
- Individual test cases can be executed selectively.
- Test cases cannot be created or edited through the UI in version 1.
- Test cases reference their associated OpenAPI operations but remain functionally independent of the OpenAPI file.

### 5.4 Test Results

A result contains at least:

- test ID and description,
- status,
- expected and actual result,
- details or error description,
- execution duration.

The complete HTTP request and response can be expanded when needed. Confidential information such as tokens and passwords is not disclosed.

### 5.5 Dashboard

The dashboard deliberately remains simple and displays:

- selected target system,
- connection status,
- API and specification version,
- last test execution,
- distribution of test results.

Historical analysis and extensive charts are not planned.

---

## 6. Test Concept

Version 1 focuses on positive and negative API checks.

### 6.1 Test Areas

1. **Authentication and Authorization**
   - valid token issued for the target system,
   - missing token,
   - invalid or expired token,
   - presence of the required `read` scope.

2. **Read-only GET Endpoints**
   - all read-only GET endpoints in the integrated OpenAPI specification,
   - insofar as they can be tested meaningfully and safely without a preceding state change,
   - for example info, export, clients, groups, roles, and individual resources.

3. **Error Handling for Read-only Calls**
   - invalid parameters and non-existent resources in read-only calls,
   - expected 404 behavior,
   - RFC 9457 problem details, especially for authentication and authorization errors.

   Testing request bodies, unknown fields (422), and references requires write calls and is therefore not part of version 1.

4. **ETag Checks**
   - check ETag format and headers in GET responses.
   - do not perform changes using `If-Match`.

5. **Broker Errors**
   - no broker-error simulation in version 1.

### 6.2 Security Boundary for Tests

Version 1 is strictly read-only. Write operations are rejected on the server; merely disabling them in the UI is not sufficient. Negative tests also do not call write PMSS endpoints in version 1. Such tests require a separately approved conformance-test environment and a dedicated security design.

Read-only negative tests, such as a missing or invalid token and a non-existent resource, are permitted provided they do not cause a state change.

Cases that cannot be executed meaningfully or safely receive `NOT TESTED` or `BLOCKED`, never `PASS`.

### 6.3 Test Levels and Unit Tests

Quality assurance follows a layered test strategy:

1. **Unit Tests**
   - test isolated configuration, validation, and functional test logic,
   - use table-driven tests for status evaluation, scopes, headers, ETags, RFC 9457 problem details, and the `PASS`, `FAIL`, `BLOCKED`, and `NOT TESTED` states,
   - use `httptest.Server` and `httptest.ResponseRecorder` for deterministic HTTP scenarios,
   - specifically test the read-only restriction and the separation of Core API and PMSS Extra.

2. **Integration Tests**
   - test the interaction of multiple backend components and complete HTTP processing,
   - use local test servers and reproducible test data where possible,
   - must not require production PMSS targets or mutating calls.

3. **End-to-End Tests**
   - test central user flows including login, the API Explorer, and result presentation,
   - run with Playwright,
   - limit visual regression tests to central UI states.

4. **Reference Tests**
   - test a complete run against a PMSS reference instance explicitly provided for this purpose,
   - run separately from deterministic unit and integration tests,
   - document the target system, version, permissions, and test prerequisites.

Unit tests verify the correct implementation of the tester. The functional test procedures, in contrast, verify the behavior of the target system; neither level replaces the other.

---

## 7. OpenAPI Integration

- The OpenAPI specification is loaded at startup and kept in memory.
- If the specification is invalid or incompatible with the application, the application does not continue in a partially initialized state.
- Request and response schema validation are handled separately.
- A targeted negative test must not be prevented indiscriminately by preliminary request-schema validation.
- Functional test cases are maintained separately from the OpenAPI specification and reference its operations.
- `x-pmss-extra: true` is evaluated so that Core functions and implementation extensions can be displayed and assessed separately.

---

## 8. Security and Privacy

Decisions:

- Access tokens are kept exclusively in server-side memory.
- Tokens, passwords, and secrets are never logged.
- TLS certificate verification is enabled by default.
- The tester operates in read-only mode by default.
- The backend communicates with Keycloak and the configured PMSS target system.
- Logs are emitted as JSON exclusively through Docker stdout/stderr.
- The Phylax Syslog Service is not integrated; a later extension remains possible.
- The time zone is configured through `TZ`. API timestamps remain UTC according to the specification.

Logs must not contain tokens, passwords, or sensitive request or response content.

---

## 9. Authentication and Keycloak

### 9.1 Identity Provider

Keycloak is the central identity provider. The tester has no separate user or password management.

The existing realm is used:

`Tasmofleet`

This also permits reuse of the existing TasmoFleet login theme. No separate realm is created for the PMSS API Tester.

### 9.2 OIDC Client

A dedicated Keycloak client is provided for the tester:

`mqtt-security-api-tester`

The client runs as a server-side web client (confidential client). The client secret is provided exclusively to the Go backend and configured securely.

Login uses the OpenID Connect Authorization Code Flow with PKCE.

### 9.3 Server-side Session and Token Management

The Go backend acts as a backend-for-frontend (BFF):

- The backend starts and processes the OIDC login.
- The backend exchanges the authorization code for tokens.
- The user access token obtained this way is kept exclusively in server-side memory and used as the `subject_token` for the PMSS token exchange.
- The browser receives no access token.
- The browser receives a secure session cookie with `HttpOnly`, `Secure`, and an appropriate `SameSite` setting.
- The backend uses only the token exchanged for PMSS for PMSS calls.

The exact session lifetime, token renewal, and logout behavior remain to be defined. UI actions that modify a session must be protected against CSRF.

### 9.4 User-specific Token Exchange for PMSS Calls

After login, the backend exchanges the user's access token server-side according to RFC 8693 for a token for the PMSS target system. It uses the user token as the `subject_token`, requests the access-token type, and authenticates the tester client with Keycloak. The exchange preserves the user's identity; a service-account token must not replace the user token.

The exchanged token is used server-side for API calls. PMSS checks its scopes; the tester does not substitute locally interpreted user roles for this authorization.

### 9.5 Scopes and Audience

For the PMSS reference implementation, the token exchange requests the audience specified in the specification: `phylax-mqtt-security-service`. For other conforming target systems, the audience must be configurable per target system.

Version 1 requests only the `read` scope and performs no write API calls. The scope actually returned by the token exchange is authoritative; a missing `read` scope is made visible as missing authorization (`BLOCKED`). `write` is not part of the version 1 operating scope.

---

## 10. Open Detailed Decisions

The overall architecture and the read-only scope of version 1 are defined. The following implementation details still need to be specified:

1. **Keycloak client:** redirect URI, client authentication, required permissions for RFC 8693, and secret management.
2. **Scopes and users without `read`:** verify the Keycloak role/scope mapping for PMSS; missing `read` must be displayed as `BLOCKED`.
3. **Token lifecycle:** refresh-token use, expiration handling, and logout.
4. **Session security:** session lifetime, cookie parameters, and concrete CSRF measures.
5. **Target-system configuration:** define the version 1 scope for one or more targets, protection rules for base URLs, and optional session-specific overrides.
6. **HTTP timeout:** verify the proposed `10s` default against the API calls.
7. **Write tests:** outside version 1; before a future conformance test mode, the test environment, approval, test data, and cleanup must be defined separately.

---

## 11. Summary of Architectural Decisions

| Area | Decision |
|---|---|
| Purpose | Functional API and conformance tester |
| First target implementation | Phylax |
| Normative basis | Markdown specification |
| Technical interface description | Integrated OpenAPI specification |
| Backend | Go `net/http` / `ServeMux` |
| UI | `templ` and HTMX 2.x |
| UI design | TasmoFleet GUI guidelines |
| Browser framework | No React |
| Deployment | Docker, one container |
| Build | Multi-stage, Go binary with embedded assets |
| Database | None |
| Test results | In memory only |
| Identity provider | Keycloak |
| Keycloak realm | `Tasmofleet` |
| OIDC client | `mqtt-security-api-tester` |
| OIDC flow | Authorization Code Flow with PKCE |
| Client type | Confidential client, server-side |
| PMSS authentication | RFC 8693 token exchange with user identity |
| PMSS audience (reference implementation) | `phylax-mqtt-security-service` |
| PMSS permission in version 1 | `read` only |
| Operating mode | Read-only |
| Logging | JSON through Docker stdout/stderr |

**Status note:** This document describes architectural decisions and open implementation details, not properties that are already implemented. The normative basis and the OpenAPI description are in the repository at `docs/mqtt-security-service-api-v1.0.md` and `api/openapi.yaml`.
