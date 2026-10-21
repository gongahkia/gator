#!/bin/sh
set -eu

# Migration message for old installer links. Do not download or install a CLI.
printf '%s\n' \
  'The standalone Gator CLI installer is retired.' \
  'Desktop releases (when published):' \
  'https://github.com/gongahkia/gator/releases/latest' \
  'For development: npm ci --prefix desktop && make desktop'
exit 1
