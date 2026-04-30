#!/usr/bin/env bash
# End-to-end smoke test for the face_control backend.
#
# Pre-requisites:
#   - The backend is running on $API (default :8080)
#   - The seed device 11111111-1111-1111-1111-111111111111 exists
#   - The camera at 192.0.0.22 is reachable
#   - A real face photo exists at $PHOTO
#
# What it does:
#   1. ping the seeded device (verifies the backend can reach 192.0.0.22:8000)
#   2. create a user with the photo
#   3. register that user on the device
#   4. verify the entry landed on the device by querying the camera directly
#   5. delete the registration via the backend
#   6. verify the entry is gone
#   7. (optional) delete the user record itself in DB cleanup-by-hand
#
# Usage:
#   ./scripts/smoke_e2e.sh                # uses defaults
#   API=http://localhost:8080 PHOTO=/path/to/face.jpg ./scripts/smoke_e2e.sh

set -euo pipefail

API=${API:-http://localhost:8080}
DEVICE_ID=${DEVICE_ID:-11111111-1111-1111-1111-111111111111}
PHOTO=${PHOTO:-/tmp/face_register.jpg}
CAMERA_HOST=${CAMERA_HOST:-192.0.0.22}
CAMERA_PORT=${CAMERA_PORT:-8000}
CAMERA_AUTH=${CAMERA_AUTH:-admin:admin}

red()   { printf '\e[31m%s\e[0m\n' "$*"; }
green() { printf '\e[32m%s\e[0m\n' "$*"; }
blue()  { printf '\e[34m%s\e[0m\n' "$*"; }

require_cmd() { command -v "$1" >/dev/null 2>&1 || { red "missing: $1"; exit 1; }; }
require_cmd curl
require_cmd jq

if [[ ! -f "$PHOTO" ]]; then
  red "photo not found: $PHOTO"
  red "convert one with: magick /path/to/source.png -strip -quality 85 -resize '1024x1024>' $PHOTO"
  exit 1
fi

blue "==> 0. ping backend"
curl -fsS "$API/healthz" | jq .

blue "==> 1. ping device through backend"
curl -fsS -X POST "$API/devices/$DEVICE_ID/ping" | jq .

blue "==> 2. create user with photo"
USER_JSON=$(curl -fsS -F "full_name=Smoke Test User" -F "photo=@$PHOTO" "$API/users")
echo "$USER_JSON" | jq .
USER_ID=$(echo "$USER_JSON" | jq -r .id)
green "user_id=$USER_ID"

blue "==> 3. register user on device"
REG_RESP=$(curl -fsS -X POST "$API/devices/$DEVICE_ID/registrations" \
  -H 'Content-Type: application/json' \
  -d "{\"user_id\":\"$USER_ID\"}")
echo "$REG_RESP" | jq .
FACE_ID=$(echo "$REG_RESP" | jq -r .face_id)
green "face_id=$FACE_ID"

blue "==> 4. verify entry exists on the camera (direct query)"
PERSONS=$(curl -fsS -X POST "http://$CAMERA_HOST:$CAMERA_PORT/" \
  -u "$CAMERA_AUTH" \
  -H 'Content-Type: application/json; charset=utf-8' \
  -H 'Expect:' \
  -d '{"version":"0.2","cmd":"request persons","role":-1,"page_no":1,"page_size":20,"feature_flag":0,"image_flag":0}')
echo "$PERSONS" | jq --arg fid "$FACE_ID" '.persons[] | select(.id == $fid) | {id, name, role}'
if echo "$PERSONS" | jq -e --arg fid "$FACE_ID" '.persons[] | select(.id == $fid)' >/dev/null; then
  green "✓ camera has the face_id"
else
  red "✗ camera does NOT have face_id=$FACE_ID"; exit 2
fi

blue "==> 5. list this device's registrations from backend"
curl -fsS "$API/devices/$DEVICE_ID/registrations" | jq .

# blue "==> 6. delete registration through backend"
# HTTP_CODE=$(curl -sS -o /tmp/del_resp.json -w '%{http_code}' \
#   -X DELETE "$API/devices/$DEVICE_ID/registrations/$USER_ID")
# echo "delete returned HTTP $HTTP_CODE"
# if [[ "$HTTP_CODE" != "204" ]]; then
#   cat /tmp/del_resp.json; red "delete failed"; exit 3
# fi

# blue "==> 7. confirm entry is gone from camera"
# PERSONS=$(curl -fsS -X POST "http://$CAMERA_HOST:$CAMERA_PORT/" \
#   -u "$CAMERA_AUTH" \
#   -H 'Content-Type: application/json; charset=utf-8' \
#   -H 'Expect:' \
#   -d '{"version":"0.2","cmd":"request persons","role":-1,"page_no":1,"page_size":20,"feature_flag":0,"image_flag":0}')
# if echo "$PERSONS" | jq -e --arg fid "$FACE_ID" '.persons[] | select(.id == $fid)' >/dev/null; then
#   red "✗ camera STILL has face_id=$FACE_ID after delete"; exit 4
# else
#   green "✓ camera no longer has the face_id"
# fi

# green "==> ALL GOOD. user_id=$USER_ID face_id=$FACE_ID"
