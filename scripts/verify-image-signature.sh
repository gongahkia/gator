#!/bin/sh
set -eu

image="${1:?usage: scripts/verify-image-signature.sh IMAGE@sha256:DIGEST}"
: "${NORBOT_COSIGN_CERTIFICATE_IDENTITY:?set the expected certificate identity regexp}"
: "${NORBOT_COSIGN_OIDC_ISSUER:=https://token.actions.githubusercontent.com}"
case "$image" in *@sha256:*) ;; *) echo "image must use an immutable sha256 digest" >&2; exit 2;; esac
command -v cosign >/dev/null || { echo "cosign is required" >&2; exit 2; }
cosign verify --certificate-identity-regexp "$NORBOT_COSIGN_CERTIFICATE_IDENTITY" --certificate-oidc-issuer "$NORBOT_COSIGN_OIDC_ISSUER" "$image"
