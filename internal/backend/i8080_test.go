package backend_test

import (
	"strings"
	"testing"
	"time"

	"github.com/retronet-labs/retronet-api/internal/backend"
	"github.com/retronet-labs/retronet-asm/asmlib"
)

func TestI8080LoadAndRunHalts(t *testing.T) {
	src := ".arch i8080\nMVI A, 0x48\nOUT 1\nMVI A, 0x49\nOUT 1\nHLT\n"
	result, err := asmlib.Assemble(src, "")
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}

	b, err := backend.New("8080")
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
	if !strings.Contains(out, "HI") {
		t.Fatalf("output=%q, want contenere \"HI\"", out)
	}
}

func TestI8080RerunAfterHaltStartsFresh(t *testing.T) {
	// INR conta quante volte gira il loop prima di uscire leggendo un carattere
	// di stop dall'input; un secondo Run senza nuovo Load deve ripartire da
	// registri azzerati, non continuare da dove si era fermato.
	src := ".arch i8080\nMVI A, 0x41\nOUT 1\nHLT\n"
	result, err := asmlib.Assemble(src, "")
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	b, err := backend.New("8080")
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Load(result.ROM, result.LoadAddress); err != nil {
		t.Fatalf("load: %v", err)
	}
	for i := 0; i < 2; i++ {
		run := b.Run(1000, time.Second)
		if !run.Halted {
			t.Fatalf("run %d = %+v, want Halted", i, run)
		}
		out := string(b.Terminal().DrainOutput())
		if out != "A" {
			t.Fatalf("run %d output=%q, want \"A\"", i, out)
		}
	}
}

func TestI8080InputFeedsRunningProgram(t *testing.T) {
	src := ".arch i8080\nloop: IN 0\nCPI 0x2E\nJZ done\nOUT 1\nJMP loop\ndone: HLT\n"
	result, err := asmlib.Assemble(src, "")
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	b, err := backend.New("8080")
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Load(result.ROM, result.LoadAddress); err != nil {
		t.Fatalf("load: %v", err)
	}

	done := make(chan backend.RunResult, 1)
	go func() { done <- b.Run(50_000_000, 5*time.Second) }()

	time.Sleep(20 * time.Millisecond) // lascia che il loop entri in polling
	if err := b.Input([]byte("XY.")); err != nil {
		t.Fatal(err)
	}

	run := <-done
	if !run.Halted {
		t.Fatalf("run = %+v, want Halted", run)
	}
	out := string(b.Terminal().DrainOutput())
	if !strings.Contains(out, "XY") {
		t.Fatalf("output=%q, want contenere \"XY\"", out)
	}
}
