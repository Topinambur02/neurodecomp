FROM golang:1.26.5-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /gateway ./cmd/app

FROM alpine:3.23

RUN apk add --no-cache ca-certificates && addgroup -S gateway && adduser -S -G gateway gateway

WORKDIR /app

COPY --from=builder /gateway /usr/local/bin/gateway

COPY configs/config.prod.json ./configs/config.prod.json

ENV GATEWAY_CONFIG=/app/configs/config.prod.json

RUN mkdir logs && chown gateway:gateway logs

USER gateway

EXPOSE 8080

ENTRYPOINT ["gateway"]