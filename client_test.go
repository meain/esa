package main

import (
	"testing"
	"time"
)

func TestCalculateRetryDelay_Jitter(t *testing.T) {
	// Run multiple times to verify jitter adds randomness
	for attempt := 0; attempt < 3; attempt++ {
		baseDelay := baseRetryDelay * (1 << uint(attempt))
		if baseDelay > maxRetryDelay {
			baseDelay = maxRetryDelay
		}

		delays := make(map[time.Duration]bool)
		for i := 0; i < 100; i++ {
			d := calculateRetryDelay(attempt)
			delays[d] = true

			// Verify delay is within expected range: [baseDelay, baseDelay + baseDelay/2)
			if d < baseDelay {
				t.Errorf("attempt %d: delay %v is less than base delay %v", attempt, d, baseDelay)
			}
			maxExpected := baseDelay + baseDelay/2
			if d >= maxExpected {
				t.Errorf("attempt %d: delay %v exceeds max expected %v", attempt, d, maxExpected)
			}
		}

		// With 100 samples, we should see more than 1 unique value (jitter is random)
		if len(delays) < 2 {
			t.Errorf("attempt %d: expected multiple unique delay values due to jitter, got %d", attempt, len(delays))
		}
	}
}

func TestCalculateRetryDelay_Cap(t *testing.T) {
	// Very high attempt number should cap at maxRetryDelay + jitter
	d := calculateRetryDelay(100)
	maxExpected := maxRetryDelay + maxRetryDelay/2
	if d >= maxExpected {
		t.Errorf("delay %v exceeds max expected %v for high attempt", d, maxExpected)
	}
}
