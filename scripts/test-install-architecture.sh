#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
test_root="$(mktemp -d)"
trap 'rm -rf "$test_root"' EXIT
printf '#!/bin/sh\necho test-agent\n' > "$test_root/ironstate"
tar -czf "$test_root/fixture.tar.gz" -C "$test_root" ironstate

uname() {
  case "$1" in
    -s) printf '%s\n' "$test_os" ;;
    -m) printf '%s\n' "$test_arch" ;;
    -o) printf 'GNU/Linux\n' ;;
    *) return 1 ;;
  esac
}

curl() {
  local destination="" url=""
  while [ "$#" -gt 0 ]; do
    case "$1" in
      -o) destination="$2"; shift 2 ;;
      https://*) url="$1"; shift ;;
      *) shift ;;
    esac
  done
  case "$url" in
    */checksums.txt)
      printf '%s  %s\n' "$(sha256sum "$test_root/fixture.tar.gz" | awk '{print $1}')" "$test_asset" > "$destination"
      ;;
    */"$test_asset") cp "$test_root/fixture.tar.gz" "$destination" ;;
    *) echo "Unexpected download: $url" >&2; return 1 ;;
  esac
}

run_case() {
  local test_os="$1" test_arch="$2" expected_arch="$3"
  local case_dir="$test_root/${test_os}-${test_arch}"
  local test_asset="ironstate_1.2.3_$(printf '%s' "$test_os" | tr '[:upper:]' '[:lower:]')_${expected_arch}.tar.gz"
  mkdir -p "$case_dir"
  if (
    set -- --version v1.2.3 --no-checksum --dir "$case_dir/bin" --man-dir "$case_dir/man"
    source "$repo_root/install/install.sh"
  ) > "$case_dir/output" 2>&1; then
    if [ -z "$expected_arch" ] || [ ! -f "$case_dir/bin/ironstate" ] || ! grep -Fq "$test_asset" "$case_dir/output"; then
      cat "$case_dir/output" >&2
      echo "Incorrect installer result for $test_os/$test_arch" >&2
      return 1
    fi
  elif [ -n "$expected_arch" ] || ! grep -q 'unsupported OS/architecture' "$case_dir/output"; then
    cat "$case_dir/output" >&2
    echo "Unexpected installer failure for $test_os/$test_arch" >&2
    return 1
  fi
  echo "PASS $test_os/$test_arch"
}

run_case Linux armv7l armv7
run_case Linux armv7 armv7
run_case Linux aarch64 arm64
run_case Linux x86_64 amd64
run_case Linux armv6l ""
run_case Darwin armv7l ""