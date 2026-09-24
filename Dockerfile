# syntax=docker/dockerfile:1

# Build stage: compile a static binary. It runs on the build machine's own platform and cross-compiles
# for the target platform, which is much faster than emulating the target (e.g. arm64 on an amd64 runner).
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build

ARG TARGETOS
ARG TARGETARCH
# Release version shown in the startup log; set by the release workflow
ARG VERSION=dev

WORKDIR /src

# Download modules before copying the source, so they stay cached until go.mod/go.sum change
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .

# CGO_ENABLED=0 gives a fully static binary that runs on scratch.
# The timetzdata tag embeds the time zone database, since scratch has none (TZ still works).
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -tags timetzdata -ldflags="-s -w -X main.version=${VERSION}" -o /out/price_tracker_bot .

# Mount point for the state volume; copied below with the non-root user as owner
RUN mkdir -p /out/data

# Final stage: an empty image with only what the bot needs
FROM scratch

# Trusted root certificates for HTTPS to Telegram and the tracked websites
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /out/price_tracker_bot /app/price_tracker_bot
COPY --from=build --chown=65532:65532 /out/data /data

# Tracker configs are not part of the image (they're private and the image is public);
# mount them at /app/tracker_configs, see deployment/docker-compose.yml
WORKDIR /app

# Run as an unprivileged user; scratch has no /etc/passwd, so a numeric ID is used
USER 65532:65532

# Defaults; values from the .env file override them
ENV TZ=UTC
ENV STATE_FILE=/data/state.json

ENTRYPOINT ["/app/price_tracker_bot"]
