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
chown root:nanny /etc/nanny/nanny.toml
chmod 0640 /etc/nanny/nanny.toml

if command -v systemctl >/dev/null 2>&1; then
	systemctl daemon-reload >/dev/null 2>&1 || true
	systemctl enable nanny.service >/dev/null
	if [ -d /run/systemd/system ]; then
		if [ -n "${2:-}" ]; then
			systemctl try-restart nanny.service
		else
			systemctl start nanny.service
		fi
	fi
fi
