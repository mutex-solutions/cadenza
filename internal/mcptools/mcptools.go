// Package mcptools defines Cadenza's MCP tools once, for both places they are
// served: `cadenza mcp` on an agent's machine (local stdio; file paths are
// read and uploaded by the CLI) and /mcp on the server (for cloud agents with
// nothing installed; files are uploaded by signed link first). Names and
// descriptions come from internal/spec.
package mcptools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mutex-solutions/cadenza/internal/media"
	"github.com/mutex-solutions/cadenza/internal/spec"
)

// MaxWait caps one wait call: Codex and Cursor end MCP tool calls after about
// 60 s (plan C1). The agent calls wait again to keep waiting.
const MaxWait = 45 * time.Second

// ShowArgs is the show tool's input.
type ShowArgs struct {
	Title       string          `json:"title" jsonschema:"thread title when starting one; also the post title"`
	Summary     string          `json:"summary,omitempty" jsonschema:"one or two sentences, markdown"`
	Template    string          `json:"template,omitempty" jsonschema:"built-in or custom template name, e.g. ui-change; see the templates tool"`
	Data        map[string]any  `json:"data,omitempty" jsonschema:"the template's slots as a JSON object"`
	Files       []string        `json:"files,omitempty" jsonschema:"local file paths to show as attachments (local server only)"`
	FileRefs    []media.FileRef `json:"file_refs,omitempty" jsonschema:"already uploaded files to show (see the upload tool)"`
	ThreadID    string          `json:"thread_id,omitempty" jsonschema:"continue this thread"`
	NewThread   bool            `json:"new_thread,omitempty" jsonschema:"start a new thread"`
	NotifyTitle string          `json:"notify_title,omitempty" jsonschema:"short title for the notification"`
}

// AskArgs is the ask tool's input: spec.AskInput without the retry id, which
// is derived from the arguments.
type AskArgs struct {
	ThreadID    string   `json:"thread_id,omitempty" jsonschema:"thread to ask in; defaults to your latest open thread"`
	PostID      string   `json:"post_id,omitempty" jsonschema:"the post the question is about"`
	Text        string   `json:"text" jsonschema:"the question, markdown"`
	Choices     []string `json:"choices,omitempty" jsonschema:"optional answers to pick from"`
	Waiting     bool     `json:"waiting,omitempty" jsonschema:"you are blocked on this answer"`
	Replaces    string   `json:"replaces,omitempty" jsonschema:"id of an older open question this one replaces"`
	NotifyTitle string   `json:"notify_title,omitempty"`
	NewThread   bool     `json:"new_thread,omitempty"`
	Title       string   `json:"title,omitempty" jsonschema:"thread title if a new thread is started"`
}

// Input converts the arguments to the API input with a derived retry id.
func (a AskArgs) Input() spec.AskInput {
	return spec.AskInput{ThreadID: a.ThreadID, PostID: a.PostID, Text: a.Text, Choices: a.Choices, Waiting: a.Waiting,
		Replaces: a.Replaces, Notify: a.NotifyTitle, NewThread: a.NewThread, Title: a.Title, RetryID: RetryID(a)}
}

// MakeTemplateArgs is the make-template tool's input.
type MakeTemplateArgs struct {
	Definition map[string]any `json:"definition" jsonschema:"template definition: name, use_for, guide, slots built from blocks, example"`
}

// WaitArgs is the wait tool's input.
type WaitArgs struct {
	QuestionID string `json:"question_id" jsonschema:"the id returned by ask"`
	Seconds    int    `json:"seconds,omitempty" jsonschema:"how long to wait in this call, at most 45; call again to keep waiting"`
}

// ReplyArgs is the reply tool's input.
type ReplyArgs struct {
	ReplyID string `json:"reply_id" jsonschema:"the comment's id from responses"`
	Text    string `json:"text" jsonschema:"your answer, e.g. fixed in round 2"`
}

// DoneArgs is the done tool's input.
type DoneArgs struct {
	ThreadID string `json:"thread_id,omitempty" jsonschema:"defaults to your latest open thread"`
	Summary  string `json:"summary,omitempty"`
}

// TemplatesArgs is the templates tool's input.
type TemplatesArgs struct {
	Name string `json:"name,omitempty" jsonschema:"show one template's slots, guide and example; leave out to list all"`
}

// Empty is the input of tools that take none.
type Empty struct{}

// Backend runs the tools. Results are returned to the agent as JSON.
type Backend interface {
	Show(ctx context.Context, in ShowArgs) (any, error)
	Ask(ctx context.Context, in spec.AskInput) (any, error)
	Wait(ctx context.Context, questionID string, d time.Duration) (any, error)
	// Responses returns new replies and acknowledges them once they are
	// safely delivered (files saved, for the local server).
	Responses(ctx context.Context) (any, error)
	Reply(ctx context.Context, replyID, text string) (any, error)
	Done(ctx context.Context, threadID, summary string) (any, error)
	Threads(ctx context.Context) (any, error)
	Templates(ctx context.Context, name string) (any, error)
	MakeTemplate(ctx context.Context, def json.RawMessage) (any, error)
	// Upload is offered on the remote server only: it returns signed upload
	// links for files the agent then PUTs itself.
	Upload(ctx context.Context, files []media.FileRef) (any, error)
}

// Instructions is sent to MCP clients on connect.
const Instructions = "Cadenza lets you show your work to your human and ask them questions; they answer from their phone. " +
	"Prefer templates (see the templates tool). ask returns at once: keep working, then call wait or responses later."

// Register adds the agent tools to s. Remote servers also get the upload tool.
func Register(s *mcp.Server, b Backend, remote bool) {
	desc := func(name, extra string) string {
		e, ok := spec.Lookup(name)
		if !ok {
			panic("mcptools: unknown endpoint " + name)
		}
		return e.Summary + extra
	}
	add(s, "show", desc("show", " Pass a template with data, or files."), func(ctx context.Context, in ShowArgs) (any, error) {
		return b.Show(ctx, in)
	})
	add(s, "ask", desc("ask", ""), func(ctx context.Context, in AskArgs) (any, error) {
		return b.Ask(ctx, in.Input())
	})
	add(s, "wait", desc("wait", " Each call waits at most 45 s."), func(ctx context.Context, in WaitArgs) (any, error) {
		d := time.Duration(in.Seconds) * time.Second
		if d <= 0 || d > MaxWait {
			d = MaxWait
		}
		return b.Wait(ctx, in.QuestionID, d)
	})
	add(s, "responses", desc("responses", ""), func(ctx context.Context, _ Empty) (any, error) { return b.Responses(ctx) })
	add(s, "reply", desc("reply", ""), func(ctx context.Context, in ReplyArgs) (any, error) { return b.Reply(ctx, in.ReplyID, in.Text) })
	add(s, "done", desc("done", ""), func(ctx context.Context, in DoneArgs) (any, error) { return b.Done(ctx, in.ThreadID, in.Summary) })
	add(s, "threads", desc("threads", ""), func(ctx context.Context, _ Empty) (any, error) { return b.Threads(ctx) })
	add(s, "templates", desc("templates", " With a name: "+desc("template", "")), func(ctx context.Context, in TemplatesArgs) (any, error) {
		return b.Templates(ctx, in.Name)
	})
	add(s, "make-template", desc("make-template", ""), func(ctx context.Context, in MakeTemplateArgs) (any, error) {
		def, err := json.Marshal(in.Definition)
		if err != nil {
			return nil, err
		}
		return b.MakeTemplate(ctx, def)
	})
	if remote {
		add(s, "upload", desc("upload", " PUT each file's bytes to its url with exactly the given headers, then pass the refs to show as file_refs or inside template data as {\"file\": <ref>}."),
			func(ctx context.Context, in spec.UploadRequest) (any, error) { return b.Upload(ctx, in.Files) })
	}
}

func add[In any](s *mcp.Server, name, description string, fn func(context.Context, In) (any, error)) {
	mcp.AddTool(s, &mcp.Tool{Name: name, Description: description}, func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
		out, err := fn(ctx, in)
		return Result(out, err), nil, nil
	})
}

// Result turns a value or error into a tool result with JSON text.
func Result(v any, err error) *mcp.CallToolResult {
	if err != nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}
	}
	b, merr := json.Marshal(v)
	if merr != nil {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: merr.Error()}}}
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}
}

// RetryID derives a retry id from a call's arguments, so a repeated identical
// call never makes a duplicate.
func RetryID(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return "auto-" + hex.EncodeToString(h[:12])
}

// DataJSON returns the template data as JSON, or nil when there is none.
func (a ShowArgs) DataJSON() json.RawMessage {
	if a.Data == nil {
		return nil
	}
	b, _ := json.Marshal(a.Data)
	return b
}
