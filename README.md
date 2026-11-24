# PMSS API Tester

A Go-based, read-only API explorer and conformance tester for the MQTT Security Service API.

## Run

Build and start the Phase 1 prototype with Docker:

```sh
docker build -t pmss-api-tester .
docker run --rm -p 8080:8080 pmss-api-tester
```

Open <http://localhost:8080>. The application listens on `:8080` by default; set `HTTP_ADDR` to
change the listen address. `GET /healthz` returns the application's health status.

At startup, the embedded `api/openapi.yaml` is parsed and validated as OpenAPI 3.2.1. Invalid
specifications cause the application to exit instead of serving a partial application.

The UI follows TasmoFleet's design direction: Geist typography, a compact neutral interface, blue
accents, softly rounded surfaces, and light and dark themes. HTMX 2.0.11 and the licensed Geist font
files are served locally from embedded application assets. Version 1 remains strictly read-only;
authentication and PMSS connectivity are implemented in later phases.

## Acknowledgements

Thanks to [TasmoFleet](https://github.com/Noschvie/TasmoFleet) for the design inspiration. This
project is an API explorer and conformance tester for the
[Phylax MQTT Security Service](https://github.com/Noschvie/phylax-mqtt-security-service).
