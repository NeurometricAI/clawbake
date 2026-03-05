FROM golang:1.26-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .

ARG BINARY=server
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -ldflags "-X github.com/clawbake/clawbake/internal/version.Version=${VERSION}" -o /app/bin/${BINARY} ./cmd/${BINARY}

FROM alpine:3.21
RUN apk --no-cache add ca-certificates
WORKDIR /app

ARG BINARY=server
COPY --from=builder /app/bin/${BINARY} /app/clawbake
COPY --from=builder /app/web/static /app/web/static

ENTRYPOINT ["/app/clawbake"]
