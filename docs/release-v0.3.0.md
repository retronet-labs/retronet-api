# Release v0.3.0

Release di supporto a `retronet-ui`.

## Novita'

- CORS configurabile lato server
- flag CLI `-cors-origin`
- origini locali abilitate di default:
  - `http://127.0.0.1:18081`
  - `http://localhost:18081`
- risposta preflight `OPTIONS`
- test per origine consentita e origine rifiutata

## Perche' Serve

Una UI browser servita da `retronet-ui` su una porta diversa da
`retronet-api` non puo' chiamare `POST /sessions` senza header CORS. Questa
release abilita il caso locale in modo esplicito, senza aprire l'API a qualunque
origine per default.

## Uso

Avvio standard per laboratorio locale:

```powershell
go run ./cmd/retronet-api -addr 127.0.0.1:8080
```

Limitare a una sola origine:

```powershell
go run ./cmd/retronet-api -cors-origin "http://127.0.0.1:18081"
```

Disabilitare CORS:

```powershell
go run ./cmd/retronet-api -cors-origin ""
```

## Limiti

- non introduce autenticazione
- non introduce TLS
- non cambia il protocollo WebSocket
- non include asset, ROM, BIOS, BDOS o immagini storiche

## Verifica

```powershell
gofmt -l .
go vet ./...
go test -count=1 ./...
go run ./cmd/retronet-api -conformance
git diff --check
```
