FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/cloudflare-exporter ./cmd/cloudflare-exporter

FROM gcr.io/distroless/static-debian12
COPY --from=builder /out/cloudflare-exporter /cloudflare-exporter
EXPOSE 9103
ENTRYPOINT ["/cloudflare-exporter"]
