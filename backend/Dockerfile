FROM golang:1.26-alpine AS build

WORKDIR /src
RUN apk add --no-cache build-base
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/data-vision ./cmd/server

FROM alpine:3.22
RUN apk add --no-cache ca-certificates libgcc
WORKDIR /app
COPY --from=build /out/data-vision ./data-vision
EXPOSE 18085
HEALTHCHECK --interval=10s --timeout=5s --retries=10 CMD wget -qO- http://127.0.0.1:18085/api/v1/health || exit 1
ENTRYPOINT ["/app/data-vision"]
