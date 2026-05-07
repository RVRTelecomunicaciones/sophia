package fakes_test

import (
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia/internal/ports/outbound"
	"github.com/RVRTelecomunicaciones/sophia/test/fakes"
)

func TestFakeClockImplementsClock(t *testing.T) {
	var _ outbound.Clock = (*fakes.FakeClock)(nil)
}

func TestFakeClockReturnsConfiguredTime(t *testing.T) {
	want := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)
	c := fakes.NewFakeClock(want)
	if got := c.Now(); !got.Equal(want) {
		t.Errorf("Now() = %v, want %v", got, want)
	}
}

func TestFakeClockAdvance(t *testing.T) {
	c := fakes.NewFakeClock(time.Unix(0, 0).UTC())
	c.Advance(5 * time.Minute)
	want := time.Unix(300, 0).UTC()
	if got := c.Now(); !got.Equal(want) {
		t.Errorf("Now() = %v, want %v", got, want)
	}
}
