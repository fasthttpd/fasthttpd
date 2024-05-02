#!/bin/sh

set -eu

cd "$(dirname "$0")"
VERSION="$1"

MACHINE="$(uname -m)"
GOOS="linux"
GOARCH=""
ARCH="${MACHINE}"
case "${MACHINE}" in
    "aarch64"   ) GOARCH="arm64" ; ARCH="arm64" ;;
    "x86_64"    ) GOARCH="amd64" ; ARCH="amd64" ;;
    "i386"      ) GOARCH="386"                  ;;
esac

if [ -z "${GOARCH}" ]; then
    echo "Error: unsupported machine: ${MACHINE}"
    exit 1
fi

apt update -y
DEBIAN_FRONTEND=noninteractive apt install -y --no-install-recommends \
    build-essential fakeroot devscripts cdbs debhelper curl

DEST="fasthttpd-${VERSION}"
mkdir -p \
  "${DEST}/bin" \
  "${DEST}/src/etc/fasthttpd" \
  "${DEST}/src/usr/share/fasthttpd/html"
cp -rf debian "${DEST}"

curl -fsSL "https://github.com/fasthttpd/fasthttpd/releases/download/v${VERSION}/fasthttpd_${VERSION}_${GOOS}_${GOARCH}.tar.gz" | \
    tar xz -C "${DEST}/bin" fasthttpd
cp -f ../../examples/config.default.yaml "${DEST}/src/etc/fasthttpd/config.yaml"
cp -rf ../../examples/public/* "${DEST}/src/usr/share/fasthttpd/html"

(
    cd "${DEST}/debian"

    sed -e "s/<VERSION>/${VERSION}/g" \
        -e "s/<DATE>/$(LANG=en-US date '+%a, %d %b %Y %H:%M:%S %z')/g" \
        changelog.tpl >changelog
    sed -e "s/<ARCH>/${ARCH}/g" control.tpl >control
    debuild -uc -us -b
)