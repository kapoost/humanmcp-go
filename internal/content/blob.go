package content

import (
	"bufio"
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// BlobType classifies the kind of data
type BlobType string

const (
	BlobImage    BlobType = "image"    // photo/illustration — base64 or file ref
	BlobContact  BlobType = "contact"  // address, phone, email — structured JSON
	BlobVector   BlobType = "vector"   // float32 embeddings — agents only
	BlobDocument BlobType = "document" // PDF, binary — file ref
	BlobDataset  BlobType = "dataset"  // JSON/CSV structured data
	BlobCapsule    BlobType = "capsule"    // any schema — agent declares what it needs
	BlobProvenance BlobType = "provenance" // artwork provenance document (certificate, sale, opinion)
)

// AudienceEntry is a named access grant
type AudienceEntry struct {
	Kind string // "agent", "human"
	ID   string // agent name, email, handle, or "*" for any of that kind
}

// Blob is a typed data artifact
type Blob struct {
	Slug        string          `json:"Slug"`
	Title       string          `json:"Title"`
	BlobType    BlobType        `json:"BlobType"`
	Description string          `json:"Description"`
	Published   time.Time       `json:"Published"`
	Access      AccessLevel     `json:"Access"`
	Audience    []AudienceEntry `json:"Audience"`
	Gate        GateType        `json:"Gate"`
	Challenge   string          `json:"Challenge"`
	Answer      string          `json:"Answer"`
	TextData    string          `json:"TextData"`    // inline text/JSON/CSV
	Base64Data  string          `json:"Base64Data"`  // inline binary as base64
	FileRef     string          `json:"FileRef"`     // path under blobs/files/
	Schema      string          `json:"Schema"`      // e.g. "text-embedding-3-small"
	MimeType    string          `json:"MimeType"`    // e.g. "image/jpeg"
	Dimensions  int             `json:"Dimensions"`  // for vectors
	Encoding    string          `json:"Encoding"`    // "base64-float32", "utf-8"
	Signature   string          `json:"Signature"`
	Tags        []string        `json:"Tags"`
	// Provenance fields — link document to an artwork
	Artwork     string          `json:"Artwork,omitempty"`  // slug of parent artwork piece
	DocType     string          `json:"DocType,omitempty"`  // certificate, sale, opinion, appraisal, restoration, exhibition, provenance
	DocDate     string          `json:"DocDate,omitempty"`  // date of the document (YYYY-MM-DD)
	IssuedBy    string          `json:"IssuedBy,omitempty"` // who issued it (gallery, expert, auction house)
	FilePath    string          `json:"-"`
}

// IsAccessibleTo checks audience membership
func (b *Blob) IsAccessibleTo(callerKind, callerID string) bool {
	if b.Access == AccessPublic {
		return true
	}
	for _, a := range b.Audience {
		if a.ID == "*" && strings.EqualFold(a.Kind, callerKind) {
			return true
		}
		if strings.EqualFold(a.Kind, callerKind) && strings.EqualFold(a.ID, callerID) {
			return true
		}
	}
	return false
}

// BlobStore manages typed data artifacts stored as .blob files
type BlobStore struct {
	dir   string
	cache *Cache[[]*Blob]
}

func NewBlobStore(contentDir string) *BlobStore {
	return &BlobStore{
		dir:   filepath.Join(filepath.Dir(contentDir), "blobs"),
		cache: NewCache[[]*Blob](5 * time.Second),
	}
}

func (bs *BlobStore) Load() ([]*Blob, error) {
	if cached, ok := bs.cache.Get(); ok {
		return cached, nil
	}
	entries, err := os.ReadDir(bs.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var blobs []*Blob
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".blob") {
			continue
		}
		b, err := parseBlob(filepath.Join(bs.dir, e.Name()))
		if err != nil {
			continue
		}
		blobs = append(blobs, b)
	}
	sort.Slice(blobs, func(i, j int) bool {
		return blobs[i].Published.After(blobs[j].Published)
	})
	bs.cache.Set(blobs)
	return blobs, nil
}

func (bs *BlobStore) Get(slug string) (*Blob, error) {
	blobs, err := bs.Load()
	if err != nil {
		return nil, err
	}
	for _, b := range blobs {
		if b.Slug == slug {
			return b, nil
		}
	}
	return nil, fmt.Errorf("blob not found: %s", slug)
}

func (bs *BlobStore) Save(b *Blob) error {
	if err := os.MkdirAll(bs.dir, 0755); err != nil {
		return err
	}
	if b.Published.IsZero() {
		b.Published = time.Now()
	}
	path := filepath.Join(bs.dir, b.Slug+".blob")
	b.FilePath = path

	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.WriteString(marshalBlobMeta(b))
	buf.WriteString("---\n")
	if b.TextData != "" {
		buf.WriteString(b.TextData)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		return err
	}
	bs.cache.Invalidate()
	return nil
}

func (bs *BlobStore) Delete(slug string) error {
	err := os.Remove(filepath.Join(bs.dir, slug+".blob"))
	if err == nil { bs.cache.Invalidate() }
	return err
}

// StoreFile saves a binary file under blobs/files/ and returns the ref
func (bs *BlobStore) StoreFile(slug, filename string, data []byte) (string, error) {
	dir := filepath.Join(bs.dir, "files")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	ext := filepath.Ext(filename)
	ref := "files/" + slug + ext
	if err := os.WriteFile(filepath.Join(bs.dir, ref), data, 0644); err != nil {
		return "", err
	}
	return ref, nil
}

// ReadFile returns the raw bytes of a file-ref blob
func (bs *BlobStore) ReadFile(ref string) ([]byte, error) {
	return os.ReadFile(filepath.Join(bs.dir, ref))
}

// Provenance returns all provenance documents for a given artwork slug, sorted by doc_date
func (bs *BlobStore) Provenance(artworkSlug string) ([]*Blob, error) {
	blobs, err := bs.Load()
	if err != nil {
		return nil, err
	}
	var out []*Blob
	for _, b := range blobs {
		if b.BlobType == BlobProvenance && b.Artwork == artworkSlug {
			out = append(out, b)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].DocDate < out[j].DocDate
	})
	return out, nil
}

// SignBlob signs a blob's content with an Ed25519 key
func SignBlob(b *Blob, kp *KeyPair) (string, error) {
	canonical := b.Slug + "|" + b.Title + "|" + b.TextData + "|" + b.Base64Data + "|" + b.FileRef
	hash := sha256.Sum256([]byte(canonical))
	sig := ed25519.Sign(kp.PrivateKey, hash[:])
	return base64.StdEncoding.EncodeToString(sig), nil
}

// VerifyBlob checks a blob's signature
func VerifyBlob(b *Blob, pubKeyHex string) (bool, string) {
	if b.Signature == "" {
		return false, "unsigned"
	}
	pub, err := PublicKeyFromHex(pubKeyHex)
	if err != nil {
		return false, "invalid public key"
	}
	sigBytes, err := base64.StdEncoding.DecodeString(b.Signature)
	if err != nil {
		return false, "malformed signature"
	}
	canonical := b.Slug + "|" + b.Title + "|" + b.TextData + "|" + b.Base64Data + "|" + b.FileRef
	hash := sha256.Sum256([]byte(canonical))
	if !ed25519.Verify(pub, hash[:], sigBytes) {
		return false, "invalid — content may have been modified"
	}
	return true, "verified — signed by kapoost's key"
}

// --- file parsing ---

func parseBlob(path string) (*Blob, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	b := &Blob{FilePath: path, Access: AccessPublic}
	var fmLines, bodyLines []string
	inFM, fmDone := false, false
	lineNum := 0

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 10*1024*1024), 10*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		lineNum++
		if lineNum == 1 && line == "---" { inFM = true; continue }
		if inFM && line == "---" { inFM = false; fmDone = true; continue }
		if inFM { fmLines = append(fmLines, line) } else if fmDone { bodyLines = append(bodyLines, line) }
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	parseBlobMeta(fmLines, b)
	if len(bodyLines) > 0 {
		b.TextData = strings.TrimPrefix(strings.Join(bodyLines, "\n"), "\n")
	}
	if b.Slug == "" {
		b.Slug = strings.TrimSuffix(filepath.Base(path), ".blob")
	}
	return b, nil
}

func parseBlobMeta(lines []string, b *Blob) {
	for _, line := range lines {
		k, v, ok := splitKV(line)
		if !ok { continue }
		switch k {
		case "slug":        b.Slug = unquote(v)
		case "title":       b.Title = unquote(v)
		case "blob_type":   b.BlobType = BlobType(unquote(v))
		case "description": b.Description = unquote(v)
		case "access":      b.Access = AccessLevel(unquote(v))
		case "gate":        b.Gate = GateType(unquote(v))
		case "challenge":   b.Challenge = unquote(v)
		case "answer":      b.Answer = unquote(v)
		case "mime_type":   b.MimeType = unquote(v)
		case "schema":      b.Schema = unquote(v)
		case "encoding":    b.Encoding = unquote(v)
		case "file_ref":    b.FileRef = unquote(v)
		case "base64_data": b.Base64Data = strings.TrimSpace(v)
		case "signature":   b.Signature = unquote(v)
		case "tags":        b.Tags = parseStringSlice(v)
		case "artwork":     b.Artwork = unquote(v)
		case "doc_type":    b.DocType = unquote(v)
		case "doc_date":    b.DocDate = unquote(v)
		case "issued_by":   b.IssuedBy = unquote(v)
		case "dimensions":  fmt.Sscanf(strings.TrimSpace(v), "%d", &b.Dimensions)
		case "audience":
			for _, p := range parseStringSlice(v) {
				parts := strings.SplitN(p, ":", 2)
				if len(parts) == 2 {
					b.Audience = append(b.Audience, AudienceEntry{Kind: parts[0], ID: parts[1]})
				}
			}
		case "published":
			for _, layout := range []string{time.RFC3339, "2006-01-02"} {
				if t, err := time.Parse(layout, strings.TrimSpace(unquote(v))); err == nil {
					b.Published = t
					break
				}
			}
		}
	}
}

func marshalBlobMeta(b *Blob) string {
	var sb strings.Builder
	wf := func(k, v string) { if v != "" { sb.WriteString(k + ": " + v + "\n") } }
	wf("slug", b.Slug)
	wf("title", quoteIfNeeded(b.Title))
	wf("blob_type", string(b.BlobType))
	wf("description", quoteIfNeeded(b.Description))
	wf("access", string(b.Access))
	if b.Gate != "" { wf("gate", string(b.Gate)) }
	if b.Challenge != "" { wf("challenge", quoteIfNeeded(b.Challenge)) }
	if b.Answer != "" { wf("answer", quoteIfNeeded(b.Answer)) }
	if b.MimeType != "" { wf("mime_type", b.MimeType) }
	if b.Schema != "" { wf("schema", quoteIfNeeded(b.Schema)) }
	if b.Encoding != "" { wf("encoding", b.Encoding) }
	if b.Dimensions > 0 { sb.WriteString(fmt.Sprintf("dimensions: %d\n", b.Dimensions)) }
	if b.FileRef != "" { wf("file_ref", b.FileRef) }
	if b.Base64Data != "" { sb.WriteString("base64_data: " + b.Base64Data + "\n") }
	if b.Signature != "" { wf("signature", b.Signature) }
	if b.Artwork != "" { wf("artwork", b.Artwork) }
	if b.DocType != "" { wf("doc_type", b.DocType) }
	if b.DocDate != "" { wf("doc_date", b.DocDate) }
	if b.IssuedBy != "" { wf("issued_by", quoteIfNeeded(b.IssuedBy)) }
	if len(b.Tags) > 0 { sb.WriteString("tags: [" + strings.Join(b.Tags, ", ") + "]\n") }
	if len(b.Audience) > 0 {
		parts := make([]string, len(b.Audience))
		for i, a := range b.Audience { parts[i] = a.Kind + ":" + a.ID }
		sb.WriteString("audience: [" + strings.Join(parts, ", ") + "]\n")
	}
	if !b.Published.IsZero() { sb.WriteString("published: " + b.Published.Format("2006-01-02") + "\n") }
	return sb.String()
}
