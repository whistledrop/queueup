#!/usr/bin/env bash
# Switch QueueUp from test money to real money.
#
# Run this on your own machine, with your live Stripe key. The key is read
# without echoing and never leaves this shell: it is handed to Stripe and to
# Fly and is not written to a file, a log or your shell history.
#
#   ./scripts/go-live.sh TIKTOK YOUTUBE          mints those channel codes too
#
# The order matters and is the whole point of having a script. The gate goes on
# LAST, after there is a way through it: billing on with Stripe half configured
# locks every customer out of the thing they came for.
set -euo pipefail
cd "$(dirname "$0")/.."

app="${FLY_APP:-queueup-relay}"
fly="$(ls -d /opt/homebrew/Cellar/flyctl/*/bin/flyctl 2>/dev/null | tail -1 || true)"
[ -x "${fly:-}" ] || fly="$(command -v fly || command -v flyctl)"
[ -x "$fly" ] || { echo "flyctl not found. Try: brew link --overwrite flyctl"; exit 1; }

echo "Paste your LIVE Stripe secret key (it will not be shown):"
read -rs QUEUEUP_STRIPE_SECRET_KEY
export QUEUEUP_STRIPE_SECRET_KEY
echo

# rk_live_ is a restricted key, which Stripe's newer dashboard creates by
# default when you ask for a secret key. It works exactly as well as sk_live_
# provided the permissions it was given cover what we do; if one is missing it
# fails later, at a checkout, which is why the step below checks before
# anything is switched on.
case "$QUEUEUP_STRIPE_SECRET_KEY" in
  sk_live_*|rk_live_*) ;;
  sk_test_*|rk_test_*) echo "That is a TEST key. This script is for going live."; exit 1 ;;
  *)                   echo "That does not look like a Stripe key."; exit 1 ;;
esac

echo "==> Checking the key can do everything QueueUp needs"
probe() {
  code=$(curl -s -o /tmp/qu-probe.json -w "%{http_code}" -u "$QUEUEUP_STRIPE_SECRET_KEY:" "$@")
  if [ "$code" = "403" ]; then
    echo "    MISSING PERMISSION: $(python3 -c "import json;print(json.load(open('/tmp/qu-probe.json'))['error']['message'])" 2>/dev/null)"
    return 1
  fi
  return 0
}
missing=0
probe https://api.stripe.com/v1/products?limit=1          || missing=1
probe https://api.stripe.com/v1/prices?limit=1            || missing=1
probe https://api.stripe.com/v1/coupons?limit=1           || missing=1
probe https://api.stripe.com/v1/promotion_codes?limit=1   || missing=1
probe https://api.stripe.com/v1/subscriptions?limit=1     || missing=1
probe https://api.stripe.com/v1/customers?limit=1         || missing=1
probe https://api.stripe.com/v1/webhook_endpoints?limit=1 || missing=1
rm -f /tmp/qu-probe.json
if [ "$missing" = "1" ]; then
  cat <<'NEEDS'

That key cannot do everything QueueUp needs, and a key that is short a
permission does not fail now, it fails later at somebody's checkout.

In Stripe: Developers, API keys, click the key, and give it full access, or
Write on Products, Prices, Coupons, Promotion codes, Checkout Sessions,
Subscriptions, Customers, Webhook Endpoints and the Customer portal.

Nothing has been changed. Run this again when it is sorted.
NEEDS
  exit 1
fi
echo "    the key can do everything"


echo "==> Creating the product, price, discounts and webhook in Stripe"
out="$(go run ./cmd/relay stripe-setup)"
echo "$out" | grep -v "SECRET" | sed 's/^/    /'

ids="$(echo "$out" | grep -oE 'QUEUEUP_STRIPE_[A-Z_]+=[^ ]+' || true)"
for want in PRICE_ID INTRO_COUPON_ID REFERRAL_COUPON_ID WEBHOOK_SECRET; do
  echo "$ids" | grep -q "QUEUEUP_STRIPE_$want=" || { echo "Stripe setup did not return $want; stopping before anything is switched on."; exit 1; }
done

echo "==> Storing them on the relay"
# shellcheck disable=SC2086
"$fly" secrets set QUEUEUP_STRIPE_SECRET_KEY="$QUEUEUP_STRIPE_SECRET_KEY" $ids -a "$app" >/dev/null
echo "    stored"

for code in "$@"; do
  echo "==> Minting $code"
  QUEUEUP_STRIPE_INTRO_COUPON_ID="$(echo "$ids" | grep INTRO_COUPON_ID | cut -d= -f2)" \
    go run ./cmd/relay stripe-code "$code" | sed 's/^/    /'
done

echo "==> Checking the relay is healthy before opening the gate"
for _ in $(seq 1 20); do
  curl -fsS https://queueup-relay.fly.dev/healthz >/dev/null 2>&1 && break
  sleep 3
done
curl -fsS https://queueup-relay.fly.dev/healthz >/dev/null || { echo "Relay is not answering. Billing left OFF."; exit 1; }

echo "==> Turning the subscription gate on"
"$fly" secrets set QUEUEUP_BILLING=on -a "$app" >/dev/null

cat <<'DONE'

Live.

Now do this before any marketing goes out:

  1. Open queueuprust.com in a private window and subscribe with a REAL card.
  2. Check the money appears in your Stripe dashboard.
  3. Cancel it from Settings, and check that reads correctly too.

Five pounds to prove the whole loop with real money is the cheapest
insurance you will buy. A broken checkout found by your first customer
costs the customer.
DONE
