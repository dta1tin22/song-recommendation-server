# syntax=docker/dockerfile:1

# --- build stage ---
FROM golang:1.27-alpine AS build
WORKDIR /src

# Cache module downloads separately from source changes (go.mod + go.sum).
COPY go.* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

# --- runtime stage ---
FROM alpine:3.24
RUN adduser -D -u 10001 app
USER app

COPY --from=build /out/api /api
EXPOSE 8080
ENTRYPOINT ["/api"]
