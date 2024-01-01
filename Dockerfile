FROM docker.io/golang:1.21 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY *.go ./
RUN CGO_ENABLED=0 go build -trimpath -o /http-filter-proxy


FROM scratch

COPY --from=builder /http-filter-proxy /http-filter-proxy

USER 1000:1000
EXPOSE 8080
ENTRYPOINT ["/http-filter-proxy"]
