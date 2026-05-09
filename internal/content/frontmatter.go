package content

import (
	"strconv"
	"fmt"
	"strings"
	"time"
)

func parseFrontmatter(lines []string, p *Piece) {
	for _, line := range lines {
		k, v, ok := splitKV(line)
		if !ok { continue }
		switch k {
		case "slug":        p.Slug = unquote(v)
		case "title":       p.Title = unquote(v)
		case "type":        p.Type = unquote(v)
		case "access":      p.Access = AccessLevel(unquote(v))
		case "gate":        p.Gate = GateType(unquote(v))
		case "challenge":   p.Challenge = unquote(v)
		case "answer":      p.Answer = unquote(v)
		case "description": p.Description = unquote(v)
		case "price":       p.Price = unquote(v)
		case "price_sats":
			n, _ := strconv.Atoi(strings.TrimSpace(v))
			p.PriceSats = n
		case "tags":
			p.Tags = parseStringSlice(v)
		case "unlock_after":
			for _, layout := range []string{time.RFC3339, "2006-01-02 15:04", "2006-01-02"} {
				if t, err := time.Parse(layout, strings.TrimSpace(unquote(v))); err == nil {
					p.UnlockAfter = t
					break
				}
			}
		case "signature":
			p.Signature = unquote(v)
		case "license":
			p.License = unquote(v)
			fmt.Sscanf(strings.TrimSpace(v), "%d", &p.PriceSats)
		case "human_use":
			p.HumanUse = unquote(v)
		case "agent_use":
			p.AgentUse = unquote(v)
		case "lang":
			p.Lang = unquote(v)
		case "published":
			for _, layout := range []string{time.RFC3339, "2006-01-02", "2006-01-02T15:04:05Z"} {
				if t, err := time.Parse(layout, strings.TrimSpace(unquote(v))); err == nil {
					p.Published = t
					break
				}
			}
		}
	}
}

func splitKV(line string) (string, string, bool) {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) < 2 { return "", "", false }
	k := strings.TrimSpace(parts[0])
	v := strings.TrimSpace(parts[1])
	return k, v, k != ""
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

func parseStringSlice(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		v := unquote(strings.TrimSpace(p))
		if v != "" { out = append(out, v) }
	}
	return out
}

func marshalFrontmatter(p *Piece) string {
	var sb strings.Builder
	wf := func(k, v string) {
		if v != "" { sb.WriteString(k + ": " + v + "\n") }
	}
	wf("slug", p.Slug)
	wf("title", quoteIfNeeded(p.Title))
	wf("type", p.Type)
	wf("access", string(p.Access))
	if p.Gate != "" { wf("gate", string(p.Gate)) }
	if p.Challenge != "" { wf("challenge", quoteIfNeeded(p.Challenge)) }
	if p.Answer != ""    { wf("answer", quoteIfNeeded(p.Answer)) }
	if p.Description != "" { wf("description", quoteIfNeeded(p.Description)) }
	if p.Price != "" { wf("price", quoteIfNeeded(p.Price)) }
	if p.PriceSats > 0 { sb.WriteString("price_sats: " + strconv.Itoa(p.PriceSats) + "\n") }
	if !p.UnlockAfter.IsZero() { wf("unlock_after", p.UnlockAfter.Format("2006-01-02 15:04")) }
	if len(p.Tags) > 0 { sb.WriteString("tags: [" + strings.Join(p.Tags, ", ") + "]\n") }
	if p.Signature != "" { wf("signature", p.Signature) }
	if p.License != "" { wf("license", p.License) }
	if p.Lang != "" { wf("lang", p.Lang) }
	if p.HumanUse != "" { wf("human_use", p.HumanUse) }
	if p.AgentUse != "" { wf("agent_use", p.AgentUse) }
	if !p.Published.IsZero() { sb.WriteString("published: " + p.Published.Format("2006-01-02") + "\n") }
	return sb.String()
}

func quoteIfNeeded(s string) string {
	if strings.ContainsAny(s, ":#\"'[]{}|>&!") || strings.Contains(s, ": ") {
		return "\"" + strings.ReplaceAll(s, "\"", "\\\"") + "\""
	}
	return s
}
