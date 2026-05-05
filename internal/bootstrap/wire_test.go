package bootstrap_test

import (
	"testing"

	"github.com/RVRTelecomunicaciones/sophia-cli/internal/bootstrap"
)

func TestNewProducesRoot(t *testing.T) {
	root, err := bootstrap.New(bootstrap.Config{LogWriter: nil})
	if err != nil {
		t.Fatal(err)
	}
	if root.Use != "sophia" {
		t.Errorf("root use = %q", root.Use)
	}
}

func TestNewWiresM2Commands(t *testing.T) {
	root, err := bootstrap.New(bootstrap.Config{LogWriter: nil})
	if err != nil {
		t.Fatal(err)
	}
	for _, sub := range []string{"doctor", "start", "stop", "version"} {
		c, _, err := root.Find([]string{sub})
		if err != nil {
			t.Errorf("%s not found: %v", sub, err)
		}
		if c == nil {
			t.Errorf("%s cmd is nil", sub)
		}
	}
}

func TestNewWiresM3Commands(t *testing.T) {
	root, err := bootstrap.New(bootstrap.Config{LogWriter: nil})
	if err != nil {
		t.Fatal(err)
	}
	for _, sub := range []string{"init", "status"} {
		c, _, err := root.Find([]string{sub})
		if err != nil {
			t.Errorf("%s not found: %v", sub, err)
		}
		if c == nil {
			t.Errorf("%s cmd is nil", sub)
		}
	}
}
