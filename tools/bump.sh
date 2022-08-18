#!/bin/sh

version="$1"
if [ -z "${version}" ]; then
    echo "Usage: $0 [version without v]"
    exit 0
fi

# NOTE: Following in-place option only works BSD sed.
sed -i "" -e "s|const version = \".*\"|const version = \"${version}\"|" pkg/cmd/fasthttpd.go
sed -i "" -e "s|VERSION=[^ ]*|VERSION=${version}|" README.md

git commit -m "Bump version to ${version}" \
    pkg/cmd/fasthttpd.go \
    README.md
git push
git tag "v${version}"
git push origin "v${version}"
