# Sicurezza

`retronet-api` e' pensato per demo locali e laboratorio didattico. Anche cosi',
la v0.1 evita le scorciatoie piu' rischiose.

## Drive

- Nessun path host arbitrario viene accettato dai client.
- Ogni sessione crea un drive temporaneo.
- Il drive ha limite dimensione file e numero file.
- La directory viene rimossa quando la sessione viene chiusa o scade.

## Sessioni

- `-max-sessions` limita il numero di sessioni attive.
- `-session-ttl` limita la durata inattiva.

## Fuori Scope v0.1

- autenticazione utenti
- TLS
- quote multi-tenant robuste
- upload file
- isolamento OS/container per sessione

Questi punti diventeranno importanti prima di esporre il servizio fuori da una
macchina di sviluppo.
