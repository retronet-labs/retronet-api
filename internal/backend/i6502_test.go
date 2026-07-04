package backend_test

import (
	"strings"
	"testing"
	"time"

	"github.com/retronet-labs/retronet-api/internal/backend"
	"github.com/retronet-labs/retronet-asm/asmlib"
)

const i6502ConsoleSrc = `.arch i6502
.orgbase $8000
.equ OUT $F001
.equ STATUS $F002
.equ IN $F004

start:
loop:   LDA STATUS
        AND #$01
        BEQ loop
        LDA IN
        CMP #$2E
        BEQ halt
        STA OUT
        JMP loop
halt:   JMP halt

        .org $FFFC
        .word start
`

func TestI6502LoadAndRunHalts(t *testing.T) {
	result, err := asmlib.Assemble(i6502ConsoleSrc, "")
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if result.LoadAddress != 0x8000 {
		t.Fatalf("load address = 0x%X, want 0x8000", result.LoadAddress)
	}

	b, err := backend.New("6502")
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

func TestI6502RerunAfterHaltStartsFresh(t *testing.T) {
	result, err := asmlib.Assemble(i6502ConsoleSrc, "")
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	b, err := backend.New("6502")
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Load(result.ROM, result.LoadAddress); err != nil {
		t.Fatalf("load: %v", err)
	}

	for i := 0; i < 2; i++ {
		done := make(chan backend.RunResult, 1)
		go func() { done <- b.Run(50_000_000, 5*time.Second) }()
		time.Sleep(20 * time.Millisecond)
		if err := b.Input([]byte(".")); err != nil {
			t.Fatal(err)
		}
		run := <-done
		if !run.Halted {
			t.Fatalf("run %d = %+v, want Halted", i, run)
		}
	}
}

func TestI6502StepLimitStopsInfiniteWork(t *testing.T) {
	// Nessun input in coda, mai un '.': il loop di attesa non finisce mai da
	// solo, il limite di step deve intervenire.
	result, err := asmlib.Assemble(i6502ConsoleSrc, "")
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	b, err := backend.New("6502")
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
