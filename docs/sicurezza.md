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

## Fuori Scope v0.2

- autenticazione utenti
- TLS
- quote multi-tenant robuste
- upload file
- isolamento OS/container per sessione

Questi punti diventeranno importanti prima di esporre il servizio fuori da una
macchina di sviluppo.
