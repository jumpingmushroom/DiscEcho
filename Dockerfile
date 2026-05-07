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
# volume-label probe). libdvd-pkg lives in Debian's `contrib` archive,
# which the python:slim base doesn't enable by default.
RUN echo "deb http://deb.debian.org/debian bookworm main contrib" \
        > /etc/apt/sources.list.d/contrib.list \
 && apt-get update \
 && apt-get install -y --no-install-recommends \
        ca-certificates eject cdparanoia libcdio-utils whipper \
        handbrake-cli libdvd-pkg genisoimage \
        libbluray-bdj libbluray2 libbluray-bin \
 && DEBIAN_FRONTEND=noninteractive dpkg-reconfigure libdvd-pkg \
 && rm -rf /var/lib/apt/lists/* \
 && pip install --no-cache-dir apprise

# MakeMKV — built from source. Used by the BDMV + UHD pipelines (M3.1).
# Build deps installed, MakeMKV compiled + installed, build deps purged.
ARG MAKEMKV_VERSION=1.17.5
RUN apt-get update \
 && apt-get install -y --no-install-recommends \
        build-essential pkg-config libc6-dev libssl-dev libexpat1-dev \
        libavcodec-dev libgl1-mesa-dev qtbase5-dev zlib1g-dev curl \
 && curl -fsSL "https://www.makemkv.com/download/makemkv-oss-${MAKEMKV_VERSION}.tar.gz" \
        | tar xz -C /tmp \
 && curl -fsSL "https://www.makemkv.com/download/makemkv-bin-${MAKEMKV_VERSION}.tar.gz" \
        | tar xz -C /tmp \
 && cd "/tmp/makemkv-oss-${MAKEMKV_VERSION}" \
        && ./configure --disable-gui && make -j"$(nproc)" && make install \
 && cd "/tmp/makemkv-bin-${MAKEMKV_VERSION}" \
        && mkdir -p tmp && echo accepted > tmp/eula_accepted \
        && make install \
 && apt-get purge -y --auto-remove \
        build-essential pkg-config libssl-dev libexpat1-dev libavcodec-dev \
        libgl1-mesa-dev qtbase5-dev zlib1g-dev curl \
 && rm -rf /var/lib/apt/lists/* /tmp/makemkv-*

WORKDIR /app
COPY --from=daemon-build /out/discecho /app/discecho

ENV DISCECHO_ADDR=":8088" \
    DISCECHO_LIBRARY="/library" \
    DISCECHO_DATA="/var/lib/discecho"

EXPOSE 8088
USER root
ENTRYPOINT ["/app/discecho"]
