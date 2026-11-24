# PMSS API Tester – Planung und Umsetzung

**Dokumentstatus:** Arbeitsgrundlage für den ersten Prototyp  
**Version:** 0.1  
**Stand:** 24.11.2026

## 1. Ziel

Der PMSS API Tester ist ein generisches Werkzeug zum Erkunden, Prüfen und Testen von Implementierungen der **MQTT Security Service API**.

Die normative Grundlage ist `mqtt-security-service-api-v1.0.md`. Die Datei `api/openapi.yaml` stellt die maschinenlesbare Beschreibung derselben API bereit. Die Markdown-Spezifikation bleibt maßgeblich, wenn sich Beschreibungen widersprechen.

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
| Testziel | Konfigurierbare MQTT Security Service API, zunächst PMSS |

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
3. Das Backend fordert beim Token Exchange ein Token für die Audience des jeweiligen MQTT Security Service an.
4. Die angeforderten Scopes sind `read` und – sofern der Benutzer dazu berechtigt ist – `write`.
5. Das erhaltene Service-Token wird serverseitig für die API-Aufrufe verwendet.

Die konkrete Audience und die Keycloak-Client-Konfiguration müssen je Testziel konfigurierbar sein. Für PMSS ist die in der Spezifikation genannte Audience zu berücksichtigen.

### 3.3 Rechte und UI-Verhalten

- `read` ist Voraussetzung für lesende API-Aufrufe.
- `write` ist Voraussetzung für mutierende API-Aufrufe.
- `write` impliziert nicht automatisch `read`.
- Die tatsächlich im Token enthaltenen Scopes sind maßgeblich.
- Funktionen ohne erforderliche Berechtigung werden deaktiviert oder als nicht verfügbar angezeigt.
- Fehlende Berechtigungen werden als Testergebnis sichtbar gemacht und nicht als technischer Fehler verschleiert.

## 4. Umgang mit der API-Spezifikation

### 4.1 Normative Quelle und OpenAPI

Im Repository des MQTT Security Service liegen die beiden API-Artefakte getrennt:

- `doc/mqtt-security-service-api-v1.0.md` – normative Spezifikation
- `api/openapi.yaml` – maschinenlesbare OpenAPI-Beschreibung

Der Tester soll beide Artefakte berücksichtigen. OpenAPI kann für die Darstellung von Operationen, Parametern und Schemas verwendet werden. Normative Verhaltensregeln, die in OpenAPI nicht vollständig ausgedrückt sind, müssen zusätzlich anhand der Markdown-Spezifikation implementiert bzw. getestet werden.

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
- Parameter und Request-Body
- verwendete Authentifizierung und Scopes
- HTTP-Status
- Response-Header
- Response-Body
- Laufzeit des Aufrufs

Der Tester muss die für die Spezifikation relevanten Header wie `ETag`, `If-Match`, `Content-Type` und Cache-Control sichtbar machen.

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

Bei `504 broker_timeout` kann der Zustand einer Änderung unbekannt sein. Der Tester darf mutierende Aufrufe daher nicht blind wiederholen. Stattdessen muss der aktuelle Zustand erneut gelesen und angezeigt werden.

## 6. Sicherheits- und Testmodi

### 6.1 Safe Mode – Standard

Der Standardmodus ist zunächst lesend:

- `GET /healthz`
- `GET /api/v1/info`
- Export und lesende Einzelabfragen
- Authentifizierungs- und Berechtigungsprüfungen
- Prüfung von Response-Format, Headern und Schemas

Mutierende Operationen sind im Safe Mode gesperrt.

### 6.2 Conformance Test Mode – späterer Ausbauschritt

Für eine vollständige Konformitätsprüfung sind kontrollierte Schreibtests erforderlich. Diese dürfen nur in einer ausdrücklich dafür vorgesehenen Testumgebung ausgeführt werden.

Vor der Umsetzung dieses Modus sind festzulegen:

- explizite Aktivierung durch den Benutzer
- deutliche Kennzeichnung des Zielsystems
- Verwendung eindeutig gekennzeichneter Testobjekte
- Bereinigung nach den Tests
- Behandlung von Teilfehlern und unbekanntem Zustand
- Schutz vor versehentlichen Tests gegen produktive Systeme

Schreibtests werden nicht stillschweigend im ersten Prototyp aktiviert.

## 7. Geplante Testbereiche

Die folgende Liste bildet den geplanten Umfang ab. Die genaue Zuordnung zu einzelnen API-Operationen wird bei der Implementierung anhand der v1.0-Spezifikation und `api/openapi.yaml` erstellt.

| Testbereich | Inhalt | Priorität |
|---|---|---|
| Erreichbarkeit | `/healthz`, Verbindung und Timeout | Prototyp |
| API-Information | `/api/v1/info`, API- und Spezifikationsversion, Limits | Prototyp |
| Authentifizierung | fehlendes, ungültiges und gültiges Token | Prototyp |
| Autorisierung | `read` und `write`, fehlende Scopes | Prototyp |
| Lesende Core-Endpunkte | Export und verfügbare GET-Operationen | Prototyp |
| Response-Konformität | Status, Header, JSON-Struktur und Problem Details | Prototyp |
| ETag | Vorhandensein und Format bei den spezifizierten Ressourcen | Ausbau |
| If-Match | fehlender Header (`428`), falscher ETag (`412`), gültiger ETag | Conformance Mode |
| Validierung | unbekannte Felder (`422`), ungültige Werte und Request-Limits (`413`) | Ausbau |
| Pfadkodierung | URL-kodierte Pfadparameter | Ausbau |
| Passwortverhalten | keine Rückgabe übermittelter Passwörter, Cache-Control | Conformance Mode |
| Beziehungen | Gruppenmitgliedschaften und Rollen-/ACL-Zuordnungen | Conformance Mode |
| Brokerfehler | Verhalten bei `503` und `504` | später / kontrollierte Umgebung |
| PMSS Extra | separate Prüfung implementierungsspezifischer Erweiterungen | optional |

## 8. Umsetzungsschritte

### Phase 1 – Projektgrundlage

- [ ] Neues Go-Projekt und Repository-Struktur anlegen
- [ ] `net/http` / `ServeMux` als HTTP-Grundlage einrichten
- [ ] `templ` und HTMX integrieren
- [ ] TasmoFleet-nahe Layout- und Navigationsstruktur erstellen
- [ ] Statische Assets in das Binary einbetten
- [ ] Docker Multi-Stage-Build erstellen
- [ ] Health-Endpunkt des Testers und grundlegende Konfiguration implementieren

**Ergebnis:** Startfähiger Docker-Prototyp mit Grundgerüst der Oberfläche.

### Phase 2 – Keycloak-Anmeldung

- [ ] Keycloak-Client für den Tester konfigurieren
- [ ] Authorization Code Flow mit PKCE implementieren
- [ ] BFF-Session und sichere Cookies einrichten
- [ ] Tokens ausschließlich serverseitig halten
- [ ] Logout und Session-Ablauf behandeln
- [ ] Serverseitigen Token Exchange nach RFC 8693 implementieren
- [ ] Audience und Scopes konfigurierbar machen
- [ ] Fehlerfälle beim Token Exchange sichtbar darstellen

**Ergebnis:** Benutzer kann sich anmelden; das Backend kann ein benutzerbezogenes Token für das konfigurierte API-Ziel beziehen.

### Phase 3 – API Explorer

- [ ] Zielsystem konfigurieren (Basis-URL, API-Version, Audience)
- [ ] OpenAPI-Datei laden und Operationen darstellen
- [ ] Core API und PMSS Extra getrennt kennzeichnen
- [ ] API-Aufrufe serverseitig ausführen
- [ ] Request-Parameter und Request-Body erfassen
- [ ] Status, Header, Body und Laufzeit darstellen
- [ ] Fehlerantworten und fehlende Scopes verständlich anzeigen

**Ergebnis:** Manuelle, nachvollziehbare API-Aufrufe gegen PMSS.

### Phase 4 – Erste automatisierte Prüfungen

- [ ] Erreichbarkeit und `/healthz` prüfen
- [ ] `/api/v1/info` und Versionsangaben prüfen
- [ ] Authentifizierung testen
- [ ] `read`- und `write`-Berechtigungen prüfen
- [ ] ausgewählte GET-Endpunkte ausführen
- [ ] Response-Status, Header und JSON-Struktur prüfen
- [ ] Ergebnisse mit `PASS`, `FAIL`, `NOT TESTED` und `BLOCKED` anzeigen

**Ergebnis:** Erste reproduzierbare Core-Prüfungen im Safe Mode.

### Phase 5 – Erweiterte Konformitätsprüfungen

- [ ] ETag-Prüfungen ergänzen
- [ ] `If-Match`-Verhalten prüfen
- [ ] Validierungs- und Fehlerfälle ergänzen
- [ ] URL-Encoding und Request-Limits prüfen
- [ ] Passwort- und Cache-Verhalten prüfen
- [ ] kontrollierten Conformance Test Mode entwerfen und absichern
- [ ] Schreibtests und Bereinigung implementieren
- [ ] PMSS-Extra-Prüfungen separat ergänzen

**Ergebnis:** Erweiterte, kontrollierte Konformitätsprüfung der API v1.0.

### Phase 6 – Qualitätssicherung und Bereitstellung

- [ ] Unit-Tests für API- und Testlogik
- [ ] Playwright-E2E-Tests für Login und zentrale Workflows
- [ ] visuelle Regressionstests nach TasmoFleet-Konventionen
- [ ] Docker-Build und Laufzeittest
- [ ] Dokumentation der Konfiguration und des Betriebs
- [ ] Testlauf gegen PMSS als Referenzimplementierung

**Ergebnis:** Reproduzierbar baubares und testbares Release des API Testers.

## 9. Offene Entscheidungen

Vor bzw. während der jeweiligen Phase sind noch folgende Punkte konkret festzulegen:

1. **Keycloak-Client:** Client-ID, Client-Typ, Redirect-URIs und erforderliche Token-Exchange-Berechtigungen.
2. **Token Exchange:** konkrete Audience und Scope-Anforderung je Zielsystem; Umgang mit Benutzern ohne `security-admin`.
3. **Zielsystem-Konfiguration:** ob mehrere API-Ziele verwaltet werden können und wo Zugangsdaten bzw. Konfiguration abgelegt werden.
4. **OpenAPI-Quelle:** lokale Datei, eingebettete Datei oder konfigurierbare URL; Umgang mit nicht unterstützten OpenAPI-Features.
5. **Timeouts:** Standard-Timeouts für API-Aufrufe und getrennte Behandlung von Verbindungs- und Antwort-Timeouts.
6. **Schreibtests:** Umfang, Testobjekt-Namensschema, Bereinigung und Schutz vor produktiven Zielsystemen.
7. **Ergebnisexport:** ob Testergebnisse später als JSON oder Markdown exportiert werden sollen.

Offene Entscheidungen werden nicht durch Annahmen ersetzt, sondern vor der betroffenen Implementierungsphase geklärt.

## 10. Abnahmekriterien für den ersten Prototyp

Der erste Prototyp gilt als erreicht, wenn:

- der Tester als einzelner Docker-Container startet,
- die Oberfläche die TasmoFleet-Gestaltung aufgreift,
- ein Benutzer sich über Keycloak anmelden kann,
- das Backend einen Token Exchange mit Benutzeridentität durchführen kann,
- ein konfiguriertes PMSS-Ziel erreichbar ist,
- ausgewählte GET-Endpunkte über den API Explorer aufgerufen werden können,
- HTTP-Status, Header und Response-Body sichtbar sind,
- erste Authentifizierungs-, Berechtigungs- und Response-Prüfungen ausführbar sind,
- Testergebnisse nachvollziehbar als `PASS`, `FAIL`, `NOT TESTED` oder `BLOCKED` erscheinen,
- keine mutierenden Tests unbeabsichtigt ausgeführt werden können.

---

**Leitlinie:** Zuerst ein sicherer, benutzerbezogener und gut nachvollziehbarer API Explorer. Darauf aufbauend die automatisierten Core-Prüfungen. Schreibende Konformitätstests folgen erst, wenn Testumgebung, Testdaten und Bereinigung zuverlässig abgesichert sind.
