# retronet-api

`retronet-api` e' il backend HTTP/WebSocket dell'ecosistema RetroNet. In v0.2
orchestra sessioni CP/M-like sopra `retronet-cpm/session` e
`retronet-terminal`, con stato sessione esplicito e percorso interattivo
`run/input/output`. Non include ROM, BIOS, BDOS storico o immagini disco.

## Quick Start

```powershell
go test ./...
go run ./cmd/retronet-api -conformance
go run ./cmd/retronet-api -addr 127.0.0.1:8080
```

Per usare `retronet-ui` locale il server abilita di default CORS solo per:

- `http://127.0.0.1:18081`
- `http://localhost:18081`

Si puo' cambiare o disabilitare:

```powershell
go run ./cmd/retronet-api -cors-origin ""
go run ./cmd/retronet-api -cors-origin "http://127.0.0.1:18081"
```

Esempio REST:

```powershell
$s = Invoke-RestMethod -Method Post http://127.0.0.1:8080/sessions
Invoke-RestMethod -Method Post http://127.0.0.1:8080/sessions/$($s.id)/command `
  -ContentType application/json `
  -Body '{"command":"HELP"}'
```

Endpoint iniziali:

- `GET /health`
- `GET /version`
- `POST /sessions`
- `GET /sessions/{id}`
- `DELETE /sessions/{id}`
- `POST /sessions/{id}/command`
- `POST /sessions/{id}/run`
- `POST /sessions/{id}/input`
- `GET /sessions/{id}/output`
- `GET /sessions/{id}/ws`

## Sessioni Interattive

`command` resta sincrono ed e' comodo per test e automazioni brevi. Per un
terminale o una futura UI si usa invece il flusso v0.2:

```powershell
$s = Invoke-RestMethod -Method Post http://127.0.0.1:8080/sessions
Invoke-RestMethod -Method Post "http://127.0.0.1:8080/sessions/$($s.id)/run" `
  -ContentType application/json `
  -Body '{"command":"HELP"}'
Invoke-RestMethod "http://127.0.0.1:8080/sessions/$($s.id)/output"
```

Per simulare una tastiera:

```powershell
Invoke-RestMethod -Method Post "http://127.0.0.1:8080/sessions/$($s.id)/input" `
  -ContentType application/json `
  -Body '{"data":"DIR\r"}'
```

Gli stati principali sono `idle`, `running`, `closed` ed `error`.

## Sicurezza

Ogni sessione CP/M-like usa un drive temporaneo confinato. Di default:

- scrittura abilitata solo dentro la directory temporanea della sessione
- limite dimensione file
- limite numero file
- cleanup alla chiusura o scadenza sessione

L'API non accetta path host arbitrari dai client.

## Documentazione

- [Architettura](docs/architettura.md)
- [API HTTP](docs/api.md)
- [WebSocket](docs/websocket.md)
- [Sicurezza](docs/sicurezza.md)
- [Release v0.1.0](docs/release-v0.1.0.md)
- [Release v0.2.0](docs/release-v0.2.0.md)
