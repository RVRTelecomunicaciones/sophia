package httpclient_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/RVRTelecomunicaciones/sophia/internal/infrastructure/httpclient"
)

func TestNewReturnsClient(t *testing.T) {
	c := httpclient.New(httpclient.Config{Timeout: 5 * time.Second})
	if c == nil {
		t.Fatal("nil client")
	}
	if _, ok := any(c).(*http.Client); !ok {
		t.Errorf("expected *http.Client, got %T", c)
	}
}

func TestNewAppliesTimeout(t *testing.T) {
	c := httpclient.New(httpclient.Config{Timeout: 7 * time.Second})
	if c.Timeout != 7*time.Second {
		t.Errorf("Timeout = %v", c.Timeout)
	}
}

func TestNewAppliesDefaultTimeoutWhenZero(t *testing.T) {
	c := httpclient.New(httpclient.Config{})
	if c.Timeout == 0 {
		t.Error("expected non-zero default timeout")
	}
}
