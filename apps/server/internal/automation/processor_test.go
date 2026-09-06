package automation

import (
	"context"
	"errors"
	"testing"
	"time"
)

type failingGenerator struct{ err error }

func (generator failingGenerator) Generate(context.Context, Request) (Result, error) {
	return Result{}, generator.err
}

type memoryRecorder struct {
	failed    bool
	succeeded bool
	cause     error
}

func (recorder *memoryRecorder) Failed(_ context.Context, _ string, cause error, _ time.Time) error {
	recorder.failed, recorder.cause = true, cause
	return nil
}

func (recorder *memoryRecorder) Succeeded(_ context.Context, _ string, _ Result, _ time.Time) error {
	recorder.succeeded = true
	return nil
}

func TestModelTimeoutFailsAutomationWithoutHidingAlertFact(t *testing.T) {
	eventVisible := true
	recorder := &memoryRecorder{}
	err := Process(context.Background(), Request{RunID: "run-1", PerceptionEventID: "event-1", ProjectID: 17},
		failingGenerator{err: context.DeadlineExceeded}, recorder, time.Now())
	if err != nil || !recorder.failed || recorder.succeeded || !errors.Is(recorder.cause, context.DeadlineExceeded) {
		t.Fatalf("model timeout was not isolated: err=%v recorder=%+v", err, recorder)
	}
	if !eventVisible {
		t.Fatal("automation failure hid the committed alert fact")
	}
}

func TestUnavailableModelFailsOnlyAutomationRun(t *testing.T) {
	recorder := &memoryRecorder{}
	if err := Process(context.Background(), Request{RunID: "run-2", PerceptionEventID: "event-2"}, UnavailableGenerator{}, recorder, time.Now()); err != nil {
		t.Fatal(err)
	}
	if !recorder.failed || !errors.Is(recorder.cause, ErrModelUnavailable) {
		t.Fatalf("unavailable model was not recorded: %+v", recorder)
	}
}
