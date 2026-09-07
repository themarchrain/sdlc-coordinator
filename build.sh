#!/usr/bin/env bash
set -euo pipefail

TAG="${TAG:-latest}"
IMAGE_NAME="sdlc-coordinator"
WEB_DIR="${WEB_DIR:-web}"

echo "Building web assets (npm ci + npm run build)..."

(
  cd "${WEB_DIR}"
  npm ci
  npm run build
)

GOPROXY="${GOPROXY:-https://proxy.golang.org,direct}"

echo "Building ${IMAGE_NAME}:${TAG}"

docker build \
  --build-arg GOPROXY="${GOPROXY}" \
  -t "${IMAGE_NAME}:${TAG}" \
  .

echo "Done: ${IMAGE_NAME}:${TAG}"
