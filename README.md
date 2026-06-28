# retronet-api

`retronet-api` e' il backend HTTP/WebSocket dell'ecosistema RetroNet. In v0.1
orchestra sessioni CP/M-like sopra `retronet-cpm/session` e
`retronet-terminal`, senza includere ROM, BIOS, BDOS storico o immagini disco.

## Quick Start

```powershell
go test ./...
go run ./cmd/retronet-api -conformance
go run ./cmd/retronet-api -addr 127.0.0.1:8080
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
- `GET /sessions/{id}/ws`

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
