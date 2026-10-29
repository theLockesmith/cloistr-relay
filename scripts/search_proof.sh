#!/usr/bin/env bash
# Read-only NIP-50 search proof with a control.
# For each kind, take recent events via a plain REQ (the control: proves the
# events exist), pick a distinctive word from one of them, then NIP-50 search
# for that word. Search works iff the search REQ returns the control event's id
# and a search for a random nonsense word returns nothing (negative control).
# A kind that does not PASS is retried once after RETRY_PAUSE seconds (default
# 5); every failed attempt prints why (NO CONTROL / NO USABLE WORD / FAIL /
# TIMEOUT / ERROR). Requires nak and python3. Exits 0 only if every kind
# PASSes, on its first attempt or its retry, and then prints GATE PASS; 1
# otherwise.
# Usage: scripts/search_proof.sh wss://relay.cloistr.xyz [kind ...]   (default kinds: 1 33301)
set -u
# nak req reads filters from stdin whenever stdin is not a terminal, so with an
# open, silent stdin (a CI runner, an agent shell) it hangs until the timeout.
# Every nak call below gets </dev/null for that reason.
R="$1"; shift
KINDS="${*:-1 33301}"

# One attempt for one kind. Prints its findings and a final "RESULT" line;
# returns 0 only on PASS.
check_kind() {
  local K=$1 RAW RC CTRL PICK ID WORD MINLEN HITS N
  RAW=$(timeout 30 nak req -k "$K" -l "${SAMPLE:-200}" "$R" </dev/null 2>/dev/null); RC=$?
  [ "$RC" = 124 ] && { echo "RESULT kind $K: TIMEOUT (control REQ took over 30s)"; return 1; }
  CTRL=$(printf '%s\n' "$RAW" | grep '^{')
  echo "control events: $(printf '%s\n' "$CTRL" | grep -c '^{')"
  if [ -z "$CTRL" ]; then
    [ "$RC" != 0 ] && { echo "RESULT kind $K: ERROR (control REQ failed, nak exit $RC)"; return 1; }
    echo "RESULT kind $K: NO CONTROL (no events of this kind)"; return 1
  fi
  # Pick the most distinctive word across the whole sample: the longest whole
  # whitespace-delimited word, preferring 7+ letters and falling back to 5+
  # (minus stopwords, which the english config drops from the index) when no
  # recent post has a long word. Only content is indexed, so only content is
  # read. A letter run cut out of a longer token (a nostr: URI, a bech32
  # string, a URL) is not a token in the index and can never match, so those
  # are skipped.
  PICK=$(printf '%s\n' "$CTRL" | python3 -I -c '
import sys, json, re
BECH32 = re.compile(r"^(npub|nsec|note|nevent|nprofile|naddr|nrelay|lnurl|lnbc)1", re.I)
STOP = set("about above after again against because before being below between could doing during further having other ought their theirs there these those through under until where which while whom would yours yourself yourselves itself myself ourselves themselves".split())
def words(text, minlen):
      out = set()
      for tok in text.split():
          if tok.lower().startswith(("nostr:", "http:", "https:", "wss:")) or BECH32.match(tok):
              continue
          w = tok.strip(".,;:!?()[]{}\"'\''")
          if len(w) >= minlen and w.isascii() and w.isalpha() and w.lower() not in STOP:
              out.add(w.lower())
      return out
events = [json.loads(l) for l in sys.stdin if l.strip()]
for minlen in (7, 5):
      best = None
      for e in events:
          for w in words(e.get("content", ""), minlen):
              if best is None or len(w) > len(best[1]):
                  best = (e["id"], w)
      if best:
          print(best[0], best[1], minlen); break')
  [ -z "$PICK" ] && { echo "RESULT kind $K: NO USABLE WORD in $(printf '%s\n' "$CTRL" | grep -c '^{') control events"; return 1; }
  read -r ID WORD MINLEN <<<"$PICK"
  echo "control id: $ID  word: $WORD  (min length $MINLEN)"
  RAW=$(timeout 20 nak req -k "$K" --search "$WORD" -l 500 "$R" </dev/null 2>/dev/null); RC=$?
  [ "$RC" = 124 ] && { echo "RESULT kind $K: TIMEOUT (search REQ took over 20s)"; return 1; }
  HITS=$(printf '%s\n' "$RAW" | grep '^{')
  N=$(printf '%s\n' "$HITS" | grep -c '^{')
  # Second signal, independent of the picked word: a common term.
  echo "common-word search 'nostr': $(timeout 20 nak req -k "$K" --search nostr -l 50 "$R" </dev/null 2>/dev/null | grep -c '^{') hits"
  if ! printf '%s\n' "$HITS" | grep -q "\"id\":\"$ID\""; then
    echo "RESULT kind $K: FAIL (search returned $N, control missing, nak exit $RC)"; return 1
  fi
  # Negative control: a relay that ignores the search field answers a plain
  # REQ, which also contains the control. A random 14-letter word matches
  # nothing, so any hit means search was not applied.
  local NONSENSE NN
  NONSENSE=zq$(LC_ALL=C tr -dc 'a-z' </dev/urandom | head -c 12)
  RAW=$(timeout 20 nak req -k "$K" --search "$NONSENSE" -l 50 "$R" </dev/null 2>/dev/null); RC=$?
  [ "$RC" = 124 ] && { echo "RESULT kind $K: TIMEOUT (nonsense-word search took over 20s)"; return 1; }
  NN=$(printf '%s\n' "$RAW" | grep -c '^{')
  if [ "$NN" != 0 ]; then
    echo "RESULT kind $K: FAIL (nonsense word '$NONSENSE' returned $NN events; relay is not applying search)"; return 1
  fi
  echo "RESULT kind $K: PASS (search returned $N, includes control; nonsense word returned 0)"
}

OK=1
for K in $KINDS; do
  echo "=== kind $K ==="
  check_kind "$K" && continue
  echo "--- kind $K: retrying once in ${RETRY_PAUSE:-5}s ---"
  sleep "${RETRY_PAUSE:-5}"
  check_kind "$K" || { echo "GATE kind $K: failed on first attempt and retry"; OK=0; }
done
[ "$OK" = 1 ] || exit 1
echo "GATE PASS: search works for kinds $KINDS"
