#!/usr/bin/env bash

version=$(grep -oP "Version = \"(\K.*)(?=\")" pkg/version/version.go)
tar -zcvf "nanny.$version.tar.gz" nanny nanny.toml