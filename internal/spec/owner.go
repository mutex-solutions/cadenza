package spec

import (
	"encoding/json"

	"github.com/mutex-solutions/cadenza/internal/media"
)

// ThreadView is everything the apps need to show one thread.
type ThreadView struct {
	Thread    ThreadSummary  `json:"thread"`
	Posts     []PostView     `json:"posts"`
	Questions []QuestionView `json:"questions"`
	Replies   []ReplyView    `json:"replies"`
}

type PostView struct {
	ID          string           `json:"id"`
	Seq         int              `json:"seq"`
	Title       string           `json:"title"`
	SummaryHTML string           `json:"summary_html,omitempty"`
	Attachments []AttachmentView `json:"attachments"`
	CreatedAt   int64            `json:"created_at"`
}

// AttachmentView is a stored attachment. For templates, Data is the filled
// data with markdown slots already rendered to sanitized HTML.
type AttachmentView struct {
	Index    int             `json:"index"`
	Template string          `json:"template,omitempty"`
	Version  int             `json:"version,omitempty"`
	Data     json.RawMessage `json:"data,omitempty"`
	File     *media.FileRef  `json:"file,omitempty"`
}

type QuestionView struct {
	ID         string   `json:"id"`
	PostID     string   `json:"post_id,omitempty"`
	TextHTML   string   `json:"text_html"`
	Choices    []string `json:"choices,omitempty"`
	Waiting    bool     `json:"waiting"`
	Status     string   `json:"status"`
	ReplacedBy string   `json:"replaced_by,omitempty"`
	CreatedAt  int64    `json:"created_at"`
}

type ReplyView struct {
	Response
	AgentReply   string `json:"agent_reply,omitempty"`
	AgentReplyAt int64  `json:"agent_reply_at,omitempty"`
	Delivered    bool   `json:"delivered"`
}

// AnswerInput answers a question.
type AnswerInput struct {
	Choice string          `json:"choice,omitempty"`
	Text   string          `json:"text,omitempty"`
	Files  []media.FileRef `json:"files,omitempty"`
}

// ReplyInput is a note, verdict, pinned comment or sketch on a post.
type ReplyInput struct {
	PostID  string          `json:"post_id"`
	Kind    string          `json:"kind"`
	Text    string          `json:"text,omitempty"`
	Verdict string          `json:"verdict,omitempty"`
	Spot    *Spot           `json:"spot,omitempty"`
	Files   []media.FileRef `json:"files,omitempty"`
	Extra   json.RawMessage `json:"extra,omitempty"` // sketch: text labels
}

type ReplyOutput struct {
	ReplyID string `json:"reply"`
}

type ThreadStatusInput struct {
	Status string `json:"status"` // approved, archived, active
}

// Event is one numbered change (plan D15).
type Event struct {
	Seq      int64  `json:"seq"`
	Kind     string `json:"kind"`
	ThreadID string `json:"thread,omitempty"`
	RefID    string `json:"ref,omitempty"`
	At       int64  `json:"at"`
}

type EventList struct {
	Events []Event `json:"events"`
	// Reset is true when the caller is too far behind: reload everything.
	Reset bool  `json:"reset,omitempty"`
	Last  int64 `json:"last"`
}

// Event kinds.
const (
	EventThreadCreated   = "thread.created"
	EventThreadStatus    = "thread.status"
	EventThreadReady     = "thread.ready" // the agent finished (cadenza done)
	EventPostAdded       = "post.added"
	EventQuestionAsked   = "question.asked"
	EventQuestionChanged = "question.changed"
	EventReplyAdded      = "reply.added"
	EventReplyDelivered  = "reply.delivered"
	EventAgentReplied    = "reply.agent_replied"
	EventMachineChanged  = "machine.changed"
)

// Device is a phone or browser that receives notifications.
type DeviceInput struct {
	Provider string          `json:"provider"` // webpush, expo, fcm
	Token    json.RawMessage `json:"token"`    // provider specific: web push subscription JSON, Expo token string, FCM token
	Name     string          `json:"name"`
}

type DeviceOutput struct {
	DeviceID string `json:"device_id"`
}

// Device is a registered phone or browser, without its token.
type Device struct {
	ID        string `json:"id"`
	Provider  string `json:"provider"`
	Name      string `json:"name"`
	CreatedAt int64  `json:"created_at"`
	LastOKAt  int64  `json:"last_ok_at"`
}

type DeviceList struct {
	Devices []Device `json:"devices"`
}

type PushConfig struct {
	Providers      []string `json:"providers"`
	VAPIDPublicKey string   `json:"vapid_public_key,omitempty"`
}

type NotificationLog struct {
	Items []NotificationEntry `json:"items"`
}

type NotificationEntry struct {
	ID        string `json:"id"`
	Device    string `json:"device"`
	Provider  string `json:"provider"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	Status    string `json:"status"` // sent, failed
	Error     string `json:"error,omitempty"`
	CreatedAt int64  `json:"created_at"`
}
