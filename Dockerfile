FROM golang:1.25-alpine AS builder

ARG GOPROXY=https://proxy.golang.org,direct

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY web/dist ./web/dist

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/coordinator ./cmd/coordinator

FROM alpine:3.20

RUN apk add --no-cache tzdata ca-certificates git

WORKDIR /app

COPY --from=builder /out/coordinator .

RUN mkdir -p /app/data /app/logs

EXPOSE 8080

ENV TZ=Asia/Shanghai \
    COORDINATOR_HTTP_ADDR=:8080 \
    COORDINATOR_LOG_LEVEL=info \
    COORDINATOR_LOG_DIR=/app/logs

ENTRYPOINT ["./coordinator"]
