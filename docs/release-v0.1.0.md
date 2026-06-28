# Release v0.1.0

Prima release di `retronet-api`.

Novita':

- server HTTP Go con libreria standard
- `GET /health`
- `GET /version`
- session manager in memoria
- sessioni CP/M-like tramite `retronet-cpm/session`
- drive temporaneo confinato per ogni sessione
- `POST /sessions`
- `GET /sessions/{id}`
- `DELETE /sessions/{id}`
- `POST /sessions/{id}/command`
- `GET /sessions/{id}/ws`
- WebSocket JSON minimale senza dipendenze esterne
- Dockerfile e CI
- documentazione italiana

Limiti:

- niente autenticazione/TLS in v0.1
- niente upload file
- `RUN` CP/M resta sincrono
- nessuna ROM, BDOS, BIOS o immagine disco storica inclusa

Verifica:

```powershell
gofmt -l .
go vet ./...
go test -count=1 ./...
go run ./cmd/retronet-api -conformance
git diff --check
```
