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

func TestNewWiresDoctor(t *testing.T) {
	root, err := bootstrap.New(bootstrap.Config{LogWriter: nil})
	if err != nil {
		t.Fatal(err)
	}
	doctor, _, err := root.Find([]string{"doctor"})
	if err != nil {
		t.Fatalf("doctor not found: %v", err)
	}
	if doctor == nil {
		t.Fatal("doctor cmd is nil")
	}
}
