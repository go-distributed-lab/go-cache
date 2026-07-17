# ── build stage ───────────────────────────────────────────────────────────────
FROM golang:1.24-alpine AS builder

WORKDIR /app
COPY go.mod ./
COPY . .

RUN go build -ldflags="-s -w" -o /go-cache ./cmd/server

# ── final stage ───────────────────────────────────────────────────────────────
FROM scratch

COPY --from=builder /go-cache /go-cache

EXPOSE 8080

ENTRYPOINT ["/go-cache"]