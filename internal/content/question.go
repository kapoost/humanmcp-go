package content

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Question struct {
	ID        string    `json:"id"`       // random 16-byte hex token
	From      string    `json:"from"`     // agent name / handle
	Question  string    `json:"question"` // the question text
	Context   string    `json:"context"`  // optional context
	Answer    string    `json:"answer"`   // human's answer (empty = pending)
	AskedAt   time.Time `json:"asked_at"`
	Answered  time.Time `json:"answered_at,omitempty"`
	FetchedAt time.Time `json:"fetched_at,omitempty"` // when the answer was first picked up
	FetchedBy string    `json:"fetched_by,omitempty"` // "agent" | "owner" — who picked it up
}

func (q *Question) IsPending() bool        { return q.Answer == "" }
func (q *Question) IsFetched() bool        { return !q.FetchedAt.IsZero() }
func (q *Question) IsAgentPickedUp() bool  { return q.IsFetched() && q.FetchedBy == "agent" }
// NeedsPickup: answered but the asking agent hasn't fetched yet.
// Owner peeking through the UI does NOT count as delivery.
func (q *Question) NeedsPickup() bool      { return !q.IsPending() && !q.IsAgentPickedUp() }

type QuestionStore struct {
	dir string
}

func NewQuestionStore(contentDir string) *QuestionStore {
	return &QuestionStore{dir: filepath.Join(filepath.Dir(contentDir), "questions")}
}

// Ask saves a new question and returns its ID (secret token).
func (qs *QuestionStore) Ask(from, question, context string) (*Question, error) {
	from = sanitiseField(from, 64)
	question = sanitiseField(question, 2000)
	context = sanitiseField(context, 2000)

	if question == "" {
		return nil, fmt.Errorf("question text is required")
	}

	if err := os.MkdirAll(qs.dir, 0755); err != nil {
		return nil, fmt.Errorf("cannot create questions dir: %w", err)
	}

	id := generateQuestionID()
	q := &Question{
		ID:       id,
		From:     from,
		Question: question,
		Context:  context,
		AskedAt:  time.Now().UTC(),
	}

	if err := qs.save(q); err != nil {
		return nil, err
	}
	return q, nil
}

// GetAnswer retrieves a question by ID. Only the agent with the ID can access it.
func (qs *QuestionStore) GetAnswer(id string) (*Question, error) {
	path := filepath.Join(qs.dir, id+".txt")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("question not found")
		}
		return nil, err
	}
	return parseQuestionFile(string(data)), nil
}

// Answer records the human's answer to a question.
func (qs *QuestionStore) AnswerQuestion(id, answer string) error {
	q, err := qs.GetAnswer(id)
	if err != nil {
		return err
	}
	q.Answer = sanitiseField(answer, 4000)
	q.Answered = time.Now().UTC()
	// Reset pickup tracking on re-answer so the next call re-stamps.
	q.FetchedAt = time.Time{}
	q.FetchedBy = ""
	return qs.save(q)
}

// MarkFetched stamps FetchedAt/FetchedBy on get_answer.
// First touch wins, with one exception: an owner peek is "upgraded" to an
// agent pickup if the agent eventually fetches. Subsequent same-or-weaker
// touches are no-ops.
//
//   call seq                  → final FetchedBy
//   (nothing) → owner         → owner
//   (nothing) → agent         → agent   (delivered)
//   owner → agent             → agent   (delivered — upgrade)
//   owner → owner             → owner
//   agent → owner             → agent   (don't downgrade)
//   agent → agent             → agent
//
// `by` must be "agent" or "owner".
func (qs *QuestionStore) MarkFetched(id, by string) error {
	q, err := qs.GetAnswer(id)
	if err != nil {
		return err
	}
	if q.IsPending() {
		return nil
	}
	if q.IsAgentPickedUp() {
		return nil // already delivered to agent — final
	}
	if q.IsFetched() && by != "agent" {
		return nil // owner peeking again, nothing changes
	}
	q.FetchedAt = time.Now().UTC()
	q.FetchedBy = by
	return qs.save(q)
}

// ListAll returns every question sorted newest-first (owner dashboard).
func (qs *QuestionStore) ListAll() ([]*Question, error) {
	return qs.listAll()
}

// ListPending returns unanswered questions (owner dashboard).
func (qs *QuestionStore) ListPending() ([]*Question, error) {
	all, err := qs.listAll()
	if err != nil {
		return nil, err
	}
	var out []*Question
	for _, q := range all {
		if q.IsPending() {
			out = append(out, q)
		}
	}
	return out, nil
}

// ListAll returns all questions sorted newest-first (owner use).
func (qs *QuestionStore) listAll() ([]*Question, error) {
	entries, err := os.ReadDir(qs.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var questions []*Question
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".txt") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(qs.dir, e.Name()))
		if err != nil {
			continue
		}
		q := parseQuestionFile(string(data))
		if q != nil && q.ID != "" {
			questions = append(questions, q)
		}
	}

	sort.Slice(questions, func(i, j int) bool {
		return questions[i].AskedAt.After(questions[j].AskedAt)
	})
	return questions, nil
}

func (qs *QuestionStore) save(q *Question) error {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("id: %s\n", q.ID))
	sb.WriteString(fmt.Sprintf("asked_at: %s\n", q.AskedAt.Format("2006-01-02 15:04 UTC")))
	if q.From != "" {
		sb.WriteString(fmt.Sprintf("from: %s\n", q.From))
	}
	if q.Context != "" {
		sb.WriteString(fmt.Sprintf("context: %s\n", q.Context))
	}
	if q.Answer != "" {
		sb.WriteString(fmt.Sprintf("answered_at: %s\n", q.Answered.Format("2006-01-02 15:04 UTC")))
	}
	if !q.FetchedAt.IsZero() {
		sb.WriteString(fmt.Sprintf("fetched_at: %s\n", q.FetchedAt.Format("2006-01-02 15:04 UTC")))
		if q.FetchedBy != "" {
			sb.WriteString(fmt.Sprintf("fetched_by: %s\n", q.FetchedBy))
		}
	}
	sb.WriteString(fmt.Sprintf("\n%s\n", q.Question))
	if q.Answer != "" {
		sb.WriteString(fmt.Sprintf("\n---answer---\n%s\n", q.Answer))
	}

	path := filepath.Join(qs.dir, q.ID+".txt")
	return os.WriteFile(path, []byte(sb.String()), 0644)
}

func parseQuestionFile(content string) *Question {
	q := &Question{}
	lines := strings.Split(content, "\n")
	bodyStart := -1
	answerStart := -1

	for i, line := range lines {
		if line == "" && bodyStart == -1 {
			bodyStart = i + 1
			continue
		}
		if bodyStart != -1 {
			if line == "---answer---" {
				answerStart = i + 1
			}
			continue
		}
		if strings.HasPrefix(line, "id: ") {
			q.ID = strings.TrimSpace(strings.TrimPrefix(line, "id: "))
		} else if strings.HasPrefix(line, "from: ") {
			q.From = strings.TrimSpace(strings.TrimPrefix(line, "from: "))
		} else if strings.HasPrefix(line, "context: ") {
			q.Context = strings.TrimSpace(strings.TrimPrefix(line, "context: "))
		} else if strings.HasPrefix(line, "asked_at: ") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "asked_at: "))
			t, _ := time.Parse("2006-01-02 15:04 UTC", val)
			q.AskedAt = t
		} else if strings.HasPrefix(line, "answered_at: ") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "answered_at: "))
			t, _ := time.Parse("2006-01-02 15:04 UTC", val)
			q.Answered = t
		} else if strings.HasPrefix(line, "fetched_at: ") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "fetched_at: "))
			t, _ := time.Parse("2006-01-02 15:04 UTC", val)
			q.FetchedAt = t
		} else if strings.HasPrefix(line, "fetched_by: ") {
			q.FetchedBy = strings.TrimSpace(strings.TrimPrefix(line, "fetched_by: "))
		}
	}

	if bodyStart >= 0 {
		end := len(lines)
		if answerStart > 0 {
			end = answerStart - 1 // skip "---answer---" line
		}
		q.Question = strings.TrimSpace(strings.Join(lines[bodyStart:end], "\n"))
	}
	if answerStart > 0 && answerStart < len(lines) {
		q.Answer = strings.TrimSpace(strings.Join(lines[answerStart:], "\n"))
	}
	return q
}

func generateQuestionID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
