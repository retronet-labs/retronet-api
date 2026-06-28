# AGENTS

## Obiettivo

Mantenere `retronet-api`: backend HTTP/WebSocket didattico per orchestrare
sessioni RetroNet, a partire da CP/M-like.

## Regole

- Documentazione pubblica in italiano.
- Commit piccoli e atomici.
- Nessuna ROM, BIOS, BDOS, immagine disco, font o manuale storico copiato.
- Le sessioni remote devono usare drive temporanei o radici esplicite con limiti.
- Non esporre path host arbitrari via API.
- Preferire libreria standard Go quando basta.

## Gate

```powershell
gofmt -l .
go vet ./...
go test -count=1 ./...
go run ./cmd/retronet-api -conformance
git diff --check
```
