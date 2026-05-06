package cli

// White-box tests for private helpers.
// Coexists with the package cli_test black-box tests in this directory.

import (
	"os"
	"testing"
)

func TestChooseJSONSinkReturnsNonNilDefault(t *testing.T) {
	got := chooseJSONSink(Deps{}) // JSONSinkOverride is nil → returns jsonsink.New
	if got == nil {
		t.Fatal("chooseJSONSink with nil override returned nil")
	}
}

func TestChooseTUIOutputReturnsStdout(t *testing.T) {
	got := chooseTUIOutput(Deps{}) // TUIOutput is nil → returns os.Stdout
	if got != os.Stdout {
		t.Errorf("chooseTUIOutput with nil override = %v, want os.Stdout", got)
	}
}
