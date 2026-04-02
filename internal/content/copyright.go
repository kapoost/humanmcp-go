package content

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode"
)

// LicenseType defines how intellectual property may be used
type LicenseType string

const (
	LicenseFree       LicenseType = "free"        // read, share, attribute — no commercial use
	LicenseCCBY       LicenseType = "cc-by"        // Creative Commons Attribution 4.0
	LicenseCCBYNC     LicenseType = "cc-by-nc"     // CC BY Non-Commercial
	LicenseCommercial LicenseType = "commercial"   // pay N sats for commercial use
	LicenseExclusive  LicenseType = "exclusive"    // contact to negotiate transfer
	LicenseAllRights  LicenseType = "all-rights"   // full IP sale available
)

// OriginalityIndex is a composite score of content uniqueness.
// It does NOT claim to detect AI — it measures human-style originality signals.
// Score: 0.0 (generic/uniform) → 1.0 (highly original/distinctive)
type OriginalityIndex struct {
	// Component scores (each 0.0-1.0)
	Burstiness      float64 `json:"burstiness"`       // sentence length variation (Fano Factor)
	LexicalDensity  float64 `json:"lexical_density"`  // vocabulary richness (TTR corrected)
	ShannonEntropy  float64 `json:"shannon_entropy"`  // character-level unpredictability
	StructuralSig   float64 `json:"structural_sig"`   // line length signature

	// Derived
	Combined        float64 `json:"combined"`         // weighted composite 0.0-1.0
	Grade           string  `json:"grade"`            // S / A / B / C / D
	WordCount       int     `json:"word_count"`
	SentenceCount   int     `json:"sentence_count"`
	UniqueWords     int     `json:"unique_words"`
	Notes           string  `json:"notes"`            // human-readable explanation
}

// Copyright holds the IP metadata embedded in a piece
type Copyright struct {
	Author       string          `json:"author"`
	Title        string          `json:"title"`
	Created      time.Time       `json:"created"`
	ContentHash  string          `json:"content_hash"`  // sha256(body)
	Signature    string          `json:"signature"`      // ed25519
	OTSProof     string          `json:"ots_proof"`      // OpenTimestamps base64 stub
	License      LicenseType     `json:"license"`
	PriceSats    int             `json:"price_sats"`    // 0 = free
	Originality  OriginalityIndex `json:"originality"`
	PublicKey    string          `json:"public_key"`
}

// ComputeOriginality calculates the originality index for a body of text.
// Uses four components based on current NLP research (2024-2025):
// burstiness (Fano Factor), lexical density (corrected TTR),
// Shannon entropy, and structural signature.
func ComputeOriginality(text string) OriginalityIndex {
	idx := OriginalityIndex{}
	if strings.TrimSpace(text) == "" {
		idx.Grade = "D"
		idx.Notes = "Empty content."
		return idx
	}

	// --- 1. BURSTINESS via Fano Factor (weight: 0.30) ---
	// Measures sentence/line length variation.
	// High variation = more human-like rhythm.
	// Uses newlines AND punctuation as boundaries.
	lines := splitSentences(text)
	idx.SentenceCount = len(lines)
	var sentLens []float64
	for _, s := range lines {
		w := len(strings.Fields(s))
		if w > 0 { sentLens = append(sentLens, float64(w)) }
	}
	var burstScore float64
	if len(sentLens) >= 3 {
		m := mean(sentLens)
		v := variance(sentLens, m)
		if m > 0 {
			// Fano factor, normalized. Good human text: Fano 2-15
			fano := v / m
			burstScore = math.Min(fano/8.0, 1.0)
		}
	} else if len(sentLens) >= 2 {
		burstScore = 0.15 // too few lines
	}
	idx.Burstiness = round2(burstScore)

	// --- 2. LEXICAL DENSITY — Corrected TTR (weight: 0.30) ---
	// CTTR = unique / sqrt(2 * total) — length-corrected vocab richness.
	words := tokenizeWords(text)
	idx.WordCount = len(words)
	unique := uniqueWords(words)
	idx.UniqueWords = len(unique)
	var lexScore float64
	if len(words) >= 3 {
		cttr := float64(len(unique)) / math.Sqrt(2.0*float64(len(words)))
		// CTTR ~4-6 = typical; ~8+ = very rich. Normalize to 1.0 at 10.
		lexScore = math.Min(cttr/8.0, 1.0)
	}
	idx.LexicalDensity = round2(lexScore)

	// --- 3. SHANNON ENTROPY at character level (weight: 0.25) ---
	// Measures character-level unpredictability.
	// Human creative text: typically 3.8-4.8 bits/char
	// Simple repetitive text: < 3.5
	freq := make(map[rune]int)
	totalChars := 0
	for _, r := range text {
		if !unicode.IsSpace(r) { freq[r]++; totalChars++ }
	}
	var shannon float64
	if totalChars > 0 {
		for _, count := range freq {
			p := float64(count) / float64(totalChars)
			shannon -= p * math.Log2(p)
		}
	}
	// Normalize: 2.0 bits = 0.0, 5.5 bits = 1.0
	shannonScore := math.Max(0, math.Min((shannon-2.0)/3.5, 1.0))
	idx.ShannonEntropy = round2(shannonScore)

	// --- 4. STRUCTURAL SIGNATURE — line length CV (weight: 0.15) ---
	// CV of character-level line lengths.
	// Short poem lines mixed with title lines = high CV.
	// Prose paragraphs = low CV.
	nonEmpty := nonEmptyLines(text)
	var structScore float64
	if len(nonEmpty) >= 3 {
		var lineLens []float64
		for _, l := range nonEmpty {
			lineLens = append(lineLens, float64(len(strings.TrimSpace(l))))
		}
		m := mean(lineLens)
		if m > 0 {
			cv := math.Sqrt(variance(lineLens, m)) / m
			// CV > 0.5 = distinctive structure
			structScore = math.Min(cv/1.0, 1.0)
		}
	}
	idx.StructuralSig = round2(structScore)

	// --- COMPOSITE ---
	combined := idx.Burstiness*0.30 +
		idx.LexicalDensity*0.30 +
		idx.ShannonEntropy*0.25 +
		idx.StructuralSig*0.15
	idx.Combined = round2(combined)

	switch {
	case combined >= 0.72: idx.Grade = "S"
	case combined >= 0.58: idx.Grade = "A"
	case combined >= 0.44: idx.Grade = "B"
	case combined >= 0.30: idx.Grade = "C"
	default:               idx.Grade = "D"
	}

	idx.Notes = buildNotes(idx)
	return idx
}

func buildNotes(idx OriginalityIndex) string {
	var parts []string
	if idx.Burstiness >= 0.7 {
		parts = append(parts, "high sentence rhythm variation")
	} else if idx.Burstiness < 0.3 {
		parts = append(parts, "uniform sentence lengths")
	}
	if idx.LexicalDensity >= 0.7 {
		parts = append(parts, "rich vocabulary")
	} else if idx.LexicalDensity < 0.3 {
		parts = append(parts, "limited vocabulary range")
	}
	if idx.ShannonEntropy >= 0.7 {
		parts = append(parts, "high character-level entropy")
	}
	if idx.StructuralSig >= 0.6 {
		parts = append(parts, "distinctive structural pattern")
	}
	if idx.WordCount < 20 {
		parts = append(parts, "too short for reliable scoring")
	}
	if len(parts) == 0 {
		return "moderate originality signals"
	}
	return strings.Join(parts, "; ")
}

// ContentHash returns sha256 hex of the content body
func ContentHash(body string) string {
	h := sha256.Sum256([]byte(body))
	return hex.EncodeToString(h[:])
}

// BuildCopyright creates a Copyright record for a piece
func BuildCopyright(p *Piece, authorName, publicKeyHex string) *Copyright {
	return &Copyright{
		Author:      authorName,
		Title:       p.Title,
		Created:     p.Published,
		ContentHash: ContentHash(p.Body),
		Signature:   p.Signature,
		OTSProof:    p.OTSProof,
		License:     LicenseType(p.License),
		PriceSats:   p.PriceSats,
		Originality: ComputeOriginality(p.Body),
		PublicKey:   publicKeyHex,
	}
}

// FormatCertificate returns a human+agent readable IP certificate
func FormatCertificate(c *Copyright) string {
	var sb strings.Builder
	sb.WriteString("INTELLECTUAL PROPERTY CERTIFICATE\n")
	sb.WriteString(strings.Repeat("─", 44) + "\n")
	sb.WriteString(fmt.Sprintf("title:          %s\n", c.Title))
	sb.WriteString(fmt.Sprintf("author:         %s\n", c.Author))
	sb.WriteString(fmt.Sprintf("created:        %s\n", c.Created.Format("2 January 2006")))
	sb.WriteString(fmt.Sprintf("license:        %s\n", c.License))
	if c.PriceSats > 0 {
		sb.WriteString(fmt.Sprintf("price:          %d sats (commercial use)\n", c.PriceSats))
	}
	sb.WriteString("\n")
	sb.WriteString("AUTHENTICITY\n")
	sb.WriteString(fmt.Sprintf("  content_hash: %.16s...\n", c.ContentHash))
	if c.Signature != "" {
		sb.WriteString(fmt.Sprintf("  signature:    %.16s...\n", c.Signature))
	}
	if c.PublicKey != "" {
		sb.WriteString(fmt.Sprintf("  public_key:   %.16s...\n", c.PublicKey))
	}
	if c.OTSProof != "" {
		proofLen := len(c.OTSProof)
		preview := c.OTSProof
		if proofLen > 16 { preview = c.OTSProof[:16] }
		// Base64-encoded stub >266 chars ≈ >200 decoded bytes = upgraded proof
		if proofLen > 266 {
			sb.WriteString("  timestamp:    ✓ bitcoin-anchored — run: ots verify\n")
		} else {
			sb.WriteString("  timestamp:    pending (~1hr) — run: ots upgrade\n")
		}
		sb.WriteString(fmt.Sprintf("  ots_proof:    %s...\n", preview))
	} else {
		sb.WriteString("  timestamp:    not yet timestamped\n")
	}
	sb.WriteString("\n")

	o := c.Originality
	sb.WriteString(fmt.Sprintf("ORIGINALITY INDEX  %s  (%.2f / 1.00)\n", o.Grade, o.Combined))
	sb.WriteString(fmt.Sprintf("  burstiness:      %.2f  sentence rhythm variation\n", o.Burstiness))
	sb.WriteString(fmt.Sprintf("  lexical density: %.2f  vocabulary richness\n", o.LexicalDensity))
	sb.WriteString(fmt.Sprintf("  entropy:         %.2f  character-level unpredictability\n", o.ShannonEntropy))
	sb.WriteString(fmt.Sprintf("  structure:       %.2f  line pattern signature\n", o.StructuralSig))
	sb.WriteString(fmt.Sprintf("  words: %d  sentences: %d  unique: %d\n",
		o.WordCount, o.SentenceCount, o.UniqueWords))
	if o.Notes != "" {
		sb.WriteString(fmt.Sprintf("  notes:           %s\n", o.Notes))
	}
	sb.WriteString("\n")
	sb.WriteString(licenseTerms(c.License, c.PriceSats))
	return sb.String()
}

func licenseTerms(l LicenseType, price int) string {
	switch l {
	case LicenseFree:
		return "TERMS: Free to read, share, and quote with attribution.\n" +
			"Commercial use requires author permission.\n"
	case LicenseCCBY:
		return "TERMS: Creative Commons Attribution 4.0 International.\n" +
			"Free to use, adapt, distribute with credit.\n" +
			"See: creativecommons.org/licenses/by/4.0\n"
	case LicenseCCBYNC:
		return "TERMS: CC BY Non-Commercial 4.0.\n" +
			"Non-Commercial use only, with attribution.\n" +
			"See: creativecommons.org/licenses/by-nc/4.0\n"
	case LicenseCommercial:
		return fmt.Sprintf("TERMS: Commercial use license available for %d sats.\n"+
			"Contact author or use purchase_rights tool to acquire.\n", price)
	case LicenseExclusive:
		return "TERMS: Exclusive rights available. Contact author to negotiate.\n" +
			"This work may not be used commercially without a signed agreement.\n"
	case LicenseAllRights:
		return "TERMS: Full IP transfer available. Author is open to selling all rights.\n" +
			"Contact to discuss terms and price.\n"
	default:
		return "TERMS: All rights reserved. Contact author for any use beyond reading.\n"
	}
}

// --- text analysis helpers ---

func splitSentences(text string) []string {
	// Split on sentence-ending punctuation OR newlines.
	// Newlines are treated as sentence boundaries for poetry/verse.
	var sentences []string
	var buf strings.Builder
	for _, r := range text {
		if r == '\n' {
			if s := strings.TrimSpace(buf.String()); len(strings.Fields(s)) >= 1 {
				sentences = append(sentences, s)
			}
			buf.Reset()
		} else if r == '.' || r == '!' || r == '?' {
			buf.WriteRune(r)
			if s := strings.TrimSpace(buf.String()); len(strings.Fields(s)) >= 2 {
				sentences = append(sentences, s)
				buf.Reset()
			}
		} else {
			buf.WriteRune(r)
		}
	}
	if s := strings.TrimSpace(buf.String()); len(strings.Fields(s)) >= 1 {
		sentences = append(sentences, s)
	}
	return sentences
}

func tokenizeWords(text string) []string {
	var words []string
	for _, word := range strings.Fields(text) {
		clean := strings.TrimFunc(word, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r)
		})
		if len([]rune(clean)) > 1 {
			words = append(words, clean)
		}
	}
	return words
}

func uniqueWords(words []string) map[string]bool {
	m := make(map[string]bool)
	for _, w := range words {
		m[strings.ToLower(w)] = true
	}
	return m
}

func nonEmptyLines(text string) []string {
	var out []string
	for _, l := range strings.Split(text, "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

func mean(vals []float64) float64 {
	if len(vals) == 0 { return 0 }
	sum := 0.0
	for _, v := range vals { sum += v }
	return sum / float64(len(vals))
}

func variance(vals []float64, m float64) float64 {
	if len(vals) == 0 { return 0 }
	sum := 0.0
	for _, v := range vals { sum += (v - m) * (v - m) }
	return sum / float64(len(vals))
}

func round2(f float64) float64 {
	return math.Round(f*100) / 100
}

