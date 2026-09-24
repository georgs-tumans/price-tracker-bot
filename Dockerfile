# syntax=docker/dockerfile:1

# Build stage: compile a static binary
FROM golang:1.27-alpine AS build

WORKDIR /src

# Download modules before copying the source, so they stay cached until go.mod/go.sum change
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .

# CGO_ENABLED=0 gives a fully static binary that runs on scratch.
# The timetzdata tag embeds the time zone database, since scratch has none (TZ still works).
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -tags timetzdata -ldflags="-s -w" -o /out/price_tracker_bot .

# Mount point for the state volume; copied below with the non-root user as owner
RUN mkdir -p /out/data

# Final stage: an empty image with only what the bot needs
FROM scratch

# Trusted root certificates for HTTPS to Telegram and the tracked websites
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /out/price_tracker_bot /app/price_tracker_bot
COPY --from=build --chown=65532:65532 /out/data /data
COPY tracker_configs /app/tracker_configs

WORKDIR /app

# Run as an unprivileged user; scratch has no /etc/passwd, so a numeric ID is used
USER 65532:65532

# Defaults; values from the .env file override them
ENV TZ=UTC
ENV STATE_FILE=/data/state.json

ENTRYPOINT ["/app/price_tracker_bot"]
