package arbiterclaim

import (
	"context"
	"errors"
	"log"

	"github.com/fiatjaf/khatru"
	"github.com/nbd-wtf/go-nostr"
)

// RegisterHandlers installs the claim-enforcement RejectEvent handler.
//
// THE ORDER OF THE BRANCHES IN THIS HANDLER IS LOAD-BEARING.
//
// Kind 30078 is shared. WoT settings, HAVEN settings, stash file and folder
// metadata, cloistr docs/sheets/slides and third-party clients all publish it to
// this relay -- 94 distinct d-tags observed live. Every one of those writes
// passes through this function. The FIRST branch must reject the possibility of
// this being a claim and return, with no database call, or a claim feature adds a
// Postgres round trip to every settings write on the relay and a bug here fails
// stash rather than failing claims.
func RegisterHandlers(relay *khatru.Relay, store *Store) {
	relay.RejectEvent = append(relay.RejectEvent, func(ctx context.Context, event *nostr.Event) (bool, string) {
		// FIRST BRANCH, NO I/O: anything that is not in the arbiter claim
		// namespace leaves immediately. Do not add work above this line.
		if !IsClaim(event) {
			return false, ""
		}

		// From here on the event is definitely a claim, so cost is acceptable.
		//
		// Note the discriminator is the d-tag prefix ALONE. A claim that omits the
		// t=arbiter-claim marker is malformed and rejected here -- it is not
		// treated as a non-claim, because that would let any claimant bypass
		// enforcement simply by leaving the tag off.
		claim, err := Validate(event)
		if err != nil {
			return true, "invalid: " + err.Error()
		}

		if err := store.TryClaim(claim); err != nil {
			var lost ErrLost
			if errors.As(err, &lost) {
				// The rejection carries the WINNING event id. A loser must be able
				// to tell "I lost the race" from "the relay was unreachable", and
				// must learn the fencing token it has to verify against. A bare
				// failure is indistinguishable from a network error, which is the
				// failure mode this whole mechanism exists to remove.
				return true, "conflict: " + lost.Error()
			}
			// A database failure is NOT a lost race. Say so distinctly rather than
			// letting an outage masquerade as a contested claim.
			log.Printf("arbiterclaim: store error for task %s: %v", claim.TaskID, err)
			return true, "error: claim store unavailable, retry"
		}

		return false, ""
	})

	log.Println("Arbiter claim enforcement registered (kind 30078, d=" + DTagPrefix + "*)")
}
