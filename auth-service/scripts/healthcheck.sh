#!/bin/bash
set -euo pipefail

healthcheck_host="${HEALTHCHECK_HOST:?Set HEALTHCHECK_HOST}"
healthcheck_port="${KC_HTTP_MANAGEMENT_PORT:?Set KC_HTTP_MANAGEMENT_PORT}"

exec 3<>"/dev/tcp/${healthcheck_host}/${healthcheck_port}"
printf 'GET /health/ready HTTP/1.0\r\nHost: %s:%s\r\n\r\n' "$healthcheck_host" "$healthcheck_port" >&3
head -n 1 <&3 | grep -q ' 200 '
