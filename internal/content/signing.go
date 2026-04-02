package content

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// KeyPair holds an Ed25519 signing keypair
type KeyPair struct {
	PublicKey  ed25519.PublicKey
	PrivateKey ed25519.PrivateKey
}

// GenerateKeyPair creates a new Ed25519 keypair
func GenerateKeyPair() (*KeyPair, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return &KeyPair{PublicKey: pub, PrivateKey: priv}, nil
}

// PublicKeyHex returns the public key as a hex string (safe to publish)
func (kp *KeyPair) PublicKeyHex() string {
	return hex.EncodeToString(kp.PublicKey)
}

// PrivateKeyBase64 returns the private key as base64 (store as secret)
func (kp *KeyPair) PrivateKeyBase64() string {
	return base64.StdEncoding.EncodeToString(kp.PrivateKey)
}

// KeyPairFromBase64 loads a keypair from a base64-encoded private key
func KeyPairFromBase64(privBase64 string) (*KeyPair, error) {
	privBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(privBase64))
	if err != nil {
		return nil, fmt.Errorf("invalid private key: %w", err)
	}
	if len(privBytes) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("invalid private key length: got %d, want %d", len(privBytes), ed25519.PrivateKeySize)
	}
	priv := ed25519.PrivateKey(privBytes)
	pub := priv.Public().(ed25519.PublicKey)
	return &KeyPair{PublicKey: pub, PrivateKey: priv}, nil
}

// PublicKeyFromHex loads just a public key from hex (for verification only)
func PublicKeyFromHex(hexStr string) (ed25519.PublicKey, error) {
	b, err := hex.DecodeString(strings.TrimSpace(hexStr))
	if err != nil {
		return nil, fmt.Errorf("invalid public key hex: %w", err)
	}
	if len(b) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid public key length")
	}
	return ed25519.PublicKey(b), nil
}

// SignPiece signs a piece's content and returns a base64 signature.
// The signed payload is: sha256(slug + title + body)
func SignPiece(p *Piece, kp *KeyPair) (string, error) {
	payload := piecePayload(p)
	sig := ed25519.Sign(kp.PrivateKey, payload)
	return base64.StdEncoding.EncodeToString(sig), nil
}

// VerifyPiece checks a piece's signature against a public key.
// Returns true if valid, plus a human-readable status.
func VerifyPiece(p *Piece, pubKeyHex string) (bool, string) {
	if p.Signature == "" {
		return false, "unsigned — this piece has no signature"
	}
	pub, err := PublicKeyFromHex(pubKeyHex)
	if err != nil {
		return false, "invalid public key"
	}
	sigBytes, err := base64.StdEncoding.DecodeString(p.Signature)
	if err != nil {
		return false, "malformed signature"
	}
	payload := piecePayload(p)
	if !ed25519.Verify(pub, payload, sigBytes) {
		return false, "invalid signature — content may have been modified"
	}
	return true, "verified — signed by kapoost's key"
}

// piecePayload builds the canonical bytes to sign: sha256(slug|title|body)
func piecePayload(p *Piece) []byte {
	canonical := p.Slug + "|" + p.Title + "|" + p.Body
	hash := sha256.Sum256([]byte(canonical))
	return hash[:]
}

// --- OpenTimestamps Integration ---
//
// OpenTimestamps anchors a SHA256 hash into the Bitcoin blockchain,
// providing an independent, trustless proof that content existed at a point in time.
//
// Workflow:
//   1. On save: TimestampPiece() → POSTs digest to OTS calendar → stores stub in OTSProof
//   2. On demand: UpgradeTimestamp() → fetches upgraded proof (after ~1hr Bitcoin confirm)
//   3. Verify: anyone can run `ots verify` with the proof bytes + public OTS client
//
// The stub returned immediately is an incomplete proof — it proves submission to the
// calendar. After Bitcoin confirmation (~1hr), UpgradeTimestamp returns the full proof
// anchored to a Bitcoin block hash.

const otsCalendar = "https://alice.btc.calendar.opentimestamps.org"

// TimestampPiece submits the piece's content hash to the OpenTimestamps calendar.
// Returns base64-encoded OTS stub bytes, or empty string on failure (non-fatal).
// The stub is an incomplete proof — upgrade after ~1hr for full Bitcoin anchor.
func TimestampPiece(p *Piece) (string, error) {
	hash := pieceContentHash(p)
	return submitToOTS(hash)
}

// TimestampBlob submits a blob's content hash to OpenTimestamps.
func TimestampBlob(slug, title, textData string) (string, error) {
	canonical := slug + "|" + title + "|" + textData
	h := sha256.Sum256([]byte(canonical))
	return submitToOTS(h[:])
}

// UpgradeTimestamp fetches a more complete OTS proof from the calendar.
// Call after ~1 hour to get the full Bitcoin-anchored proof.
// Returns updated base64 OTS bytes, or the original if upgrade not yet ready.
func UpgradeTimestamp(otsProofBase64 string) (string, error) {
	if otsProofBase64 == "" {
		return "", fmt.Errorf("no proof to upgrade")
	}
	stub, err := base64.StdEncoding.DecodeString(otsProofBase64)
	if err != nil {
		return "", fmt.Errorf("invalid proof: %w", err)
	}

	// Extract the SHA256 digest from the stub (first 32 bytes after OTS magic)
	// OTS stub format: magic(8) + version(1) + filehash_op(1) + hash_type(1) + digest(32)
	if len(stub) < 43 {
		return "", fmt.Errorf("stub too short")
	}
	digest := stub[11:43]

	digestHex := hex.EncodeToString(digest)
	url := otsCalendar + "/timestamp/" + digestHex

	resp, err := http.Get(url)
	if err != nil {
		return otsProofBase64, nil // return original on network error
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// Not yet confirmed — return original stub
		return otsProofBase64, nil
	}
	if resp.StatusCode != http.StatusOK {
		return otsProofBase64, nil
	}

	upgraded, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil || len(upgraded) == 0 {
		return otsProofBase64, nil
	}

	return base64.StdEncoding.EncodeToString(upgraded), nil
}

// OTSProofInfo returns human-readable info about an OTS proof.
func OTSProofInfo(otsProofBase64 string) string {
	if otsProofBase64 == "" {
		return "no timestamp proof"
	}
	stub, err := base64.StdEncoding.DecodeString(otsProofBase64)
	if err != nil {
		return "invalid proof encoding"
	}
	// OTS magic bytes: 0x00 0x4f 0x70 0x65 0x6e 0x54 0x69 0x6d 0x65 0x73 0x74 0x61 0x6d 0x70 0x73...
	// Simplified check — just report size and whether it looks upgraded
	if len(stub) > 200 {
		return fmt.Sprintf("bitcoin-anchored proof (%d bytes) — verifiable with: ots verify", len(stub))
	}
	return fmt.Sprintf("pending proof (%d bytes) — bitcoin confirmation in ~1hr, then run: ots upgrade", len(stub))
}

// pieceContentHash returns the SHA256 of the piece payload as raw bytes
func pieceContentHash(p *Piece) []byte {
	canonical := p.Slug + "|" + p.Title + "|" + p.Body
	h := sha256.Sum256([]byte(canonical))
	return h[:]
}

// PiecePayloadHex returns the hex-encoded SHA256 that gets sent to OTS calendar.
// This is what gets anchored in Bitcoin — sha256(slug|title|body).
func PiecePayloadHex(p *Piece) string {
	return hex.EncodeToString(pieceContentHash(p))
}

// submitToOTS POSTs a 32-byte digest to the OTS calendar and returns the stub.
func submitToOTS(digest []byte) (string, error) {
	if len(digest) != 32 {
		return "", fmt.Errorf("digest must be 32 bytes")
	}

	req, err := http.NewRequest("POST", otsCalendar+"/digest", bytes.NewReader(digest))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Accept", "application/vnd.opentimestamps.v1")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("OTS calendar unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OTS calendar returned %d", resp.StatusCode)
	}

	stub, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil || len(stub) == 0 {
		return "", fmt.Errorf("empty OTS response")
	}

	return base64.StdEncoding.EncodeToString(stub), nil
}
