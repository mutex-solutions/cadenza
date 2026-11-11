package template

import (
	"slices"
	"testing"
)

func TestQuestionFromOptions(t *testing.T) {
	d, _ := Builtin("options")
	text, choices, ok := d.Question(d.Example)
	if !ok || text != "Which database should the trial mode use?" || !slices.Equal(choices, []string{"SQLite", "Embedded Postgres"}) {
		t.Fatalf("got %q %v %v", text, choices, ok)
	}
}

func TestQuestionFallbacks(t *testing.T) {
	base := Definition{Name: "q", Slots: []Slot{
		{Name: "q", Block: BlockText},
		{Name: "opts", Block: BlockChoices, Required: true},
		{Name: "pic", Block: BlockImage},
	}}
	data := raw(`{"opts": [{"title": "A"}, {"title": "B"}], "pic": {"file": "./a.png"}}`)

	d := base
	d.Ask = &QuestionSpec{FromSlot: "opts", TextSlot: "q", Text: "Pick one?"}
	if text, _, ok := d.Question(data); !ok || text != "Pick one?" {
		t.Fatalf("text fallback: %q %v", text, ok)
	}
	d.Ask = &QuestionSpec{FromSlot: "opts"}
	if text, choices, ok := d.Question(data); !ok || text != DefaultQuestion || len(choices) != 2 {
		t.Fatalf("default: %q %v", text, ok)
	}
	// Stored references work too.
	if _, _, ok := d.Question(raw(`{"opts": [{"title": "A"}, {"title": "B"}], "pic": {"file": ` + webp + `}}`)); !ok {
		t.Fatal("stored ref rejected")
	}
}

func TestQuestionNotAsked(t *testing.T) {
	d := Definition{Name: "q", Slots: []Slot{{Name: "opts", Block: BlockChoices}}}
	if _, _, ok := d.Question(raw(`{"opts": [{"title": "A"}, {"title": "B"}]}`)); ok {
		t.Fatal("template without question asked one")
	}
	d.Ask = &QuestionSpec{FromSlot: "opts"}
	if _, _, ok := d.Question(raw(`{"opts": [{"title": "A"}]}`)); ok {
		t.Fatal("invalid data asked a question")
	}
	if _, _, ok := d.Question(raw(`{}`)); ok {
		t.Fatal("no choices asked a question")
	}
}
