FROM golang:1.24 AS builder
WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/api ./cmd/api

FROM gcr.io/distroless/base-debian12
WORKDIR /app
COPY --from=builder /bin/api /api
COPY --from=builder /app/templates ./templates
COPY --from=builder /app/web ./web
COPY --from=builder /app/migrations ./migrations
EXPOSE 8080 1812/udp 1813/udp
ENTRYPOINT ["/api"]
