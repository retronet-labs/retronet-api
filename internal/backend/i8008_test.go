package backend_test

import (
	"strings"
	"testing"
	"time"

	"github.com/retronet-labs/retronet-api/internal/backend"
	"github.com/retronet-labs/retronet-asm/asmlib"
)

func TestI8008LoadAndRunHalts(t *testing.T) {
	src := ".arch i8008\nLAI 0x48\nOUT 8\nLAI 0x49\nOUT 8\nHLT\n"
	result, err := asmlib.Assemble(src, "")
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}

	b, err := backend.New("8008")
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

func TestI8008InputFeedsRunningProgram(t *testing.T) {
	src := ".arch i8008\nloop: INP 0\nCPI 0x2E\nJTZ done\nOUT 8\nJMP loop\ndone: HLT\n"
	result, err := asmlib.Assemble(src, "")
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	b, err := backend.New("8008")
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Load(result.ROM, result.LoadAddress); err != nil {
		t.Fatalf("load: %v", err)
	}

	done := make(chan backend.RunResult, 1)
	go func() { done <- b.Run(50_000_000, 5*time.Second) }()

	time.Sleep(20 * time.Millisecond)
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
