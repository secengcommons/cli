package cli

import (
	"runtime/debug"
	"testing"
)

func TestProductionModuleGraphRemainsEmpty(t *testing.T) {
	t.Parallel()
	information, available := debug.ReadBuildInfo()
	if !available {
		t.Fatal("build information is unavailable")
	}
	if information.Main.Path != "github.com/secengcommons/cli" || len(information.Deps) != 0 {
		t.Fatalf("module graph = (%q, %d dependencies)", information.Main.Path, len(information.Deps))
	}
}
