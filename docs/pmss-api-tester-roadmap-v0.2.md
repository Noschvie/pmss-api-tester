# PMSS API Tester – Planung und Umsetzung

**Dokumentstatus:** Umsetzungsroadmap für Version 1
**Version:** 0.2
**Stand:** 24.11.2026

Die Architekturentscheidungen richten sich nach `docs/design/pmss-api-tester-design-v0.3.md`. Bei Abweichungen ist das Design maßgeblich; die normative API-Grundlage bleibt die MQTT Security Service API-Spezifikation.

## 1. Ziel

Der PMSS API Tester ist ein generisches Werkzeug zum Erkunden, Prüfen und Testen von Implementierungen der **MQTT Security Service API**.

Die normative Grundlage ist `docs/mqtt-security-service-api-v1.0.md`. Die Datei `api/openapi.yaml` stellt die maschinenlesbare Beschreibung derselben API bereit. Die Markdown-Spezifikation bleibt maßgeblich, wenn sich Beschreibungen widersprechen. Beide Dateien liegen im Tester-Repository; die OpenAPI-Datei wird fest eingebettet und nicht zur Laufzeit von einer externen URL geladen.

Der Phylax MQTT Security Service (PMSS) ist zunächst das erste Testziel. Die Architektur des Testers soll jedoch nicht von PMSS-internen Implementierungsdetails abhängen.

### Nicht-Ziele für den ersten Prototyp

- Kein MQTT-Client und keine direkte MQTT-Kommunikation
- Keine Verwaltung des Broker-Dynamic-Security-Zustands durch den Tester
- Keine produktive Administrationsoberfläche
- Keine Datenbank oder dauerhafte Testhistorie
- Keine Lasttests oder CI/CD-Integration im ersten Schritt

## 2. Grundlegende Architekturentscheidungen

| Bereich | Festlegung |
|---|---|
| Backend | Go mit `net/http` / `ServeMux` |
| UI | `templ`, HTMX 2.x und eingebettete statische Assets |
| UI-Stil | Gleiche technische Basis und Gestaltungskonventionen wie TasmoFleet |
| Auslieferung | Ein Docker-Container, Multi-Stage-Build |
| Persistenz | Keine Datenbank; Laufzeitdaten und Testergebnisse zunächst im Arbeitsspeicher |
| API-Beschreibung | OpenAPI 3.2.1 als maschinenlesbare Grundlage; normative Markdown-Spezifikation hat Vorrang |
| Testziel | Konfigurierbare HTTPS-Basis-URL einer MQTT Security Service API, zunächst PMSS |

Der Tester soll sich optisch und technisch in die TasmoFleet-Produktfamilie einfügen. React und TypeScript sind für dieses Vorhaben nicht vorgesehen.

## 3. Authentifizierung und Autorisierung

### 3.1 Anmeldung am Tester

- Anmeldung über den bestehenden Keycloak-Realm `Tasmofleet`.
- Der Tester erhält eine eigene Keycloak-Client-Konfiguration.
- Für den Browser-Login wird Authorization Code Flow mit PKCE verwendet.
- Der Tester arbeitet als Backend-for-Frontend (BFF).
- Tokens verbleiben serverseitig; der Browser erhält nur eine sichere Session-Cookie.
- Client Secret und Tokens dürfen nicht an den Browser ausgeliefert werden.

### 3.2 Zugriff auf den MQTT Security Service

Für Aufrufe an den MQTT Security Service wird die Identität des angemeldeten Benutzers beibehalten.

Der Tester verwendet daher **keinen Service Account als Ersatz für den Benutzer**. Stattdessen führt das Backend einen serverseitigen OAuth 2.0 Token Exchange nach RFC 8693 durch:

1. Der Benutzer meldet sich am Tester über Keycloak an.
2. Das Backend verwendet das Access Token des angemeldeten Benutzers als `subject_token`.
3. Das Backend fordert beim Token Exchange ein Access Token für die Audience des Zielsystems an. Für PMSS ist dies `phylax-mqtt-security-service`.
4. Version 1 fordert ausschließlich den Scope `read` an.
5. Das erhaltene, benutzerbezogene Token wird serverseitig für die lesenden API-Aufrufe verwendet. Ein Service-Account-Token ersetzt nicht die Benutzeridentität.

Für weitere konforme Zielsysteme muss die Audience zielsystemspezifisch konfigurierbar sein. Der Client wird gegenüber Keycloak serverseitig authentifiziert.

### 3.3 Rechte und UI-Verhalten

- `read` ist Voraussetzung für lesende API-Aufrufe.
- Die tatsächlich im Token enthaltenen Scopes sind maßgeblich.
- Fehlt `read`, werden geschützte Prüfungen als `BLOCKED` ausgewiesen und nicht als technischer Fehler verschleiert.
- Version 1 fordert `write` nicht an und führt keine mutierenden API-Aufrufe aus.

## 4. Umgang mit der API-Spezifikation

### 4.1 Normative Quelle und OpenAPI

Im Repository des Testers liegen die beiden API-Artefakte:

- `docs/mqtt-security-service-api-v1.0.md` – normative Spezifikation
- `api/openapi.yaml` – maschinenlesbare OpenAPI-Beschreibung

Die OpenAPI-Datei wird in die Anwendung eingebettet und beim Start validiert. Bei einer fehlerhaften oder inkompatiblen Spezifikation startet die Anwendung nicht unvollständig. OpenAPI dient der Darstellung von Operationen, GET-Parametern und Schemas. Normative Verhaltensregeln, die dort nicht vollständig ausgedrückt sind, werden anhand der Markdown-Spezifikation implementiert bzw. getestet.

### 4.2 Core API und PMSS Extra

Die Spezifikation unterscheidet zwischen:

- **Core API:** Bestandteil der standardisierten Schnittstelle und Grundlage der Konformitätsprüfung.
- **PMSS Extra:** Implementierungsspezifische Erweiterungen, gekennzeichnet durch den Tag `pmss-extra` und `x-pmss-extra: true`.

Diese Bereiche müssen im Tester getrennt dargestellt und ausgewertet werden. Ein nicht unterstützter PMSS-Extra-Endpunkt darf nicht als Fehler der Core-Konformität gewertet werden.

### 4.3 Versionsprüfung

Der Tester zeigt mindestens an:

- API-Version
- Spezifikationsversion
- Zielsystem / Implementierung
- Erreichbarkeit und Authentifizierungsstatus

Abweichungen zwischen erwarteter und gemeldeter Version werden sichtbar ausgewiesen.

## 5. Funktionsumfang des Testers

### 5.1 API Explorer

Der API Explorer ermöglicht es, API-Operationen gezielt aufzurufen und die Ergebnisse zu untersuchen.

Anzuzeigen sind mindestens:

- HTTP-Methode und Pfad
- GET-Parameter und Request-/Response-Schemas
- verwendete Authentifizierung und Scopes
- HTTP-Status
- Response-Header
- Response-Body
- Laufzeit des Aufrufs

Version 1 führt ausschließlich GET-Aufrufe aus. Andere HTTP-Methoden müssen serverseitig abgewiesen und als `BLOCKED` ausgewiesen werden; ein bloßes Verbergen oder Deaktivieren in der UI genügt nicht. Relevante Response-Header wie `ETag`, `Content-Type` und Cache-Control werden angezeigt. `If-Match` wird in Version 1 nicht zum Ausführen von Änderungen verwendet.

### 5.2 Automatisierte Prüfungen

Der Tester führt definierte Prüfungen aus und bewertet sie mit:

- `PASS` – Prüfung erfolgreich
- `FAIL` – Prüfung fehlgeschlagen
- `NOT TESTED` – Prüfung nicht ausgeführt
- `BLOCKED` – Prüfung konnte wegen einer Voraussetzung oder fehlenden Berechtigung nicht ausgeführt werden

Core-Konformität und PMSS-Extras werden getrennt ausgewertet.

### 5.3 Fehlerbehandlung

Die Prüfung berücksichtigt insbesondere:

- Fehlerantworten nach RFC 9457 (`application/problem+json`)
- normative Auswertung von HTTP-Status und `code`
- unbekannte Fehlercodes innerhalb derselben API-Hauptversion nicht automatisch als Fehler werten, wenn der Status das erwartete Verhalten erfüllt
- Unterscheidung zwischen `503 broker_unavailable` und `504 broker_timeout`

`503 broker_unavailable` und `504 broker_timeout` werden unterscheidbar dargestellt. Version 1 wiederholt keine Aufrufe automatisch. Für spätere mutierende Tests gilt bei `504 broker_timeout`: Zustand erneut lesen, niemals blind wiederholen.

## 6. Sicherheits- und Testmodi

### 6.1 Version 1 – strikt Read-only

Version 1 erlaubt ausschließlich lesende Aufrufe:

- `GET /healthz`
- `GET /api/v1/info`
- Export und lesende Einzelabfragen
- Authentifizierungs- und `read`-Berechtigungsprüfungen
- Prüfung von Response-Format, Headern und Schemas

Schreiboperationen sind serverseitig gesperrt. Auch negative Tests, die einen schreibenden Endpunkt aufrufen würden, sind ausgeschlossen. Lesende Negativtests (z. B. fehlendes oder ungültiges Token sowie nicht vorhandene Ressource) sind zulässig. Nicht ausführbare Prüfungen erhalten `NOT TESTED` oder `BLOCKED`, niemals `PASS`.

### 6.2 Schreibende Konformitätstests – außerhalb von Version 1

Schreibende Prüfungen gehören nicht zu Version 1. Ein späterer Conformance Test Mode erfordert vor der Implementierung ein eigenes Sicherheitsdesign und darf nur in einer ausdrücklich freigegebenen Testumgebung ausgeführt werden. Mindestens festzulegen sind:

- explizite Aktivierung und Bestätigung durch den Benutzer
- deutliche Kennzeichnung des Zielsystems
- technische Absicherung gegen produktive Ziele
- Verwendung eindeutig gekennzeichneter Testobjekte
- Bereinigung nach den Tests
- Behandlung von Teilfehlern und unbekanntem Zustand

Schreibtests werden nicht stillschweigend aktiviert oder aus der Version-1-Oberfläche erreichbar gemacht.

## 7. Geplante Testbereiche

Die folgende Liste bildet den geplanten Umfang ab. Die genaue Zuordnung zu einzelnen API-Operationen wird bei der Implementierung anhand der v1.0-Spezifikation und `api/openapi.yaml` erstellt.

| Testbereich | Inhalt | Priorität |
|---|---|---|
| Erreichbarkeit | `/healthz`, Verbindung und Timeout | Prototyp |
| API-Information | `/api/v1/info`, API- und Spezifikationsversion, Limits | Prototyp |
| Authentifizierung | fehlendes, ungültiges und gültiges Token | Prototyp |
| Autorisierung | Vorhandensein von `read`; fehlender Scope als `BLOCKED` | Prototyp |
| Lesende Core-Endpunkte | Export und verfügbare GET-Operationen | Prototyp |
| Response-Konformität | Status, Header, JSON-Struktur und Problem Details | Prototyp |
| ETag | Vorhandensein und Format in lesenden Antworten | Version 1 |
| Pfadkodierung | URL-kodierte Pfadparameter bei GET-Aufrufen | Version 1 |
| PMSS Extra | separate, optionale Prüfung lesender Erweiterungen | Version 1 / optional |
| Validierung | unbekannte Felder (`422`), ungültige Schreibwerte und Request-Limits (`413`) | später / Schreibtestmodus |
| If-Match | fehlender Header (`428`), falscher ETag (`412`), gültiger ETag | später / Schreibtestmodus |
| Passwortverhalten | keine Rückgabe übermittelter Passwörter | später / Schreibtestmodus |
| Beziehungen | Gruppenmitgliedschaften und Rollen-/ACL-Zuordnungen durch Änderungen | später / Schreibtestmodus |
| Brokerfehler | Darstellung von `503` und `504`; keine Simulation oder automatische Wiederholung | später / kontrollierte Umgebung |

## 8. Umsetzungsschritte

### Phase 1 – Projektgrundlage

- [x] Neues Go-Projekt und Repository-Struktur anlegen
- [x] `net/http` / `ServeMux` als HTTP-Grundlage einrichten
- [x] `templ` und HTMX integrieren
- [x] TasmoFleet-nahe Layout- und Navigationsstruktur erstellen
- [x] Statische Assets in das Binary einbetten
- [x] Docker Multi-Stage-Build erstellen
- [x] Eigenen Health-Endpunkt des Testers und grundlegende Konfiguration implementieren
- [x] Eingebettete OpenAPI-Spezifikation beim Start validieren und bei Fehlern fail-fast beenden

**Ergebnis:** Startfähiger Docker-Prototyp mit Grundgerüst der Oberfläche, geprüftem Konfigurationsstart und eigenem Health-Endpunkt.

### Phase 2 – Keycloak-Anmeldung

- [ ] Keycloak-Client für den Tester konfigurieren
- [ ] Authorization Code Flow mit PKCE implementieren
- [ ] BFF-Session und sichere Cookies einrichten
- [ ] Tokens ausschließlich serverseitig halten
- [ ] Logout, Session-Ablauf und CSRF-Schutz für zustandsändernde UI-Aktionen behandeln
- [ ] Serverseitigen Token Exchange nach RFC 8693 implementieren
- [ ] Audience zielsystemspezifisch konfigurierbar machen (PMSS: `phylax-mqtt-security-service`)
- [ ] Ausschließlich `read` anfordern und zurückgegebene Scopes auswerten
- [ ] Fehlerfälle beim Token Exchange sichtbar darstellen

**Ergebnis:** Benutzer kann sich anmelden; das Backend kann serverseitig ein benutzerbezogenes `read`-Token für das konfigurierte API-Ziel beziehen.

### Phase 3 – API Explorer

- [ ] PMSS-Ziel über HTTPS-Basis-URL konfigurieren
- [ ] Eingebettete OpenAPI-Datei laden und ausschließlich lesende Operationen zur Ausführung freigeben
- [ ] Core API und PMSS Extra getrennt kennzeichnen
- [ ] GET-Aufrufe serverseitig ausführen und andere Methoden serverseitig als `BLOCKED` abweisen
- [ ] GET-Parameter erfassen und Request-/Response-Schemas darstellen
- [ ] Status, Header, Body und Laufzeit darstellen
- [ ] Fehlerantworten und fehlende `read`-Berechtigung verständlich anzeigen

**Ergebnis:** Manuelle, nachvollziehbare und strikt lesende API-Aufrufe gegen PMSS.

### Phase 4 – Erste automatisierte Prüfungen

- [ ] Erreichbarkeit und `/healthz` prüfen
- [ ] `/api/v1/info` und Versionsangaben prüfen
- [ ] Authentifizierung testen
- [ ] `read`-Berechtigung prüfen; fehlendes `read` als `BLOCKED` ausweisen
- [ ] ausgewählte GET-Endpunkte ausführen
- [ ] Response-Status, Header und JSON-Struktur prüfen
- [ ] RFC-9457-Problem-Details und `503`/`504` unterscheidbar auswerten
- [ ] ETag-Header und URL-kodierte GET-Pfadparameter prüfen
- [ ] Ergebnisse mit `PASS`, `FAIL`, `NOT TESTED` und `BLOCKED` anzeigen

**Ergebnis:** Erste reproduzierbare Core-Prüfungen im strikt lesenden Version-1-Modus.

### Phase 5 – Qualitätssicherung und Bereitstellung

- [ ] Unit-Tests für Konfiguration, OpenAPI-Validierung und HTTP-Handler
- [ ] tabellarische Unit-Tests für Statusauswertung, Scopes, Header, ETags,
      RFC-9457-Problem-Details und `PASS`/`FAIL`/`BLOCKED`/`NOT TESTED`
- [ ] HTTP-Szenarien mit `httptest.Server` und `httptest.ResponseRecorder` abdecken
- [ ] Read-only-Sperre sowie Core-/PMSS-Extra-Trennung testen
- [ ] Integrationstests für die vollständige Backend- und HTTP-Verarbeitung
- [ ] Playwright-E2E-Tests für Login, API Explorer und zentrale Workflows
- [ ] begrenzte visuelle Regressionstests nach TasmoFleet-Konventionen
- [ ] Docker-Build und Laufzeittest
- [ ] Dokumentation der Konfiguration und des Betriebs
- [ ] dokumentierten Testlauf gegen eine PMSS-Referenzimplementierung durchführen

**Ergebnis:** Reproduzierbar baubares und testbares Release des API Testers.

### Ausblick – spätere Erweiterungen (nicht Version 1)

- [ ] Eigenständiges Sicherheitsdesign und ausdrückliche Freigabe für einen Conformance Test Mode erstellen
- [ ] Testziel-Absicherung, Testobjekte, Bereinigung und Behandlung unbekannter Zustände festlegen
- [ ] Schreibtests für `If-Match`, Validierung, Request-Limits, Passwörter und Beziehungen implementieren
- [ ] PMSS-Extra-Prüfungen unabhängig von Core-Konformität erweitern

**Ergebnis:** Nur nach separater Freigabe: kontrollierte Erweiterung über den Read-only-Umfang von Version 1 hinaus.

## 9. Offene Entscheidungen

Vor bzw. während der jeweiligen Phase sind noch folgende Punkte konkret festzulegen:

1. **Keycloak-Client:** Redirect-URIs, Client-Authentifizierung, Token-Exchange-Berechtigungen und Secret-Verwaltung.
2. **Scopes und Rollen:** Zuordnung in Keycloak verifizieren; das ausgegebene `read`-Scope ist maßgeblich, fehlendes `read` wird `BLOCKED`.
3. **Zielsystem-Konfiguration:** ein oder mehrere Ziele, Ablage der Konfiguration und Absicherung der Basis-URL; UI-Überschreibungen sind nicht für Version 1 festgelegt.
4. **Timeouts:** den vorgeschlagenen Standard von `10s` sowie getrennte Behandlung von Verbindungs- und Antwort-Timeouts verifizieren.
5. **Session:** Laufzeit, Refresh-Verhalten und Logout konkretisieren.
6. **Ergebnisexport:** ob Testergebnisse später als JSON oder Markdown exportiert werden sollen.
7. **Spätere Schreibtests:** Sicherheitsdesign, Aktivierung, Testobjekte und Bereinigung vor einem zukünftigen Conformance Test Mode festlegen.

Offene Entscheidungen werden nicht durch Annahmen ersetzt, sondern vor der betroffenen Implementierungsphase geklärt.

## 10. Abnahmekriterien für den ersten Prototyp

Der erste Prototyp gilt als erreicht, wenn:

- der Tester als einzelner Docker-Container startet,
- die Oberfläche die TasmoFleet-Gestaltung aufgreift,
- ein Benutzer sich über Keycloak anmelden kann,
- das Backend einen Token Exchange mit Benutzeridentität durchführen kann,
- ausschließlich lesende PMSS-Aufrufe möglich sind und andere Methoden serverseitig als `BLOCKED` abgewiesen werden,
- der Token Exchange für PMSS `read` und die Audience `phylax-mqtt-security-service` verwendet,
- ein konfiguriertes PMSS-Ziel erreichbar ist,
- ausgewählte GET-Endpunkte über den API Explorer aufgerufen werden können,
- die eingebettete OpenAPI-Spezifikation beim Start validiert wird,
- HTTP-Status, Header und Response-Body sichtbar sind,
- erste Authentifizierungs-, Berechtigungs- und Response-Prüfungen ausführbar sind,
- Testergebnisse nachvollziehbar als `PASS`, `FAIL`, `NOT TESTED` oder `BLOCKED` erscheinen,
- die vorhandenen Unit- und Integrationstests erfolgreich ausführbar sind,
- mutierende Aufrufe auch bei direkter Anfrage an das Backend nicht ausgeführt werden können.

---

**Leitlinie:** Version 1 bleibt strikt Read-only: zuerst ein sicherer, benutzerbezogener API Explorer, darauf aufbauend automatisierte lesende Core-Prüfungen. Schreibende Konformitätstests sind spätere, separat freizugebende Erweiterungen und kein Bestandteil der Version-1-Abnahme.
