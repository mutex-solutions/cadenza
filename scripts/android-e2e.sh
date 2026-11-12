#!/bin/sh
# End-to-end test of the Android app on a running emulator, with nothing faked:
# a real Cadenza server, the real cadenza CLI as the agent, the real APK, and a
# Maestro flow tapping through the app as the owner.
#
#   scripts/android-e2e.sh path/to/app.apk
#
# Needs: a booted emulator (adb devices), Go, python3 and maestro on PATH
# (or MAESTRO=/path/to/maestro). Exits non-zero if any step fails.
set -eu

apk="${1:?usage: scripts/android-e2e.sh path/to/app.apk}"
root=$(cd "$(dirname "$0")/.." && pwd)
work=$(mktemp -d)
port="${CADENZA_E2E_PORT:-18383}"
host_url="http://127.0.0.1:$port"
device_url="http://10.0.2.2:$port" # the host as the emulator sees it
password="maestro e2e password"
title="Maestro check $(date +%s)"
question="Ship the change?"
choice="Ship it"
maestro="${MAESTRO:-maestro}"
server_pid=""
wait_pid=""

cleanup() {
  [ -n "$wait_pid" ] && kill "$wait_pid" 2>/dev/null || true
  [ -n "$server_pid" ] && kill "$server_pid" 2>/dev/null || true
  rm -rf "$work"
}
trap cleanup EXIT

step() { printf '\n== %s\n' "$1"; }

step "Build the server and the CLI"
(cd "$root" && CGO_ENABLED=0 go build -tags nodynamic -o "$work/cadenza-server" ./cmd/cadenza-server)
(cd "$root" && CGO_ENABLED=0 go build -tags nodynamic -o "$work/cadenza" ./cmd/cadenza)

step "Start a server (trial mode) on port $port"
CADENZA_DATA_DIR="$work/data" CADENZA_ADDR="0.0.0.0:$port" CADENZA_PUBLIC_URL="$device_url" \
  "$work/cadenza-server" >"$work/server.log" 2>&1 &
server_pid=$!
for _ in $(seq 1 50); do curl -sf "$host_url/healthz" >/dev/null && break; sleep 0.2; done
token=$(curl -sf -XPOST -H 'Content-Type: application/json' -d "{\"password\":\"$password\"}" "$host_url/v1/auth/setup" |
  python3 -c 'import json,sys; print(json.load(sys.stdin)["token"])')

step "Pair the CLI as an agent machine"
export CADENZA_CONFIG_DIR="$work/cfg" CADENZA_SESSION="maestro-e2e" CADENZA_SERVER="$host_url"
"$work/cadenza" login "$host_url" >"$work/login.out" 2>"$work/login.err" &
login_pid=$!
code=""
for _ in $(seq 1 50); do
  code=$(sed -n 's/.*Pairing code: \([^ ]*\).*/\1/p' "$work/login.err" | head -1)
  [ -n "$code" ] && break
  sleep 0.2
done
curl -sf -XPOST -H "Authorization: Bearer $token" -H 'Content-Type: application/json' \
  -d "{\"code\":\"$code\",\"name\":\"e2e-agent\"}" "$host_url/v1/pair/approve" >/dev/null
wait "$login_pid"

step "The agent shows work and asks a question"
"$work/cadenza" show --new --title "$title" --template ui-change \
  --data '{"summary":"Checked end to end by Maestro.","changes":["One button moved"]}'
qid=$("$work/cadenza" ask "$question" --choice "$choice" --choice "Hold" --waiting |
  python3 -c 'import json,sys; print(json.load(sys.stdin)["question"])')
"$work/cadenza" wait "$qid" --timeout 300s >"$work/wait.out" 2>"$work/wait.err" &
wait_pid=$!

step "Install the app"
adb install -r "$apk"

step "Run the Maestro flow (the owner answers on the phone)"
"$maestro" test "$root/web/apps/android/e2e/answer-question.yaml" \
  -e SERVER_URL="$device_url" -e PASSWORD="$password" -e THREAD_TITLE="$title" \
  -e QUESTION="$question" -e CHOICE="$choice"

step "The agent's cadenza wait gets the answer"
if ! wait "$wait_pid"; then
  wait_pid=""
  echo "cadenza wait failed:"; cat "$work/wait.out" "$work/wait.err"; exit 1
fi
wait_pid=""
python3 - "$work/wait.out" "$choice" <<'PY'
import json, sys
out = json.load(open(sys.argv[1]))
assert out["status"] == "answered", out
assert out["answer"]["choice"] == sys.argv[2], out
print("answered with", repr(out["answer"]["choice"]))
PY
echo "\nPASS: the owner answered in the Android app and the agent received it."
