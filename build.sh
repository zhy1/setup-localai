#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT_DIR"

sanitize_name() {
  local value="$1"
  printf '%s' "$value" | tr -cs 'A-Za-z0-9._-' '-'
}

projectname="${PROJECTNAME:-$(basename "$ROOT_DIR")}" 
projectname="$(sanitize_name "$projectname")"

version="${VERSION:-$(git describe --tags --always 2>/dev/null || echo "dev")}" 
version="$(sanitize_name "${version#v}")"

platform="${PLATFORM:-$(go env GOOS)}"
platform="$(sanitize_name "$platform")"

arch="${ARCH:-$(go env GOARCH)}"
arch="$(sanitize_name "$arch")"

timestamp="$(date '+%y%m%d_%H%M%S')"

output_dir="$ROOT_DIR/outputs"
mkdir -p "$output_dir"

build_dir="$ROOT_DIR/.build_tmp/${projectname}_${version}_${platform}_${arch}"
rm -rf "$build_dir"
mkdir -p "$build_dir"

binary_name="$projectname"
if [[ "$platform" == "windows" ]]; then
  binary_name="${binary_name}.exe"
fi

echo "Building $projectname version $version for $platform/$arch"
GOOS="$platform" GOARCH="$arch" CGO_ENABLED="${CGO_ENABLED:-0}" \
  go build -trimpath -ldflags='-s -w' -o "$build_dir/$binary_name" .

archive_ext="tar.gz"
if [[ "$platform" == "windows" ]]; then
  archive_ext="zip"
fi

archive_name="${projectname}_${version}_${platform}_${arch}_${timestamp}.${archive_ext}"
archive_path="$output_dir/$archive_name"

if [[ "$archive_ext" == "zip" ]]; then
  (cd "$build_dir" && zip -rq "$archive_path" "$binary_name")
else
  tar -czf "$archive_path" -C "$build_dir" "$binary_name"
fi

echo "Created $archive_path"
