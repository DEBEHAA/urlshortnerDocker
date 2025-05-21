# syntax=docker/dockerfile:1

# Build Stage
FROM golang:1.24.3-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o urlshortener main.go

# Final Stage
FROM alpine:latest
WORKDIR /app
RUN apk add --no-cache ca-certificates

COPY --from=builder /app/urlshortener .
COPY --from=builder /app/index.html .
COPY --from=builder /app/logo.png .
COPY --from=builder /app/icon.jpeg .
COPY --from=builder /app/bg.png .

EXPOSE 8080
CMD ["./urlshortener"]
