package backend_test

import (
	"strings"
	"testing"
	"time"

	"github.com/retronet-labs/retronet-api/internal/backend"
	"github.com/retronet-labs/retronet-asm/asmlib"
)

func TestI4004LoadAndRunHalts(t *testing.T) {
	src := ".arch i4004\nLDM 0\nDCL\nLDM 5\nWMP\nhalt: JUN halt\n"
	result, err := asmlib.Assemble(src, "")
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}

	b, err := backend.New("4004")
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Load(result.ROM, result.LoadAddress); err != nil {
		t.Fatalf("load: %v", err)
	}

	run := b.Run(10_000, time.Second)
	if !run.Halted {
		t.Fatalf("run = %+v, want Halted", run)
	}
	out := string(b.Terminal().DrainOutput())
	if !strings.Contains(out, "5") {
		t.Fatalf("output=%q, want contenere \"5\"", out)
	}
}

func TestI4004StepLimitStopsInfiniteWork(t *testing.T) {
	// Nessun JUN a se stesso, nessun HLT: la CPU 4004 non ha modo di fermarsi
	// da sola, quindi il limite di step deve intervenire.
	src := ".arch i4004\nloop: LDM 1\nJUN loop\n"
	result, err := asmlib.Assemble(src, "")
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}

	b, err := backend.New("4004")
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Load(result.ROM, result.LoadAddress); err != nil {
		t.Fatalf("load: %v", err)
	}

	run := b.Run(1000, time.Second)
	if !run.StepLimit {
		t.Fatalf("run = %+v, want StepLimit", run)
	}
}
