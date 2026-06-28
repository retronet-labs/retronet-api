FROM golang:1.26-alpine AS builder

WORKDIR /src
COPY . .

RUN go build -o /out/retronet-api ./cmd/retronet-api

FROM alpine:latest

WORKDIR /app
COPY --from=builder /out/retronet-api /app/retronet-api

EXPOSE 8080
ENTRYPOINT ["/app/retronet-api"]
CMD ["-addr", ":8080"]
