package prepare

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mutex-solutions/cadenza/internal/kernel"
	"github.com/mutex-solutions/cadenza/internal/media"
)

// videoInfo is what Cadenza checks about an MP4's video track.
type videoInfo struct {
	codec  string // sample entry type, e.g. "avc1", "hvc1"
	width  int
	height int
}

func (v videoInfo) h264() bool { return v.codec == "avc1" || v.codec == "avc3" }

// probeMP4 reads the first video track's codec and size from moov.
func probeMP4(moov *node) (videoInfo, error) {
	for _, trak := range moov.kids {
		if trak.typ != "trak" {
			continue
		}
		hdlr := trak.path("mdia", "hdlr")
		if hdlr == nil || len(hdlr.data) < 12 || string(hdlr.data[8:12]) != "vide" {
			continue
		}
		var v videoInfo
		// stsd: version/flags(4) count(4), then the first sample entry box:
		// size(4) type(4) reserved(6) data_ref(2) pre_defined/reserved(16)
		// width(2) height(2).
		if stsd := trak.path("mdia", "minf", "stbl", "stsd"); stsd != nil && len(stsd.data) >= 16 {
			v.codec = string(stsd.data[12:16])
			if len(stsd.data) >= 44 {
				v.width = int(binary.BigEndian.Uint16(stsd.data[40:42]))
				v.height = int(binary.BigEndian.Uint16(stsd.data[42:44]))
			}
		}
		if v.codec == "" {
			return v, fmt.Errorf("%w: video track has no sample description", errBadMP4)
		}
		if v.height == 0 {
			v.width, v.height = tkhdSize(trak.child("tkhd"))
		}
		return v, nil
	}
	return videoInfo{}, errNoVideo
}

var errNoVideo = errors.New("no video track")

// tkhdSize reads the 16.16 fixed-point display size from a tkhd box.
func tkhdSize(tkhd *node) (w, h int) {
	if tkhd == nil || len(tkhd.data) < 1 {
		return 0, 0
	}
	at := 76 // version 0
	if tkhd.data[0] == 1 {
		at = 88
	}
	if len(tkhd.data) < at+8 {
		return 0, 0
	}
	return int(binary.BigEndian.Uint32(tkhd.data[at:]) >> 16), int(binary.BigEndian.Uint32(tkhd.data[at+4:]) >> 16)
}

// ffmpegArgs is the conversion Cadenza asks for: H.264 at most 1080 pixels
// tall, AAC audio, moov at the front.
func ffmpegArgs(in, out string) []string {
	return []string{"-y", "-i", in, "-c:v", "libx264", "-preset", "veryfast", "-crf", "23",
		"-vf", "scale=-2:'min(1080,ih)'", "-movflags", "+faststart", "-c:a", "aac", out}
}

// ffmpegFix is the exact command shown to the agent.
func ffmpegFix(in, out string) string {
	return fmt.Sprintf(`ffmpeg -i %s -c:v libx264 -preset veryfast -crf 23 -vf "scale=-2:'min(1080,ih)'" -movflags +faststart -c:a aac %s`,
		shellQuote(in), shellQuote(out))
}

// convertedName is where the fix command writes: next to the input, with
// an .mp4 extension (or -h264.mp4 when the input is already .mp4).
func convertedName(in string) string {
	ext := filepath.Ext(in)
	stem := strings.TrimSuffix(in, ext)
	if strings.EqualFold(ext, ".mp4") {
		return stem + "-h264.mp4"
	}
	return stem + ".mp4"
}

// shellQuote quotes s for a POSIX shell when it has special characters.
func shellQuote(s string) string {
	if s != "" && strings.IndexFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_./+=:,@%", r))
	}) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// runFFmpeg runs the real ffmpeg found on PATH.
func runFFmpeg(ctx context.Context, name string, args []string) error {
	bin, err := exec.LookPath(name)
	if err != nil {
		return errNoFFmpeg
	}
	out, err := exec.CommandContext(ctx, bin, args...).CombinedOutput()
	if err != nil {
		tail := string(out)
		if len(tail) > 500 {
			tail = tail[len(tail)-500:]
		}
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(tail))
	}
	return nil
}

var errNoFFmpeg = errors.New("ffmpeg is not on PATH")

// prepareVideo checks an MP4 (H.264, at most 1080 pixels tall, size limit)
// and makes it fast-start. Other videos, and MP4s that fail the codec or
// height check, are converted with ffmpeg when allowed, or refused with the
// exact ffmpeg command.
func prepareVideo(ctx context.Context, path string, src media.Source, size int64, opts Options) (*Prepared, error) {
	if src == media.SourceMP4 {
		p, why, err := checkMP4(path, size, opts)
		if err != nil || why == "" {
			return p, err
		}
		return convertVideo(ctx, path, why, opts)
	}
	return convertVideo(ctx, path, "is not an MP4 file", opts)
}

// mustConvert builds the refusal shown when a video needs ffmpeg.
func mustConvert(path, why string) *kernel.Error {
	return kernel.Invalid("%s %s; Cadenza plays H.264 MP4 up to 1080p", filepath.Base(path), why).
		WithFix("%s", ffmpegFix(path, convertedName(path)))
}

// checkMP4 validates an MP4. When the file is a real MP4 that ffmpeg can
// fix (wrong codec or too tall), it returns no Prepared and why it needs
// converting.
func checkMP4(path string, size int64, opts Options) (p *Prepared, why string, err error) {
	lim := opts.Limits
	f, err := os.Open(path)
	if err != nil {
		return nil, "", kernel.Internal(err, "cannot open %s", path)
	}
	defer f.Close()
	boxes, err := readBoxes(f, 0, size)
	if err != nil {
		return nil, "", kernel.Invalid("%s is not a readable MP4: %v", filepath.Base(path), err).
			WithFix("%s", ffmpegFix(path, convertedName(path)))
	}
	moov, _, err := readMoov(f, boxes)
	if err != nil {
		return nil, "", kernel.Invalid("%s is not a readable MP4: %v", filepath.Base(path), err).
			WithFix("%s", ffmpegFix(path, convertedName(path)))
	}
	info, err := probeMP4(moov)
	if errors.Is(err, errNoVideo) {
		return nil, "", kernel.Invalid("%s has no video track", filepath.Base(path)).
			WithFix("attach audio-only files as plain files, or record a screen video")
	}
	if err != nil {
		return nil, "", kernel.Invalid("%s is not a readable MP4: %v", filepath.Base(path), err).
			WithFix("%s", ffmpegFix(path, convertedName(path)))
	}
	if !info.h264() {
		return nil, fmt.Sprintf("uses %q video, not H.264", info.codec), nil
	}
	if info.height > lim.VideoHeight {
		return nil, fmt.Sprintf("is %d pixels tall", info.height), nil
	}
	if size > lim.VideoBytes {
		return nil, "", tooBig(path, size, lim.VideoBytes,
			"keep a shorter part, e.g. "+strings.Replace(ffmpegFix(path, convertedName(path)), "-crf 23", "-crf 28 -t 120", 1))
	}

	p = &Prepared{Path: path}
	if needsFastStart(boxes) {
		dir, err := os.MkdirTemp(opts.TempDir, "cadenza-prepare-*")
		if err != nil {
			return nil, "", kernel.Internal(err, "cannot make a temp folder")
		}
		p.tempDir = dir
		out := filepath.Join(dir, replaceExt(path, ".mp4"))
		if err := writeFastStart(f, size, out); err != nil {
			_ = p.Cleanup()
			return nil, "", kernel.Invalid("%s could not be made fast-start: %v", filepath.Base(path), err).
				WithFix("%s", ffmpegFix(path, convertedName(path)))
		}
		p.Path = out
		p.Warnings = append(p.Warnings, filepath.Base(path)+": moved the index (moov) to the front so it plays while loading")
	}
	sum, n, err := HashFile(p.Path)
	if err != nil {
		_ = p.Cleanup()
		return nil, "", err
	}
	p.File = media.FileRef{SHA256: sum, Size: n, Type: media.TypeMP4, Name: replaceExt(path, ".mp4")}
	if err := p.File.Validate(); err != nil {
		_ = p.Cleanup()
		return nil, "", err
	}
	return p, "", nil
}

func writeFastStart(src *os.File, size int64, out string) error {
	w, err := os.Create(out)
	if err != nil {
		return err
	}
	if err := fastStart(src, size, w); err != nil {
		w.Close()
		return err
	}
	return w.Close()
}

// convertVideo runs ffmpeg when allowed and available, then checks the
// result like any MP4. Otherwise it returns the refusal with the command.
func convertVideo(ctx context.Context, path, why string, opts Options) (*Prepared, error) {
	refusal := mustConvert(path, why)
	if !opts.AllowFFmpeg {
		return nil, refusal
	}
	run := opts.FFmpeg
	if run == nil {
		run = runFFmpeg
	}
	dir, err := os.MkdirTemp(opts.TempDir, "cadenza-prepare-*")
	if err != nil {
		return nil, kernel.Internal(err, "cannot make a temp folder")
	}
	out := filepath.Join(dir, replaceExt(path, ".mp4"))
	if err := run(ctx, "ffmpeg", ffmpegArgs(path, out)); err != nil {
		_ = os.RemoveAll(dir)
		e := *refusal
		if errors.Is(err, errNoFFmpeg) {
			e.Msg += " (ffmpeg is not installed, so it could not be converted here)"
		} else {
			e.Msg += " (ffmpeg failed)"
			e.Err = err
		}
		return nil, &e
	}
	st, err := os.Stat(out)
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, kernel.Internal(err, "ffmpeg wrote no output for %s", path)
	}
	p, why, err := checkMP4(out, st.Size(), opts)
	if err == nil && why != "" {
		err = kernel.Invalid("ffmpeg output for %s still %s", filepath.Base(path), why).
			WithFix("%s", ffmpegFix(path, convertedName(path)))
	}
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	if p.tempDir != "" { // fast-start made a second temp folder
		inner := p.tempDir
		p.tempDir = dir
		defer os.RemoveAll(inner)
		moved := filepath.Join(dir, "faststart-"+filepath.Base(p.Path))
		if err := os.Rename(p.Path, moved); err != nil {
			_ = os.RemoveAll(dir)
			return nil, kernel.Internal(err, "cannot move converted video")
		}
		p.Path = moved
	} else {
		p.tempDir = dir
	}
	p.Warnings = append(p.Warnings, filepath.Base(path)+": converted to H.264 MP4 with ffmpeg")
	return p, nil
}
