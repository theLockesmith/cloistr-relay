#!/usr/bin/env bash
# Read-only NIP-50 search proof with a control.
# For each kind, take recent events via a plain REQ (the control: proves the
# events exist), pick a distinctive word from one of them, then NIP-50 search
# for that word. Search works iff the search REQ returns the control event's id.
# Requires nak and python3. Exits 0 only if every kind PASSes, 1 otherwise.
# Usage: scripts/search_proof.sh wss://relay.cloistr.xyz [kind ...]   (default kinds: 1 33301)
set -u
R="$1"; shift
KINDS="${*:-1 33301}"
OK=1
for K in $KINDS; do
  echo "=== kind $K ==="
  CTRL=$(timeout 30 nak req -k "$K" -l "${SAMPLE:-200}" "$R" 2>/dev/null | grep '^{')
  echo "control events: $(printf '%s\n' "$CTRL" | grep -c '^{')"
  [ -z "$CTRL" ] && { echo "RESULT kind $K: NO CONTROL (no events of this kind)"; OK=0; continue; }
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
  [ -z "$PICK" ] && { echo "RESULT kind $K: NO USABLE WORD in $(printf %s\\n "$CTRL" | grep -c ^{) control events"; OK=0; continue; }
  read -r ID WORD MINLEN <<<"$PICK"
  echo "control id: $ID  word: $WORD  (min length $MINLEN)"
  HITS=$(timeout 20 nak req -k "$K" --search "$WORD" -l 500 "$R" 2>/dev/null | grep '^{')
  N=$(printf '%s\n' "$HITS" | grep -c '^{')
  # Second signal, independent of the picked word: a common term.
  echo "common-word search 'nostr': $(timeout 20 nak req -k "$K" --search nostr -l 50 "$R" 2>/dev/null | grep -c '^{') hits"
  if printf '%s\n' "$HITS" | grep -q "\"id\":\"$ID\""; then
    echo "RESULT kind $K: PASS (search returned $N, includes control)"
  else
    echo "RESULT kind $K: FAIL (search returned $N, control missing)"; OK=0
  fi
done
[ "$OK" = 1 ]
