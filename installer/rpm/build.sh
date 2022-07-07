#!/bin/sh

set -eu

cd "$(dirname "$0")"
VERSION="$1"

MACHINE="$(uname -m)"
GOOS="Linux"
GOARCH=""
ARCH="${MACHINE}"
case "${MACHINE}" in
    "aarch64"   ) GOARCH="arm64" ; ARCH="arm64" ;;
    "amd64"     ) GOARCH="x86_64"               ;;
    "x86_64"    ) GOARCH="x86_64"; ARCH="amd64" ;;
    "i386"      ) GOARCH="i386"                 ;;
esac

if [ -z "${GOARCH}" ]; then
    echo "Error: unsupported machine: ${MACHINE}"
    exit 1
fi

apt update -y
DEBIAN_FRONTEND=noninteractive apt install -y --no-install-recommends \
    rpm curl ca-certificates

BUILD_DIR="$(pwd)/build"
mkdir -p "${BUILD_DIR}/SOURCES" "${BUILD_DIR}/SPECS"

curl -fsSL "https://github.com/fasthttpd/fasthttpd/releases/download/v${VERSION}/fasthttpd_${VERSION}_${GOOS}_${GOARCH}.tar.gz" | \
    tar xz -C "${BUILD_DIR}/SOURCES" fasthttpd
cp -f ./fasthttpd.service "${BUILD_DIR}/SOURCES"
cp -f ../../examples/config.default.yaml "${BUILD_DIR}/SOURCES"
cp -rf ../../examples/public/* "${BUILD_DIR}/SOURCES"

sed -e "s/<VERSION>/${VERSION}/g" \
    ./fasthttpd.spec.tpl >"${BUILD_DIR}/SPECS/fasthttpd.spec"

rpmbuild --define "_topdir ${BUILD_DIR}" -bb "${BUILD_DIR}/SPECS/fasthttpd.spec"