#!/bin/sh
set -eu

if ! getent group nanny >/dev/null; then
	addgroup --system nanny
fi

if ! getent passwd nanny >/dev/null; then
	adduser --system --disabled-login --disabled-password \
		--home /var/lib/nanny --no-create-home --ingroup nanny nanny
fi

install -d -o nanny -g nanny -m 0750 /var/lib/nanny
if [ -e /etc/nanny/nanny.toml ]; then
	chown root:nanny /etc/nanny/nanny.toml
	chmod 0640 /etc/nanny/nanny.toml
fi

if [ -z "${2:-}" ] && command -v deb-systemd-helper >/dev/null 2>&1; then
	deb-systemd-helper unmask nanny.service >/dev/null || true
	deb-systemd-helper enable nanny.service >/dev/null || true
fi

if [ -d /run/systemd/system ] && command -v deb-systemd-invoke >/dev/null 2>&1; then
	systemctl daemon-reload >/dev/null || true
	if [ -n "${2:-}" ]; then
		deb-systemd-invoke try-restart nanny.service >/dev/null || true
	else
		deb-systemd-invoke start nanny.service >/dev/null || true
	fi
fi
