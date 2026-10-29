#!/usr/bin/env bash
# Tests for search_proof.sh against a stub nak (no relay needed).
# Usage: scripts/search_proof_test.sh [path/to/search_proof.sh]
set -u
SCRIPT=${1:-$(dirname "$0")/search_proof.sh}
TMP=$(mktemp -d); trap 'rm -rf "$TMP"' EXIT

# Stub nak. Plain REQ and the picked-word search return the control event; the
# 'nostr' search returns nothing; the nonsense-word search (always 'zq...')
# behaves per $NONSENSE_MODE: "empty" (exit 0, no events) or "fail" (exit 1).
cat >"$TMP/nak" <<'STUB'
#!/usr/bin/env bash
EVT='{"id":"aaaa","kind":1,"content":"wonderful elephants gather"}'
SEARCH=""
while [ $# -gt 0 ]; do [ "$1" = --search ] && SEARCH=$2; shift; done
case "$SEARCH" in
  "")      echo "$EVT" ;;
  zq*)     [ "${NONSENSE_MODE:-empty}" = fail ] && { echo "connection refused" >&2; exit 1; } ;;
  nostr)   ;;
  *)       echo "$EVT" ;;
esac
exit 0
STUB
chmod +x "$TMP/nak"

FAILS=0
run() { PATH="$TMP:$PATH" RETRY_PAUSE=0 NONSENSE_MODE=$1 bash "$SCRIPT" wss://stub 1 2>&1; }

OUT=$(run empty); RC=$?
if [ $RC = 0 ] && grep -q "GATE PASS" <<<"$OUT"; then echo "ok   healthy relay passes"
else echo "FAIL healthy relay should pass (exit $RC)"; echo "$OUT"; FAILS=1; fi

OUT=$(run fail); RC=$?
if [ $RC != 0 ] && grep -q "ERROR (nonsense-word search failed" <<<"$OUT"; then echo "ok   failed nonsense-word request reports ERROR"
else echo "FAIL failed nonsense-word request must report ERROR and exit non-zero (exit $RC)"; echo "$OUT"; FAILS=1; fi

exit $FAILS
