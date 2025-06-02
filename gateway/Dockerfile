FROM golang:1.21-alpine AS builder

RUN apk add --no-cache git

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o gateway cmd/gateway/main.go

FROM python:3.12-slim

RUN apt-get update && apt-get install -y \
    git \
    nodejs \
    npm \
    && rm -rf /var/lib/apt/lists/*

RUN pip install aider-chat

WORKDIR /workspace

COPY --from=builder /app/gateway /usr/local/bin/gateway

EXPOSE 8080

CMD ["gateway", "--port", "8080", "--workdir", "/workspace"]