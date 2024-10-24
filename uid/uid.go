package uid

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"sync"
)

// UidTracker is a concurrent thread-safe set which tracks the UIDs currently used in the system.
// It ensures atomicity of Add and Contain results by holding a lock on the set when performing these operations.
type UidTracker struct {
	uids map[uint64]bool
	mu   sync.Mutex
}

// AddUid returns a nil error and the added uid if the given uid was successfully added to the UidTracker.
// If the returned error is not nil, this means adding the uid failed, and the returned value should be ignored.
func (t *UidTracker) AddUid(uid uint64) (uint64, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	// The uid is already in use
	if _, ok := t.uids[uid]; ok {
		for {
			recommended := rand.Uint64()
			if _, ok = t.uids[recommended]; !ok {
				// Recommend
				return 0, fmt.Errorf("UID %d already used in the system, please retry with %d", uid, recommended)
			}
		}
	}
	// If not used, add it and return
	t.uids[uid] = true
	return uid, nil
}

// Init initializes a UidTracker with the elements in the provided array. Any duplicates in this array will only be added once.
func (t *UidTracker) Init(initialElems []uint64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.uids = make(map[uint64]bool)
	for _, elem := range initialElems {
		t.uids[elem] = true
	}
}

// GenerateAndAdd attempts to generate a non-used UID. If the context times-out or interrupts before a non-used UID is found,
// an error is returned. If the error is nil, the value can be used as a valid UID.
func (t *UidTracker) GenerateAndAdd(ctx context.Context) (uint64, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	select {
	case <-ctx.Done():
		return 0, errors.New("UID generation timed out.")
	default:
		// Continue trying to generate a unique UID
		try := rand.Uint64()
		if _, ok := t.uids[try]; !ok {
			t.uids[try] = true
			return try, nil
		}
	}
	return 0, nil
}

// Contains returns true if the uids map in the struct contains an entry for the elem uid.
func (t *UidTracker) Contains(elem uint64) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	_, ok := t.uids[elem]
	return ok
}
