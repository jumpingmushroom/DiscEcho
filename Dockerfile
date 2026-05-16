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
# Stage — build HandBrakeCLI from source on Debian bookworm
#
# Debian bookworm's `handbrake-cli` package (1.6.1) is compiled without
# NVENC support. We build HandBrake from source so the resulting binary
# links against bookworm's own libraries (no cross-distro ABI issues)
# and is compiled with --enable-nvenc (on by default for x86_64-linux).
# NVENC requires only the nv-codec-headers at build time; no GPU is
# needed during the build — the runtime driver is dlopen'd at job start.
###############################################################################
FROM debian:bookworm-slim AS handbrake-build
ARG HANDBRAKE_VERSION=1.11.1
RUN apt-get update \
 && apt-get install -y --no-install-recommends \
        build-essential cmake git nasm ninja-build meson m4 patch pkg-config \
        python3 tar curl ca-certificates \
        libtool libtool-bin autoconf automake \
        libass-dev libbz2-dev libfontconfig-dev libfreetype6-dev \
        libfribidi-dev libharfbuzz-dev libjansson-dev liblzma-dev \
        libmp3lame-dev libnuma-dev libogg-dev libopus-dev \
        libsamplerate0-dev libspeex-dev libtheora-dev \
        libturbojpeg0-dev libvorbis-dev libx264-dev libxml2-dev \
        libvpx-dev libdvdread-dev libdvdnav-dev libbluray-dev \
        libva-dev libdrm-dev \
        zlib1g-dev \
 && curl -fsSL "https://github.com/HandBrake/HandBrake/releases/download/${HANDBRAKE_VERSION}/HandBrake-${HANDBRAKE_VERSION}-source.tar.bz2" \
        | tar xj -C /tmp \
 && cd "/tmp/HandBrake-${HANDBRAKE_VERSION}" \
 && ./configure --disable-gtk --launch-jobs="$(nproc)" --launch \
 && make --directory=build install \
 && rm -rf /tmp/HandBrake*

###############################################################################
# Stage — build chdman from MAME source
#
# Debian bookworm's mame-tools package ships chdman 0.251, which is missing
# the `createdvd` subcommand (added in MAME 0.252, April 2023). PS2 / Xbox
# rips use createdvd to produce DVD-typed CHD files that emulators expect.
# We build MAME's tools subset here (TOOLS=1 EMULATOR=0 skips the full
# emulator and its Qt deps) and copy out just the chdman binary.
# MAME does not ship source tarballs in its GitHub releases; we shallow-clone
# the tagged commit instead.
###############################################################################
FROM debian:bookworm-slim AS chdman-build
ARG MAME_VERSION=0.275
ARG MAME_TAG=mame0275
RUN apt-get update \
 && apt-get install -y --no-install-recommends \
        build-essential git python3 ca-certificates \
        libsdl2-dev libsdl2-ttf-dev \
        libxinerama-dev libxi-dev \
        libfontconfig-dev libasound2-dev \
 && rm -rf /var/lib/apt/lists/*
RUN git clone --depth 1 --branch "${MAME_TAG}" \
        https://github.com/mamedev/mame.git /src/mame
WORKDIR /src/mame
# USE_QTDEBUG=0: on Linux the Genie build system defaults USE_QTDEBUG=1
# which requires Qt5Widgets headers and moc. We have no use for the Qt
# debugger UI in a CHD-tools-only build, so disable it explicitly.
RUN make -j"$(nproc)" TOOLS=1 EMULATOR=0 USE_QTDEBUG=0 IGNORE_GIT=1
RUN strip /src/mame/chdman \
 && /src/mame/chdman --help 2>&1 | head -5

###############################################################################
# Stage 4 — runtime: python slim + apprise + the daemon binary
###############################################################################
FROM python:3.12-slim-bookworm AS runtime
# whipper is not on PyPI, so install it from Debian apt. cdparanoia +
# libcdio-utils provide the lower-level rippers and cd-info that
# identify/classify use. libdvd-pkg + dvdbackup + genisoimage
# provide DVD ripping (libdvdcss CSS bypass, isoinfo for volume-label
# probe). HandBrakeCLI itself comes from the handbrake-build stage
# below — the Debian package lacks NVENC support. libdvd-pkg lives in
# Debian's `contrib` archive, which the python:slim base doesn't enable
# by default. libbluray-bin ships bd_info (UHD AACS2 detection);
# libssl3 + libexpat1 + libavcodec59 are makemkvcon's runtime
# shared-lib deps. libass9 + libturbojpeg0 are HandBrakeCLI runtime
# deps not pulled in transitively by anything else in this image.
# libsdl2-2.0-0 is the sole runtime dep of the chdman binary built from
# MAME source in the chdman-build stage above (chdman links ocore_sdl).
RUN echo "deb http://deb.debian.org/debian bookworm main contrib" \
        > /etc/apt/sources.list.d/contrib.list \
 && apt-get update \
 && apt-get install -y --no-install-recommends \
        ca-certificates eject cdparanoia libcdio-utils whipper \
        python3-cdio \
        libdvd-pkg dvdbackup genisoimage \
        libbluray-bdj libbluray2 libbluray-bin \
        libssl3 libexpat1 libavcodec59 \
        libass9 libturbojpeg0 \
        libsdl2-2.0-0 \
 && DEBIAN_FRONTEND=noninteractive dpkg-reconfigure libdvd-pkg \
 && rm -rf /var/lib/apt/lists/* \
 && pip install --no-cache-dir apprise

# Copy MakeMKV's built binary + shared libs from the build stage.
COPY --from=makemkv-build /usr/bin/makemkvcon /usr/bin/makemkvcon
COPY --from=makemkv-build /lib/libmakemkv.so.1 /lib/libmakemkv.so.1
COPY --from=makemkv-build /lib/libdriveio.so.0 /lib/libdriveio.so.0

# HandBrake built from source on Debian bookworm. The binary links
# against the same bookworm shared libs already present in the runtime
# image, so no extra lib COPYs are needed.
COPY --from=handbrake-build /usr/local/bin/HandBrakeCLI /usr/bin/HandBrakeCLI

# chdman built from MAME source (see chdman-build stage). Replaces the
# bookworm mame-tools package (0.251) which predates the createdvd
# subcommand needed for PS2 / Xbox DVD-typed CHDs.
COPY --from=chdman-build /src/mame/chdman /usr/bin/chdman

# redumper — pre-built static Linux binary released on GitHub.
# Pinned via REDUMPER_VERSION build arg.
ARG REDUMPER_VERSION=b720
RUN apt-get update \
 && apt-get install -y --no-install-recommends curl unzip ca-certificates \
 && curl -fsSLo /tmp/redumper.zip \
        "https://github.com/superg/redumper/releases/download/${REDUMPER_VERSION}/redumper-${REDUMPER_VERSION}-linux-x64.zip" \
 && unzip /tmp/redumper.zip -d /tmp/redumper \
 && install -m 0755 /tmp/redumper/redumper-${REDUMPER_VERSION}-linux-x64/bin/redumper /usr/local/bin/redumper \
 && apt-get purge -y --auto-remove curl unzip \
 && rm -rf /var/lib/apt/lists/* /tmp/redumper /tmp/redumper.zip

WORKDIR /app
COPY --from=daemon-build /out/discecho /app/discecho

ENV DISCECHO_ADDR=":8088" \
    DISCECHO_LIBRARY="/library" \
    DISCECHO_DATA="/var/lib/discecho"

EXPOSE 8088
USER root
ENTRYPOINT ["/app/discecho"]
