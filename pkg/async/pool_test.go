package async

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestTryRunSafeReturnsSubmitFailure(t *testing.T) {
	ReplaceGlobal(nil)
	t.Cleanup(func() { ReplaceGlobal(nil) })

	err := TryRunSafe(context.Background(), func(context.Context) {}, time.Second)

	if !errors.Is(err, ErrNotInitialized) {
		t.Fatalf("TryRunSafe() error = %v, want %v", err, ErrNotInitialized)
	}
}
