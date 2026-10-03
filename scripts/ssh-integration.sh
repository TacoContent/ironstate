#!/usr/bin/env bash
# Runs TestSSHIntegration against a throwaway sshd container. Shared by
# 'task test:ssh' and the ci.yml 'ssh-integration' job.
set -euo pipefail

if ! command -v docker >/dev/null 2>&1 || ! docker info >/dev/null 2>&1; then
  echo "⚠ skipping ssh integration test: docker is not available - CI still runs this job." >&2
  exit 0
fi
for tool in ssh ssh-keygen go; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo "⚠ skipping ssh integration test: '$tool' not found." >&2
    exit 0
  fi
done

image="${IRONSTATE_SSHD_IMAGE:-lscr.io/linuxserver/openssh-server:latest}"
work="$(mktemp -d)"
name="ironstate-sshd-$$"
cleanup() {
  docker rm -f "$name" >/dev/null 2>&1 || true
  rm -rf "$work"
}
trap cleanup EXIT

ssh-keygen -q -t ed25519 -N '' -f "$work/id"
docker run -d --rm --name "$name" -p 127.0.0.1::2222 \
  -e PUBLIC_KEY="$(cat "$work/id.pub")" -e USER_NAME=ironstate "$image" >/dev/null
port="$(docker port "$name" 2222/tcp | head -n1 | sed 's/.*://')"

# accept-new is only for this throwaway container's freshly generated host key.
cat >"$work/config" <<EOF
Host ironstate-test
  HostName 127.0.0.1
  Port $port
  User ironstate
  IdentityFile $work/id
  IdentitiesOnly yes
  UserKnownHostsFile $work/known_hosts
  StrictHostKeyChecking accept-new
EOF
chmod 600 "$work/config"

for _ in $(seq 1 60); do
  if ssh -F "$work/config" -o BatchMode=yes ironstate-test true 2>/dev/null; then
    ready=1
    break
  fi
  sleep 1
done
[ "${ready:-}" = 1 ] || { echo "sshd did not come up" >&2; docker logs "$name" >&2; exit 1; }

case "$(docker exec "$name" uname -m)" in
  x86_64) arch=amd64 ;;
  aarch64) arch=arm64 ;;
  *) echo "unsupported container arch" >&2; exit 1 ;;
esac
CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -o "$work/ironstate" ./cmd/ironstate

IRONSTATE_SSH_TEST_TARGET=ironstate-test \
IRONSTATE_SSH_TEST_CONFIG="$work/config" \
IRONSTATE_SSH_TEST_AGENT="linux/$arch=$work/ironstate" \
  go test ./internal/remoteexec/ -run TestSSHIntegration -count=1 -v
