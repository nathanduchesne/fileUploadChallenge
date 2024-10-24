package uid

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestInit(t *testing.T) {
	tracker := UidTracker{}
	initialUids := []uint64{32, 48, 12939303003, 0, 326, 129393030031}
	tracker.Init(initialUids)

	for _, elem := range initialUids {
		if !tracker.Contains(elem) {
			t.Errorf("Initialization missed value %d", elem)
		}
	}
}

func TestUniquenessConcurrent(t *testing.T) {
	tracker := UidTracker{}
	initialUids := []uint64{32, 48, 12939303003, 0, 326, 129393030031}
	tracker.Init(initialUids)

	wg := sync.WaitGroup{}
	wg.Add(2)

	const newElem uint64 = 49

	var addedElem1, addedElem2 uint64
	var error1, error2 error

	go func() {
		defer wg.Done()
		addedElem1, error1 = tracker.AddUid(newElem)
	}()

	go func() {
		defer wg.Done()
		addedElem2, error2 = tracker.AddUid(newElem)
	}()

	wg.Wait()

	if error1 == nil && error2 == nil {
		t.Errorf("Both goroutines were told their uid was added, hence not unique")
	} else if error1 != nil && error2 != nil {
		t.Errorf("Both goroutines were told their uid was already in, but it isn't")
	}

	if error1 == nil && addedElem1 != newElem {
		t.Errorf("Goroutine was told adding uid %d succeeded, but returned that %d was added", newElem, addedElem1)
	} else if error2 == nil && addedElem2 != newElem {
		t.Errorf("Goroutine was told adding uid %d succeeded, but returned that %d was added", newElem, addedElem2)
	}
}

func TestGenerateAndAdd(t *testing.T) {
	tracker := UidTracker{}
	initialUids := []uint64{32, 48, 12939303003, 326, 129393030031}
	tracker.Init(initialUids)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Millisecond)
	defer cancel()

	added, err := tracker.GenerateAndAdd(ctx)
	if err == nil && !tracker.Contains(added) {
		t.Errorf("If no error was thrown, the new element should have been added.")
	}
}

func TestGenerateAndAddTimeouts(t *testing.T) {
	tracker := UidTracker{}
	initialUids := []uint64{32, 48, 12939303003, 326, 129393030031}
	tracker.Init(initialUids)

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()

	done := make(chan struct{})

	go func() {
		// Call the function you're testing
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Millisecond)
		defer cancel()
		added, err := tracker.GenerateAndAdd(ctx)
		if err == nil && !tracker.Contains(added) {
			t.Errorf("If no error was thrown, the new element should have been added.")
		}
		// Signal that the function completed
		close(done)
	}()

	// Wait for either the function to complete or the context to timeout
	select {
	case <-done:
		return
		// The function completed successfully before the timeout
	case <-ctx.Done():
		// The function took too long, fail the test
		t.Fatal("The function should have timed out but didn't")
	}
}
