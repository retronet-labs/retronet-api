package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func createBareSession(t *testing.T, baseURL string, arch string) string {
	t.Helper()
	body := bytes.NewBufferString(`{"kind":"bare","arch":"` + arch + `"}`)
	resp, err := http.Post(baseURL+"/sessions", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create bare(%s) status=%d", arch, resp.StatusCode)
	}
	var created struct {
		ID   string `json:"id"`
		Kind string `json:"kind"`
		Arch string `json:"arch"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Kind != "bare" || created.Arch != arch {
		t.Fatalf("created=%+v, want kind=bare arch=%s", created, arch)
	}
	return created.ID
}

func assembleSource(t *testing.T, baseURL string, sessionID string, source string) (int, map[string]any) {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"source": source})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(baseURL+"/sessions/"+sessionID+"/assemble", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, out
}

func runBare(t *testing.T, baseURL string, sessionID string) asyncResult {
	t.Helper()
	resp, err := http.Post(baseURL+"/sessions/"+sessionID+"/run", "application/json", bytes.NewBufferString(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("run status=%d", resp.StatusCode)
	}
	var accepted asyncResult
	if err := json.NewDecoder(resp.Body).Decode(&accepted); err != nil {
		t.Fatal(err)
	}
	return accepted
}

func pollBareOutput(t *testing.T, baseURL string, sessionID string, want string) string {
	t.Helper()
	var all strings.Builder
	for i := 0; i < 50; i++ {
		resp, err := http.Get(baseURL + "/sessions/" + sessionID + "/output")
		if err != nil {
			t.Fatal(err)
		}
		var result commandResult
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			t.Fatal(err)
		}
		resp.Body.Close()
		all.WriteString(result.Output)
		if strings.Contains(all.String(), want) {
			return all.String()
		}
		if result.State != SessionRunning {
			return all.String()
		}
		time.Sleep(10 * time.Millisecond)
	}
	return all.String()
}

// TestBareAssembleWithoutArchDirective riproduce il caso reale della UI:
// l'utente scrive il sorgente senza una riga ".arch" propria (la sceglie dal
// menu a tendina), quindi retronet-api deve passare ad asmlib un hint che
// asmlib riconosca. I nomi arch di retronet-api ("4004", "8080", ...) e quelli
// di asmlib ("i4004", "i8080", ...) sono diversi: un test con ".arch" sempre
// esplicito nel sorgente non lo scopre, perche' la direttiva del sorgente ha
// sempre priorita' sull'hint.
func TestBareAssembleWithoutArchDirective(t *testing.T) {
	app := New(Config{Version: "test", SessionTTL: time.Minute})
	ts := httptest.NewServer(app.Handler())
	defer ts.Close()

	cases := map[string]string{
		"4004": "LDM 5\nWMP\nhalt: JUN halt\n",
		"6502": "start:\nhalt: JMP halt\n.org $FFFC\n.word start\n",
		"8008": "LAI 0x48\nOUT 8\nHLT\n",
		"8080": "MVI A, 0x48\nOUT 1\nHLT\n",
	}
	for arch, src := range cases {
		sessionID := createBareSession(t, ts.URL, arch)
		status, result := assembleSource(t, ts.URL, sessionID, src)
		if status != http.StatusOK {
			t.Errorf("%s: assemble status=%d body=%v (deve funzionare anche senza .arch esplicito)", arch, status, result)
		}
	}
}

// TestBareAssembleRejectsArchMismatch verifica che una sessione creata per
// una CPU non accetti un sorgente con una riga ".arch" esplicita per
// un'altra CPU: caricarlo comunque produrrebbe byte eseguiti dalla CPU
// sbagliata, senza nessun errore visibile.
func TestBareAssembleRejectsArchMismatch(t *testing.T) {
	app := New(Config{Version: "test", SessionTTL: time.Minute})
	ts := httptest.NewServer(app.Handler())
	defer ts.Close()

	sessionID := createBareSession(t, ts.URL, "4004")
	src := ".arch i8080\nMVI A, 0x48\nOUT 1\nHLT\n"
	status, result := assembleSource(t, ts.URL, sessionID, src)
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400; body=%v", status, result)
	}
	if !strings.Contains(fmt.Sprint(result["error"]), "i8080") {
		t.Errorf("error=%v, want menzioni i8080", result["error"])
	}
}

// TestBareAssembleAndRunNoInput assembla ed esegue un programma 8080 che non
// legge input (stampa "HI" ed esegue HLT), verificando il ciclo completo
// assemble -> run -> halt -> sessione di nuovo idle e riusabile.
func TestBareAssembleAndRunNoInput(t *testing.T) {
	app := New(Config{Version: "test", SessionTTL: time.Minute})
	ts := httptest.NewServer(app.Handler())
	defer ts.Close()

	sessionID := createBareSession(t, ts.URL, "8080")

	src := ".arch i8080\nMVI A, 0x48\nOUT 1\nMVI A, 0x49\nOUT 1\nHLT\n"
	status, result := assembleSource(t, ts.URL, sessionID, src)
	if status != http.StatusOK {
		t.Fatalf("assemble status=%d body=%v", status, result)
	}
	if result["load_address"] != float64(0) {
		t.Fatalf("load_address=%v, want 0", result["load_address"])
	}

	accepted := runBare(t, ts.URL, sessionID)
	if !accepted.Accepted {
		t.Fatal("run non accettato")
	}

	output := accepted.Output + pollBareOutput(t, ts.URL, sessionID, "HI")
	if !strings.Contains(output, "HI") {
		t.Fatalf("output=%q", output)
	}

	resp, err := http.Get(ts.URL + "/sessions/" + sessionID)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var info SessionInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		t.Fatal(err)
	}
	if info.State != SessionIdle {
		t.Fatalf("state dopo halt = %q, want idle (riusabile, non closed)", info.State)
	}

	// La sessione resta utilizzabile: si puo' rieseguire senza un nuovo assemble.
	accepted = runBare(t, ts.URL, sessionID)
	output = accepted.Output + pollBareOutput(t, ts.URL, sessionID, "HI")
	if !strings.Contains(output, "HI") {
		t.Fatalf("secondo run, output=%q", output)
	}
}

// TestBareInputEchoAndHalt verifica input dal vivo: un loop 8080 che legge
// dalla porta 0, ristampa sulla porta 1 e si ferma leggendo '.'.
func TestBareInputEchoAndHalt(t *testing.T) {
	app := New(Config{Version: "test", SessionTTL: time.Minute})
	ts := httptest.NewServer(app.Handler())
	defer ts.Close()

	sessionID := createBareSession(t, ts.URL, "8080")
	src := ".arch i8080\nloop: IN 0\nCPI 0x2E\nJZ done\nOUT 1\nJMP loop\ndone: HLT\n"
	if status, result := assembleSource(t, ts.URL, sessionID, src); status != http.StatusOK {
		t.Fatalf("assemble status=%d body=%v", status, result)
	}

	runBare(t, ts.URL, sessionID)

	input := bytes.NewBufferString(`{"data":"AB."}`)
	resp, err := http.Post(ts.URL+"/sessions/"+sessionID+"/input", "application/json", input)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("input status=%d", resp.StatusCode)
	}

	output := pollBareOutput(t, ts.URL, sessionID, "AB")
	if !strings.Contains(output, "AB") {
		t.Fatalf("output=%q, want contenere \"AB\"", output)
	}
}

// TestBare6502InputEchoAndHalt ripete lo stesso scenario di
// TestBareInputEchoAndHalt sul backend 6502, che usa la convenzione console a
// indirizzi mappati in memoria (docs/io-console-i6502.md) invece delle porte
// I/O di 8008/8080.
func TestBare6502InputEchoAndHalt(t *testing.T) {
	app := New(Config{Version: "test", SessionTTL: time.Minute})
	ts := httptest.NewServer(app.Handler())
	defer ts.Close()

	sessionID := createBareSession(t, ts.URL, "6502")
	src := ".arch i6502\n.orgbase $8000\n.equ OUT $F001\n.equ STATUS $F002\n.equ IN $F004\n" +
		"start:\nloop: LDA STATUS\nAND #$01\nBEQ loop\nLDA IN\nCMP #$2E\nBEQ halt\nSTA OUT\nJMP loop\n" +
		"halt: JMP halt\n.org $FFFC\n.word start\n"
	if status, result := assembleSource(t, ts.URL, sessionID, src); status != http.StatusOK {
		t.Fatalf("assemble status=%d body=%v", status, result)
	}

	runBare(t, ts.URL, sessionID)

	input := bytes.NewBufferString(`{"data":"AB."}`)
	resp, err := http.Post(ts.URL+"/sessions/"+sessionID+"/input", "application/json", input)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("input status=%d", resp.StatusCode)
	}

	output := pollBareOutput(t, ts.URL, sessionID, "AB")
	if !strings.Contains(output, "AB") {
		t.Fatalf("output=%q, want contenere \"AB\"", output)
	}
}

// TestBareAssembleErrorReportsLine verifica che un errore di compilazione
// torni 422 con la riga corretta, invece di rompere la sessione.
func TestBareAssembleErrorReportsLine(t *testing.T) {
	app := New(Config{Version: "test", SessionTTL: time.Minute})
	ts := httptest.NewServer(app.Handler())
	defer ts.Close()

	sessionID := createBareSession(t, ts.URL, "8080")
	src := ".arch i8080\nMVI A, 0x48\nPIPPO\n"
	status, result := assembleSource(t, ts.URL, sessionID, src)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d, want 422; body=%v", status, result)
	}
	errs, ok := result["errors"].([]any)
	if !ok || len(errs) != 1 {
		t.Fatalf("errors=%v", result["errors"])
	}
	first, ok := errs[0].(map[string]any)
	if !ok {
		t.Fatalf("errors[0]=%v", errs[0])
	}
	if line, _ := first["line"].(float64); line != 3 {
		t.Errorf("line=%v, want 3 (err=%v)", first["line"], first)
	}
}

// TestBareCommandEndpointUnsupported verifica che l'endpoint /command (shell
// CP/M) non abbia senso per una sessione bare e risponda con 404, non un panic
// o un errore generico.
func TestBareCommandEndpointUnsupported(t *testing.T) {
	app := New(Config{Version: "test", SessionTTL: time.Minute})
	ts := httptest.NewServer(app.Handler())
	defer ts.Close()

	sessionID := createBareSession(t, ts.URL, "4004")
	body := bytes.NewBufferString(`{"command":"DIR"}`)
	resp, err := http.Post(ts.URL+"/sessions/"+sessionID+"/command", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", resp.StatusCode)
	}
}

// TestBareRunWithoutAssembleFails verifica che /run prima di /assemble dia un
// errore chiaro invece di eseguire memoria vuota.
func TestBareRunWithoutAssembleFails(t *testing.T) {
	app := New(Config{Version: "test", SessionTTL: time.Minute})
	ts := httptest.NewServer(app.Handler())
	defer ts.Close()

	sessionID := createBareSession(t, ts.URL, "8008")
	resp, err := http.Post(ts.URL+"/sessions/"+sessionID+"/run", "application/json", bytes.NewBufferString(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status=%d, want 409", resp.StatusCode)
	}
}

// TestBareUnknownArchRejected verifica che un'architettura non supportata sia
// rifiutata alla creazione della sessione.
func TestBareUnknownArchRejected(t *testing.T) {
	app := New(Config{Version: "test", SessionTTL: time.Minute})
	ts := httptest.NewServer(app.Handler())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/sessions", "application/json", bytes.NewBufferString(`{"kind":"bare","arch":"z80"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", resp.StatusCode)
	}
}
