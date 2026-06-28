# Release v0.2.0

Seconda release di `retronet-api`, focalizzata sulle sessioni interattive.

## Novita'

- stato sessione esplicito: `idle`, `running`, `closed`, `error`
- `POST /sessions/{id}/run` per avviare comandi in modo asincrono
- `POST /sessions/{id}/input` per inviare byte al terminale della sessione
- `GET /sessions/{id}/output` per leggere e svuotare l'output raw
- WebSocket con messaggi `accepted`, `state`, `output` e `snapshot`
- polling WebSocket interno per inviare output mentre un comando gira
- protezione da comandi concorrenti nella stessa sessione (`409 Conflict`)
- conformance aggiornata per coprire anche il percorso asincrono

## Come Usarla

Avvio server:

```powershell
go run ./cmd/retronet-api -addr 127.0.0.1:8080
```

Creazione sessione e comando asincrono:

```powershell
$s = Invoke-RestMethod -Method Post http://127.0.0.1:8080/sessions
Invoke-RestMethod -Method Post "http://127.0.0.1:8080/sessions/$($s.id)/run" `
  -ContentType application/json `
  -Body '{"command":"HELP"}'
Invoke-RestMethod "http://127.0.0.1:8080/sessions/$($s.id)/output"
```

Input stile tastiera:

```powershell
Invoke-RestMethod -Method Post "http://127.0.0.1:8080/sessions/$($s.id)/input" `
  -ContentType application/json `
  -Body '{"data":"DIR\r"}'
```

## Nota Didattica

`command` e `run` non sono duplicati:

- `command` e' sincrono: semplice per test e automazioni brevi
- `run` e' asincrono: adatto a terminali e UI, perche' il client puo' continuare
  a inviare input mentre il programma e' in esecuzione

Il modello rimane CP/M-like: nessun CP/M storico, nessun BDOS/BIOS originale e
nessuna immagine disco storica vengono redistribuiti.

## Limiti

- niente autenticazione/TLS
- niente upload file
- il drive resta temporaneo e confinato
- un solo consumatore dovrebbe leggere l'output di una sessione
- WebSocket minimale: niente frame frammentati

## Verifica

```powershell
gofmt -l .
go vet ./...
go test -count=1 ./...
go run ./cmd/retronet-api -conformance
git diff --check
```
