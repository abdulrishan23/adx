#!/usr/bin/env sh
# Builds a release into its own directory and switches the `current` symlinks
# only when the build and the readiness check pass. A failed build leaves the
# running release untouched. Run from a checkout on the server as the adx user.
#
#   deploy/release.sh /srv/adx
#
set -eu
ROOT=${1:?usage: release.sh ROOT_DIR}
STAMP=$(date -u +%Y%m%dT%H%M%SZ)
SRC=$(cd "$(dirname "$0")/.." && pwd)
SERVER_RELEASE="$ROOT/server/releases/$STAMP"
WEB_RELEASE="$ROOT/web/releases/$STAMP"

mkdir -p "$SERVER_RELEASE" "$WEB_RELEASE"

# 1. Build both halves into the new release directories. Any failure exits
#    here, before anything that serves traffic changes.
(cd "$SRC/server" && go build -o "$SERVER_RELEASE/adx" ./cmd/adx)
(cd "$SRC/web" && pnpm install --frozen-lockfile && pnpm build && cp -R dist/. "$WEB_RELEASE/")

# 2. Switch the server binary and restart. systemd starts the new binary; if
#    it can't open its storage it exits and systemd keeps retrying the old
#    symlink target isn't restored automatically, so we check readiness next.
PREVIOUS=$(readlink "$ROOT/server/current" || true)
ln -sfn "$SERVER_RELEASE" "$ROOT/server/current.next" && mv -Tf "$ROOT/server/current.next" "$ROOT/server/current"
sudo systemctl restart adx

# 3. Wait for readiness. On failure, point back at the previous release and
#    restart, so the last working deployment is what serves.
for _ in $(seq 1 30); do
  if curl -fsS http://127.0.0.1:8080/api/ready >/dev/null 2>&1; then
    ln -sfn "$WEB_RELEASE" "$ROOT/web/current.next" && mv -Tf "$ROOT/web/current.next" "$ROOT/web/current"
    echo "released $STAMP"
    exit 0
  fi
  sleep 1
done

echo "release $STAMP never became ready; rolling the server back" >&2
if [ -n "$PREVIOUS" ]; then
  ln -sfn "$PREVIOUS" "$ROOT/server/current.next" && mv -Tf "$ROOT/server/current.next" "$ROOT/server/current"
  sudo systemctl restart adx
fi
exit 1
