# API HTTP

## Health

```http
GET /health
```

Risposta:

```json
{"service":"retronet-api","status":"ok"}
```

## Versione

```http
GET /version
```

## Crea Sessione

```http
POST /sessions
```

La v0.1 non accetta path host. Il server crea un drive temporaneo confinato.

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

`EXIT` chiude la sessione CP/M-like.
