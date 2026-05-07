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
# Stage 3 — build MakeMKV from source
#
# MakeMKV has no apt package and depends on heavy build-time toolchains
# (qtbase5-dev, libgl1-mesa-dev) that we don't want shipped in the
# runtime image. We compile it in this isolated stage and the runtime
# stage copies only the resulting binary + shared libs.
###############################################################################
FROM debian:bookworm-slim AS makemkv-build
ARG MAKEMKV_VERSION=1.18.3
RUN apt-get update \
 && apt-get install -y --no-install-recommends \
        build-essential pkg-config libc6-dev libssl-dev libexpat1-dev \
        libavcodec-dev libgl1-mesa-dev qtbase5-dev zlib1g-dev curl \
        ca-certificates \
 && curl -fsSL "https://www.makemkv.com/download/makemkv-oss-${MAKEMKV_VERSION}.tar.gz" \
        | tar xz -C /tmp \
 && curl -fsSL "https://www.makemkv.com/download/makemkv-bin-${MAKEMKV_VERSION}.tar.gz" \
        | tar xz -C /tmp \
 && cd "/tmp/makemkv-oss-${MAKEMKV_VERSION}" \
        && ./configure --disable-gui && make -j"$(nproc)" && make install \
 && cd "/tmp/makemkv-bin-${MAKEMKV_VERSION}" \
        && mkdir -p tmp && echo accepted > tmp/eula_accepted \
        && make install

###############################################################################
# Stage 4 — runtime: python slim + apprise + the daemon binary
###############################################################################
FROM python:3.12-slim-bookworm AS runtime
# whipper is not on PyPI, so install it from Debian apt. cdparanoia +
# libcdio-utils provide the lower-level rippers and cd-info that
# identify/classify use. handbrake-cli + libdvd-pkg + genisoimage
# provide DVD ripping (HandBrake, libdvdcss CSS bypass, isoinfo for
# volume-label probe). libdvd-pkg lives in Debian's `contrib` archive,
# which the python:slim base doesn't enable by default.
# libbluray-bin ships bd_info (UHD AACS2 detection); libssl3 +
# libexpat1 + libavcodec59 are makemkvcon's runtime shared-lib deps.
RUN echo "deb http://deb.debian.org/debian bookworm main contrib" \
        > /etc/apt/sources.list.d/contrib.list \
 && apt-get update \
 && apt-get install -y --no-install-recommends \
        ca-certificates eject cdparanoia libcdio-utils whipper \
        handbrake-cli libdvd-pkg genisoimage \
        libbluray-bdj libbluray2 libbluray-bin \
        libssl3 libexpat1 libavcodec59 \
 && DEBIAN_FRONTEND=noninteractive dpkg-reconfigure libdvd-pkg \
 && rm -rf /var/lib/apt/lists/* \
 && pip install --no-cache-dir apprise

# Copy MakeMKV's built binary + shared libs from the build stage.
COPY --from=makemkv-build /usr/bin/makemkvcon /usr/bin/makemkvcon
COPY --from=makemkv-build /lib/libmakemkv.so.1 /lib/libmakemkv.so.1
COPY --from=makemkv-build /lib/libdriveio.so.0 /lib/libdriveio.so.0

WORKDIR /app
COPY --from=daemon-build /out/discecho /app/discecho

ENV DISCECHO_ADDR=":8088" \
    DISCECHO_LIBRARY="/library" \
    DISCECHO_DATA="/var/lib/discecho"

EXPOSE 8088
USER root
ENTRYPOINT ["/app/discecho"]
