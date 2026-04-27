package content

import "testing"

func TestSignAndVerify(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil { t.Fatalf("GenerateKeyPair: %v", err) }

	p := &Piece{
		Slug:  "test-poem",
		Title: "Test Poem",
		Body:  "Hello world.",
	}

	sig, err := SignPiece(p, kp)
	if err != nil { t.Fatalf("SignPiece: %v", err) }
	if sig == "" { t.Fatal("signature should not be empty") }

	p.Signature = sig
	ok, status := VerifyPiece(p, kp.PublicKeyHex())
	if !ok { t.Errorf("valid signature should verify: %s", status) }
}

func TestVerifyFailsOnModifiedBody(t *testing.T) {
	kp, _ := GenerateKeyPair()
	p := &Piece{Slug: "poem", Title: "Poem", Body: "Original body."}
	sig, _ := SignPiece(p, kp)
	p.Signature = sig

	// Tamper with body
	p.Body = "Modified body."
	ok, status := VerifyPiece(p, kp.PublicKeyHex())
	if ok { t.Error("modified body should fail verification") }
	if status == "" { t.Error("should return status message") }
}

func TestVerifyFailsOnModifiedTitle(t *testing.T) {
	kp, _ := GenerateKeyPair()
	p := &Piece{Slug: "poem", Title: "Original Title", Body: "body"}
	sig, _ := SignPiece(p, kp)
	p.Signature = sig
	p.Title = "Hacked Title"

	ok, _ := VerifyPiece(p, kp.PublicKeyHex())
	if ok { t.Error("modified title should fail verification") }
}

func TestVerifyUnsigned(t *testing.T) {
	kp, _ := GenerateKeyPair()
	p := &Piece{Slug: "poem", Title: "Poem", Body: "body", Signature: ""}

	ok, status := VerifyPiece(p, kp.PublicKeyHex())
	if ok { t.Error("unsigned piece should not verify") }
	if status == "" { t.Error("should return status") }
}

func TestVerifyWrongKey(t *testing.T) {
	kp1, _ := GenerateKeyPair()
	kp2, _ := GenerateKeyPair()

	p := &Piece{Slug: "poem", Title: "Poem", Body: "body"}
	sig, _ := SignPiece(p, kp1)
	p.Signature = sig

	ok, _ := VerifyPiece(p, kp2.PublicKeyHex())
	if ok { t.Error("wrong key should fail verification") }
}

func TestKeyPairRoundTrip(t *testing.T) {
	kp, _ := GenerateKeyPair()
	b64 := kp.PrivateKeyBase64()

	loaded, err := KeyPairFromBase64(b64)
	if err != nil { t.Fatalf("KeyPairFromBase64: %v", err) }

	if kp.PublicKeyHex() != loaded.PublicKeyHex() {
		t.Error("public keys should match after roundtrip")
	}

	// Sign with original, verify with loaded
	p := &Piece{Slug: "s", Title: "T", Body: "B"}
	sig, _ := SignPiece(p, kp)
	p.Signature = sig
	ok, _ := VerifyPiece(p, loaded.PublicKeyHex())
	if !ok { t.Error("should verify with loaded key") }
}

func TestSignAndVerifyListing(t *testing.T) {
	kp, _ := GenerateKeyPair()
	l := &Listing{
		Slug:  "listing-1",
		Type:  ListingSell,
		Title: "Test Listing",
		Body:  "Selling something.",
		Price: "100 PLN",
	}

	sig, err := SignListing(l, kp)
	if err != nil {
		t.Fatalf("SignListing: %v", err)
	}
	l.Signature = sig

	ok, status := VerifyListing(l, kp.PublicKeyHex())
	if !ok {
		t.Errorf("valid listing signature should verify: %s", status)
	}
}

func TestVerifyListingFailsOnModifiedBody(t *testing.T) {
	kp, _ := GenerateKeyPair()
	l := &Listing{Slug: "l", Type: ListingSell, Title: "T", Body: "Original", Price: "10"}
	sig, _ := SignListing(l, kp)
	l.Signature = sig

	l.Body = "Modified"
	ok, _ := VerifyListing(l, kp.PublicKeyHex())
	if ok {
		t.Error("modified listing body should fail verification")
	}
}

func TestVerifyListingUnsigned(t *testing.T) {
	kp, _ := GenerateKeyPair()
	l := &Listing{Slug: "l", Type: ListingSell, Title: "T", Body: "B"}
	ok, status := VerifyListing(l, kp.PublicKeyHex())
	if ok {
		t.Error("unsigned listing should not verify")
	}
	if status != "unsigned" {
		t.Errorf("expected 'unsigned', got '%s'", status)
	}
}

func TestVerifyListingWrongKey(t *testing.T) {
	kp1, _ := GenerateKeyPair()
	kp2, _ := GenerateKeyPair()
	l := &Listing{Slug: "l", Type: ListingSell, Title: "T", Body: "B", Price: "5"}
	sig, _ := SignListing(l, kp1)
	l.Signature = sig

	ok, _ := VerifyListing(l, kp2.PublicKeyHex())
	if ok {
		t.Error("wrong key should fail verification")
	}
}

func TestInvalidPrivateKey(t *testing.T) {
	_, err := KeyPairFromBase64("notvalidbase64!!!")
	if err == nil { t.Error("invalid base64 should return error") }

	_, err = KeyPairFromBase64("dGVzdA==") // valid base64 but wrong length
	if err == nil { t.Error("wrong length key should return error") }
}
