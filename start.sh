#!/bin/sh
# start.sh — Tailscale userspace bootstrap + humanMCP launch
#
# Flow:
#   1. Start tailscaled w userspace mode (no TUN device on Fly.io)
#   2. Bring up node using TS_AUTHKEY (tagged + ephemeral + reusable)
#   3. Wait until node has tailnet IP (max 30s)
#   4. exec humanmcp — process inherits stdout/stderr, gets PID 1
#
# Behavior bez TS_AUTHKEY: skip tailscale entirely, run plain humanmcp.
# This keeps the image deployable even before TS_AUTHKEY is configured.

set -eu

if [ -n "${TS_AUTHKEY:-}" ]; then
    echo "[start.sh] Tailscale enabled — bringing up userspace node"
    /usr/sbin/tailscaled \
        --tun=userspace-networking \
        --socks5-server=localhost:1055 \
        --outbound-http-proxy-listen=localhost:1055 \
        --state=/var/lib/tailscale/tailscaled.state \
        > /var/log/tailscaled.log 2>&1 &
    TSD_PID=$!

    # Wait for daemon to be ready
    sleep 1

    /usr/bin/tailscale up \
        --authkey="${TS_AUTHKEY}" \
        --hostname="${TS_HOSTNAME:-fly-humanmcp}" \
        --advertise-tags="${TS_TAGS:-tag:fly-humanmcp}" \
        --accept-routes \
        --ssh=false \
        --reset

    # Wait up to 30s for tailnet IP
    i=0
    while [ $i -lt 30 ]; do
        if /usr/bin/tailscale ip -4 2>/dev/null | grep -q '^100\.'; then
            IP=$(/usr/bin/tailscale ip -4 2>/dev/null | head -1)
            echo "[start.sh] tailnet IP: ${IP}"
            break
        fi
        sleep 1
        i=$((i + 1))
    done

    if [ $i -ge 30 ]; then
        echo "[start.sh] WARNING: tailnet IP not assigned within 30s; humanmcp will start anyway"
    fi

    # Configure HTTP_PROXY so Go http.Client uses tailscale's outbound proxy
    # This is critical for MagicDNS (http://macbook-air-3:7331) to resolve.
    export HTTP_PROXY="http://localhost:1055"
    export HTTPS_PROXY="http://localhost:1055"
    export NO_PROXY="localhost,127.0.0.1,0.0.0.0,${TS_NO_PROXY:-}"
else
    echo "[start.sh] TS_AUTHKEY not set — skipping tailscale bootstrap"
fi

echo "[start.sh] launching humanmcp"
exec ./humanmcp
