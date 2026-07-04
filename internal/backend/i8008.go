package backend

import (
	"sync/atomic"
	"time"

	"github.com/retronet-labs/retronet-8008/cpu"
	"github.com/retronet-labs/retronet-8008/machine"
	rt "github.com/retronet-labs/retronet-terminal"
)

// i8008Backend esegue una ROM su una CPU Intel 8008 "nuda": nessun BDOS, nessun
// disco. Il terminale è collegato direttamente alle porte convenzionali
// input 0 / output 8 (machine.DefaultTerminalConfig), bypassando il tipo
// machine.Terminal del repo 8008 in favore di retronet-terminal, così
// Snapshot()/DrainOutput() sono gli stessi già parlati dalla UI per CP/M.
type i8008Backend struct {
	term *rt.Terminal

	rom  []byte
	base uint16

	panel *machine.FrontPanel

	stopRequested atomic.Bool
}

func newI8008() Backend {
	return &i8008Backend{term: rt.New(rt.Config{ANSI: true})}
}

func (b *i8008Backend) Load(rom []byte, loadAddress int) error {
	b.rom = append([]byte(nil), rom...)
	b.base = uint16(loadAddress)
	b.term.Reset()
	return b.setup()
}

// setup ricostruisce memoria, I/O, CPU e front panel da zero e ricarica la
// ROM: chiamato da Load e all'inizio di ogni Run, così una nuova esecuzione
// riparte sempre da uno stato pulito (niente RAM sporcata dal run precedente).
func (b *i8008Backend) setup() error {
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

	c := cpu.NewCPU8008()
	panel, err := machine.NewFrontPanel(c, mem, ports)
	if err != nil {
		return err
	}
	b.panel = panel
	return nil
}

func (b *i8008Backend) readInput(_ byte, latched byte) byte {
	if v, err := b.term.ReadByte(); err == nil {
		return v
	}
	// L'INP dell'8008 non blocca mai: un programma che attende tastiera fa
	// polling stretto. Una piccola pausa evita che quel polling consumi lo
	// step limit a velocita' nativa mentre aspetta l'utente.
	time.Sleep(2 * time.Millisecond)
	return latched
}

func (b *i8008Backend) writeOutput(_ byte, value byte) {
	_ = b.term.WriteByte(value)
}

func (b *i8008Backend) Run(stepLimit uint64, timeout time.Duration) RunResult {
	if b.rom == nil {
		return RunResult{Err: ErrNoProgramLoaded}
	}
	if err := b.setup(); err != nil {
		return RunResult{Err: err}
	}
	b.stopRequested.Store(false)

	// L'8008 storicamente parte fermo (Reset porta Halted=Stopped=true) e
	// richiede una jam instruction esterna: gli riproduciamo un JMP verso
	// l'indirizzo di caricamento, come fa la CLI (cmd/retronet-8008).
	if err := b.panel.Jam(cpu.JMP(), byte(b.base), byte(b.base>>8)); err != nil {
		return RunResult{Err: err}
	}

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

func (b *i8008Backend) Stop() {
	b.stopRequested.Store(true)
	if b.panel != nil {
		b.panel.Stop()
	}
}

func (b *i8008Backend) Input(data []byte) error {
	b.term.QueueInput(data)
	return nil
}

func (b *i8008Backend) Terminal() *rt.Terminal {
	return b.term
}
