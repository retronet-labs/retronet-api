package backend

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/retronet-labs/retronet-4004/cpu"
	rt "github.com/retronet-labs/retronet-terminal"
)

// romSize è lo spazio indirizzabile a 12 bit del 4004 (0x000-0xFFF).
const romSize = 4096

// i4004Backend esegue una ROM su una CPU Intel 4004 "nuda", con lo stesso
// ponte tastiera/display "stupido" della CLI (-io): una cifra in, un nibble
// out. A differenza di 8008/8080, il 4004 non ha HLT né porte I/O: usa i
// callback KeyboardFunc/DisplayFunc e la convenzione storica "JUN a se
// stesso" per fermarsi (vedi isHalt).
type i4004Backend struct {
	term *rt.Terminal

	rom  []byte
	base int

	inputCh chan uint8

	stopOnce sync.Once
	stopCh   chan struct{}
	timedOut atomic.Bool
}

func newI4004() Backend {
	return &i4004Backend{
		term:    rt.New(rt.Config{ANSI: true}),
		inputCh: make(chan uint8, 1024),
		// stopCh e' gia' valido da subito: Stop() puo' arrivare (es. una
		// DELETE sulla sessione) prima che Run() sia mai stato chiamato, e
		// close() su un canale nil va in panic.
		stopCh: make(chan struct{}),
	}
}

func (b *i4004Backend) Load(rom []byte, loadAddress int) error {
	if loadAddress < 0 || loadAddress+len(rom) > romSize {
		return ErrROMOutOfRange
	}
	b.rom = append([]byte(nil), rom...)
	b.base = loadAddress
	b.term.Reset()
	return nil
}

// readKey mappa un carattere ASCII della tastierina della calcolatrice nel
// nibble atteso da RDR, stessa convenzione di cmd/retronet-4004 (-io).
func readKey(ch byte) (uint8, bool) {
	switch {
	case ch >= '0' && ch <= '9':
		return ch - '0', true
	case ch == '+':
		return 10, true
	case ch == '-':
		return 11, true
	case ch == '*':
		return 12, true
	case ch == '/':
		return 13, true
	case ch == '=':
		return 14, true
	case ch == '.':
		return 15, true
	}
	return 0, false
}

// displayChar è l'inverso di readKey per il nibble scritto da WMP.
func displayChar(n uint8) (byte, bool) {
	switch n {
	case 11:
		return '-', true
	case 15:
		return '.', true
	}
	if n <= 9 {
		return '0' + n, true
	}
	return 0, false
}

func (b *i4004Backend) Input(data []byte) error {
	for _, ch := range data {
		nibble, ok := readKey(ch)
		if !ok {
			continue
		}
		select {
		case b.inputCh <- nibble:
		default:
			// coda piena: input interattivo, va bene scartare in eccesso
			// piuttosto che bloccare il chiamante HTTP.
		}
	}
	return nil
}

func (b *i4004Backend) Run(stepLimit uint64, timeout time.Duration) RunResult {
	if b.rom == nil {
		return RunResult{Err: ErrNoProgramLoaded}
	}

	buf := make([]byte, romSize)
	copy(buf[b.base:], b.rom)
	rom := cpu.NewROM(buf)
	ram := cpu.NewRAM()
	c := cpu.NewCPU4004()
	c.PC = uint16(b.base)

	b.stopOnce = sync.Once{}
	b.stopCh = make(chan struct{})
	b.timedOut.Store(false)

	timer := time.AfterFunc(timeout, func() {
		b.timedOut.Store(true)
		b.requestStop()
	})
	defer timer.Stop()

	c.KeyboardFunc = func() uint8 {
		select {
		case v := <-b.inputCh:
			return v
		case <-b.stopCh:
			return 0
		}
	}
	c.DisplayFunc = func(n uint8) {
		if ch, ok := displayChar(n); ok {
			_ = b.term.WriteByte(ch)
		}
	}

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
		if isHalt4004(rom, c.PC) {
			return RunResult{Halted: true}
		}
		if err := c.Step(rom, ram); err != nil {
			return RunResult{Err: err}
		}
		steps++
	}
	return RunResult{StepLimit: true}
}

// isHalt4004 riconosce l'idioma storico del 4004 (nessun HLT): un JUN che
// punta a se stesso, stessa convenzione di cmd/retronet-4004 (isHalt).
func isHalt4004(rom *cpu.ROM, pc uint16) bool {
	if int(pc)+1 >= len(rom.Data) {
		return false
	}
	op := rom.Data[pc]
	if op&0xF0 != cpu.OP_JUN {
		return false
	}
	target := (uint16(op&0x0F) << 8) | uint16(rom.Data[pc+1])
	return target == pc
}

func (b *i4004Backend) requestStop() {
	b.stopOnce.Do(func() { close(b.stopCh) })
}

func (b *i4004Backend) Stop() {
	b.requestStop()
}

func (b *i4004Backend) Terminal() *rt.Terminal {
	return b.term
}
