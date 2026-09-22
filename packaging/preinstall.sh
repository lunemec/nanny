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
