package backend

import (
	"sync/atomic"
	"time"

	"github.com/retronet-labs/retronet-8080/cpu"
	"github.com/retronet-labs/retronet-8080/machine"
	rt "github.com/retronet-labs/retronet-terminal"
)

// i8080Backend esegue una ROM su una CPU Intel 8080 "nuda": nessun BDOS,
// nessun disco (a differenza delle sessioni CP/M, che usano retronet-cpm).
// Porte convenzionali: input 0 / output 1 (machine.DefaultTerminalConfig),
// diverse da quelle dell'8008 — non vanno assunte uguali.
type i8080Backend struct {
	term *rt.Terminal

	rom  []byte
	base uint16

	panel *machine.FrontPanel

	stopRequested atomic.Bool
}

func newI8080() Backend {
	return &i8080Backend{term: rt.New(rt.Config{ANSI: true})}
}

func (b *i8080Backend) Load(rom []byte, loadAddress int) error {
	b.rom = append([]byte(nil), rom...)
	b.base = uint16(loadAddress)
	b.term.Reset()
	return b.setup()
}

// setup ricostruisce memoria, I/O, CPU e front panel da zero e ricarica la
// ROM: chiamato da Load e all'inizio di ogni Run, così una nuova esecuzione
// riparte sempre da uno stato pulito.
func (b *i8080Backend) setup() error {
	profile, _ := machine.Lookup("generic")
	mem, err := profile.NewMemory()
	if err != nil {
		return err
	}
	if err := machine.LoadBytes(mem, b.base, b.rom); err != nil {
		return err
	}

	ports := profile.NewIO()
	if err := ports.OnInput(machine.TerminalInputPort, b.readInput); err != nil {
		return err
	}
	if err := ports.OnOutput(machine.TerminalOutputPort, b.writeOutput); err != nil {
		return err
	}

	c := cpu.NewCPU8080()
	c.PC = b.base // l'8080 parte eseguibile subito: niente jam instruction.
	panel, err := machine.NewFrontPanel(c, mem, ports)
	if err != nil {
		return err
	}
	b.panel = panel
	return nil
}

func (b *i8080Backend) readInput(_ byte, latched byte) byte {
	if v, err := b.term.ReadByte(); err == nil {
		return v
	}
	// IN sull'8080 non blocca mai: un programma che attende tastiera fa
	// polling stretto. Una piccola pausa evita che quel polling consumi lo
	// step limit a velocita' nativa mentre aspetta l'utente.
	time.Sleep(2 * time.Millisecond)
	return latched
}

func (b *i8080Backend) writeOutput(_ byte, value byte) {
	_ = b.term.WriteByte(value)
}

func (b *i8080Backend) Run(stepLimit uint64, timeout time.Duration) RunResult {
	if b.rom == nil {
		return RunResult{Err: ErrNoProgramLoaded}
	}
	if err := b.setup(); err != nil {
		return RunResult{Err: err}
	}
	b.stopRequested.Store(false)

	timer := time.AfterFunc(timeout, b.panel.Stop)
	defer timer.Stop()

	result, err := b.panel.Run(stepLimit, nil)
	if err != nil {
		return RunResult{Err: err}
	}
	switch result.Reason {
	case machine.PanelStoppedByCPU:
		return RunResult{Halted: true}
	case machine.PanelStoppedByLimit:
		return RunResult{StepLimit: true}
	case machine.PanelStoppedByRequest:
		if b.stopRequested.Load() {
			return RunResult{}
		}
		return RunResult{TimedOut: true}
	default:
		return RunResult{}
	}
}

func (b *i8080Backend) Stop() {
	b.stopRequested.Store(true)
	if b.panel != nil {
		b.panel.Stop()
	}
}

func (b *i8080Backend) Input(data []byte) error {
	b.term.QueueInput(data)
	return nil
}

func (b *i8080Backend) Terminal() *rt.Terminal {
	return b.term
}
