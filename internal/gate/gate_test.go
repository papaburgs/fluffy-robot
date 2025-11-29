package gate

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestGate_Latch_AllowsProceed(t *testing.T) {
	g := New(2, 0)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		g.Latch(ctx)
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-ctx.Done():
		t.Fatal("Latch did not proceed in time")
	}
}

func TestGate_Latch_ContextCancel(t *testing.T) {
	g := New(2, 0)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		g.Latch(ctx)
		close(done)
	}()

	select {
	case <-done:
		// Should finish due to context cancel
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Latch did not return after context cancel")
	}
}

// TestGateInitialBlast does 20, they should all finish in less than a second
func TestGateInitialBlast(t *testing.T) {
	g := New(2, 20)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	done := make(chan struct{}, 30)
	go func() {
		for i := 0; i < 20; i++ {
			g.Latch(ctx)
			done <- struct{}{}
		}
	}()

	count := 0
	for {
		select {
		case <-done:
			count++
			t.Logf("finished %d", count)
			// Should finish due to context cancel
		case <-ctx.Done():
			if count < 20 {
				t.Fatalf("Latch did not let through all 20, count: %d", count)
			}
			return
		}
	}
}

// TestGateInitialBlast tries to do 100 at once
// After 10 seconds, we should only have 40 done
// if there is more than that, the gate is leaking
func TestGateInitialBlastTooMany(t *testing.T) {
	g := New(2, 20)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan struct{}, 40)
	go func() {
		for i := 0; i < 100; i++ {
			g.Latch(ctx)
			done <- struct{}{}
		}
	}()

	count := 0
	for {
		select {
		case <-done:
			count++
			t.Logf("finished %d", count)
			// Should finish due to context cancel
		case <-ctx.Done():
			if count > 30 { // 2 in each of the 5 seconds, plus the 20 blast
				t.Fatalf("Latch let through too many of the initial 100 blast; count: %d", count)
			}
			return
		}
	}
}

func TestGateAfterMinute(t *testing.T) {

	if os.Getenv("GO_TEST_LONG") != "true" {
		t.Skip("Skipping long-running test. Set GO_TEST_LONG=true to run.")
	}
	g := New(2, 20)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute+(5*time.Second))
	defer cancel()

	done := make(chan struct{}, 40)
	go func() {
		for i := 0; i < 200; i++ {
			g.Latch(ctx)
			done <- struct{}{}
		}
	}()

	count := 0
	for {
		select {
		case <-done:
			count++
			t.Logf("finished %d", count)
			// Should finish due to context cancel
		case <-ctx.Done():
			// after 65 seconds, we should have two rounds of the 20, plus 2 * 65
			// add a couple extra to count to take care of rounding
			est := (2 * 20) + (2 * 65)
			if count > est-2 && count < est+2 {
				t.Fatalf("Latch let through unexpected number; estimated: %d; count: %d", est, count)
			}
			return
		}
	}
}
