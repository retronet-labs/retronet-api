# WebSocket

Endpoint:

```text
GET /sessions/{id}/ws
```

Il protocollo e' JSON testuale.

Messaggi client:

```json
{"type":"command","command":"DIR"}
{"type":"run","command":"RUN HELLO"}
{"type":"input","data":"HELP\r"}
{"type":"output"}
{"type":"snapshot"}
```

Messaggi server:

```json
{"type":"output","data":"A>HELP\r\n..."}
{"type":"accepted","accepted":true,"state":"running"}
{"type":"state","state":"idle","closed":false}
{"type":"snapshot","snapshot":{},"state":"idle"}
{"type":"error","error":"..."}
```

`input` simula una tastiera minimale: echo dei caratteri, Backspace, Invio,
`Ctrl+L` e `Ctrl+Q/Ctrl+C/Ctrl+D` quando la shell e' `idle`. Se la sessione e'
`running`, i byte vengono accodati al terminale della sessione e possono essere
consumati dal programma CP/M-like tramite BDOS console.

## Flusso Consigliato

Per un terminale vero il client apre il WebSocket, legge l'output iniziale `A>`,
poi invia i tasti come messaggi `input`:

```json
{"type":"input","data":"DIR\r"}
```

Quando l'input completa una riga, l'API avvia il comando in background. Il
WebSocket resta libero di ricevere altri byte mentre il comando o il programma e'
in esecuzione. Il server invia:

- `output` quando ci sono byte raw nuovi
- `state` quando la sessione cambia stato
- `snapshot` quando cambia output o stato

`command` resta disponibile per compatibilita' e automazioni brevi: e'
sincrono. `run` e' il messaggio esplicito per avviare un comando asincrono.

Il WebSocket usa un'implementazione minimale RFC 6455 scritta con libreria
standard Go. Non supporta frame frammentati; basta per il terminale didattico e
potra' essere sostituita senza toccare il core CP/M.
