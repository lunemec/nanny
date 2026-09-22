#!/bin/sh
set -eu

if [ "${1:-}" = "purge" ] && command -v deb-systemd-helper >/dev/null 2>&1; then
	deb-systemd-helper purge nanny.service >/dev/null || true
	deb-systemd-helper unmask nanny.service >/dev/null || true
fi

if [ -d /run/systemd/system ] && command -v systemctl >/dev/null 2>&1; then
	systemctl daemon-reload >/dev/null || true
fi
