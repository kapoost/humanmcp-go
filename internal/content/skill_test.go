package content

import (
	"testing"
	"time"
)

func TestSkillSaveAndGet(t *testing.T) {
	ss := NewSkillStore(t.TempDir())
	sk := &Skill{
		Slug:     "go-stack",
		Category: "tech",
		Title:    "Go stack",
		Body:     "Go 1.22, fly.io, net/http only",
		Tags:     []string{"go", "backend"},
	}
	if err := ss.SaveSkill(sk); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	got, err := ss.GetSkill("go-stack")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if got.Title != "Go stack" {
		t.Errorf("title mismatch: %s", got.Title)
	}
	if got.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be set on save")
	}
}

func TestSkillSaveRequiresSlug(t *testing.T) {
	ss := NewSkillStore(t.TempDir())
	err := ss.SaveSkill(&Skill{Title: "no slug"})
	if err == nil {
		t.Error("save without slug should fail")
	}
}

func TestSkillList(t *testing.T) {
	ss := NewSkillStore(t.TempDir())
	ss.SaveSkill(&Skill{Slug: "s1", Category: "tech", Title: "T1", Body: "b"})
	ss.SaveSkill(&Skill{Slug: "s2", Category: "workflow", Title: "T2", Body: "b"})
	ss.SaveSkill(&Skill{Slug: "s3", Category: "tech", Title: "T3", Body: "b"})

	all, _ := ss.ListSkills("")
	if len(all) != 3 {
		t.Errorf("expected 3 skills, got %d", len(all))
	}

	tech, _ := ss.ListSkills("tech")
	if len(tech) != 2 {
		t.Errorf("expected 2 tech skills, got %d", len(tech))
	}
}

func TestSkillDelete(t *testing.T) {
	ss := NewSkillStore(t.TempDir())
	ss.SaveSkill(&Skill{Slug: "to-delete", Category: "x", Title: "x", Body: "x"})

	if err := ss.DeleteSkill("to-delete"); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	_, err := ss.GetSkill("to-delete")
	if err == nil {
		t.Error("get after delete should fail")
	}
}

func TestSkillDeleteNotFound(t *testing.T) {
	ss := NewSkillStore(t.TempDir())
	if err := ss.DeleteSkill("nieistniejacy"); err == nil {
		t.Error("deleting nonexistent skill should return error")
	}
}

func TestPersonaSaveAndGet(t *testing.T) {
	ss := NewSkillStore(t.TempDir())
	p := &Persona{
		Slug:   "hermiona",
		Name:   "Hermiona",
		Role:   "Intent Analyst",
		Prompt: "Jesteś Hermiona...",
		Tags:   []string{"context", "memory"},
	}
	if err := ss.SavePersona(p); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	got, err := ss.GetPersona("hermiona")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if got.Name != "Hermiona" {
		t.Errorf("name mismatch: %s", got.Name)
	}
	if got.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be set")
	}
}

func TestPersonaSaveRequiresSlug(t *testing.T) {
	ss := NewSkillStore(t.TempDir())
	err := ss.SavePersona(&Persona{Name: "no slug"})
	if err == nil {
		t.Error("save without slug should fail")
	}
}

func TestPersonaList(t *testing.T) {
	ss := NewSkillStore(t.TempDir())
	ss.SavePersona(&Persona{Slug: "p1", Name: "A", Role: "r", Prompt: "x"})
	ss.SavePersona(&Persona{Slug: "p2", Name: "B", Role: "r", Prompt: "x"})

	personas, _ := ss.ListPersonas()
	if len(personas) != 2 {
		t.Errorf("expected 2 personas, got %d", len(personas))
	}
}

func TestPersonaDelete(t *testing.T) {
	ss := NewSkillStore(t.TempDir())
	ss.SavePersona(&Persona{Slug: "to-del", Name: "x", Role: "x", Prompt: "x"})
	if err := ss.DeletePersona("to-del"); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	_, err := ss.GetPersona("to-del")
	if err == nil {
		t.Error("get after delete should fail")
	}
}

func TestSkillCategories(t *testing.T) {
	ss := NewSkillStore(t.TempDir())
	ss.SaveSkill(&Skill{Slug: "a", Category: "tech", Title: "a", Body: "b"})
	ss.SaveSkill(&Skill{Slug: "b", Category: "tech", Title: "b", Body: "b"})
	ss.SaveSkill(&Skill{Slug: "c", Category: "workflow", Title: "c", Body: "b"})

	cats, err := ss.Categories()
	if err != nil {
		t.Fatalf("categories failed: %v", err)
	}
	if len(cats) != 2 {
		t.Errorf("expected 2 categories, got %d: %v", len(cats), cats)
	}
}

func TestSkillUpdatedAtChangesOnSave(t *testing.T) {
	ss := NewSkillStore(t.TempDir())
	sk := &Skill{Slug: "s", Category: "x", Title: "x", Body: "v1"}
	ss.SaveSkill(sk)
	first, _ := ss.GetSkill("s")

	time.Sleep(2 * time.Millisecond)
	sk.Body = "v2"
	ss.SaveSkill(sk)
	second, _ := ss.GetSkill("s")

	if !second.UpdatedAt.After(first.UpdatedAt) {
		t.Error("UpdatedAt should increase on re-save")
	}
}
