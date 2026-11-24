# MQTT Security Service API v1
## Schnittstellenspezifikation zwischen TasmoFleet und einem externen MQTT Security Service

**Status:** Zur Durchsicht durch die TasmoFleet-Maintainer  
**Version:** 1.0  
**Datum:** 23. November 2026  
**Grundlage:** Architektur-Überblick (`mqtt-security-overview`) und PMSS-Detaildokument (`phylax-mqtt-security-service-design`), jeweils in der aktuellen Fassung  
**Referenz-Implementierung:** Phylax MQTT Security Service (PMSS), https://github.com/Noschvie/phylax-mqtt-security-service  
**Maschinenlesbare Fassung:** `api/openapi.yaml` im PMSS-Repository (OpenAPI 3.2.1, Abschnitt 7.1)  
**Broker:** Eclipse Mosquitto ab 2.1 mit Dynamic Security Plugin (DynSec)

> **Zweck**
> Dieses Dokument beschreibt, was TasmoFleet von einem externen MQTT Security Service erwartet und was es selbst tun muss. Es beantwortet Overview-Frage 16: TasmoFleet hängt nur von dieser Spezifikation ab, nicht von PMSS. PMSS ist eine Implementierung davon.

> **Kennzeichnung**
> - ✅ = im PMSS-Dokument oder in der Overview entschieden (D-…, SEC-…, Abschnittsverweise „PMSS x.y“)
> - ✅ S-… = in dieser Spezifikation entschieden; Übersicht in Abschnitt 8

## Einleitung

**Ausgangslage.** Viele Tasmota-Geräte und Anwendungen teilen sich heute am Mosquitto-Broker einen gemeinsamen MQTT-Benutzer. Damit kann jeder Client die Topics aller anderen lesen und beschreiben: Er kann fremde Geräte schalten, ihnen über `cmnd/…` neue Zugangsdaten setzen oder über `stat/<topic>/FILEDOWNLOAD` ihre Konfigurations-Backups mitlesen. Eine Trennung nach Client-ID hilft dagegen nicht, weil die Client-ID nicht authentifiziert wird.

**Ziel.** Jedes Gerät und jede Anwendung erhält eigene Zugangsdaten und nur die Rechte, die es braucht. Technische Grundlage ist das Dynamic Security Plugin von Mosquitto: Clients, Gruppen, Rollen und ACLs lassen sich damit zur Laufzeit ändern, ohne Neustart des Brokers.

**Aufgabenteilung.** TasmoFleet kennt die Geräte: Topics, GroupTopics, FallbackTopic, MAC und den Weg, neue Zugangsdaten per `Backlog` auf ein Gerät zu bringen. Es soll deshalb die Oberfläche bieten, aus diesen Daten die nötigen Rechte ableiten und Änderungen zur Freigabe vorlegen. Die Änderungen am Broker selbst führt ein eigener Dienst aus, der MQTT Security Service. Er ist der einzige Teilnehmer mit Admin-Rechten am Broker, meldet sich dort nur per Zertifikat an und hat keinen Zugriff auf die Nutzdaten. TasmoFleet bleibt ein normaler MQTT-Client und braucht keine Broker-Admin-Rechte.

**Warum eine Spezifikation.** TasmoFleet ist ein Upstream-Projekt, der Security Service ein eigenes Projekt. Damit TasmoFleet nicht von einer bestimmten Implementierung abhängt, beschreibt dieses Dokument nur die Schnittstelle zwischen beiden: eine REST-API über HTTPS, abgesichert mit Tokens von Keycloak. Der Phylax MQTT Security Service (PMSS) ist die Referenz-Implementierung; eine andere Implementierung derselben Spezifikation wäre für TasmoFleet gleichwertig.

**Inhalt.** Die Spezifikation legt fest:

- wie TasmoFleet sich gegenüber dem Security Service ausweist (Token Exchange, Scopes `read`/`write`),
- welche Endpunkte TasmoFleet nutzt, mit Request und Response als JSON-Beispiel,
- wie gleichzeitige Änderungen erkannt werden (`ETag`/`If-Match`),
- wie Passwörter erzeugt und einmalig übergeben werden,
- welche Fehler auftreten können und wie TasmoFleet darauf reagiert,
- welche Aufgaben TasmoFleet selbst übernimmt (Soll-Zustand, Diff, Freigabe, Audit der Freigaben).

**Nicht Gegenstand** dieses Dokuments sind der Betrieb des Brokers, die Listener- und Zertifikatskonfiguration, die interne Anbindung des Security Service an den Broker und die Migration der Bestandsgeräte. Diese Themen beschreiben der Architektur-Überblick (`mqtt-security-overview`) und das PMSS-Detaildokument (`phylax-mqtt-security-service-design`).

**Zielgruppe** sind die Maintainer von TasmoFleet und alle, die den Security Service implementieren oder testen.

## 1. Grundsätze

- **Nur Mosquitto mit Dynamic Security.** Die Spezifikation verwendet bewusst die Begriffe und Feldnamen von DynSec (`clients`, `groups`, `roles`, `acls`, `acltype`, `rolename` …). Es gibt kein eigenes Abstraktionsmodell (Entscheidung zu Frage 16, Variante A). ✅ D-26
- **TasmoFleet ist nie DynSec-Admin.** Alle Änderungen an Zugangsdaten und Rechten laufen über diese API. ✅ D-27
- **Der Service hält keinen Zustand.** Soll-Zustand, Diff, Freigabe, Drift-Report und Sicherung liegen in TasmoFleet. Der Service liefert den Ist-Zustand über `GET /api/v1/export`. ✅ D-43
- **Nur verwaltete Objekte sind änderbar.** Rollen und Gruppen mit Präfix `tf_`, Clients mit `managed-by: pmss` in `textdescription`. Alles andere ist lesbar, aber nicht änderbar (`403`). ✅ D-39
- **Feldnamen:** Felder aus DynSec behalten ihren DynSec-Namen (Kleinschreibung, z. B. `textname`, `rolename`). Eigene Felder des Service sind in camelCase (z. B. `etag`, `managed`, `authType`). ✅ S-8

### 1.1 Blockschaltbild

```text
┌──────────────────┐ (1) Anmeldung (OIDC) ┌───────────────────────────────────────────┐
│ Browser          │─────────────────────►│ Keycloak                                  │
│ security-admin   │                      │ Rollen operator / admin / security-admin  │
└────────┬─────────┘                      │ Scopes read / write, zweiter Faktor       │
         │ Oberfläche                     └──────▲─────────────────────────┬──────────┘
         ▼                                       │ (2) Token Exchange      │ (3) JWKS
┌────────────────────────────────────────────────┴─────┐                   │
│ TasmoFleet                                           │                   │
│  Security Manager: Soll-Zustand, Diff, Freigabe,     │                   │
│  Drift-Report, Audit der Freigaben                   │                   │
│  Security-Service-Client: Token, ETag/If-Match       │                   │
└───────┬───────────────────────────────┬──────────────┘                   │
        │                               │                                  │
        │ (6) MQTT als                  │ (4) HTTPS, Bearer-Token, JSON    │
        │     normaler Client           │ ★ Schnittstellenspezifikation    │
        │     (Backlog an Geräte)       ▼                                  │
        │                       ┌──────────────────────────────────────┐   │
        │                       │ NPM (TLS-Terminierung)               │   │
        │                       └───────┬──────────────────────────────┘   │
        │                               ▼                                  │
        │                       ┌──────────────────────────────────────┐   │
        │                       │ MQTT Security Service (PMSS)         │◄──┘
        │                       │ Token-Prüfung, Validierung, ETag,    │
        │                       │ Ausgleichsweg, Audit-Log             │
        │                       └───────┬──────────────────────────────┘
        │                               │ (5) MQTT 5, mTLS :8884, Admin-Zertifikat
        │                               ▼     $CONTROL/dynamic-security/v1
        │                       ┌──────────────────────────────────────┐
        └──────────────────────►│ Mosquitto 2.1 + Dynamic Security     │
          :1883 / :8883         └───────▲──────────────────────────────┘
                                        │ (7) :1883 Passwort
                                        │     :8883 TLS + Passwort
                                        │     :8885 App-Zertifikat
                                ┌───────┴──────────────────────────────┐
                                │ Tasmota-Geräte und Anwendungen       │
                                └──────────────────────────────────────┘
```

| Nr. | Verbindung | Protokoll / Absicherung | In dieser Spezifikation |
|---|---|---|---|
| 1 | Browser → Keycloak (über TasmoFleet) | OIDC-Anmeldung, zweiter Faktor für `security-admin` | – |
| 2 | TasmoFleet → Keycloak | Standard Token Exchange (RFC 8693), `aud` = Security Service | ✅ Abschnitt 2.2 |
| 3 | Keycloak → Security Service | JWKS (öffentliche Schlüssel zur Token-Prüfung) | Abschnitt 2.1 |
| 4 | TasmoFleet → NPM → Security Service | HTTPS, Bearer-Token, JSON | ✅ **Kern dieser Spezifikation**, Abschnitte 3 und 4 |
| 5 | Security Service → Mosquitto | MQTT 5 über mTLS, Port 8884, Admin-Zertifikat, DynSec Control API | – (PMSS-Dokument 7.2) |
| 6 | TasmoFleet → Mosquitto | MQTT als normaler Client (1883/8883), u. a. `Backlog MqttUser …; MqttPassword …` an Geräte | – (Overview 6.2) |
| 7 | Geräte und Anwendungen → Mosquitto | 1883 Passwort, 8883 TLS + Passwort, 8885 App-Zertifikat | – (Overview 7.1) |

TasmoFleet kennt nur die Verbindungen 1, 2, 4 und 6. Ob hinter (4) PMSS oder eine andere Implementierung dieser Spezifikation steht, ist für TasmoFleet unerheblich. Der Broker-Admin-Zugang (5) liegt vollständig im Security Service.

## 2. Authentifizierung und Autorisierung

### 2.1 Token

- Jeder Aufruf außer `/healthz` trägt `Authorization: Bearer <JWT>`. ✅
- Aussteller ist ausschließlich Keycloak. Der Service prüft Signatur (JWKS), `iss`, `aud`, `exp`/`nbf` und den Algorithmus. ✅ SEC-15
- Scopes: `read` für alle `GET`, `write` für alle ändernden Aufrufe. `write` schließt `read` nicht ein. ✅
- Keycloak vergibt `write` nur an Benutzer mit der Rolle `security-admin`. Der Service prüft keine Rollen. ✅ D-48

### 2.2 Token Exchange in TasmoFleet

TasmoFleet ruft den Service nie mit seinem eigenen Token und nie mit einem Service-Account mit `write` auf, sondern tauscht das Token des angemeldeten Benutzers (RFC 8693, Keycloak ab 26.2). ✅ D-31, SEC-34

```bash
curl -s -u tasmofleet:<client-secret> \
  -d grant_type=urn:ietf:params:oauth:grant-type:token-exchange \
  -d subject_token=<access-token-des-benutzers> \
  -d subject_token_type=urn:ietf:params:oauth:token-type:access_token \
  -d requested_token_type=urn:ietf:params:oauth:token-type:access_token \
  -d audience=<client-id-des-security-service> \
  -d scope="read write" \
  https://<keycloak>/realms/<realm>/protocol/openid-connect/token
```

Antwort von Keycloak (gekürzt):

```json
{
  "access_token": "eyJhbGciOiJSUzI1NiIsImtpZCI6...",
  "token_type": "Bearer",
  "expires_in": 300,
  "scope": "read write",
  "issued_token_type": "urn:ietf:params:oauth:token-type:access_token"
}
```

> **Wichtig: Fehlendes `write` ist kein Fehler**
> Für einen Benutzer ohne `security-admin` antwortet Keycloak mit `200` und `"scope": "read"`. TasmoFleet muss `scope` auswerten und ändernde Funktionen dann nicht anbieten. ✅ Getestet mit Keycloak 26.8.2 (PMSS 7.9)

Konfiguration in TasmoFleet ✅ S-12:

| Einstellung | Beispiel |
|---|---|
| Basis-URL des Service | `https://pmss.example.org` |
| Audience für den Token Exchange | `phylax-mqtt-security-service` |
| Scope-Namen | `read`, `write` |
| Kennzeichnung verwalteter Objekte, Präfixe, Grenzen | nicht konfiguriert, sondern aus `GET /api/v1/info` (Abschnitt 4.1) |

## 3. Gemeinsame Regeln

### 3.1 Formate und Header

| Thema | Regel | |
|---|---|---|
| Content-Type | `application/json; charset=utf-8` | ✅ |
| Unbekannte Felder im Request | `422` (`DisallowUnknownFields`) | ✅ |
| Body-Größe | mindestens 1 MiB; tatsächliche Grenze in `GET /api/v1/info` (`limits.maxRequestBytes`), darüber `413 payload_too_large` | ✅ S-11 |
| Fehler | `application/problem+json` nach RFC 9457 (Abschnitt 3.4) | ✅ S-1 |
| Zeitstempel | RFC 3339, UTC, z. B. `2026-11-20T21:30:12Z` | ✅ S-12 |
| Pfadparameter | URL-kodiert (RFC 3986); `CN=homeassistant,O=mqtt-apps` wird zu `CN%3Dhomeassistant%2CO%3Dmqtt-apps` | ✅ S-12 |

### 3.2 `ETag` und `If-Match`

✅ D-46, SEC-31

- Jede Antwort mit einem einzelnen Client, einer Gruppe oder einer Rolle trägt einen `ETag`-Header: einen Hash über die Felder, die dem Objekt gehören (Tabelle unten), ohne Passwort-Hash.
- `GET /api/v1/export` liefert das `etag` je Objekt im JSON-Body.
- Jeder ändernde Aufruf auf ein **bestehendes** Objekt verlangt `If-Match` mit diesem Wert:
  - Abweichung → `412`, nichts wird geändert.
  - Fehlt `If-Match` → `428`.
- Anlegen (`POST /api/v1/clients`, `/api/v1/groups`, `/api/v1/roles`) braucht kein `If-Match`.

**Eigentümer von Beziehungen** ✅ S-4: Eine Zuordnung zwischen Client und Gruppe steht in DynSec bei beiden Objekten. Damit jede Änderung genau ein ETag ändert, gehört jede Beziehung genau einem Objekt. Nur dessen ETag enthält sie.

| Objekt | Felder im ETag | Angezeigt, aber nicht im ETag |
|---|---|---|
| Client | `username`, `textname`, `textdescription`, `disabled`, `roles` | `groups`, `authType` |
| Gruppe | `groupname`, `textname`, `roles`, `clients` | – |
| Rolle | `rolename`, `textname`, `acls` | – |

`If-Match` ist immer das ETag des Objekts im Pfad, und nur dieses ETag ändert sich:

| Aufruf | `If-Match` von | Neues ETag von |
|---|---|---|
| `POST/DELETE /api/v1/clients/{username}/roles…` | Client | Client |
| `POST/DELETE /api/v1/groups/{group}/clients…` | Gruppe | Gruppe |
| `POST/DELETE /api/v1/groups/{group}/roles…` | Gruppe | Gruppe |
| `PUT /api/v1/roles/{role}/acls` | Rolle | Rolle |

Für TasmoFleet heißt das: Ob ein Client in einer Gruppe ist, wird über die Gruppe verglichen (`groups[].clients`), nicht über `clients[].groups`. Nach einer Gruppen-Zuordnung bleibt das ETag des Clients gültig.

### 3.3 Passwörter

✅ D-41, D-42, SEC-28

- Genau 24 Zeichen aus `A–Z a–z 0–9`, sonst `422`.
- Fehlt das Passwort im Request, erzeugt der Service eines (`crypto/rand`).
- Ein erzeugtes Passwort steht genau einmal in der Antwort, mit `Cache-Control: no-store`. Der Service speichert und loggt es nicht.
- Zertifikats-Clients (Name beginnt mit `CN=`) haben nie ein Passwort.

### 3.4 Fehlerantworten

✅ S-1: Jede Fehlerantwort (`4xx`, `5xx`) hat den Content-Type `application/problem+json` nach RFC 9457.

| Feld | Pflicht | Bedeutung |
|---|---|---|
| `status` | ja | HTTP-Status, gleich dem Status der Antwort |
| `code` | ja | Maschinenlesbarer Fehlercode aus der Liste in 3.5 |
| `title` | ja | Kurzer, gleichbleibender Text zum `code` (Englisch) |
| `detail` | nein | Beschreibung des konkreten Falls; nur zur Anzeige, nicht auswerten |
| `type` | nein | URI, **von der Implementierung festgelegt**; nicht Teil der Spezifikation. Fehlt das Feld, gilt `about:blank` |
| `instance` | nein | Pfad der Anfrage |
| `errors` | bei `422` | Liste der Einzelfehler mit `pointer` (JSON Pointer, RFC 6901), `code`, `detail` |

Regeln für TasmoFleet:

- Ausgewertet werden nur `status` und `code`, nie `type`, `title` oder `detail`.
- Ein unbekannter `code` wird wie der Status behandelt (z. B. ein neuer `403`-Code wie `403 insufficient_scope`). Neue Codes können innerhalb von 1.x hinzukommen (Abschnitt 7).
- `detail` kann angezeigt werden; es enthält nie Passwörter oder Tokens.

Beispiel:

```http
HTTP/1.1 403 Forbidden
Content-Type: application/problem+json
```

```json
{
  "type": "https://pmss.example.org/errors/not_managed",
  "title": "Object is not managed by the security service",
  "status": 403,
  "code": "not_managed",
  "detail": "Client 'legacy-shared-user' has no 'managed-by: pmss' marker.",
  "instance": "/api/v1/clients/legacy-shared-user/password"
}
```

Bei `422` mit Einzelfehlern:

```json
{
  "title": "Validation failed",
  "status": 422,
  "code": "validation_failed",
  "errors": [
    { "pointer": "/acls/1/allow", "code": "required", "detail": "Field 'allow' is required." },
    { "pointer": "/acls/2/acltype", "code": "invalid_acltype", "detail": "Unknown acltype 'publish'." }
  ]
}
```

Das zweite Beispiel hat kein `type`; es gilt `about:blank`.

Bei `401` und `403` wegen des Scopes zusätzlich `WWW-Authenticate` nach RFC 6750, z. B. `Bearer error="insufficient_scope", scope="write"`.

### 3.5 Statuscodes und Fehlercodes

✅ S-1: Die Fehlercodes sind verbindlich. Ein Code gehört immer zu genau dem angegebenen Status.

| Status | `code` | Wann |
|---|---|---|
| `200` | – | Lesen, Ändern erfolgreich |
| `201` | – | Objekt angelegt; `Location`- und `ETag`-Header |
| `204` | – | Löschen oder Zuordnung entfernt |
| `401` | `invalid_token` | Token fehlt, ist ungültig, abgelaufen, falscher `iss` oder `aud` ✅ |
| `403` | `insufficient_scope` | Scope fehlt ✅ |
| `403` | `not_managed` | Objekt ist nicht verwaltet ✅ |
| `403` | `reserved_name` | `pmss-*`, `role_security_manager` ✅ |
| `404` | `not_found` | Objekt existiert nicht |
| `409` | `already_exists` | Objekt mit diesem Namen existiert bereits |
| `409` | `certificate_client` | `POST /api/v1/clients/{username}/password` für einen `CN=`-Client ✅. Beim Anlegen mit Passwort: `422 validation_failed`. PMSS verwendet den Code auch für `enable`/`disable` eines `CN=`-Clients (außerhalb der Spezifikation, Abschnitt 4) |
| `412` | `precondition_failed` | `If-Match` passt nicht ✅ |
| `413` | `payload_too_large` | Body zu groß |
| `422` | `validation_failed` | Ungültige Felder, Passwort, ACL, Name, Präfix fehlt ✅ |
| `422` | `unknown_role` / `unknown_group` | Zuordnung zu nicht existierender oder nicht verwalteter Rolle bzw. Gruppe ✅ |
| `428` | `precondition_required` | `If-Match` fehlt ✅ |
| `500` | `readback_mismatch` | Nach `modifyRole` weicht der zurückgelesene Zustand ab ✅ |
| `502` | `broker_error` | DynSec meldet einen Fehler, der keinem anderen Code entspricht |
| `502` | `partial_failure` | Ausgleich gescheitert, Objekt existiert halb angelegt ✅ |
| `503` | `broker_unavailable` | Keine Verbindung zum Broker; **das Kommando wurde nicht gesendet** ✅ S-13 |
| `504` | `broker_timeout` | Keine Antwort vom Broker innerhalb des Timeouts; **der Zustand ist unbekannt**, die Änderung kann ausgeführt sein (Abschnitt 3.7) ✅ S-13 |

TasmoFleet behandelt mindestens: `401` (Token neu tauschen), `403` (Funktion nicht anbieten), `412` (neu einlesen, Diff erneut zur Freigabe), `422` (Fehler anzeigen), `503` (später erneut versuchen), `504` (neu einlesen, Abschnitt 3.7) und die übrigen `5xx` (abbrechen, nichts als erledigt markieren).

### 3.6 Warnhinweise

✅ S-12

Antworten auf ändernde Aufrufe mit Body können ein Feld `warnings` enthalten (Liste mit `code`, `pointer`, `detail`; leer als `[]`). Ein Warnhinweis verhindert die Änderung nicht. Bisher gibt es einen Fall (✅ PMSS 7.3): eine Deny-Regel ohne höhere Priorität als eine Allow-Regel desselben Typs in derselben Rolle.

```json
"warnings": [
  {
    "code": "deny_priority_not_higher",
    "pointer": "/acls/1",
    "detail": "Deny rule for 'stat/+/FILEDOWNLOAD' has priority 0, same as allow rule 'stat/#'; evaluation order is undefined."
  }
]
```

TasmoFleet zeigt Warnhinweise bei der Freigabe an.

### 3.7 Nicht erreichbarer Broker und unklarer Zustand

✅ S-13

Der Service sendet Änderungen als DynSec-Kommandos über MQTT. Bricht die Verbindung ab, nachdem ein Kommando den Broker erreicht hat, kann die Antwort verloren gehen, obwohl der Broker das Kommando ausgeführt hat. DynSec-Kommandos sind nicht idempotent: Eine Wiederholung führt sie ein zweites Mal aus und meldet dann z. B. `already_exists` (PMSS 7.2, A-9).

**Regeln für den Service:**

- `503 broker_unavailable` nur, wenn das Kommando **nicht** an den Broker gesendet wurde. Der Service sendet es auch später nicht nach.
- Bleibt nach dem Senden die Antwort aus, liest der Service das betroffene Objekt zurück, bevor er antwortet:
  - Entspricht der Ist-Zustand dem Ziel der Anfrage, antwortet er wie bei Erfolg (`200`, `201`, `204`).
  - Lässt sich der Zustand nicht feststellen oder entspricht er nicht dem Ziel, antwortet er mit `504 broker_timeout`.
- Der Service wiederholt ändernde Kommandos nicht von sich aus.
- Ein Passwortwechsel lässt sich nicht zurücklesen, weil DynSec keinen Passwort-Hash liefert (S-5). Bleibt die Antwort auf `POST …/password` aus, antwortet der Service immer mit `504`. Beim Anlegen eines Clients mit erzeugtem Passwort genügt das Zurücklesen des Clients: Der Service kennt das Passwort, das er gesendet hat, und gibt es mit `201` zurück.

**Regeln für TasmoFleet:**

- Bei `503` ist nichts geändert. TasmoFleet darf die Anfrage später unverändert erneut senden.
- Bei `504` gilt die Änderung weder als erledigt noch als gescheitert. TasmoFleet wiederholt die Anfrage nicht blind, sondern liest den Ist-Zustand neu ein (`GET /api/v1/export` bzw. das einzelne Objekt), bildet den Diff neu und legt ihn bei Abweichungen erneut zur Freigabe vor.
- Bei `504` auf einen Passwortwechsel gilt das bisherige Passwort als möglicherweise ungültig. TasmoFleet fordert erneut ein neues Passwort an (das ETag ist unverändert, S-5) und überträgt dieses auf das Gerät. Die Regel „ein laufender Passwortwechsel pro Client“ (Abschnitt 4.4) bleibt bis dahin bestehen.
- Das Audit der Freigaben (D-51) vermerkt einen `504` als unklaren Ausgang.

Beispiel:

```http
HTTP/1.1 504 Gateway Timeout
Content-Type: application/problem+json
```

```json
{
  "title": "Broker did not answer in time; state unknown",
  "status": 504,
  "code": "broker_timeout",
  "detail": "No response to setClientPassword for 'wz-steckdose-tv'. The password may or may not have been changed.",
  "instance": "/api/v1/clients/wz-steckdose-tv/password"
}
```

## 4. Endpunkte für TasmoFleet

TasmoFleet braucht nur die folgenden Endpunkte. Die übrigen Endpunkte von PMSS v1 (Listen, einzelne ACLs per `POST`/`DELETE`, `PATCH` des Textnamens, `enable`/`disable`) bleiben erlaubt, sind aber nicht Teil der Spezifikation. In der OpenAPI-Datei tragen sie den Tag `pmss-extra` (Abschnitt 7.1).

| Methode | Endpunkt | Scope | Zweck |
|---|---|---|---|
| `GET` | `/api/v1/info` | `read` | Version, Präfixe, Kennzeichnung, Passwortregel ✅ S-2 |
| `GET` | `/api/v1/export` | `read` | Ist-Zustand aller verwalteten Objekte |
| `GET` | `/api/v1/clients/{username}` | `read` | Ein Client mit `ETag` |
| `POST` | `/api/v1/clients` | `write` | Client anlegen, optional mit Rollen, Gruppen, Passwort |
| `POST` | `/api/v1/clients/{username}/password` | `write` | Neues Passwort |
| `DELETE` | `/api/v1/clients/{username}` | `write` | Client löschen (auch: sperren) |
| `POST` | `/api/v1/clients/{username}/roles` | `write` | Rolle zuweisen |
| `DELETE` | `/api/v1/clients/{username}/roles/{role}` | `write` | Rolle entziehen |
| `GET` | `/api/v1/groups/{group}` | `read` | Eine Gruppe mit `ETag` |
| `POST` | `/api/v1/groups` | `write` | Gruppe anlegen |
| `DELETE` | `/api/v1/groups/{group}` | `write` | Gruppe löschen |
| `POST` | `/api/v1/groups/{group}/clients` | `write` | Client in Gruppe aufnehmen |
| `DELETE` | `/api/v1/groups/{group}/clients/{username}` | `write` | Client aus Gruppe entfernen |
| `POST` | `/api/v1/groups/{group}/roles` | `write` | Rolle der Gruppe zuweisen |
| `DELETE` | `/api/v1/groups/{group}/roles/{role}` | `write` | Rolle der Gruppe entziehen |
| `GET` | `/api/v1/roles/{role}` | `read` | Eine Rolle mit ACLs und `ETag` |
| `POST` | `/api/v1/roles` | `write` | Rolle anlegen, optional mit ACLs ✅ S-6 |
| `PUT` | `/api/v1/roles/{role}/acls` | `write` | ACL-Liste vollständig ersetzen |
| `DELETE` | `/api/v1/roles/{role}` | `write` | Rolle löschen |

`enable`/`disable` sind bewusst nicht in der Spezifikation: `disableClient` sperrt keine Zertifikatsanmeldung (✅ PMSS 5.2). TasmoFleet sperrt einen Client mit `DELETE`. PMSS lehnt `enable`/`disable` für `CN=`-Clients mit `409 certificate_client` ab, ohne ein Kommando an den Broker zu senden (✅ PMSS 7.3, Variante A).

### 4.1 `GET /api/v1/info`

✅ S-2: Pflicht-Endpunkt, Scope `read`. Damit TasmoFleet erkennt, mit welchem Service und welcher Kennzeichnung es arbeitet, ohne diese Werte fest einzubauen. Präfix und Kennzeichnung sind in PMSS konfigurierbar (✅ PMSS 6.4).

```http
GET /api/v1/info
Authorization: Bearer <token>
```

```json
{
  "apiVersion": "1.0",
  "implementation": {
    "name": "phylax-mqtt-security-service",
    "version": "0.1.0"
  },
  "broker": {
    "product": "mosquitto",
    "version": "2.1.2"
  },
  "managed": {
    "rolePrefix": "tf_",
    "groupPrefix": "tf_",
    "clientMarker": "managed-by: pmss"
  },
  "passwordPolicy": {
    "length": 24,
    "alphabet": "A-Za-z0-9"
  },
  "reservedNames": ["pmss-*", "role_security_manager"],
  "limits": {
    "maxRequestBytes": 1048576
  }
}
```

| Feld | Bedeutung |
|---|---|
| `apiVersion` | Version dieser Spezifikation, `MAJOR.MINOR` |
| `implementation` | Name und Version der Implementierung; nur zur Anzeige |
| `broker` | Produkt und Version des Brokers; nur zur Anzeige |
| `managed.rolePrefix`, `managed.groupPrefix` | Präfix, den neue Rollen bzw. Gruppen tragen müssen |
| `managed.clientMarker` | Inhalt von `textdescription` verwalteter Clients |
| `passwordPolicy` | Länge und Alphabet der Passwörter |
| `reservedNames` | Reservierte Namen; `*` steht für beliebige Zeichen |
| `limits.maxRequestBytes` | Größter erlaubter Request-Body in Byte, mindestens 1048576 (1 MiB) ✅ S-11 |

Regeln für TasmoFleet:

- Vor dem ersten Export einer Sitzung `GET /api/v1/info` lesen.
- Ist die Hauptversion von `apiVersion` nicht `1`, keine Änderungen anbieten und den Fehler anzeigen.
- Neue Rollen und Gruppen mit den Präfixen aus `managed` benennen; Tasmota-Topics gegen `reservedNames` prüfen (Overview 12).
- Ein mitgegebenes Passwort vor dem Senden gegen `passwordPolicy` prüfen.

### 4.2 `GET /api/v1/export`

✅ D-43: alle verwalteten Objekte, mit `etag` je Objekt. Nicht verwaltete Objekte (Admins, ein Bestandsbenutzer wie `legacy-shared-user`, `role_legacy`) sind nicht enthalten. Aufbau ✅ S-3:

```http
GET /api/v1/export
Authorization: Bearer <token>
```

```http
HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8
Cache-Control: no-store
```

```json
{
  "exportedAt": "2026-11-20T21:30:12Z",
  "apiVersion": "1.0",
  "clients": [
    {
      "username": "CN=homeassistant,O=mqtt-apps",
      "textname": "Home Assistant",
      "textdescription": "managed-by: pmss",
      "authType": "certificate",
      "disabled": false,
      "roles": [
        { "rolename": "tf_role_homeassistant", "priority": -1 }
      ],
      "groups": [],
      "etag": "\"c-41d0e8c2\""
    },
    {
      "username": "wz-steckdose-tv",
      "textname": "Wohnzimmer Steckdose TV",
      "textdescription": "managed-by: pmss",
      "authType": "password",
      "disabled": false,
      "roles": [
        { "rolename": "tf_role_dev_a4cf12345678", "priority": -1 }
      ],
      "groups": [
        { "groupname": "tf_group_tasmota", "priority": -1 }
      ],
      "etag": "\"c-9f3b2a71\""
    }
  ],
  "groups": [
    {
      "groupname": "tf_group_tasmota",
      "textname": "Tasmota-Geräte",
      "roles": [
        { "rolename": "tf_role_tasmota", "priority": -1 }
      ],
      "clients": [
        { "username": "wz-steckdose-tv" }
      ],
      "etag": "\"g-0a77c315\""
    }
  ],
  "roles": [
    {
      "rolename": "tf_role_dev_a4cf12345678",
      "textname": "Gerät wz-steckdose-tv: GroupTopics, FallbackTopic, Discovery",
      "acls": [
        { "acltype": "publishClientReceive", "topic": "cmnd/a4cf12345678_fb/#", "allow": true, "priority": 0 },
        { "acltype": "publishClientReceive", "topic": "cmnd/wohnzimmer/#",      "allow": true, "priority": 0 },
        { "acltype": "publishClientSend",    "topic": "tasmota/discovery/A4CF12345678/#", "allow": true, "priority": 0 },
        { "acltype": "subscribePattern",     "topic": "cmnd/a4cf12345678_fb/#", "allow": true, "priority": 0 },
        { "acltype": "subscribePattern",     "topic": "cmnd/wohnzimmer/#",      "allow": true, "priority": 0 }
      ],
      "etag": "\"r-e2b6a019\""
    },
    {
      "rolename": "tf_role_homeassistant",
      "textname": "Home Assistant",
      "acls": [
        { "acltype": "publishClientReceive", "topic": "stat/#",               "allow": true,  "priority": 0 },
        { "acltype": "publishClientReceive", "topic": "stat/+/FILEDOWNLOAD",  "allow": false, "priority": 10 },
        { "acltype": "publishClientReceive", "topic": "tele/#",               "allow": true,  "priority": 0 },
        { "acltype": "publishClientSend",    "topic": "cmnd/wz-steckdose-tv/POWER", "allow": true, "priority": 0 },
        { "acltype": "subscribePattern",     "topic": "stat/#",               "allow": true,  "priority": 0 },
        { "acltype": "subscribePattern",     "topic": "tele/#",               "allow": true,  "priority": 0 }
      ],
      "etag": "\"r-77f0c3aa\""
    },
    {
      "rolename": "tf_role_tasmota",
      "textname": "Tasmota-Grundprofil",
      "acls": [
        { "acltype": "publishClientReceive", "topic": "cmnd/%u/#", "allow": true, "priority": 0 },
        { "acltype": "publishClientSend",    "topic": "stat/%u/#", "allow": true, "priority": 0 },
        { "acltype": "publishClientSend",    "topic": "tele/%u/#", "allow": true, "priority": 0 },
        { "acltype": "subscribePattern",     "topic": "cmnd/%u/#", "allow": true, "priority": 0 }
      ],
      "etag": "\"r-5c1e9d40\""
    }
  ]
}
```

**Felder** ✅ S-3:

| Objekt | Felder |
|---|---|
| Export | `exportedAt`, `apiVersion`, `clients`, `groups`, `roles` |
| Client | `username`, `textname`, `textdescription`, `authType`, `disabled`, `roles`, `groups`, `etag` |
| Gruppe | `groupname`, `textname`, `roles`, `clients`, `etag` |
| Rolle | `rolename`, `textname`, `acls`, `etag` |
| Zuordnung | `rolename` bzw. `groupname` mit `priority`; Clients einer Gruppe nur `username` |
| ACL | `acltype`, `topic`, `allow`, `priority` |

- Alle Felder sind immer vorhanden. Leere Listen werden als `[]` geliefert, fehlender Text als `""`.
- `priority` steht immer da, auch beim Standardwert (Zuordnung −1, ACL 0).
- `authType` leitet der Service aus dem Namen ab: `CN=…` → `certificate`, sonst `password`.

**Sortierung und Vergleich** ✅ S-3:

- Der Service sortiert, damit zwei Exporte desselben Zustands gleich aussehen und sich zeilenweise vergleichen lassen:
  - `clients` nach `username`, `groups` nach `groupname`, `roles` nach `rolename`,
  - Zuordnungen nach Rollen- bzw. Gruppen- bzw. Benutzername,
  - `acls` nach `acltype`, dann nach `topic`.
- Sortiert wird nach Unicode-Codepunkten, ohne Rücksicht auf Groß- und Kleinschreibung oder Sprache (z. B. `CN=…` vor `wz-…`, `stat/#` vor `stat/+/…`).
- Die Reihenfolge hat **keine Bedeutung**. TasmoFleet vergleicht Soll und Ist über Schlüssel, nicht über Positionen:

| Element | Schlüssel |
|---|---|
| Client | `username` |
| Gruppe | `groupname` |
| Rolle | `rolename` |
| ACL innerhalb einer Rolle | `acltype` + `topic` |
| Zuordnung | Name der zugeordneten Rolle, Gruppe bzw. des Clients |

- Auch in Requests (`PUT …/acls`, `POST /api/v1/roles`) ist die Reihenfolge ohne Bedeutung. Einzelobjekte (`GET /api/v1/roles/{role}` usw.) haben denselben Aufbau und dieselbe Sortierung wie im Export.
- Verschwindet ein Objekt aus dem Export, weil seine Kennzeichnung oder sein Präfix per Break-glass entfernt wurde, zeigt der Drift-Report es als fehlend. TasmoFleet legt es nicht automatisch neu an.

Weitere Hinweise:

- **Rolle pro Tasmota-Gerät** ✅ S-7: GroupTopic, FallbackTopic und Discovery passen nicht in das Schema mit `%u` (Overview Abschnitt 4, D-53). Sie stehen in einer eigenen Rolle je Gerät:
  - Name `tf_role_dev_<mac>`, `<mac>` = MAC in Kleinbuchstaben, 12 Hex-Zeichen ohne Trennzeichen, z. B. `tf_role_dev_a4cf12345678`.
  - `textname` enthält das Topic, z. B. `Gerät wz-steckdose-tv: GroupTopics, FallbackTopic, Discovery`.
  - Grund: Die MAC ist eindeutig, hat eine feste Länge und keine Sonderzeichen; FallbackTopic und Discovery hängen ohnehin an ihr. Ändert sich das Topic, entsteht ein neuer Client (Benutzername = Topic, D-15), die Geräterolle bleibt und wird dem neuen Client zugewiesen.
  - Topics in den ACLs behalten ihre Schreibweise, z. B. `tasmota/discovery/A4CF12345678/#` in Großbuchstaben.
- Der Export ist die Sicherung, die TasmoFleet vor dem Anwenden ablegt. ✅ D-43

### 4.3 `POST /api/v1/clients` – Client anlegen

✅ PMSS 7.3/7.4: Rollen und Gruppen optional, Passwort optional. Gesetzt wird die Kennzeichnung `managed-by: pmss`; der Aufrufer kann `textdescription` nicht setzen.

**Request – Passwort vom Service erzeugen lassen (Normalfall):**

```http
POST /api/v1/clients
Authorization: Bearer <token>
Content-Type: application/json
```

```json
{
  "username": "wz-steckdose-tv",
  "textname": "Wohnzimmer Steckdose TV",
  "groups": [
    { "groupname": "tf_group_tasmota" }
  ],
  "roles": [
    { "rolename": "tf_role_dev_a4cf12345678" }
  ]
}
```

**Response:**

```http
HTTP/1.1 201 Created
Location: /api/v1/clients/wz-steckdose-tv
ETag: "c-9f3b2a71"
Cache-Control: no-store
Content-Type: application/json; charset=utf-8
```

```json
{
  "client": {
    "username": "wz-steckdose-tv",
    "textname": "Wohnzimmer Steckdose TV",
    "textdescription": "managed-by: pmss",
    "authType": "password",
    "disabled": false,
    "roles": [ { "rolename": "tf_role_dev_a4cf12345678", "priority": -1 } ],
    "groups": [ { "groupname": "tf_group_tasmota", "priority": -1 } ]
  },
  "password": "Q7mXr2Lk9pTz4WbN8cVd3HsF",
  "passwordGenerated": true
}
```

**Request – Passwort mitgeben:** wie oben, zusätzlich `"password": "<24 Zeichen>"`. Die Antwort enthält dann **kein** Passwort und `"passwordGenerated": false`. ✅ S-12

**Request – Zertifikats-Client („TLS erforderlich“):**

```json
{
  "username": "CN=homeassistant,O=mqtt-apps",
  "textname": "Home Assistant",
  "roles": [ { "rolename": "tf_role_homeassistant" } ]
}
```

Antwort ohne `password`. Ein mitgegebenes Passwort wird mit `422` abgelehnt. ✅

**Fehler:**

| Fall | Antwort |
|---|---|
| Name `pmss-…` | `403 reserved_name` |
| Passwort-Client mit `=` oder `,` im Namen | `422 validation_failed` |
| Passwort mit `;`, Leerzeichen oder anderer Länge | `422 validation_failed` |
| Client existiert bereits | `409 already_exists` |
| Rolle existiert nicht oder ist nicht verwaltet (z. B. `role_legacy`) | `422 unknown_role`; der Client wird wieder gelöscht ✅ |
| Wiederlöschen gescheitert | `502 partial_failure` mit `"clientExists": true` ✅ |
| Antwort des Brokers ausgeblieben, Zustand nicht feststellbar | `504 broker_timeout` (Abschnitt 3.7) ✅ S-13 |

Beispiel Ausgleichsweg:

```json
{
  "type": "https://pmss.example.org/errors/unknown_role",
  "title": "Unknown or unmanaged role",
  "status": 422,
  "code": "unknown_role",
  "detail": "Role 'tf_role_dev_a4cf12345678' does not exist. The client was created and deleted again.",
  "compensated": true,
  "clientExists": false
}
```

### 4.4 `POST /api/v1/clients/{username}/password` – neues Passwort

```http
POST /api/v1/clients/wz-steckdose-tv/password
Authorization: Bearer <token>
If-Match: "c-9f3b2a71"
Content-Type: application/json
```

```json
{}
```

Leerer Body: Der Service erzeugt das Passwort. Mit `{"password": "<24 Zeichen>"}` wird das mitgegebene gesetzt.

```http
HTTP/1.1 200 OK
ETag: "c-9f3b2a71"
Cache-Control: no-store
```

```json
{
  "username": "wz-steckdose-tv",
  "password": "h3RtY8vQm2NpXc6KzL4bWs9D",
  "passwordGenerated": true
}
```

**ETag beim Passwortwechsel** ✅ S-5: Ein Passwortwechsel ändert das ETag nicht, denn der Passwort-Hash gehört nicht zum ETag. DynSec liefert den Hash über `getClient` auch nicht; die Ausgabe ist vor und nach dem Passwortwechsel gleich (getestet 21.11.2026 mit Mosquitto 2.1.2).

> **Folge für TasmoFleet: nur ein Passwortwechsel pro Client gleichzeitig**
> `If-Match` schützt nicht davor, dass zwei Benutzer gleichzeitig ein neues Passwort für denselben Client anfordern. Beide Aufrufe wären erfolgreich, und das Gerät erhielte womöglich das zuerst erzeugte, bereits überschriebene Passwort. TasmoFleet lässt deshalb je Client nur einen laufenden Passwortwechsel zu: von der Anforderung bis zur bestätigten Übertragung per `Backlog` oder bis zum Abbruch.

**Ohne Antwort des Brokers** ✅ S-13: Weil sich ein Passwortwechsel nicht zurücklesen lässt, antwortet der Service in diesem Fall immer mit `504 broker_timeout`. TasmoFleet fordert dann erneut ein neues Passwort an und überträgt dieses (Abschnitt 3.7).

`CN=`-Client → `409 certificate_client`. Nicht verwalteter Client → `403 not_managed`.

### 4.5 `DELETE /api/v1/clients/{username}` – löschen bzw. sperren

```http
DELETE /api/v1/clients/CN%3Dhomeassistant%2CO%3Dmqtt-apps
Authorization: Bearer <token>
If-Match: "c-41d0e8c2"
```

```http
HTTP/1.1 204 No Content
```

Eine offene Verbindung des Clients trennt der Broker. ✅ PMSS 5.2

> **Regel für TasmoFleet: Gesperrte Namen nicht wiederverwenden**
> DynSec kennt nur den Namen, nicht das Zertifikat. Wird nach einer Sperre wegen Kompromittierung derselbe Name neu angelegt, gilt das alte Zertifikat wieder. Nachfolger erhalten eine fortlaufende Nummer, z. B. `CN=homeassistant-2,O=mqtt-apps`. ✅ D-40, SEC-23. Der Service kennt gelöschte Namen nicht; TasmoFleet muss sie sich merken.

### 4.6 Rollen zuweisen und entziehen

Beispiel: Home Assistant erhält zusätzlich die Rolle `tf_role_mqtt_viewer`.

```http
POST /api/v1/clients/CN%3Dhomeassistant%2CO%3Dmqtt-apps/roles
Authorization: Bearer <token>
If-Match: "c-41d0e8c2"
Content-Type: application/json
```

```json
{ "rolename": "tf_role_mqtt_viewer", "priority": -1 }
```

`priority` ist optional, Standard −1 wie DynSec. ✅

```http
HTTP/1.1 200 OK
ETag: "c-6a2d91f0"
```

✅ S-10: Antwort-Body ist der geänderte Client wie in `GET /api/v1/clients/{username}`, mit `warnings`. `DELETE` antwortet mit `204` und dem neuen `ETag`-Header.

Entziehen:

```http
DELETE /api/v1/clients/CN%3Dhomeassistant%2CO%3Dmqtt-apps/roles/tf_role_mqtt_viewer
Authorization: Bearer <token>
If-Match: "c-6a2d91f0"
```

```http
HTTP/1.1 204 No Content
ETag: "c-41d0e8c2"
```

Das ETag ist ein Hash über den Zustand: Nach dem Entziehen ist es wieder dasselbe wie vorher.

Die Gruppen-Endpunkte (`/api/v1/groups/{group}/clients`, `/api/v1/groups/{group}/roles`) funktionieren gleich, mit `{"username": "…"}` bzw. `{"rolename": "…", "priority": -1}` und dem `ETag` der Gruppe. Es ändert sich nur das ETag der Gruppe (S-4). `POST` liefert die geänderte Gruppe wie in `GET /api/v1/groups/{group}` (S-10).

### 4.7 `POST /api/v1/roles` – Rolle anlegen

Präfix aus `GET /api/v1/info` (`tf_`) Pflicht, sonst `422`. ✅

✅ S-6: Die ACL-Liste kann gleich mitgegeben werden, damit die Rolle in einem Schritt vollständig entsteht. `acls` ist optional; ohne ACLs entsteht eine leere Rolle, die wegen Default-Deny keine Rechte gibt.

Ablauf im Service:

1. Jeden Eintrag prüfen wie bei `PUT …/acls` (Abschnitt 4.8); bei Fehlern `422`, ohne Kommando an den Broker.
2. `createRole` mit der vollständigen Liste senden (ein Kommando).
3. Rolle per `getRole` zurücklesen und mit der gesendeten Liste vergleichen.
4. Bei Abweichung die Rolle wieder löschen (`deleteRole`) und `500 readback_mismatch` mit `"compensated": true` melden. Scheitert das Löschen, `502 partial_failure` mit `"roleExists": true`.

```http
POST /api/v1/roles
Authorization: Bearer <token>
Content-Type: application/json
```

```json
{
  "rolename": "tf_role_dev_a4cf12345678",
  "textname": "Gerät wz-steckdose-tv: GroupTopics, FallbackTopic, Discovery",
  "acls": [
    { "acltype": "subscribePattern",     "topic": "cmnd/wohnzimmer/#", "allow": true },
    { "acltype": "publishClientReceive", "topic": "cmnd/wohnzimmer/#", "allow": true }
  ]
}
```

```http
HTTP/1.1 201 Created
Location: /api/v1/roles/tf_role_dev_a4cf12345678
ETag: "r-3d9a0b17"
```

Antwort-Body: die Rolle wie in `GET /api/v1/roles/{role}`, sortiert, mit `warnings` (Abschnitt 3.6).

| Fall | Antwort |
|---|---|
| Präfix fehlt | `422 validation_failed` |
| ACL ungültig (fehlendes `allow`, unbekannter `acltype`, doppelte Kombination) | `422 validation_failed` |
| Rolle existiert bereits | `409 already_exists` |
| Zurückgelesener Zustand weicht ab | `500 readback_mismatch`, Rolle wieder gelöscht |
| Wiederlöschen gescheitert | `502 partial_failure` mit `"roleExists": true` |
| Antwort des Brokers ausgeblieben, Zustand nicht feststellbar | `504 broker_timeout` (Abschnitt 3.7) |

### 4.8 `PUT /api/v1/roles/{role}/acls` – ACL-Liste ersetzen

✅ D-45: ersetzt die vollständige Liste in einem Schritt (`modifyRole`), danach liest der Service die Rolle zurück und vergleicht.

ACL-Felder ✅ (PMSS 7.3):

| Feld | Regel |
|---|---|
| `acltype` | Pflicht: `publishClientSend`, `publishClientReceive`, `subscribeLiteral`, `subscribePattern`, `unsubscribeLiteral`, `unsubscribePattern` |
| `topic` | Pflicht; zusammen mit `acltype` eindeutig in der Rolle |
| `allow` | **Pflicht, kein Standardwert** |
| `priority` | Optional, Standard 0, −100000 bis 100000 |

```http
PUT /api/v1/roles/tf_role_homeassistant/acls
Authorization: Bearer <token>
If-Match: "r-77f0c3aa"
Content-Type: application/json
```

```json
{
  "acls": [
    { "acltype": "subscribePattern",     "topic": "stat/#",              "allow": true },
    { "acltype": "publishClientReceive", "topic": "stat/#",              "allow": true },
    { "acltype": "publishClientReceive", "topic": "stat/+/FILEDOWNLOAD", "allow": false, "priority": 10 },
    { "acltype": "subscribePattern",     "topic": "tele/#",              "allow": true },
    { "acltype": "publishClientReceive", "topic": "tele/#",              "allow": true }
  ]
}
```

```http
HTTP/1.1 200 OK
ETag: "r-c09e5f28"
Content-Type: application/json; charset=utf-8
```

```json
{
  "rolename": "tf_role_homeassistant",
  "textname": "Home Assistant",
  "acls": [
    { "acltype": "publishClientReceive", "topic": "stat/#",              "allow": true,  "priority": 0 },
    { "acltype": "publishClientReceive", "topic": "stat/+/FILEDOWNLOAD", "allow": false, "priority": 10 },
    { "acltype": "publishClientReceive", "topic": "tele/#",              "allow": true,  "priority": 0 },
    { "acltype": "subscribePattern",     "topic": "stat/#",              "allow": true,  "priority": 0 },
    { "acltype": "subscribePattern",     "topic": "tele/#",              "allow": true,  "priority": 0 }
  ],
  "etag": "\"r-c09e5f28\"",
  "warnings": []
}
```

Mit `"priority": 0` an der FILEDOWNLOAD-Regel wäre die Antwort ebenfalls `200`, aber mit dem Warnhinweis `deny_priority_not_higher` (Abschnitt 3.6).

**Fehler:** fehlendes `allow`, unbekannter `acltype`, doppelte Kombination `acltype` + `topic` → `422`, bevor ein Kommando an den Broker geht ✅. Veraltetes `If-Match` → `412` ✅. Abweichung beim Zurücklesen → `500 readback_mismatch` ✅. Antwort des Brokers ausgeblieben und Zustand nicht feststellbar → `504 broker_timeout` (Abschnitt 3.7) ✅ S-13.

## 5. Ablauf: Tasmota-Gerät auf eigene Zugangsdaten umstellen

```
security-admin        TasmoFleet                         Security Service         Gerät
     │                    │                                     │                    │
     │  Umstellung wählen │                                     │                    │
     │───────────────────►│ Token Exchange (Keycloak)           │                    │
     │                    │ scope enthält write? sonst Abbruch  │                    │
     │                    │ GET /api/v1/export ────────────────►│                    │
     │                    │◄──────────────────── Ist + ETags ───│                    │
     │                    │ Export ablegen (Sicherung)          │                    │
     │                    │ Soll aus Tasmota-Daten ableiten     │                    │
     │                    │ (Topic, GroupTopic1–4, Fallback, MAC)                    │
     │◄── Diff + Warnungen│                                     │                    │
     │─── Freigabe ──────►│ Audit der Freigabe (TasmoFleet)     │                    │
     │                    │ POST /api/v1/roles (Geräterolle) ──►│                    │
     │                    │ POST /api/v1/clients ──────────────►│                    │
     │                    │◄────────────── 201 + Passwort ──────│                    │
     │                    │ Backlog MqttUser …; MqttPassword … ─────────────────────►│
     │                    │ Passwort verwerfen                  │                    │ reconnect
     │                    │ Verbindung und Rechte prüfen        │                    │
     │                    │ Fehlschlag → Rückweg über HTTP-API ─────────────────────►│
```

Vorbedingungen, die TasmoFleet vor dem ersten Aufruf prüft (✅ Overview 6.2, 12):

- [ ] Topic beginnt nicht mit `pmss-` und enthält weder `=` noch `,`.
- [ ] Alle vier GroupTopics sind gelesen (#1005).
- [ ] HTTP-API des Geräts ist erreichbar (`SetOption128` nicht `1`), sonst blockieren oder warnen.
- [ ] Getauschtes Token enthält `write`.

Antwortet ein Schritt mit `504 broker_timeout`, bricht TasmoFleet den Ablauf ab, liest den Export neu ein und legt den neuen Diff zur Freigabe vor (Abschnitt 3.7). Das Gerät erhält erst dann Zugangsdaten per `Backlog`, wenn der Client sicher angelegt ist und TasmoFleet ein Passwort dazu hat.

## 6. Aufgaben von TasmoFleet

| Aufgabe | |
|---|---|
| Soll-Zustand, Diff gegen Export, Freigabe durch `security-admin` | ✅ D-43 |
| Eigenes Audit der Freigaben, lesbar für `admin` und `security-admin` | ✅ D-51 |
| Export vor dem Anwenden ablegen | ✅ |
| Drift-Report (Soll gegen Export, ohne Anwenden) | ✅ |
| `If-Match` bei jeder Änderung; bei `412` neu einlesen und neu freigeben lassen | ✅ |
| Bei `504` nicht blind wiederholen, sondern neu einlesen; bei einem Passwortwechsel neues Passwort anfordern | ✅ S-13 |
| Passwort nur bis zur erfolgreichen Übertragung vorhalten bzw. bis zum Abbruch | ✅ D-54 (Overview, Frage 15) |
| Gesperrte Namen und Subjects merken, Nachfolger nummerieren | ✅ D-40 |
| Pro Client nur ein laufender Passwortwechsel | ✅ S-5 |
| Gruppenzugehörigkeit über die Gruppe vergleichen | ✅ S-4 |
| Geräterollen `tf_role_dev_<mac>` benennen | ✅ S-7 |
| Warnhinweise in der Freigabe anzeigen | ✅ S-12 |

TasmoFleet tut **nicht**: am Broker als DynSec-Admin handeln, auf `$CONTROL/dynamic-security/#` publizieren, nicht verwaltete Objekte ändern, mit einem Service-Account mit `write` aufrufen.

## 7. Versionierung

✅ S-9

- Alle Pfade tragen die Hauptversion: `/api/v1/…`. Ausnahme ist `/healthz` (intern, ohne Token).
- `GET /api/v1/info` liefert die genaue Version (`apiVersion`, `MAJOR.MINOR`).
- Innerhalb von 1.x nur rückwärtskompatible Änderungen: neue optionale Felder, neue Endpunkte, neue `code`-Werte, neue Warnhinweise. TasmoFleet ignoriert unbekannte Felder in Antworten und behandelt unbekannte `code`-Werte nach dem Status (Abschnitt 3.4).
- Eine inkompatible Änderung bekommt `/api/v2/…`. Eine Implementierung kann v1 und v2 eine Zeit lang parallel anbieten.

### 7.1 Maschinenlesbare Fassung (OpenAPI)

✅ S-14

- Die Spezifikation liegt zusätzlich als **`api/openapi.yaml`** (OpenAPI 3.2.1) im PMSS-Repository. Sie beschreibt Pfade, Scopes, Header (`ETag`, `If-Match`, `Location`, `Cache-Control`, `WWW-Authenticate`), Request- und Response-Schemas und die Fehlercodes je Status aus Abschnitt 3.5.
- **Maßgeblich bleibt dieses Dokument.** Ein Widerspruch zwischen beiden ist ein Fehler in der OpenAPI-Datei und wird dort behoben. Verhalten, das sich nicht als Schema ausdrücken lässt (ETag-Regeln 3.2, unklarer Zustand 3.7, Ausgleichsweg 4.3 und 4.7), beschreibt die OpenAPI-Datei nur kurz und verweist auf dieses Dokument.
- `info.version` der OpenAPI-Datei entspricht `apiVersion` (Abschnitt 4.1); `x-specification-version` nennt die Version dieses Dokuments.
- **Sicherheitsschema `keycloak` vom Typ `oauth2`** mit den Scopes `read` und `write`. Der Service prüft nur das Bearer-Token (Abschnitt 2.1); über welchen Weg es entstanden ist, spielt für ihn keine Rolle. Die Flows beschreiben die Wege aus dem PMSS-Dokument (7.5):
  - `deviceAuthorization` (RFC 8628, ab OpenAPI 3.2): Administratoren, Anmeldung und zweiter Faktor im Browser.
  - `clientCredentials`: Skripte und Monitoring, in Produktion nur `read` (SEC-35); `write` nur im Test-Realm.
  - Der Token Exchange von TasmoFleet (Abschnitt 2.2) hat in OpenAPI kein eigenes Flow-Objekt und steht deshalb nur als Text in der Beschreibung des Schemas.
  - Einen Browser-Login-Flow (`authorizationCode`) gibt es nicht; der Service hat keine Oberfläche.
- Endpunkte einer Implementierung, die nicht in der Spezifikation stehen, tragen den Tag `pmss-extra` (`kind: badge`) und `x-pmss-extra: true`. TasmoFleet kann sie beim Erzeugen eines Clients ausblenden.
- Die Fehlercodes sind in der Datei je Status aufgezählt. Für TasmoFleet gilt trotzdem Abschnitt 3.4: Neue Codes können innerhalb von 1.x hinzukommen und werden nach dem Status behandelt.
- PMSS prüft in CI, dass die Operationen der Datei genau den Routen des Service entsprechen und dass jede in den Tests aufgezeichnete Antwort der Datei entspricht: Status, Pflicht-Header, Body und Fehlercode (`test/openapi/check.py`).
- Werkzeuge, die OpenAPI 3.2 noch nicht kennen, können die Datei ablehnen. 3.2 ist abwärtskompatibel zu 3.1; mit dem Sicherheitsschema `deviceAuthorization` nutzt die Datei aber eine Eigenschaft, die es erst ab 3.2 gibt.

## 8. Entscheidungen dieser Spezifikation

| Nr. | Entscheidung | Abschnitt |
|---|---|---|
| S-1 | Fehler als `application/problem+json` nach RFC 9457; verbindlich sind `status` und `code`, nicht `type` | 3.4, 3.5 |
| S-2 | `GET /api/v1/info` ist Pflicht, Scope `read` | 4.1 |
| S-3 | Export mit drei sortierten Listen, `etag` je Objekt, `authType`; Vergleich über Schlüssel, nicht über die Reihenfolge | 4.2 |
| S-4 | Jede Beziehung hat einen Eigentümer; `If-Match` und neues ETag nur für das Objekt im Pfad | 3.2 |
| S-5 | Passwortwechsel ändert das ETag nicht; TasmoFleet erlaubt pro Client nur einen laufenden Passwortwechsel | 4.4 |
| S-6 | `POST /api/v1/roles` mit ACLs, Prüfung, Zurücklesen und Ausgleichsweg | 4.7 |
| S-7 | Rolle pro Tasmota-Gerät heißt `tf_role_dev_<mac>` | 4.2 |
| S-8 | DynSec-Feldnamen unverändert, eigene Felder in camelCase | 1 |
| S-9 | Alle Pfade mit Hauptversion `/api/v1/…`, `/healthz` ohne Version | 7 |
| S-10 | Zuordnung per `POST` liefert `200` mit dem geänderten Objekt, `DELETE` `204` mit `ETag` | 4.6 |
| S-11 | Request-Body mindestens 1 MiB, Grenze in `GET /api/v1/info` | 3.1, 4.1 |
| S-12 | Konfiguration in TasmoFleet, Zeitstempel RFC 3339 UTC, URL-kodierte Pfadparameter, Warnhinweise, keine Rückgabe eines mitgegebenen Passworts | 2.2, 3.1, 3.6, 4.3 |
| S-13 | `504 broker_timeout` = Zustand unbekannt; der Service liest vorher zurück, TasmoFleet liest neu ein statt blind zu wiederholen | 3.5, 3.7 |
| S-14 | `api/openapi.yaml` (OpenAPI 3.2.1) als maschinenlesbare Fassung; dieses Dokument bleibt maßgeblich | 7.1 |

## Referenzen

- Architektur-Überblick `mqtt-security-overview` (aktuelle Fassung), Abschnitte 4, 5.4, 7.2, 7.3, 11 (D-52, D-53), 12
- PMSS-Detaildokument `phylax-mqtt-security-service-design` (aktuelle Fassung), Abschnitte 5.2, 6.4, 7.2–7.5 (A-9), 7.9
- PMSS, OpenAPI-Datei: https://github.com/Noschvie/phylax-mqtt-security-service/blob/main/api/openapi.yaml, Prüfung `test/openapi/check.py`
- `mqtt-acl-verify`: `tests/test_dynsec_control.py`, `probes/autopaho-reconnect/` – Tests zu `correlationData`, `getClient` und Reconnect: https://github.com/Noschvie/mqtt-acl-verify
- Mosquitto Dynamic Security Plugin: https://mosquitto.org/documentation/dynamic-security/
- OpenAPI Specification 3.2.1: https://spec.openapis.org/oas/v3.2.1.html
- RFC 8628 – OAuth 2.0 Device Authorization Grant: https://www.rfc-editor.org/rfc/rfc8628
- RFC 8693 – OAuth 2.0 Token Exchange: https://www.rfc-editor.org/rfc/rfc8693
- RFC 9457 – Problem Details for HTTP APIs: https://www.rfc-editor.org/rfc/rfc9457
- RFC 6901 – JSON Pointer: https://www.rfc-editor.org/rfc/rfc6901
- RFC 6750 – Bearer Token Usage: https://www.rfc-editor.org/rfc/rfc6750
- RFC 9110 – HTTP Semantics (`ETag`, `If-Match`, `412`): https://www.rfc-editor.org/rfc/rfc9110
- RFC 6585 – `428 Precondition Required`: https://www.rfc-editor.org/rfc/rfc6585
