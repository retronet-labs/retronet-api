// Package backend astrae le sessioni "nude" (senza CP/M/BDOS): carica una ROM
// assemblata da retronet-asm su una delle CPU dell'ecosistema e la esegue,
// esponendo lo stesso terminale condiviso (retronet-terminal) usato dalle
// sessioni CP/M, così il resto di retronet-api e retronet-ui non deve sapere
// quale CPU c'è dietro una sessione.
package backend

import (
	"errors"
	"time"

	rt "github.com/retronet-labs/retronet-terminal"
)

// ErrNoProgramLoaded è restituito da Run se non è stato ancora chiamato Load.
var ErrNoProgramLoaded = errors.New("nessun programma caricato")

// ErrROMOutOfRange è restituito da Load se la ROM non entra nello spazio
// indirizzabile della CPU a partire da loadAddress.
var ErrROMOutOfRange = errors.New("la ROM non entra nello spazio indirizzabile della CPU")

// RunResult riassume come si è concluso un Run.
type RunResult struct {
	Halted    bool  // il programma ha eseguito HLT o l'idioma di auto-salto
	StepLimit bool  // fermato per limite di step
	TimedOut  bool  // fermato per timeout wall-clock
	Err       error // errore di esecuzione (opcode illegale, ecc.), nil altrimenti
}

// Backend è il contratto minimo di una CPU "nuda" gestibile da una sessione
// retronet-api. Non include concetti da shell (comandi, prompt): un backend
// sa solo caricare un programma ed eseguirlo, con I/O da terminale.
type Backend interface {
	// Load installa rom all'indirizzo loadAddress e resetta lo stato della
	// CPU, pronta per un nuovo Run.
	Load(rom []byte, loadAddress int) error

	// Run esegue fino a halt, limite di step, timeout o Stop(). Bloccante:
	// il chiamante lo esegue nella propria goroutine.
	Run(stepLimit uint64, timeout time.Duration) RunResult

	// Stop richiede l'arresto di un Run in corso; sicuro da altre goroutine.
	Stop()

	// Input accoda byte di tastiera per il programma in esecuzione.
	Input(data []byte) error

	// Terminal espone il terminale condiviso (stesso Snapshot/DrainOutput
	// JSON già parlato dalla UI per le sessioni CP/M).
	Terminal() *rt.Terminal
}
