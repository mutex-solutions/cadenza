package spec

import (
	"encoding/json"

	"github.com/mutex-solutions/cadenza/internal/media"
)

// ShowInput posts work. With neither ThreadID nor NewThread, the post goes to
// the session's most recent open thread, or a new thread named Title.
type ShowInput struct {
	ThreadID    string       `json:"thread_id,omitempty" jsonschema:"post to this thread"`
	NewThread   bool         `json:"new_thread,omitempty" jsonschema:"always start a new thread"`
	Title       string       `json:"title" jsonschema:"thread title when starting one; also the post title"`
	Summary     string       `json:"summary,omitempty" jsonschema:"one or two sentences, markdown"`
	Notify      string       `json:"notify_title,omitempty" jsonschema:"short title for the notification; defaults to the thread title"`
	RetryID     string       `json:"retry_id" jsonschema:"repeat the same retry id to retry without creating a duplicate"`
	Attachments []Attachment `json:"attachments" jsonschema:"one or more filled templates or files"`
}

// Attachment is a filled template or a single file.
type Attachment struct {
	Template string          `json:"template,omitempty"`
	Version  int             `json:"version,omitempty"` // 0 = latest
	Data     json.RawMessage `json:"data,omitempty"`
	File     *media.FileRef  `json:"file,omitempty"`
}

type ShowOutput struct {
	ThreadID string `json:"thread"`
	PostID   string `json:"post"`
	URL      string `json:"url"`
	// QuestionID is set when the post also asked a question (the options template).
	QuestionID string `json:"question,omitempty"`
}

// AskInput asks the owner a question.
type AskInput struct {
	ThreadID  string   `json:"thread_id,omitempty" jsonschema:"thread to ask in; defaults to the session's most recent open thread"`
	PostID    string   `json:"post_id,omitempty" jsonschema:"the post the question is about"`
	Text      string   `json:"text" jsonschema:"the question, markdown"`
	Choices   []string `json:"choices,omitempty" jsonschema:"optional answers to pick from"`
	Waiting   bool     `json:"waiting,omitempty" jsonschema:"the agent is blocked on this answer"`
	Replaces  string   `json:"replaces,omitempty" jsonschema:"id of an older open question this one replaces"`
	Notify    string   `json:"notify_title,omitempty"`
	RetryID   string   `json:"retry_id"`
	NewThread bool     `json:"new_thread,omitempty"`
	Title     string   `json:"title,omitempty" jsonschema:"thread title if a new thread is started"`
}

type AskOutput struct {
	QuestionID string `json:"question"`
	ThreadID   string `json:"thread"`
	URL        string `json:"url"`
}

// Question statuses.
const (
	QuestionOpen      = "open"
	QuestionAnswered  = "answered"
	QuestionReplaced  = "replaced"
	QuestionDismissed = "dismissed"
)

// WaitOutput is the state of a question after waiting.
type WaitOutput struct {
	QuestionID string `json:"question"`
	Status     string `json:"status"`
	// Answer is set when Status is answered.
	Answer *Response `json:"answer,omitempty"`
	// ReplacedBy is set when Status is replaced.
	ReplacedBy string `json:"replaced_by,omitempty"`
}

// Exit codes of `cadenza wait`.
const (
	ExitAnswered    = 0
	ExitError       = 1
	ExitStillOpen   = 2
	ExitDismissed   = 3
	ExitReplaced    = 4
	ExitUnreachable = 5
)

// Reply kinds.
const (
	ReplyAnswer  = "answer"
	ReplyNote    = "note"
	ReplyVerdict = "verdict"
	ReplyComment = "comment"
	ReplySketch  = "sketch"
)

// Verdicts.
const (
	VerdictApprove        = "approve"
	VerdictRequestChanges = "request_changes"
)

// Response is one thing the owner sent, as the agent receives it.
type Response struct {
	ID         string          `json:"id"`
	Kind       string          `json:"kind"`
	ThreadID   string          `json:"thread"`
	PostID     string          `json:"post,omitempty"`
	QuestionID string          `json:"question,omitempty"`
	Question   string          `json:"question_text,omitempty"`
	Choice     string          `json:"choice,omitempty"`
	Text       string          `json:"text,omitempty"`
	Verdict    string          `json:"verdict,omitempty"`
	Spot       *Spot           `json:"spot,omitempty"`
	Files      []ResponseFile  `json:"files,omitempty"`
	Extra      json.RawMessage `json:"extra,omitempty"`
	CreatedAt  int64           `json:"created_at"`
}

// ResponseFile is a file the owner attached. The server gives a signed URL;
// the CLI downloads it and sets Path.
type ResponseFile struct {
	media.FileRef
	URL  string `json:"url,omitempty"`
	Path string `json:"path,omitempty"`
}

// Spot is where a pinned comment points.
type Spot struct {
	Attachment int     `json:"attachment"`        // index in the post
	Block      string  `json:"block,omitempty"`   // block or slot name
	Line       int     `json:"line,omitempty"`    // code, diff: 1-based line
	Side       string  `json:"side,omitempty"`    // diff: "old" or "new"
	X          float64 `json:"x,omitempty"`       // image, webpage: 0..1 of width
	Y          float64 `json:"y,omitempty"`       // image, webpage: 0..1 of height
	W          float64 `json:"w,omitempty"`       // optional area width, 0..1
	H          float64 `json:"h,omitempty"`       // optional area height, 0..1
	TimeMS     int64   `json:"time_ms,omitempty"` // video
	Quote      string  `json:"quote,omitempty"`   // the text at that spot
}

// ResponsesOutput is everything new since the last acknowledged check.
type ResponsesOutput struct {
	// NextStep is one sentence saying what to do next.
	NextStep string     `json:"next_step"`
	Items    []Response `json:"items"`
	// Cursor acknowledges these items when passed to responses-ack.
	Cursor int64 `json:"cursor"`
}

type AckInput struct {
	Cursor int64 `json:"cursor"`
}

type AgentReplyInput struct {
	Text string `json:"text" jsonschema:"your answer to the owner's comment, e.g. fixed in round 2"`
}

type DoneInput struct {
	Summary string `json:"summary,omitempty" jsonschema:"one sentence on what was finished"`
}

// ThreadSummary is one row of a thread list.
type ThreadSummary struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	Machine   string `json:"machine"`
	Agent     string `json:"agent"`
	Posts     int    `json:"posts"`
	Open      int    `json:"open_questions"`
	Unread    bool   `json:"unread,omitempty"`
	UpdatedAt int64  `json:"updated_at"`
}

type ThreadList struct {
	Threads []ThreadSummary `json:"threads"`
}

// Thread statuses.
const (
	ThreadActive           = "active"
	ThreadChangesRequested = "changes_requested"
	ThreadReady            = "ready"
	ThreadApproved         = "approved"
	ThreadArchived         = "archived"
)

// TemplateList and TemplateInfo describe templates to agents.
type TemplateList struct {
	Templates []TemplateSummary `json:"templates"`
}

type TemplateSummary struct {
	Name    string `json:"name"`
	Version int    `json:"version"`
	UseFor  string `json:"use_for"`
	Source  string `json:"source"` // builtin, agent, owner
}

// TemplateInfo is the full definition plus guide and example.
type TemplateInfo struct {
	Name       string          `json:"name"`
	Version    int             `json:"version"`
	Source     string          `json:"source"`
	Definition json.RawMessage `json:"definition"`
	Guide      string          `json:"guide"`
	Example    json.RawMessage `json:"example"`
}

// MakeTemplateInput creates a custom template, usable at once.
type MakeTemplateInput struct {
	Definition json.RawMessage `json:"definition" jsonschema:"template definition: name, use_for, guide, slots built from blocks"`
}

type MakeTemplateOutput struct {
	Name    string `json:"name"`
	Version int    `json:"version"`
	Created bool   `json:"created"` // false when this added a version to an existing name
}
