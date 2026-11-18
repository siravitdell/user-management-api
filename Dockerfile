# syntax=docker/dockerfile:1

# ---- build stage -------------------------------------------------------------
FROM golang:1.27.4-alpine AS build
WORKDIR /src

# Cache module downloads in their own layer.
COPY go.mod go.sum* ./
RUN go mod download

COPY . .
# `go mod tidy` is a no-op when go.sum is committed; it makes the build work
# even on a fresh clone where go.sum has not been generated yet.
RUN go mod tidy && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# ---- runtime stage -----------------------------------------------------------
# Distroless: no shell, no package manager, runs as an unprivileged user.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/server /server
EXPOSE 8080 50051
USER nonroot:nonroot
ENTRYPOINT ["/server"]