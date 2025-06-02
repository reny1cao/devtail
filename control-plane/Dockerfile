FROM golang:1.21-alpine AS builder

RUN apk add --no-cache git

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o control-plane cmd/control-plane/main.go

FROM alpine:latest

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=builder /app/control-plane .
COPY migrations ./migrations

EXPOSE 8081

CMD ["./control-plane"]