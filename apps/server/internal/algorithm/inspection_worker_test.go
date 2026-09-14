package algorithm

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestInspectionWorkerRetriesTransactionFailureAndStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	calls := 0
	err := runInspectionLoop(ctx, time.Millisecond, func(context.Context) (bool, error) {
		calls++
		if calls == 1 {
			return false, errors.New("transient transaction failure")
		}
		cancel()
		return true, nil
	})
	if err != nil || calls != 2 {
		t.Fatalf("worker stopped before recovery: calls=%d err=%v", calls, err)
	}
}

func TestInspectionWorkerRejectsInvalidIntervalWithoutProcessing(t *testing.T) {
	if err := runInspectionLoop(context.Background(), 0, func(context.Context) (bool, error) { t.Fatal("processed invalid configuration"); return false, nil }); err == nil {
		t.Fatal("invalid interval accepted")
	}
}
