FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o /khomo \
    ./cmd/khomo

FROM scratch

COPY --from=builder /khomo /khomo
COPY --from=builder /app/migrations /migrations
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

VOLUME ["/data"]
EXPOSE 4019

ENV PORT=4019
ENV DATABASE_PATH=/data/khomo.db

ENTRYPOINT ["/khomo"]
