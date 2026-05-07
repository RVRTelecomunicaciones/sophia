package stdclock_test

import (
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia/internal/adapters/outbound/stdclock"
	"github.com/RVRTelecomunicaciones/sophia/internal/ports/outbound"
)

func TestStdClockImplementsClock(t *testing.T) {
	var _ outbound.Clock = stdclock.New()
}

func TestStdClockNowIsRecent(t *testing.T) {
	c := stdclock.New()
	now := c.Now()
	if time.Since(now) > time.Second {
		t.Errorf("Now() not recent: %v", now)
	}
}
