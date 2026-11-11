package review

import (
	"fmt"
	"strings"

	"github.com/mutex-solutions/cadenza/internal/spec"
)

// NextStep is the one sentence at the top of `cadenza responses`, telling the
// agent what to do with what the owner sent (idea from Crit and Plannotator).
func NextStep(items []spec.Response) string {
	if len(items) == 0 {
		return "Nothing new from the owner. Carry on."
	}
	var answers, comments, notes, sketches int
	var approved, changes bool
	for _, it := range items {
		switch it.Kind {
		case spec.ReplyAnswer:
			answers++
		case spec.ReplyComment:
			comments++
		case spec.ReplyNote:
			notes++
		case spec.ReplySketch:
			sketches++
		case spec.ReplyVerdict:
			// The latest verdict wins.
			approved = it.Verdict == spec.VerdictApprove
			changes = it.Verdict == spec.VerdictRequestChanges
		}
	}
	var parts []string
	if answers > 0 {
		parts = append(parts, count(answers, "answer"))
	}
	if comments > 0 {
		parts = append(parts, count(comments, "comment"))
	}
	if notes > 0 {
		parts = append(parts, count(notes, "note"))
	}
	if sketches > 0 {
		parts = append(parts, count(sketches, "sketch"))
	}
	got := ""
	if len(parts) > 0 {
		got = " The owner sent " + join(parts) + "."
	}
	switch {
	case changes:
		return "Changes requested: address every comment and note below, then show a new version with `cadenza show`." + got
	case approved:
		return "Approved: carry on with the next step." + got
	case comments > 0 || notes > 0 || sketches > 0:
		return "Read each item below and act on it; reply to comments with `cadenza reply <id> <text>` when done." + got
	default:
		return "Use the answers below and carry on." + got
	}
}

func count(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	if strings.HasSuffix(noun, "ch") {
		return fmt.Sprintf("%d %ses", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

func join(parts []string) string {
	if len(parts) == 1 {
		return parts[0]
	}
	return strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1]
}
