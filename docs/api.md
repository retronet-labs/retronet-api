# API HTTP

## Health

```http
GET /health
```

Risposta:

```json
{"service":"retronet-api","status":"ok"}
```

## CORS Locale

La CLI abilita CORS per le origini locali usate da `retronet-ui`:

- `http://127.0.0.1:18081`
- `http://localhost:18081`

Questo permette alla UI browser di creare sessioni e parlare con l'API da una
porta diversa. Le origini sono configurabili:

```powershell
go run ./cmd/retronet-api -cors-origin "http://127.0.0.1:18081"
```

Per disabilitare CORS:

```powershell
go run ./cmd/retronet-api -cors-origin ""
```

Il server non usa wildcard di default. `*` e' supportato solo se impostato
esplicitamente, ed e' pensato per prove locali controllate.

## Versione

```http
GET /version
```

## Crea Sessione

```http
POST /sessions
```

Il server crea un drive temporaneo confinato. Il client non passa path host.

Risposta:

```json
{
  "id": "f3...",
  "created_at": "2026-06-28T12:00:00Z",
  "expires_at": "2026-06-28T12:30:00Z",
  "closed": false,
  "state": "idle",
  "last_error": ""
}
```

`state` puo' essere:

- `idle`: shell pronta a ricevere comandi
- `running`: un comando o programma e' in esecuzione
- `closed`: sessione chiusa
- `error`: ultimo comando terminato con errore recuperabile

## Stato Sessione

```http
GET /sessions/{id}
```

## Cancella Sessione

```http
DELETE /sessions/{id}
```

Chiude la sessione e rimuove la directory temporanea.

## Esegui Comando

```http
POST /sessions/{id}/command
Content-Type: application/json

{"command":"HELP"}
```

Risposta:

```json
{
  "output": "DIR  TYPE <file> ...",
  "snapshot": {},
  "closed": false
}
```

`command` e' sincrono: la risposta arriva quando il comando e' finito. Resta
utile per test, script brevi e controlli automatici. `EXIT` chiude la sessione
CP/M-like.

## Avvia Comando Asincrono

```http
POST /sessions/{id}/run
Content-Type: application/json

{"command":"RUN HELLO"}
```

Risposta `202 Accepted`:

```json
{
  "accepted": true,
  "output": "A>",
  "snapshot": {},
  "closed": false,
  "state": "running"
}
```

`run` mette la sessione in stato `running` e ritorna subito. Serve per terminali
e UI: mentre il programma gira, il client puo' inviare input e leggere output.
Se la sessione e' gia `running`, il server risponde `409 Conflict`.

## Invia Input

```http
POST /sessions/{id}/input
Content-Type: application/json

{"data":"DIR\r"}
```

Quando la sessione e' `idle`, l'input passa dal line editor minimale della shell:
echo dei caratteri, Backspace, Invio, `Ctrl+L` e chiusura con `Ctrl+C`,
`Ctrl+D` o `Ctrl+Q`. Quando la sessione e' `running`, i byte vengono accodati al
terminale della sessione e possono essere letti dal programma CP/M-like tramite
BDOS console.

## Leggi Output

```http
GET /sessions/{id}/output
```

Restituisce e svuota il buffer output della sessione:

```json
{
  "output": "DIR  TYPE <file> ...",
  "snapshot": {},
  "closed": false,
  "state": "idle"
}
```

Nota: `output` e WebSocket condividono lo stesso buffer di terminale. In pratica
un client interattivo dovrebbe scegliere un solo consumatore dell'output per
sessione.
