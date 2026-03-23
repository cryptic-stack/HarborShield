#!/usr/bin/env sh
set -eu
cp .env.example .env
echo "Created .env from .env.example. Rotate secrets before first real use."
