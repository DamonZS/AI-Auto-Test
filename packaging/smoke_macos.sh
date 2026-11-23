#!/bin/bash
set -euo pipefail
scratch="$(mktemp -d)"
mount="$scratch/disk"
launcher="$scratch/Auto-Test.app/Contents/MacOS/Auto-Test"
data="$scratch/data"
cleanup() {
    if [[ -n "${desktop_pid:-}" ]]; then
        "$launcher" --stop --data-dir "$data"
        wait "$desktop_pid" || true
    fi
    if [[ -d "$mount" ]]; then hdiutil detach "$mount" || true; fi
    rm -rf -- "$scratch"
}
trap cleanup EXIT

hdiutil attach -readonly -nobrowse -mountpoint "$mount" "$1"
ditto "$mount/Auto-Test.app" "$scratch/Auto-Test.app"
hdiutil detach "$mount"
if [[ -n "$(find "$scratch/Auto-Test.app" -type f ! -path '*/certifi/cacert.pem' \( -name '.env*' -o -name '*.db*' -o -name '*.sqlite*' -o -name '*.key' -o -name '*.xlsx' -o -name '*.har' -o -name '*.pem' -o -name '*.pfx' \) -print -quit)" ]]; then
    echo 'Unexpected state or sample in app bundle' >&2
    exit 1
fi
port=39181
server="http://127.0.0.1:$port"
mkdir "$data"
export PLATFORM_AUTH_USERNAME=deployment-smoke
export PLATFORM_AUTH_PASSWORD="$(python3 -c 'import secrets; print(secrets.token_hex(20))')"
payload="$scratch/Auto-Test.app/Contents/Resources"
env -u PLATFORM_AUTH_USERNAME -u PLATFORM_AUTH_PASSWORD -u PLATFORM_WORKER_TOKEN \
    "$launcher" --data-dir "$data" --port "$port" --no-browser &
desktop_pid=$!
fixture="$("$payload/python/bin/python3" deploy/smoke.py "$server" prepare --desktop-payload "$payload")"
tenant="$(python3 -c 'import json,sys; print(json.load(sys.stdin)["tenant"])' <<< "$fixture")"
run="$(python3 -c 'import json,sys; print(json.load(sys.stdin)["run"])' <<< "$fixture")"
python3 deploy/smoke.py "$server" check "$tenant" "$run" || { cat "$data/desktop.log"; exit 1; }
test -f "$data/platform.db"
test -f "$data/platform.db.key"
test -f "$data/desktop-worker-token.txt"
"$launcher" --stop --data-dir "$data"
wait "$desktop_pid"
unset desktop_pid
if curl --silent --output /dev/null "$server/api/tenants"; then exit 1; fi
