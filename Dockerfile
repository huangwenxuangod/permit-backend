FROM golang:1.25 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o permit-backend ./cmd/permit-backend

FROM debian:bookworm-slim
WORKDIR /app
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY --from=builder /app/permit-backend /app/permit-backend
ENV PERMIT_PORT=5000
ENV PERMIT_ASSETS_DIR=/data/assets
ENV PERMIT_UPLOADS_DIR=/data/uploads
EXPOSE 5000
VOLUME ["/data"]
ENTRYPOINT ["/app/permit-backend"]
