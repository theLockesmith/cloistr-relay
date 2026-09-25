package storage

import (
	"context"
	"errors"
	"testing"
)

// These assert ADMISSION as well as refusal. A guard that refused every write would
// satisfy a refusal-only suite while taking the relay offline for everyone.

func TestRejectRepublishedDeletion_AdmitsAnEventNeverDeleted(t *testing.T) {
	reject, msg := RejectRepublishedDeletion(func(context.Context, string) (bool, error) {
		return false, nil
	})(context.Background(), "abc")
	if reject {
		t.Errorf("refused an event that was never deleted: %s", msg)
	}
}

func TestRejectRepublishedDeletion_RefusesARepublish(t *testing.T) {
	reject, msg := RejectRepublishedDeletion(func(context.Context, string) (bool, error) {
		return true, nil
	})(context.Background(), "abc")
	if !reject {
		t.Error("admitted a republish of a deleted event")
	}
	if msg == "" {
		t.Error("refusal carried no reason")
	}
}

// THE DISCRIMINATOR: an unhealthy database must not read as "never deleted".
func TestRejectRepublishedDeletion_LookupErrorRefuses(t *testing.T) {
	reject, msg := RejectRepublishedDeletion(func(context.Context, string) (bool, error) {
		return false, errors.New("connection refused")
	})(context.Background(), "abc")
	if !reject {
		t.Error("a failed lookup admitted the write; a broken database would resurrect deleted events")
	}
	if msg == "" {
		t.Error("refusal carried no reason")
	}
}
