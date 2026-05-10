#!/usr/bin/env bash
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
ENV="${MARCHE_ENV:-dev}"
CFG_DIR="${MARCHE_CONFIG_DIR:-$HERE/../cfg}"
exec java \
  -Dmarche.env="$ENV" \
  -Dmarche.config.dir="$CFG_DIR" \
  -cp "$HERE/lib/*" \
  io.sn.marche.polaris.wanda.Main
