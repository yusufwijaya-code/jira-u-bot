#!/usr/bin/env bash
# Runs ON the server. Unpacks a release built by .github/workflows/deploy.yml,
# switches /var/www/nudge/current to it, restarts the service and rolls back
# when /nudge/ping does not answer.
#
# Usage: tar -xzf nudge.tar.gz -O ./deploy/remote-deploy.sh | bash -s -- nudge.tar.gz <release-id>
set -euo pipefail

APP=/var/www/nudge
SERVICE=nudge
KEEP=5
tarball=$1
id=$2

rel="$APP/releases/$id"
rm -rf "$rel"
mkdir -p "$rel"
tar -xzf "$tarball" -C "$rel"
rm -f "$tarball"
chmod +x "$rel/nudge"

switch_to() {
    ln -sfn "$1" "$APP/current.tmp"
    mv -Tf "$APP/current.tmp" "$APP/current"
}

prev=$(readlink -f "$APP/current" 2>/dev/null || true)
switch_to "$rel"
systemctl restart "$SERVICE"

port=$(grep -E '^PORT=' "$APP/shared/.env" | cut -d= -f2 | tr -d '[:space:]')
ok=""
for _ in $(seq 1 20); do
    if curl -sf "http://127.0.0.1:${port:-8081}/nudge/ping" >/dev/null; then ok=1; break; fi
    sleep 2
done

if [ -z "$ok" ]; then
    echo "!! $id did not answer /nudge/ping — rolling back" >&2
    journalctl -u "$SERVICE" -n 40 --no-pager >&2 || true
    if [ -n "$prev" ] && [ "$prev" != "$rel" ]; then
        switch_to "$prev"
        systemctl restart "$SERVICE"
    fi
    exit 1
fi

# Keep the newest $KEEP releases (and never the live one).
live=$(readlink -f "$APP/current")
ls -1dt "$APP"/releases/* | tail -n +$((KEEP + 1)) | while read -r old; do
    [ "$old" = "$live" ] || rm -rf "$old"
done
echo "deployed $id"
