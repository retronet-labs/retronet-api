# Architettura

`retronet-api` e' un backend Go piccolo, pensato per esporre sessioni RetroNet
via HTTP e WebSocket.

```text
browser / client
    |
    | HTTP + WebSocket
    v
retronet-api
    |
    v
session manager
    |
    v
retronet-cpm/session -> shell -> BDOS subset -> retronet-terminal
```

Il server non emula direttamente CPU o terminali. Orchestra moduli gia
pubblicati:

- `retronet-cpm/session` per creare sessioni CP/M-like
- `retronet-cpm/disk` per drive temporanei confinati
- `retronet-terminal` per output raw e snapshot

## Confini

- L'API non espone path host arbitrari.
- Ogni sessione usa una directory temporanea isolata.
- WebSocket e REST sono adattatori: il core CP/M resta importabile e testabile.
- Nessuna ROM, BDOS, BIOS o immagine disco storica viene inclusa.
