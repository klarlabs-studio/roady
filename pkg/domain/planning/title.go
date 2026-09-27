package planning

import "strings"

// statusWords are titles that describe where a task stands rather than what
// it is. An agent that means "mark this done" and edits the title instead
// leaves a plan full of tasks called "done" — nexa's plan had 89 — whose
// intent is lost for good.
var statusWords = map[string]bool{
	"done": true, "complete": true, "completed": true, "finished": true,
	"verified": true, "blocked": true, "in progress": true, "in-progress": true,
	"in_progress": true, "pending": true, "started": true, "wip": true,
	"todo": true, "to do": true, "shipped": true, "closed": true, "ok": true,
}

// IsStatusTitle reports whether title is only a status word.
func IsStatusTitle(title string) bool {
	t := strings.ToLower(strings.Trim(strings.TrimSpace(title), ".!:✓✔ "))
	return statusWords[t]
}
