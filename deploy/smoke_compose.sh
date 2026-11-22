#!/bin/bash
set -euo pipefail
export AUTOTEST_PORT=39181
compose=(docker compose -p "autotest-smoke-$$" -f deploy/compose.yaml --profile worker)
scratch="$(mktemp -d)"
cleanup() {
    rm -f -- "$scratch/worker-token"
    rmdir -- "$scratch"
    if [[ $1 != 0 ]]; then
        "${compose[@]}" ps -a || true
        "${compose[@]}" logs --tail 60 control worker || true
    fi
    "${compose[@]}" down --volumes || true
}
trap 'cleanup $?' EXIT

"${compose[@]}" build control
export PLATFORM_AUTH_USERNAME=deployment-smoke
export PLATFORM_AUTH_PASSWORD="$(python3 -c 'import secrets; print(secrets.token_hex(20))')"
"${compose[@]}" run --rm -e PLATFORM_AUTH_USERNAME -e PLATFORM_AUTH_PASSWORD control \
    autotest-server -db /state/platform.db -provision-user -auth-tenant deployment-smoke -auth-tenant-name 'Deployment smoke'
"${compose[@]}" up -d --no-build --wait control
server="http://127.0.0.1:$AUTOTEST_PORT"
fixture="$(python3 deploy/smoke.py "$server" prepare --browser-url http://control:8090 --worker-token-file "$scratch/worker-token")"
tenant="$(python3 -c 'import json,sys; print(json.load(sys.stdin)["tenant"])' <<< "$fixture")"
run="$(python3 -c 'import json,sys; print(json.load(sys.stdin)["run"])' <<< "$fixture")"
read -r PLATFORM_WORKER_TOKEN < "$scratch/worker-token"
export PLATFORM_WORKER_TOKEN
rm -- "$scratch/worker-token"
"${compose[@]}" up -d --no-build worker
unset PLATFORM_WORKER_TOKEN
"${compose[@]}" exec -T worker python -c 'import http.client, os; connection = http.client.HTTPConnection("control", 8090, timeout=10); connection.request("GET", "/api/agent/settings", headers={"Authorization": "Bearer " + os.environ["PLATFORM_WORKER_TOKEN"]}); status = connection.getresponse().status; print("Worker settings HTTP", status); raise SystemExit(status != 200)'
python3 deploy/smoke.py "$server" check "$tenant" "$run"
"${compose[@]}" down
"${compose[@]}" up -d --no-build --wait control
python3 deploy/smoke.py "$server" check "$tenant" "$run"
