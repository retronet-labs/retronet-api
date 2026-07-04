package backend

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/retronet-labs/retronet-6502/cpu"
	rt "github.com/retronet-labs/retronet-terminal"
)

// Convenzione console I/O per il 6502 (nessuna porta I/O reale sul chip):
// documentata in retronet-asm/docs/io-console-i6502.md.
const (
	consolePortOut    uint16 = 0xF001
	consolePortStatus uint16 = 0xF002
	consolePortIn     uint16 = 0xF004
)

// consoleBus avvolge una RAM 6502 piatta intercettando i tre indirizzi della
// convenzione console; il resto dello spazio si comporta come RAM normale.
type consoleBus struct {
	ram  *cpu.RAM
	term *rt.Terminal
}

func (b *consoleBus) Read(addr uint16) byte {
	switch addr {
	case consolePortStatus:
		if b.term.PendingInput() > 0 {
			return 0x01
		}
		// Un programma corretto fa polling su STATUS in attesa di tastiera:
		// una piccola pausa evita che consumi lo step limit a velocita'
		// nativa durante un'attesa interattiva normale (stessa ragione della
		// pausa in i8008/i8080 su INP/IN a vuoto).
		time.Sleep(2 * time.Millisecond)
		return 0x00
	case consolePortIn:
		if v, err := b.term.ReadByte(); err == nil {
			return v
		}
		return 0x00
	case consolePortOut:
		return 0
	default:
		return b.ram.Read(addr)
	}
}

func (b *consoleBus) Write(addr uint16, value byte) {
	switch addr {
	case consolePortOut:
		_ = b.term.WriteByte(value)
	case consolePortStatus, consolePortIn:
		// sola lettura: scrittura ignorata.
	default:
		b.ram.Write(addr, value)
	}
}

// i6502Backend esegue una ROM su una CPU MOS 6502 "nuda". Non esiste HLT sul
// 6502: usa lo stesso idioma "JMP a se stesso" del 4004 per fermarsi.
type i6502Backend struct {
	term *rt.Terminal

	rom  []byte
	base uint16

	stopOnce sync.Once
	stopCh   chan struct{}
	timedOut atomic.Bool
}

func newI6502() Backend {
	return &i6502Backend{term: rt.New(rt.Config{ANSI: true})}
}

func (b *i6502Backend) Load(rom []byte, loadAddress int) error {
	if loadAddress < 0 || loadAddress > 0xFFFF || loadAddress+len(rom) > 0x10000 {
		return ErrROMOutOfRange
	}
	b.rom = append([]byte(nil), rom...)
	b.base = uint16(loadAddress)
	b.term.Reset()
	return nil
}

func (b *i6502Backend) Input(data []byte) error {
	b.term.QueueInput(data)
	return nil
}

func (b *i6502Backend) Run(stepLimit uint64, timeout time.Duration) RunResult {
	if b.rom == nil {
		return RunResult{Err: ErrNoProgramLoaded}
	}

	ram := cpu.NewRAM()
	ram.LoadAt(b.base, b.rom)
	bus := &consoleBus{ram: ram, term: b.term}

	c := &cpu.CPU6502{Mem: bus}
	c.Reset() // legge il vettore $FFFC/$FFFD gia' presente nella ROM caricata

	b.stopOnce = sync.Once{}
	b.stopCh = make(chan struct{})
	b.timedOut.Store(false)

	timer := time.AfterFunc(timeout, func() {
		b.timedOut.Store(true)
		b.requestStop()
	})
	defer timer.Stop()

	var steps uint64
	for steps < stepLimit {
		select {
		case <-b.stopCh:
			if b.timedOut.Load() {
				return RunResult{TimedOut: true}
			}
			return RunResult{}
		default:
		}
		if isHalt6502(bus, c.PC) {
			return RunResult{Halted: true}
		}
		if err := c.Step(); err != nil {
			return RunResult{Err: err}
		}
		steps++
	}
	return RunResult{StepLimit: true}
}

// isHalt6502 riconosce l'idioma "JMP a se stesso" (0x4C, indirizzamento
// assoluto), stessa convenzione usata per il 4004 (nessun HLT nativo).
func isHalt6502(bus *consoleBus, pc uint16) bool {
	const jmpAbsolute = 0x4C
	if bus.Read(pc) != jmpAbsolute {
		return false
	}
	lo := uint16(bus.Read(pc + 1))
	hi := uint16(bus.Read(pc + 2))
	return lo|hi<<8 == pc
}

func (b *i6502Backend) requestStop() {
	b.stopOnce.Do(func() { close(b.stopCh) })
}

func (b *i6502Backend) Stop() {
	b.requestStop()
}

func (b *i6502Backend) Terminal() *rt.Terminal {
	return b.term
}
