package content

import (
	"strings"
	"testing"
	"time"
)

func TestSessionCodeNotEmpty(t *testing.T) {
	sc := NewSessionCode(time.Hour)
	code, exp := sc.Current()
	if code == "" {
		t.Error("session code should not be empty")
	}
	if exp.Before(time.Now()) {
		t.Error("expiry should be in the future")
	}
}

func TestSessionCodeVerify(t *testing.T) {
	sc := NewSessionCode(time.Hour)
	code, _ := sc.Current()

	if !sc.Verify(code) {
		t.Error("correct code should verify")
	}
	if sc.Verify("zly kod") {
		t.Error("wrong code should not verify")
	}
	if sc.Verify("") {
		t.Error("empty code should not verify")
	}
}

func TestSessionCodeVerifyCaseInsensitive(t *testing.T) {
	sc := NewSessionCode(time.Hour)
	code, _ := sc.Current()

	if !sc.Verify(strings.ToUpper(code)) {
		t.Error("verify should be case-insensitive")
	}
}

func TestSessionCodeVerifyTrimsWhitespace(t *testing.T) {
	sc := NewSessionCode(time.Hour)
	code, _ := sc.Current()

	if !sc.Verify("  " + code + "  ") {
		t.Error("verify should trim whitespace")
	}
}

func TestSessionCodeRotate(t *testing.T) {
	sc := NewSessionCode(time.Hour)
	code1, _ := sc.Current()
	// Rotate kilka razy — w końcu powinno się zmienić
	// (przy losowaniu z ~90 fragmentów szansa powtórzenia jest mała)
	changed := false
	for i := 0; i < 20; i++ {
		newCode := sc.Rotate()
		if newCode != code1 {
			changed = true
			break
		}
	}
	if !changed {
		t.Error("rotate should eventually produce a different code")
	}
}

func TestSessionCodeRotateUpdatesVerify(t *testing.T) {
	sc := NewSessionCode(time.Hour)
	oldCode, _ := sc.Current()
	sc.Rotate()
	newCode, _ := sc.Current()

	if oldCode == newCode {
		// Może się trafić ten sam fragment — sprawdź przynajmniej że Verify działa
		if !sc.Verify(newCode) {
			t.Error("new code should verify after rotate")
		}
		return
	}
	if sc.Verify(oldCode) {
		t.Error("old code should not verify after rotate (when code changed)")
	}
	if !sc.Verify(newCode) {
		t.Error("new code should verify after rotate")
	}
}

func TestSessionCodeExpiry(t *testing.T) {
	// Bardzo krótki TTL
	sc := NewSessionCode(time.Millisecond)
	code, _ := sc.Current()
	time.Sleep(5 * time.Millisecond)
	if sc.Verify(code) {
		t.Error("expired code should not verify")
	}
}

func TestPoetryFragmentsNoDuplicates(t *testing.T) {
	seen := map[string]bool{}
	for _, f := range poetryFragments {
		if seen[f] {
			t.Errorf("duplicate poetry fragment: %q", f)
		}
		seen[f] = true
	}
}

func TestPoetryFragmentsNotEmpty(t *testing.T) {
	for i, f := range poetryFragments {
		if strings.TrimSpace(f) == "" {
			t.Errorf("empty fragment at index %d", i)
		}
	}
}

func TestSessionCodeConcurrent(t *testing.T) {
	sc := NewSessionCode(time.Hour)
	done := make(chan struct{})
	// Wiele goroutine czyta i rotuje równocześnie — nie może być data race
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				code, _ := sc.Current()
				sc.Verify(code)
				if j%10 == 0 {
					sc.Rotate()
				}
			}
			done <- struct{}{}
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}
