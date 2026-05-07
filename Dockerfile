# syntax=docker/dockerfile:1.7

###############################################################################
# Stage 1 — build the SvelteKit UI
###############################################################################
FROM node:20-bookworm-slim AS webui-build
WORKDIR /webui
RUN corepack enable && corepack prepare pnpm@9 --activate
COPY webui/package.json webui/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY webui/ ./
RUN pnpm build

###############################################################################
# Stage 2 — build the Go daemon with the UI embedded
###############################################################################
FROM golang:1.25-bookworm AS daemon-build
WORKDIR /src
COPY daemon/go.mod daemon/go.sum ./daemon/
WORKDIR /src/daemon
RUN go mod download
COPY daemon/ ./
# Drop the placeholder UI and replace with the real build from stage 1.
RUN rm -rf embed/webui_build
COPY --from=webui-build /webui/build ./embed/webui_build
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
RUN CGO_ENABLED=0 go build \
    -trimpath \
    -ldflags "-s -w \
      -X github.com/jumpingmushroom/DiscEcho/daemon/version.Version=${VERSION} \
      -X github.com/jumpingmushroom/DiscEcho/daemon/version.Commit=${COMMIT} \
      -X github.com/jumpingmushroom/DiscEcho/daemon/version.BuildDate=${BUILD_DATE}" \
    -o /out/discecho ./cmd/discecho

###############################################################################
# Stage 3 — runtime: python slim + apprise + the daemon binary
###############################################################################
FROM python:3.12-slim-bookworm AS runtime
# whipper is not on PyPI, so install it from Debian apt. cdparanoia +
# libcdio-utils provide the lower-level rippers and cd-info that
# identify/classify use. handbrake-cli + libdvd-pkg + genisoimage
# provide DVD ripping (HandBrake, libdvdcss CSS bypass, isoinfo for
# volume-label probe).
RUN apt-get update \
 && apt-get install -y --no-install-recommends \
        ca-certificates eject cdparanoia libcdio-utils whipper \
        handbrake-cli libdvd-pkg genisoimage \
 && DEBIAN_FRONTEND=noninteractive dpkg-reconfigure libdvd-pkg \
 && rm -rf /var/lib/apt/lists/* \
 && pip install --no-cache-dir apprise

WORKDIR /app
COPY --from=daemon-build /out/discecho /app/discecho

ENV DISCECHO_ADDR=":8088" \
    DISCECHO_LIBRARY="/library" \
    DISCECHO_DATA="/var/lib/discecho"

EXPOSE 8088
USER root
ENTRYPOINT ["/app/discecho"]
