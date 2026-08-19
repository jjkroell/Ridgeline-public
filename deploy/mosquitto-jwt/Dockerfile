# Mosquitto + mosquitto-go-auth, for the observer broker that requires Ed25519
# token authentication (the hostname you serve it on). The anonymous broker stays on the
# stock eclipse-mosquitto:2 image and is not affected by anything here.
#
# The plugin is a C-shared object that links against mosquitto's headers, so
# there is no prebuilt path: mosquitto is built from source to get the headers,
# then the plugin against them. Versions are pinned because a mismatch between
# the plugin and the broker's plugin ABI fails at load time, not at build time.
ARG MOSQUITTO_VERSION=2.0.18
ARG LWS_VERSION=4.2.2
ARG GO_AUTH_VERSION=3.0.0

FROM debian:bookworm-slim AS mosquitto_builder
ARG MOSQUITTO_VERSION
ARG LWS_VERSION

RUN set -ex; \
    apt-get update; \
    apt-get install -y wget build-essential cmake libssl-dev libcjson-dev

# Debian's libwebsockets is too old for mosquitto 2.x, and we need websockets:
# observers reach this broker as wss:// through the Cloudflare Tunnel.
RUN set -ex; \
    wget -q https://github.com/warmcat/libwebsockets/archive/v${LWS_VERSION}.tar.gz -O /tmp/lws.tar.gz; \
    mkdir -p /build/lws; \
    tar --strip=1 -xf /tmp/lws.tar.gz -C /build/lws; \
    rm /tmp/lws.tar.gz; \
    cd /build/lws; \
    cmake . \
        -DCMAKE_BUILD_TYPE=MinSizeRel \
        -DCMAKE_INSTALL_PREFIX=/usr \
        -DLWS_IPV6=ON \
        -DLWS_WITHOUT_BUILTIN_GETIFADDRS=ON \
        -DLWS_WITHOUT_CLIENT=ON \
        -DLWS_WITHOUT_EXTENSIONS=ON \
        -DLWS_WITHOUT_TESTAPPS=ON \
        -DLWS_WITH_HTTP2=OFF \
        -DLWS_WITH_SHARED=OFF \
        -DLWS_WITH_ZIP_FOPS=OFF \
        -DLWS_WITH_ZLIB=OFF \
        -DLWS_WITH_EXTERNAL_POLL=ON; \
    make -j "$(nproc)"; \
    rm -rf /root/.cmake

WORKDIR /app
RUN mkdir -p mosquitto/auth mosquitto/conf.d
RUN wget -q http://mosquitto.org/files/source/mosquitto-${MOSQUITTO_VERSION}.tar.gz
RUN tar xzf mosquitto-${MOSQUITTO_VERSION}.tar.gz
RUN set -ex; \
    cd mosquitto-${MOSQUITTO_VERSION}; \
    make CFLAGS="-Wall -O2 -I/build/lws/include" LDFLAGS="-L/build/lws/lib" WITH_WEBSOCKETS=yes; \
    make install

FROM golang:1-bookworm AS go_auth_builder
ARG GO_AUTH_VERSION

ENV CGO_CFLAGS="-I/usr/local/include -fPIC"
ENV CGO_LDFLAGS="-shared -Wl,-unresolved-symbols=ignore-all"
ENV CGO_ENABLED=1

RUN apt-get update && apt-get install -y git

WORKDIR /app
COPY --from=mosquitto_builder /usr/local/include/ /usr/local/include/
RUN git clone --depth 1 --branch ${GO_AUTH_VERSION} \
      https://github.com/iegomez/mosquitto-go-auth.git .
# Two passes, as upstream does: the c-archive build is what emits go-auth.h,
# which the plugin's auth-plugin.c includes. Skipping it fails the c-shared
# build with "go-auth.h: No such file or directory".
RUN set -ex; \
    go build -buildmode=c-archive go-auth.go; \
    go build -buildmode=c-shared -o go-auth.so

FROM debian:bookworm-slim

RUN set -ex; \
    apt-get update; \
    apt-get install -y libc-ares2 openssl uuid tini libssl-dev libcjson-dev; \
    rm -rf /var/lib/apt/lists/*

RUN set -ex; \
    groupadd mosquitto; \
    useradd -s /sbin/nologin mosquitto -g mosquitto -d /var/lib/mosquitto; \
    mkdir -p /var/lib/mosquitto /var/log/mosquitto /mosquitto/data; \
    chown -R mosquitto:mosquitto /var/log/mosquitto /var/lib/mosquitto /mosquitto

COPY --from=mosquitto_builder /usr/local/sbin/mosquitto /usr/sbin/mosquitto
COPY --from=mosquitto_builder /usr/local/lib/libmosquitto* /usr/local/lib/
COPY --from=mosquitto_builder /usr/local/bin/mosquitto_sub /usr/bin/mosquitto_sub
COPY --from=mosquitto_builder /usr/local/bin/mosquitto_pub /usr/bin/mosquitto_pub
COPY --from=go_auth_builder /app/go-auth.so /mosquitto/go-auth.so

RUN ldconfig

EXPOSE 1883 9001
ENTRYPOINT ["/usr/bin/tini", "--"]
CMD ["/usr/sbin/mosquitto", "-c", "/mosquitto/config/mosquitto.conf"]
