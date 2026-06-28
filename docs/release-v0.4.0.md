# Release v0.4.0

Release dedicata a dashboard sessioni e upload sicuro `.COM`.

## Novita'

- `GET /sessions` per elencare sessioni attive
- `GET /sessions/{id}/files` per elencare il drive temporaneo della sessione
- `POST /sessions/{id}/files` per caricare programmi `.COM`
- normalizzazione nomi CP/M 8.3 tramite `retronet-cpm/disk`
- rifiuto di path traversal, slash, sottodirectory e nomi non `.COM`
- limiti `-max-file-size` e `-max-files` applicati anche agli upload
- conformance aggiornata con upload/list file
- documentazione italiana aggiornata

## Esempio

```powershell
$s = Invoke-RestMethod -Method Post http://127.0.0.1:8080/sessions
Invoke-RestMethod -Method Post "http://127.0.0.1:8080/sessions/$($s.id)/files" `
  -Form @{ file = Get-Item .\HELLO.COM }
Invoke-RestMethod "http://127.0.0.1:8080/sessions/$($s.id)/files"
Invoke-RestMethod -Method Post "http://127.0.0.1:8080/sessions/$($s.id)/run" `
  -ContentType application/json `
  -Body '{"command":"RUN HELLO"}'
```

## Sicurezza E Licenze

L'upload scrive solo nel drive temporaneo della sessione. Non vengono inclusi
programmi storici nel repo: l'utente deve caricare solo software proprio,
sintetico o con licenza compatibile.

## Verifica

```powershell
gofmt -l .
go vet ./...
go test -count=1 ./...
go run ./cmd/retronet-api -conformance
git diff --check
```
