# Sicurezza

`retronet-api` e' pensato per demo locali e laboratorio didattico. Anche cosi',
la v0.2 evita le scorciatoie piu' rischiose.

## Drive

- Nessun path host arbitrario viene accettato dai client.
- Ogni sessione crea un drive temporaneo.
- Il drive ha limite dimensione file e numero file.
- La directory viene rimossa quando la sessione viene chiusa o scade.

## Sessioni

- `-max-sessions` limita il numero di sessioni attive.
- `-session-ttl` limita la durata inattiva.
- `state=running` impedisce di avviare due comandi contemporanei nella stessa
  sessione.
- `POST /sessions/{id}/input` accetta byte per la sessione corrente, ma non
  espone path, processi host o shell del sistema operativo.

## Terminale E Output

Il buffer output di `retronet-terminal` e' condiviso tra `GET /output` e
WebSocket: leggere da uno dei due canali svuota il buffer. Per evitare sorprese,
un client interattivo dovrebbe usare un solo consumatore di output per sessione.

## Upload .COM

`POST /sessions/{id}/files` carica file `.COM` solo dentro il drive temporaneo
della sessione. Non accetta path host arbitrari: il nome viene normalizzato come
CP/M 8.3 e sono rifiutati slash, `..`, sottodirectory e nomi non `.COM`.

Restano attivi i limiti:

- `-max-file-size`
- `-max-files`
- cleanup alla chiusura/scadenza sessione

Il repo non include programmi storici. Chi usa l'upload deve caricare solo
software proprio, sintetico o con licenza compatibile e documentata.

## CORS

Per supportare `retronet-ui` locale, la CLI abilita CORS solo per
`http://127.0.0.1:18081` e `http://localhost:18081`. Le origini si cambiano con
`-cors-origin`; valore vuoto significa CORS disabilitato. La wildcard `*` non e'
il default e va usata solo in laboratorio locale consapevole.

## Fuori Scope v0.2

- autenticazione utenti
- TLS
- quote multi-tenant robuste
- isolamento OS/container per sessione

Questi punti diventeranno importanti prima di esporre il servizio fuori da una
macchina di sviluppo.
