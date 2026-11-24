# PMSS API Tester – Design Document

**Version:** 0.3 (Zwischenstand)
**Stand:** 24. November 2026
**Status:** Architekturentscheidungen für Version 1; ausgewählte Betriebs- und Keycloak-Details noch offen

---

## 1. Zielsetzung

Der PMSS API Tester ist ein funktionaler API- und Konformitätstester für die REST-API des MQTT Security Service (PMSS).

Ziele:

- Manuelle API-Aufrufe gegen ein PMSS-Zielsystem ermöglichen.
- API-Antworten technisch prüfen und darstellen.
- Die Schnittstelle anhand der integrierten OpenAPI-Spezifikation validieren.
- Definierte automatisierte Testfälle ausführen.
- Ergebnisse nachvollziehbar darstellen.

Die Architektur soll grundsätzlich generisch sein. Die erste konkrete Zielimplementierung ist Phylax.

Der Tester soll langfristig auch externen Implementierern helfen. In Version 1 liegt der Schwerpunkt auf der Entwicklung und Prüfung der ersten PMSS-Implementierung.

### 1.1 Abgrenzung

Der Tester ist ausdrücklich:

- kein MQTT-Client,
- keine eigenständige ACL-Verwaltung,
- keine Implementierung des PMSS,
- keine produktive Administrationsoberfläche.

---

## 2. Spezifikationsbasis

Die PMSS-Spezifikation besteht aus einer normativen Markdown-Spezifikation und einer maschinenlesbaren OpenAPI-Beschreibung.

Festlegungen:

- Die Markdown-Spezifikation ist normativ.
- Die OpenAPI-Datei beschreibt die technische Schnittstelle.
- Bei Widersprüchen hat die Markdown-Spezifikation Vorrang.
- Die OpenAPI-Datei wird fest in die Anwendung integriert und nicht über die UI importiert.
- Die API-Version bestimmt die zugehörige OpenAPI-Version. Der verwendete Spezifikationsstand wird gemeinsam mit der API-Version dokumentiert.
- Core Conformance und Implementation Extensions (`x-pmss-extra: true`) werden getrennt ausgewiesen.
- Fachliche Prüfregeln werden unabhängig von den OpenAPI-Schemas gepflegt.

Im Repository liegen die normative Markdown-Spezifikation `docs/mqtt-security-service-api-v1.0.md` (Version 1.0, Stand 23. November 2026) und die zugehörige `api/openapi.yaml` (OpenAPI 3.2.1, API-Version 1.0) vor. Beide Artefakte sind in die Implementierung einzubeziehen; normative Verhaltensregeln werden gegen die Markdown-Spezifikation geprüft.

### 2.1 Konformitätsstatus

Ein Testergebnis verwendet einen der folgenden Status:

- `PASS`
- `FAIL`
- `NOT TESTED`
- `BLOCKED`

Nicht ausgeführte oder nicht sinnvoll prüfbare Fälle dürfen nicht als `PASS` ausgewiesen werden.

---

## 3. Umfang von Version 1

Version 1 umfasst:

- manuelle API-Aufrufe,
- benutzerbezogene Bearer-Token-Authentifizierung über einen serverseitigen Token Exchange nach RFC 8693,
- Anzeige von HTTP-Status, Response-Headern und Response-Body,
- OpenAPI-Schema-Prüfungen,
- einfache, in der Anwendung definierte automatisierte Testfälle,
- Anzeige der Testergebnisse.

Testergebnisse werden nur während der laufenden Sitzung im Arbeitsspeicher gehalten. Eine dauerhafte Speicherung ist in Version 1 nicht vorgesehen.

Nicht Bestandteil von Version 1 sind:

- komplexe Szenario-Engine,
- CI/CD-Integration,
- Mehrbenutzerverwaltung im Tester,
- umfangreiches Reporting oder historische Auswertungen,
- dynamischer Import von OpenAPI-Spezifikationen,
- parallele Unterstützung mehrerer API-Versionen,
- automatische Testdatenverwaltung,
- Broker-Fehlersimulation,
- Nebenläufigkeits- und Belastungstests.

---

## 4. Architektur

### 4.1 Grundarchitektur

Der Tester besteht aus einer Weboberfläche und einem eigenen Go-Backend. Das Backend liefert die UI aus und führt die PMSS-API-Aufrufe aus.

Festgelegter Technologie-Stack:

- Go mit `net/http` und `ServeMux`,
- `templ` für HTML-Komponenten,
- HTMX 2.x für die dynamische Bedienung,
- bei Bedarf `htmx-ext-sse`,
- CSS und statische Assets nach den TasmoFleet-GUI-Guidelines,
- Playwright für End-to-End-Tests,
- `pixelmatch` und `pngjs` für visuelle Regressionstests.

React und TypeScript werden nicht eingesetzt.

### 4.2 Produktfamilie und UI

Die Oberfläche soll sich eng am Look & Feel von TasmoFleet orientieren und als weiteres Modul derselben Produktfamilie wirken.

Übernommen werden sollen insbesondere:

- bestehende GUI-Guidelines und Design-Tokens,
- vorhandene UI-Komponenten und Gestaltungsprinzipien,
- Light- und Dark-Mode,
- der etablierte Build- und Asset-Einbettungsmechanismus.

### 4.3 Deployment

- Ausschließlich Docker.
- Ein Docker-Container für die Anwendung.
- Multi-Stage-Build.
- Das finale Image enthält das Go-Binary und die eingebetteten statischen Assets.
- Keine separate Datenbank.
- Laufzeitdaten und Testergebnisse werden nur im Arbeitsspeicher gehalten.

---

## 5. Funktionsbereiche

### 5.1 Konfiguration und Zielsystem

Die API-Basis-URL wird über eine Umgebungsvariable vorgegeben. Ein Überschreiben in der UI ist für Version 1 noch nicht festgelegt; falls es ermöglicht wird, gilt es nur für die laufende Sitzung und muss gegen unzulässige Ziele abgesichert werden.

Vorgesehene Umgebungsvariablen:

```env
PMSS_API_BASE_URL=https://pmss.example.org
PMSS_HTTP_TIMEOUT=10s

OIDC_ISSUER_URL=https://keycloak.example.org/realms/Tasmofleet
OIDC_CLIENT_ID=mqtt-security-api-tester
OIDC_CLIENT_SECRET=<secret>

TZ=Europe/Vienna
```

Hinweise:

- `PMSS_API_BASE_URL` ist die HTTPS-Basis-URL des zu testenden PMSS-Systems.
- `PMSS_HTTP_TIMEOUT` konfiguriert das HTTP-Timeout. `10s` ist ein Vorschlagswert und noch kein abschließend bestätigter Grenzwert.
- `OIDC_ISSUER_URL` verweist auf den bestehenden Keycloak-Realm `Tasmofleet`.
- `OIDC_CLIENT_ID` bezeichnet den eigenen OIDC-Client des Testers.
- `OIDC_CLIENT_SECRET` ist ein vertraulicher Konfigurationswert und darf weder in der UI noch in Logs erscheinen.
- `TZ` steuert die lokale Darstellung von Zeitangaben. Ohne Angabe gilt UTC.

Die TLS-Zertifikatsprüfung ist standardmäßig aktiviert.

### 5.2 API Explorer

Der API Explorer wird automatisch aus der integrierten OpenAPI-Spezifikation erzeugt.

Funktionen:

- Endpunkte nach HTTP-Methode und Ressource gruppieren,
- GET-Parameter dynamisch erfassen und Request-Bodies anhand der OpenAPI-Spezifikation darstellen,
- API-Aufrufe direkt aus der UI ausführen,
- in Version 1 ausschließlich `GET`-Operationen ausführen; andere Methoden serverseitig als `BLOCKED` behandeln,
- HTTP-Status, Header und Response-Body anzeigen,
- Request- und Response-Schemas prüfen.

### 5.3 Automatisierte Testfälle

- Testfälle sind fest in der Anwendung definiert.
- Die UI zeigt die verfügbaren Testfälle an.
- Einzelne Testfälle können gezielt ausgeführt werden.
- Testfälle werden in Version 1 nicht über die UI erstellt oder bearbeitet.
- Testfälle referenzieren die zugehörigen OpenAPI-Operationen, bleiben aber fachlich unabhängig von der OpenAPI-Datei.

### 5.4 Testergebnisse

Ein Ergebnis enthält mindestens:

- Test-ID und Beschreibung,
- Status,
- erwartetes und tatsächliches Ergebnis,
- Details bzw. Fehlerbeschreibung,
- Ausführungsdauer.

Der vollständige HTTP-Request und die HTTP-Response können bei Bedarf aufgeklappt werden. Vertrauliche Informationen wie Tokens und Passwörter werden dabei nicht offengelegt.

### 5.5 Dashboard

Das Dashboard bleibt bewusst einfach und zeigt:

- ausgewähltes Zielsystem,
- Verbindungsstatus,
- API- und Spezifikationsversion,
- letzte Testausführung,
- Verteilung der Testergebnisse.

Historische Auswertungen und umfangreiche Diagramme sind nicht vorgesehen.

---

## 6. Testkonzept

Version 1 konzentriert sich auf positive und negative API-Prüfungen.

### 6.1 Testbereiche

1. **Authentifizierung und Berechtigungen**
   - gültiges, für das Zielsystem ausgestelltes Token,
   - fehlendes Token,
   - ungültiges oder abgelaufenes Token,
   - Vorhandensein des erforderlichen `read`-Scopes.

2. **Lesende GET-Endpunkte**
   - alle lesenden GET-Endpunkte der integrierten OpenAPI-Spezifikation,
   - soweit sie ohne vorherige Zustandsänderung sinnvoll und sicher prüfbar sind,
   - beispielsweise Info, Export, Clients, Groups, Roles und Einzelressourcen.

3. **Fehlerbehandlung lesender Aufrufe**
   - ungültige Parameter und nicht vorhandene Ressourcen bei lesenden Aufrufen,
   - erwartetes 404-Verhalten,
   - RFC-9457-Problem-Details, insbesondere bei Authentifizierungs- und Berechtigungsfehlern.

   Prüfungen von Request-Bodies, unbekannten Feldern (422) und Referenzen setzen schreibende Aufrufe voraus und sind daher nicht Bestandteil von Version 1.

4. **ETag-Prüfungen**
   - ETag-Format und Header bei GET-Antworten prüfen.
   - Keine Änderungen mit `If-Match` ausführen.

5. **Broker-Fehler**
   - keine Broker-Fehlersimulation in Version 1.

### 6.2 Sicherheitsgrenze für Tests

Version 1 ist strikt Read-only. Schreiboperationen werden serverseitig abgewiesen; eine bloße Deaktivierung in der UI reicht nicht aus. Auch negative Tests rufen in Version 1 keine schreibenden PMSS-Endpunkte auf. Solche Tests benötigen eine spätere, ausdrücklich freizugebende Conformance-Testumgebung sowie ein eigenes Sicherheitsdesign.

Lesende Negativtests (z. B. fehlendes oder ungültiges Token und nicht vorhandene Ressource) sind zulässig, sofern sie keine Zustandsänderung auslösen.

Nicht sinnvoll oder nicht sicher ausführbare Fälle erhalten `NOT TESTED` oder `BLOCKED`, niemals `PASS`.

---

## 7. OpenAPI-Integration

- Die OpenAPI-Spezifikation wird beim Start geladen und im Speicher gehalten.
- Bei fehlerhafter oder nicht zur Anwendung passender Spezifikation startet die Anwendung nicht unvollständig weiter.
- Request- und Response-Schema-Validierung werden getrennt behandelt.
- Ein gezielter negativer Test darf nicht pauschal durch eine vorgelagerte Request-Schema-Prüfung verhindert werden.
- Fachliche Testfälle werden separat von der OpenAPI-Spezifikation gepflegt und referenzieren deren Operationen.
- `x-pmss-extra: true` wird ausgewertet, damit Core-Funktionen und Implementation Extensions getrennt dargestellt und bewertet werden können.

---

## 8. Sicherheit und Datenschutz

Festlegungen:

- Access Tokens werden ausschließlich serverseitig im Arbeitsspeicher gehalten.
- Tokens, Passwörter und Secrets werden niemals protokolliert.
- TLS-Zertifikatsprüfung ist standardmäßig aktiv.
- Der Tester arbeitet grundsätzlich im Read-only-Modus.
- Das Backend kommuniziert mit Keycloak und dem konfigurierten PMSS-Zielsystem.
- Logs werden als JSON ausschließlich über Docker stdout/stderr ausgegeben.
- Der Phylax Syslog Service wird nicht eingebunden; eine spätere Erweiterung bleibt möglich.
- Die Zeitzone wird über `TZ` konfiguriert. API-Zeitstempel bleiben gemäß Spezifikation UTC.

Logs dürfen keine Tokens, Passwörter oder sensiblen Request- bzw. Response-Inhalte enthalten.

---

## 9. Authentifizierung und Keycloak

### 9.1 Identity Provider

Keycloak ist der zentrale Identity Provider. Der Tester erhält keine eigene Benutzer- oder Passwortverwaltung.

Verwendet wird der bereits bestehende Realm:

`Tasmofleet`

Damit kann auch das bestehende TasmoFleet-Login-Theme verwendet werden. Ein eigener Realm für den PMSS API Tester wird nicht angelegt.

### 9.2 OIDC-Client

Für den Tester wird ein eigener Keycloak-Client vorgesehen:

`mqtt-security-api-tester`

Der Client wird als serverseitiger Web-Client (Confidential Client) betrieben. Das Client Secret wird ausschließlich dem Go-Backend bereitgestellt und sicher konfiguriert.

Der Login erfolgt über den OpenID Connect Authorization Code Flow mit PKCE.

### 9.3 Serverseitige Session- und Token-Verwaltung

Das Go-Backend übernimmt die Rolle eines Backend-for-Frontend (BFF):

- Das Backend startet und verarbeitet den OIDC-Login.
- Das Backend tauscht den Authorization Code gegen Tokens ein.
- Das dabei erhaltene Benutzer-Access-Token wird ausschließlich serverseitig im Arbeitsspeicher gehalten und als `subject_token` für den PMSS-Token-Exchange verwendet.
- Der Browser erhält kein Access Token.
- Der Browser erhält ein sicheres Session-Cookie mit `HttpOnly`, `Secure` und geeigneter `SameSite`-Einstellung.
- Das Backend verwendet ausschließlich das für PMSS ausgetauschte Token für PMSS-Aufrufe.

Die konkrete Session-Laufzeit, Token-Erneuerung und das Logout-Verhalten werden noch festgelegt. UI-Aktionen, die eine Session verändern, müssen gegen CSRF geschützt sein.

### 9.4 Benutzerbezogener Token Exchange für PMSS-Aufrufe

Nach dem Login tauscht das Backend das Benutzer-Access-Token serverseitig nach RFC 8693 gegen ein Token für das PMSS-Zielsystem aus. Dabei verwendet es das Benutzer-Token als `subject_token`, fordert den Token-Typ Access Token an und authentifiziert den Tester-Client gegenüber Keycloak. Der Austausch bewahrt die Benutzeridentität; ein Service-Account-Token darf das Benutzer-Token nicht ersetzen.

Das ausgetauschte Token wird serverseitig für die API-Aufrufe verwendet. PMSS prüft dessen Scopes; der Tester leitet Berechtigungen nicht ersatzweise aus lokal interpretierten Benutzerrollen ab.

### 9.5 Scopes und Audience

Für die Referenzimplementierung PMSS fordert der Token Exchange die in der Spezifikation angegebene Audience `phylax-mqtt-security-service` an. Bei anderen konformen Zielsystemen ist die Audience zielsystemspezifisch zu konfigurieren.

Version 1 fordert ausschließlich den Scope `read` an und führt keine schreibenden API-Aufrufe aus. Der vom Token Exchange tatsächlich zurückgegebene Scope ist maßgeblich; ein fehlendes `read` wird als fehlende Berechtigung (`BLOCKED`) sichtbar gemacht. `write` ist nicht Bestandteil des Betriebsumfangs von Version 1.

---

## 10. Noch offene Detailentscheidungen

Die grundsätzliche Architektur und der Read-only-Umfang von Version 1 sind festgelegt. Folgende Implementierungsdetails müssen noch konkretisiert werden:

1. **Keycloak-Client:** Redirect-URI, Client-Authentifizierung, erforderliche Berechtigungen für RFC 8693 und Secret-Verwaltung.
2. **Scopes und Benutzer ohne `read`:** Keycloak-Rollen-/Scope-Zuordnung für PMSS verifizieren; fehlendes `read` muss als `BLOCKED` dargestellt werden.
3. **Token-Lebenszyklus:** Refresh-Token-Verwendung, Ablaufbehandlung und Logout.
4. **Session-Sicherheit:** Session-Laufzeit, Cookie-Parameter und konkrete CSRF-Maßnahmen.
5. **Zielsystem-Konfiguration:** Version-1-Umfang für ein oder mehrere Ziele sowie Schutzregeln für Basis-URLs und optionale sitzungsbezogene Überschreibungen festlegen.
6. **HTTP-Timeout:** Den vorgeschlagenen Default von `10s` anhand der API-Aufrufe verifizieren.
7. **Schreibtests:** Außerhalb von Version 1; vor einem späteren Conformance Test Mode sind Testumgebung, Freigabe, Testdaten und Bereinigung gesondert festzulegen.

---

## 11. Zusammenfassung der Architekturentscheidungen

| Bereich | Entscheidung |
|---|---|
| Zweck | Funktionaler API- und Konformitätstester |
| Erste Zielimplementierung | Phylax |
| Normative Grundlage | Markdown-Spezifikation |
| Technische Schnittstellenbeschreibung | Integrierte OpenAPI-Spezifikation |
| Backend | Go `net/http` / `ServeMux` |
| UI | `templ` und HTMX 2.x |
| UI-Design | TasmoFleet-GUI-Guidelines |
| Browser-Framework | Kein React |
| Deployment | Docker, ein Container |
| Build | Multi-Stage, Go-Binary mit eingebetteten Assets |
| Datenbank | Keine |
| Testergebnisse | Nur im Arbeitsspeicher |
| Identity Provider | Keycloak |
| Keycloak-Realm | `Tasmofleet` |
| OIDC-Client | `mqtt-security-api-tester` |
| OIDC-Flow | Authorization Code Flow mit PKCE |
| Client-Typ | Confidential Client, serverseitig |
| PMSS-Authentifizierung | RFC-8693-Token-Exchange mit Benutzeridentität |
| PMSS-Audience (Referenzimplementierung) | `phylax-mqtt-security-service` |
| PMSS-Berechtigung in Version 1 | ausschließlich `read` |
| Betriebsmodus | Read-only |
| Logging | JSON über Docker stdout/stderr |

**Hinweis zum Status:** Dieses Dokument beschreibt Architekturentscheidungen und offene Implementierungsdetails, nicht bereits implementierte Eigenschaften. Die normative Grundlage und die OpenAPI-Beschreibung liegen im Repository unter `docs/mqtt-security-service-api-v1.0.md` und `api/openapi.yaml`.
