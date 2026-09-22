#!/bin/sh
set -eu

if [ "${1:-}" = "remove" ]; then
	if [ -d /run/systemd/system ] && command -v deb-systemd-invoke >/dev/null 2>&1; then
		deb-systemd-invoke stop nanny.service >/dev/null || true
	fi
	if command -v deb-systemd-helper >/dev/null 2>&1; then
		deb-systemd-helper disable nanny.service >/dev/null || true
	fi
fi
