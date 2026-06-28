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

## Modello Sessione

Una sessione v0.2 ha uno stato esplicito:

- `idle`: la shell CP/M-like e' pronta al prompt `A>`
- `running`: un comando o programma sta girando
- `closed`: la sessione e' terminata o cancellata
- `error`: l'ultimo comando ha prodotto un errore recuperabile

Questo modello serve a tre client diversi:

- script REST, che possono usare `POST /command`
- terminali interattivi, che usano `input`, `output` e WebSocket
- futura UI web, che potra' mostrare stato, snapshot e output senza conoscere
  i dettagli del core CP/M

`retronet-api` non deve sapere come funziona l'8080: vede una sessione come un
terminale con input, output e snapshot. Lo stesso schema potra' accogliere in
futuro macchine diverse, per esempio `retronet-pc`, se esporranno un adapter
terminale/video-tastiera compatibile.

## Confini

- L'API non espone path host arbitrari.
- Ogni sessione usa una directory temporanea isolata.
- WebSocket e REST sono adattatori: il core CP/M resta importabile e testabile.
- Nessuna ROM, BDOS, BIOS o immagine disco storica viene inclusa.
