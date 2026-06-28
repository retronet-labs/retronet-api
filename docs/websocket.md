# WebSocket

Endpoint:

```text
GET /sessions/{id}/ws
```

Il protocollo e' JSON testuale.

Messaggi client:

```json
{"type":"command","command":"DIR"}
{"type":"input","data":"HELP\r"}
{"type":"snapshot"}
```

Messaggi server:

```json
{"type":"output","data":"A>HELP\r\n..."}
{"type":"snapshot","snapshot":{}}
{"type":"error","error":"..."}
```

`input` simula una tastiera minimale: echo dei caratteri, Backspace, Invio,
`Ctrl+L` e `Ctrl+Q/Ctrl+C/Ctrl+D`. Come `retronet-cpm-live`, `RUN` resta
sincrono in v0.1.

Il WebSocket usa un'implementazione minimale RFC 6455 scritta con libreria
standard Go. Non supporta frame frammentati; basta per il terminale didattico e
potra' essere sostituita senza toccare il core CP/M.
