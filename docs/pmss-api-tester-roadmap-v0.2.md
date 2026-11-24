# PMSS API Tester – Planning and Implementation

**Document status:** Implementation roadmap for version 1
**Version:** 0.2
**Date:** 24 November 2026

The architectural decisions follow `docs/design/pmss-api-tester-design-v0.3.md`. In case of deviations, the design document is authoritative; the MQTT Security Service API specification remains the normative API basis.

## 1. Objective

The PMSS API Tester is a generic tool for exploring, checking, and testing implementations of the **MQTT Security Service API**.

The normative basis is `docs/mqtt-security-service-api-v1.0.md`. The file `api/openapi.yaml` provides the machine-readable description of the same API. The Markdown specification remains authoritative if descriptions conflict. Both files are part of the tester repository; the OpenAPI file is embedded and is not loaded from an external URL at runtime.

The Phylax MQTT Security Service (PMSS) is the first test target. However, the tester's architecture must not depend on PMSS-internal implementation details.

### Non-goals for the First Prototype

- No MQTT client or direct MQTT communication
- No management of the broker Dynamic Security state by the tester
- No production administration interface
- No database or persistent test history
- No load tests or CI/CD integration in the first step

## 2. Core Architectural Decisions

| Area | Decision |
|---|---|
| Backend | Go with `net/http` / `ServeMux` |
| UI | `templ`, HTMX 2.x, and embedded static assets |
| UI style | Same technical basis and design conventions as TasmoFleet |
| Delivery | One Docker container, multi-stage build |
| Persistence | No database; runtime data and test results initially kept in memory |
| API description | OpenAPI 3.2.1 as the machine-readable basis; normative Markdown specification takes precedence |
| Test target | Configurable HTTPS base URL of an MQTT Security Service API, initially PMSS |

The tester should fit visually and technically into the TasmoFleet product family. React and TypeScript are not planned for this project.

## 3. Authentication and Authorization

### 3.1 Login to the Tester

- Login through the existing `Tasmofleet` Keycloak realm.
- The tester receives its own Keycloak client configuration.
- Browser login uses the Authorization Code Flow with PKCE.
- The tester operates as a backend-for-frontend (BFF).
- Tokens remain server-side; the browser receives only a secure session cookie.
- Client secrets and tokens must not be delivered to the browser.

### 3.2 Access to the MQTT Security Service

The identity of the logged-in user is preserved for calls to the MQTT Security Service.

The tester therefore uses **no service account as a substitute for the user**. Instead, the backend performs a server-side OAuth 2.0 token exchange according to RFC 8693:

1. The user logs in to the tester through Keycloak.
2. The backend uses the logged-in user's access token as the `subject_token`.
3. During the token exchange, the backend requests an access token for the target system's audience. For PMSS, this is `phylax-mqtt-security-service`.
4. Version 1 requests only the `read` scope.
5. The resulting user-specific token is used server-side for read-only API calls. A service-account token does not replace the user's identity.

For additional conforming target systems, the audience must be configurable per target system. The client is authenticated with Keycloak server-side.

### 3.3 Permissions and UI Behavior

- `read` is required for read-only API calls.
- The scopes actually contained in the token are authoritative.
- If `read` is missing, protected checks are reported as `BLOCKED` and not obscured as a technical error.
- Version 1 does not request `write` and performs no mutating API calls.

## 4. Handling the API Specification

### 4.1 Normative Source and OpenAPI

The tester repository contains the two API artifacts:

- `docs/mqtt-security-service-api-v1.0.md` – normative specification
- `api/openapi.yaml` – machine-readable OpenAPI description

The OpenAPI file is embedded in the application and validated at startup. If the specification is invalid or incompatible, the application does not start partially. OpenAPI is used to describe operations, GET parameters, and schemas. Normative behavior that cannot be fully expressed there is implemented and tested against the Markdown specification.

### 4.2 Core API and PMSS Extra

The specification distinguishes between:

- **Core API:** part of the standardized interface and basis for conformance testing.
- **PMSS Extra:** implementation-specific extensions marked with the `pmss-extra` tag and `x-pmss-extra: true`.

These areas must be displayed and evaluated separately in the tester. An unsupported PMSS Extra endpoint must not be counted as a Core conformance error.

### 4.3 Version Checks

The tester displays at least:

- API version
- specification version
- target system / implementation
- reachability and authentication status

Deviations between expected and reported versions are visibly indicated.

## 5. Tester Features

### 5.1 API Explorer

The API Explorer allows users to call API operations selectively and inspect the results.

At minimum, it displays:

- HTTP method and path
- GET parameters and request/response schemas
- authentication and scopes used
- HTTP status
- response headers
- response body
- call duration

Version 1 performs GET calls exclusively. Other HTTP methods must be rejected server-side and reported as `BLOCKED`; merely hiding or disabling them in the UI is insufficient. Relevant response headers such as `ETag`, `Content-Type`, and Cache-Control are displayed. `If-Match` is not used to perform changes in version 1.

### 5.2 Automated Checks

The tester executes defined checks and evaluates them as:

- `PASS` – check successful
- `FAIL` – check failed
- `NOT TESTED` – check not executed
- `BLOCKED` – check could not be executed because of a prerequisite or missing permission

Core conformance and PMSS Extras are evaluated separately.

### 5.3 Error Handling

The checks specifically account for:

- error responses according to RFC 9457 (`application/problem+json`)
- normative evaluation of HTTP status and `code`
- unknown error codes within the same major API version are not automatically treated as errors when the status represents the expected behavior
- distinction between `503 broker_unavailable` and `504 broker_timeout`

`503 broker_unavailable` and `504 broker_timeout` are displayed distinctly. Version 1 does not retry calls automatically. For later mutating tests, a `504 broker_timeout` requires reading the state again; never retry blindly.

## 6. Security and Test Modes

### 6.1 Version 1 – Strictly Read-only

Version 1 permits read-only calls exclusively:

- `GET /healthz`
- `GET /api/v1/info`
- export and read-only individual queries
- authentication and `read` permission checks
- checks of response format, headers, and schemas

Write operations are blocked server-side. Negative tests that would call a write endpoint are also excluded. Read-only negative tests, such as a missing or invalid token or a non-existent resource, are permitted. Checks that cannot be executed receive `NOT TESTED` or `BLOCKED`, never `PASS`.

### 6.2 Write Conformance Tests – Outside Version 1

Write checks are not part of version 1. A future conformance test mode requires a dedicated security design before implementation and may run only in an explicitly approved test environment. At minimum, the following must be defined:

- explicit activation and confirmation by the user
- clear identification of the target system
- technical protection against production targets
- use of clearly identified test objects
- cleanup after tests
- handling of partial failures and unknown state

Write tests are not enabled implicitly or made accessible from the version 1 interface.

## 7. Planned Test Areas

The following list defines the planned scope. The exact mapping to individual API operations will be created during implementation based on the v1.0 specification and `api/openapi.yaml`.

| Test area | Content | Priority |
|---|---|---|
| Reachability | `/healthz`, connection, and timeout | Prototype |
| API information | `/api/v1/info`, API and specification version, limits | Prototype |
| Authentication | missing, invalid, and valid token | Prototype |
| Authorization | presence of `read`; missing scope as `BLOCKED` | Prototype |
| Read-only Core endpoints | export and available GET operations | Prototype |
| Response conformance | status, headers, JSON structure, and problem details | Prototype |
| ETag | presence and format in read-only responses | Version 1 |
| Path encoding | URL-encoded path parameters in GET calls | Version 1 |
| PMSS Extra | separate, optional check of read-only extensions | Version 1 / optional |
| Validation | unknown fields (`422`), invalid write values, and request limits (`413`) | Later / write test mode |
| If-Match | missing header (`428`), incorrect ETag (`412`), valid ETag | Later / write test mode |
| Password behavior | no return of submitted passwords | Later / write test mode |
| Relationships | group memberships and role/ACL assignments through changes | Later / write test mode |
| Broker errors | display of `503` and `504`; no simulation or automatic retry | Later / controlled environment |

## 8. Implementation Steps

### Phase 1 – Project Foundation

- [x] Create a new Go project and repository structure
- [x] Set up `net/http` / `ServeMux` as the HTTP foundation
- [x] Integrate `templ` and HTMX
- [x] Create a TasmoFleet-style layout and navigation structure
- [x] Embed static assets in the binary
- [x] Create the Docker multi-stage build
- [x] Implement the tester's own health endpoint and basic configuration
- [x] Validate the embedded OpenAPI specification at startup and fail fast on errors

**Result:** Runnable Docker prototype with a basic UI foundation, validated startup configuration, and its own health endpoint.

### Phase 2 – Keycloak Login

- [x] Document Keycloak client requirements (Standard Flow, PKCE, redirect URI, token-exchange permission)
- [x] Implement the Authorization Code Flow with PKCE
- [x] Set up an in-memory BFF session and secure cookies
- [x] Keep tokens exclusively server-side
- [x] Handle logout, session expiration, and CSRF protection for state-changing UI actions
- [x] Implement server-side token exchange according to RFC 8693
- [x] Make the audience configurable per target system (PMSS: `phylax-mqtt-security-service`)
- [x] Request only `read` and evaluate the returned scopes
- [x] Display token-exchange error cases clearly

**Result:** The application supports a user-specific server-side login and `read` token exchange when its Keycloak client is configured as described in the README.

### Phase 3 – API Explorer

- [x] Configure the PMSS target through an HTTPS base URL
- [x] Load the embedded OpenAPI file and allow only read-only operations to be executed
- [x] Mark Core API and PMSS Extra separately
- [x] Execute GET calls server-side and reject other methods as `BLOCKED`
- [x] Capture GET parameters and display request/response schemas
- [x] Display status, headers, body, and duration
- [x] Display error responses and missing `read` permission clearly

**Result:** Manual, traceable, strictly read-only API calls against PMSS. Calls use the configured
HTTPS base URL and the logged-in user's server-side token; non-GET operations are blocked.
Declared parameter and JSON response schemas are validated before display.

### Phase 4 – Initial Automated Checks

- [x] Check reachability and `/healthz`
- [x] Check `/api/v1/info` and version information
- [x] Test authentication
- [x] Check `read` permission; report missing `read` as `BLOCKED`
- [x] Execute selected GET endpoints
- [x] Check response status, headers, and JSON structure
- [x] Evaluate RFC 9457 problem details and distinguish `503`/`504`
- [x] Check ETag headers and URL-encoded GET path parameters
- [x] Display results as `PASS`, `FAIL`, `NOT TESTED`, and `BLOCKED`

**Result:** The `/checks` page runs repeatable read-only Core checks, reports results without
persistent history, and never retries broker failures automatically. Path-encoding checks are
`NOT TESTED` when supplied path values contain no characters requiring encoding.

### Phase 5 – Quality Assurance and Delivery

- [ ] Unit tests for configuration, OpenAPI validation, and HTTP handlers
- [ ] Table-driven unit tests for status evaluation, scopes, headers, ETags, RFC 9457 problem details, and `PASS`/`FAIL`/`BLOCKED`/`NOT TESTED`
- [ ] Cover HTTP scenarios with `httptest.Server` and `httptest.ResponseRecorder`
- [ ] Test the read-only restriction and Core/PMSS Extra separation
- [ ] Integration tests for complete backend and HTTP processing
- [ ] Playwright E2E tests for login, the API Explorer, and central workflows
- [ ] Limited visual regression tests following TasmoFleet conventions
- [ ] Docker build and runtime test
- [ ] Documented test run against a PMSS reference implementation

**Result:** A reproducibly buildable and testable release of the API Tester.

### Outlook – Later Extensions (Not Version 1)

- [ ] Create a dedicated security design and explicit approval for a conformance test mode
- [ ] Define target protection, test objects, cleanup, and handling of unknown states
- [ ] Implement write tests for `If-Match`, validation, request limits, passwords, and relationships
- [ ] Expand PMSS Extra checks independently of Core conformance

**Result:** Only after separate approval: controlled extension beyond the version 1 read-only scope.

## 9. Open Decisions

Before or during the relevant phase, the following points still need to be defined:

1. **Keycloak client:** redirect URIs, client authentication, token-exchange permissions, and secret management.
2. **Scopes and roles:** verify the mapping in Keycloak; the `read` scope returned is authoritative, and missing `read` is reported as `BLOCKED`.
3. **Target-system configuration:** one or more targets, configuration storage, and base-URL protection; UI overrides are not defined for version 1.
4. **Timeouts:** verify the proposed `10s` default and separate handling of connection and response timeouts.
5. **Session:** define lifetime, refresh behavior, and logout.
6. **Result export:** decide whether results should later be exportable as JSON or Markdown.
7. **Later write tests:** define security design, activation, test objects, and cleanup before a future conformance test mode.

Open decisions are not replaced with assumptions; they are resolved before the affected implementation phase.

## 10. Acceptance Criteria for the First Prototype

The first prototype is considered complete when:

- the tester starts as a single Docker container,
- the interface adopts the TasmoFleet design,
- a user can log in through Keycloak,
- the backend can perform a token exchange preserving user identity,
- only read-only PMSS calls are possible and other methods are rejected server-side as `BLOCKED`,
- the PMSS token exchange uses `read` and the audience `phylax-mqtt-security-service`,
- a configured PMSS target is reachable,
- selected GET endpoints can be called through the API Explorer,
- the embedded OpenAPI specification is validated at startup,
- HTTP status, headers, and response body are visible,
- initial authentication, authorization, and response checks can be executed,
- test results are traceably shown as `PASS`, `FAIL`, `NOT TESTED`, or `BLOCKED`,
- the existing unit and integration tests can be run successfully,
- mutating calls are not executed even when directly requested from the backend.

---

**Guideline:** Version 1 remains strictly read-only: first a secure, user-specific API Explorer, followed by automated read-only Core checks. Write conformance tests are later, separately approved extensions and are not part of the version 1 acceptance.
