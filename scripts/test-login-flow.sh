#!/bin/bash
# ポータル完全ログインフロー検証スクリプト
set -e
cd /tmp
rm -f portal-cookies.txt

echo "=== 1. /login (portal -> oidc authorize) ==="
LOGIN_REDIRECT=$(curl -s -o /dev/null -w '%{redirect_url}' http://localhost:7080/login)
AUTH_URL="$LOGIN_REDIRECT"
echo "auth url: ${AUTH_URL:0:80}..."

echo "=== 2. authorize -> login page ==="
AUTH_REDIRECT=$(curl -s -o /dev/null -w '%{redirect_url}' "$AUTH_URL")
LOGIN_STATE=$(echo "$AUTH_REDIRECT" | sed 's/.*state=//')
echo "login state: $LOGIN_STATE"

echo "=== 3. mock login (tanaka) -> callback ==="
CALLBACK_URL=$(curl -s -o /dev/null -w '%{redirect_url}' -X POST "http://localhost:8090/callback" \
    -d "state=$LOGIN_STATE&username=tanaka")
echo "callback url: ${CALLBACK_URL:0:80}..."

echo "=== 4. portal callback -> session cookie ==="
curl -s -c portal-cookies.txt -o /dev/null -w "status: %{http_code}\n" "$CALLBACK_URL"
SESSION=$(grep osgsuken portal-cookies.txt | awk '{print $NF}')
echo "session token: ${SESSION:0:20}..."

echo "=== 5. /api/me with session ==="
curl -s -b "osgsuken_session=$SESSION" http://localhost:7080/api/me
echo

echo "=== 6. / with session (portal page) ==="
curl -s -b "osgsuken_session=$SESSION" http://localhost:7080/ | grep -o "こんにちは" | head -1
