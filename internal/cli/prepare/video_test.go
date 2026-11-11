package prepare

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mutex-solutions/cadenza/internal/media"
)

func TestMP4FastStartThroughPrepare(t *testing.T) {
	file, _ := buildMP4(mp4Opts{})
	path := writeMP4(t, "demo.mp4", file)
	tmp := t.TempDir()
	p, err := PrepareFile(context.Background(), path, Options{TempDir: tmp})
	if err != nil {
		t.Fatal(err)
	}
	if p.Path == path || !strings.HasPrefix(p.Path, tmp) || len(p.Warnings) != 1 {
		t.Fatalf("expected a rewritten temp file, got %+v", p)
	}
	if p.Source != media.SourceMP4 || p.File.Type != media.TypeMP4 || p.File.Name != "demo.mp4" {
		t.Fatalf("got %+v", p.File)
	}
	runFastStart(t, file) // same rewrite, verified chunk by chunk
	checkRef(t, p)
	if err := p.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p.Path); !os.IsNotExist(err) {
		t.Fatal("temp file not removed")
	}
	if err := p.Cleanup(); err != nil {
		t.Fatal("second cleanup failed")
	}
}

func TestMP4AlreadyFastKept(t *testing.T) {
	file, _ := buildMP4(mp4Opts{moovFirst: true})
	path := writeMP4(t, "demo.mp4", file)
	p, err := PrepareFile(context.Background(), path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Path != path || len(p.Warnings) != 0 {
		t.Fatalf("fast-start file was rewritten: %+v", p)
	}
	checkRef(t, p)
}

func TestMP4NeedsConversion(t *testing.T) {
	cases := map[string]mp4Opts{
		"hevc":    {codec: "hvc1"},
		"vp9":     {codec: "vp09"},
		"4k avc1": {w: 3840, h: 2160},
	}
	for name, o := range cases {
		file, _ := buildMP4(o)
		dir := t.TempDir()
		path := filepath.Join(dir, "clip.mp4")
		if err := os.WriteFile(path, file, 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := PrepareFile(context.Background(), path, Options{})
		e := wantInvalid(t, err)
		want := `ffmpeg -i ` + path + ` -c:v libx264 -preset veryfast -crf 23 -vf "scale=-2:'min(1080,ih)'" -movflags +faststart -c:a aac ` +
			filepath.Join(dir, "clip-h264.mp4")
		if e.Fix != want {
			t.Errorf("%s: fix\n got %s\nwant %s", name, e.Fix, want)
		}
	}
}

func TestAVC3Accepted(t *testing.T) {
	file, _ := buildMP4(mp4Opts{codec: "avc3", moovFirst: true, w: 1920, h: 1080})
	if _, err := PrepareFile(context.Background(), writeMP4(t, "a.mp4", file), Options{}); err != nil {
		t.Fatal(err)
	}
}

func TestOtherVideoGivesFFmpegFix(t *testing.T) {
	mov := cat(mkbox("ftyp", []byte("qt  "), u32(0)), mkbox("mdat", payload(1)))
	dir := t.TempDir()
	path := filepath.Join(dir, "screen rec.mov")
	if err := os.WriteFile(path, mov, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := PrepareFile(context.Background(), path, Options{})
	e := wantInvalid(t, err)
	if !strings.HasPrefix(e.Fix, "ffmpeg -i '"+path+"' ") || !strings.HasSuffix(e.Fix, " '"+filepath.Join(dir, "screen rec.mp4")+"'") {
		t.Fatalf("fix = %s", e.Fix)
	}
}

func TestMP4Refusals(t *testing.T) {
	noVideo, _ := buildMP4(mp4Opts{noVideo: true, moovFirst: true})
	noStsd := cat(ftyp, mkmoov(track{handler: "vide", codec: "avc1", noStsd: true}), mkbox("mdat", payload(1)))
	garbage := cat(ftyp, u32(999), []byte("mdat"))
	noMoov := cat(ftyp, mkbox("mdat", payload(1)))
	for name, f := range map[string][]byte{"no video": noVideo, "no stsd": noStsd, "garbage": garbage, "no moov": noMoov} {
		_, err := PrepareFile(context.Background(), writeMP4(t, "a.mp4", f), Options{})
		if e := wantInvalid(t, err); e.Fix == "" {
			t.Errorf("%s: missing fix", name)
		}
	}
	file, _ := buildMP4(mp4Opts{moovFirst: true})
	_, err := PrepareFile(context.Background(), writeMP4(t, "a.mp4", file), Options{Limits: Limits{VideoBytes: 10}})
	if e := wantInvalid(t, err); !strings.Contains(e.Fix, "-t 120") {
		t.Fatalf("size fix = %q", e.Fix)
	}
}

func TestTkhdFallback(t *testing.T) {
	// A sample entry too short to hold a size falls back to tkhd.
	trak := mkbox("trak",
		mkbox("tkhd", full(0, make([]byte, 72), u32(640<<16), u32(2000<<16))),
		mkbox("mdia", mkbox("hdlr", full(0, u32(0), []byte("vide"))),
			mkbox("minf", mkbox("stbl", mkbox("stsd", full(0, u32(1), mkbox("avc1")))))))
	moov, err := parseNode("moov", trak)
	if err != nil {
		t.Fatal(err)
	}
	v, err := probeMP4(moov)
	if err != nil || v.codec != "avc1" || v.height != 2000 || v.width != 640 {
		t.Fatalf("got %+v %v", v, err)
	}
	if w, h := tkhdSize(nil); w != 0 || h != 0 {
		t.Fatal("nil tkhd")
	}
	v1 := &node{typ: "tkhd", data: full(1, make([]byte, 84), u32(10<<16), u32(20<<16))}
	if w, h := tkhdSize(v1); w != 10 || h != 20 {
		t.Fatalf("v1 tkhd %dx%d", w, h)
	}
	if w, h := tkhdSize(&node{data: []byte{0, 0}}); w != 0 || h != 0 {
		t.Fatal("short tkhd")
	}
}

// fakeFFmpeg writes out (the last argument) as the given MP4 and records args.
func fakeFFmpeg(out []byte, gotArgs *[]string) Runner {
	return func(_ context.Context, name string, args []string) error {
		*gotArgs = append([]string{name}, args...)
		return os.WriteFile(args[len(args)-1], out, 0o600)
	}
}

func TestFFmpegConversion(t *testing.T) {
	converted, _ := buildMP4(mp4Opts{}) // moov after mdat: also exercises fast-start
	mov := cat(mkbox("ftyp", []byte("qt  "), u32(0)), mkbox("mdat", payload(1)))
	path := writeFile(t, "rec.mov", mov)
	tmp := t.TempDir()
	var args []string
	p, err := PrepareFile(context.Background(), path, Options{TempDir: tmp, AllowFFmpeg: true, FFmpeg: fakeFFmpeg(converted, &args)})
	if err != nil {
		t.Fatal(err)
	}
	if args[0] != "ffmpeg" || args[2] != "-i" || args[3] != path || !strings.HasSuffix(args[len(args)-1], "rec.mp4") ||
		strings.Join(args[4:len(args)-1], " ") != "-c:v libx264 -preset veryfast -crf 23 -vf scale=-2:'min(1080,ih)' -movflags +faststart -c:a aac" {
		t.Fatalf("args %q", args)
	}
	if p.File.Name != "rec.mp4" || !strings.HasPrefix(p.Path, tmp) || len(p.Warnings) != 2 {
		t.Fatalf("got %+v", p)
	}
	checkRef(t, p)
	if err := p.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if left, _ := os.ReadDir(tmp); len(left) != 0 {
		t.Fatalf("temp left behind: %v", left)
	}

	// Already fast-start output is used as written.
	fast, _ := buildMP4(mp4Opts{moovFirst: true})
	p, err = PrepareFile(context.Background(), path, Options{TempDir: tmp, AllowFFmpeg: true, FFmpeg: fakeFFmpeg(fast, &args)})
	if err != nil {
		t.Fatal(err)
	}
	checkRef(t, p)
	_ = p.Cleanup()
}

func TestFFmpegFailures(t *testing.T) {
	file, _ := buildMP4(mp4Opts{codec: "hvc1"})
	path := writeMP4(t, "clip.mp4", file)
	tmp := t.TempDir()
	var args []string
	boom := errors.New("boom")
	cases := map[string]Runner{
		"missing":  func(context.Context, string, []string) error { return errNoFFmpeg },
		"fails":    func(context.Context, string, []string) error { return boom },
		"no out":   func(context.Context, string, []string) error { return nil },
		"still hv": fakeFFmpeg(file, &args),
	}
	for name, run := range cases {
		_, err := PrepareFile(context.Background(), path, Options{TempDir: tmp, AllowFFmpeg: true, FFmpeg: run})
		if err == nil {
			t.Fatalf("%s: want error", name)
		}
		if name == "fails" && !errors.Is(err, boom) {
			t.Errorf("fails: cause not wrapped: %v", err)
		}
		if name == "missing" && !strings.Contains(err.Error(), "not installed") {
			t.Errorf("missing: %v", err)
		}
	}
	if left, _ := os.ReadDir(tmp); len(left) != 0 {
		t.Fatalf("temp left behind: %v", left)
	}
}

func TestRunFFmpegDefault(t *testing.T) {
	ctx := context.Background()
	if err := runFFmpeg(ctx, "cadenza-no-such-binary", nil); !errors.Is(err, errNoFFmpeg) {
		t.Fatalf("want errNoFFmpeg, got %v", err)
	}
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("no /bin/sh")
	}
	if err := runFFmpeg(ctx, "sh", []string{"-c", "exit 0"}); err != nil {
		t.Fatal(err)
	}
	err := runFFmpeg(ctx, "sh", []string{"-c", "echo bad input >&2; printf '%600s' x >&2; exit 3"})
	if err == nil || len(err.Error()) > 600 {
		t.Fatalf("want trimmed error, got %v", err)
	}
}

func TestShellQuoteAndNames(t *testing.T) {
	cases := map[string]string{"a.mp4": "a.mp4", "my clip.mov": "'my clip.mov'", "it's.mov": `'it'\''s.mov'`, "": "''"}
	for in, want := range cases {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %s, want %s", in, got, want)
		}
	}
	if convertedName("/x/a.MP4") != "/x/a-h264.mp4" || convertedName("b.webm") != "b.mp4" {
		t.Fatal("convertedName wrong")
	}
}
