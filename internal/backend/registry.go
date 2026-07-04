package backend

import "fmt"

// registry mappa il nome CPU scelto dall'utente (via API) al costruttore del
// backend corrispondente. "6502" viene aggiunto quando l'adapter i6502 è
// pronto (vedi i6502.go).
var registry = map[string]func() Backend{
	"4004": newI4004,
	"8008": newI8008,
	"8080": newI8080,
}

// SupportedArches elenca i nomi CPU registrati (per validazione/UI).
func SupportedArches() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	return names
}

// New costruisce un Backend per la CPU indicata.
func New(archName string) (Backend, error) {
	mk, ok := registry[archName]
	if !ok {
		return nil, fmt.Errorf("architettura %q non supportata per sessioni bare (disponibili: %v)", archName, SupportedArches())
	}
	return mk(), nil
}
