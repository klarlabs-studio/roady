package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteInstructionBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")
	own := "# My project\n\nUse tabs.\n"
	if err := os.WriteFile(path, []byte(own), 0o644); err != nil {
		t.Fatal(err)
	}

	reg, err := writeInstructionBlock(dir, "CLAUDE.md")
	if err != nil {
		t.Fatal(err)
	}
	if reg.Created || reg.Updated || reg.Unchanged {
		t.Errorf("first run appends: %+v", reg)
	}
	got, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(got), own+"\n"+instructionsBegin) {
		t.Errorf("block should follow the user's content after a blank line:\n%s", got)
	}

	// Re-running never duplicates the block.
	again, err := writeInstructionBlock(dir, "CLAUDE.md")
	if err != nil || !again.Unchanged {
		t.Fatalf("second run: %+v %v", again, err)
	}
	got2, _ := os.ReadFile(path)
	if string(got2) != string(got) || strings.Count(string(got2), instructionsBegin) != 1 {
		t.Errorf("second run changed the file")
	}

	// An outdated block is replaced in place; text around it is kept.
	stale := strings.Replace(string(got), "## Planning lives in roady", "## Old wording", 1) + "\n## After\n\nkeep me\n"
	_ = os.WriteFile(path, []byte(stale), 0o644)
	upd, err := writeInstructionBlock(dir, "CLAUDE.md")
	if err != nil || !upd.Updated {
		t.Fatalf("stale block: %+v %v", upd, err)
	}
	got3, _ := os.ReadFile(path)
	s := string(got3)
	if strings.Contains(s, "Old wording") || strings.Count(s, instructionsBegin) != 1 ||
		!strings.HasPrefix(s, own) || !strings.HasSuffix(s, "## After\n\nkeep me\n") {
		t.Errorf("in-place update went wrong:\n%s", s)
	}

	// A missing file is created with just the block.
	created, err := writeInstructionBlock(dir, "AGENTS.md")
	if err != nil || !created.Created {
		t.Fatalf("create: %+v %v", created, err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md")); string(b) != roadyInstructions {
		t.Errorf("new file should hold only the block:\n%s", b)
	}

	// A stray marker is reported rather than guessed around.
	_ = os.WriteFile(filepath.Join(dir, "GEMINI.md"), []byte("x\n"+instructionsBegin+"\nno end\n"), 0o644)
	if _, err := writeInstructionBlock(dir, "GEMINI.md"); err == nil {
		t.Error("an unmatched marker must be refused")
	}
}

func TestWriteRoadySkill(t *testing.T) {
	dir := t.TempDir()
	reg, err := writeRoadySkill(dir)
	if err != nil || !reg.Created {
		t.Fatalf("%+v %v", reg, err)
	}
	b, err := os.ReadFile(filepath.Join(dir, ".claude", "skills", "roady-planning", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(b), "---\nname: roady-planning\ndescription: ") {
		t.Errorf("skill needs frontmatter with name and description:\n%.200s", b)
	}
	if again, _ := writeRoadySkill(dir); !again.Unchanged {
		t.Errorf("re-run: %+v", again)
	}
}
