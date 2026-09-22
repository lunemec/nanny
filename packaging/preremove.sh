#!/bin/sh
set -eu

if [ "${1:-}" = "remove" ] && command -v systemctl >/dev/null 2>&1; then
	systemctl stop nanny.service >/dev/null 2>&1 || true
	systemctl disable nanny.service >/dev/null 2>&1 || true
fi
