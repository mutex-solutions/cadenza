package prepare

import (
	"bytes"
	"encoding/binary"
	"os"
	"testing"
)

// This file builds minimal synthetic MP4 files by hand: ftyp, mdat and a
// moov with one trak per track down to stsd and stco/co64.

func u16(v int) []byte    { return binary.BigEndian.AppendUint16(nil, uint16(v)) }
func u32(v uint32) []byte { return binary.BigEndian.AppendUint32(nil, v) }
func cat(parts ...[]byte) []byte {
	return bytes.Join(parts, nil)
}

// mkbox encodes a box with a 32-bit size.
func mkbox(typ string, payload ...[]byte) []byte {
	p := cat(payload...)
	return cat(u32(uint32(8+len(p))), []byte(typ), p)
}

// mkbox64 encodes a box with a 64-bit size header.
func mkbox64(typ string, payload []byte) []byte {
	return cat(u32(1), []byte(typ), binary.BigEndian.AppendUint64(nil, uint64(16+len(payload))), payload)
}

// full prefixes version and flags.
func full(version byte, payload ...[]byte) []byte {
	return cat([]byte{version, 0, 0, 0}, cat(payload...))
}

// track describes one synthetic trak.
type track struct {
	handler string // "vide" or "soun"
	codec   string // sample entry type
	w, h    int
	offsets []uint64 // absolute chunk offsets
	co64    bool
	noStsd  bool
}

func (tr track) encode() []byte {
	tkhd := mkbox("tkhd", full(0, make([]byte, 72), u32(uint32(tr.w)<<16), u32(uint32(tr.h)<<16)))
	hdlr := mkbox("hdlr", full(0, u32(0), []byte(tr.handler), make([]byte, 12), []byte{0}))
	entry := mkbox(tr.codec, make([]byte, 6), u16(1), make([]byte, 16), u16(tr.w), u16(tr.h), make([]byte, 50))
	var stbl [][]byte
	if !tr.noStsd {
		stbl = append(stbl, mkbox("stsd", full(0, u32(1), entry)))
	}
	var offs []byte
	for _, o := range tr.offsets {
		if tr.co64 {
			offs = binary.BigEndian.AppendUint64(offs, o)
		} else {
			offs = binary.BigEndian.AppendUint32(offs, uint32(o))
		}
	}
	table := "stco"
	if tr.co64 {
		table = "co64"
	}
	stbl = append(stbl, mkbox("stsz", full(0, u32(0), u32(0))), mkbox(table, full(0, u32(uint32(len(tr.offsets))), offs)))
	return mkbox("trak", tkhd, mkbox("mdia", mkbox("mdhd", full(0, make([]byte, 20))), hdlr,
		mkbox("minf", mkbox("vmhd", full(0, make([]byte, 8))), mkbox("stbl", stbl...))))
}

func mkmoov(tracks ...track) []byte {
	parts := [][]byte{mkbox("mvhd", full(0, make([]byte, 96)))}
	for _, tr := range tracks {
		parts = append(parts, tr.encode())
	}
	return mkbox("moov", parts...)
}

var ftyp = mkbox("ftyp", []byte("isom"), u32(512), []byte("isomavc1"))

const chunk = 16

// payload returns n chunks with distinct bytes.
func payload(n int) []byte {
	b := make([]byte, n*chunk)
	for i := range b {
		b[i] = byte(i*7 + i/chunk)
	}
	return b
}

// mp4Opts controls the synthetic file layout.
type mp4Opts struct {
	codec     string
	w, h      int
	moovFirst bool
	co64      bool
	mdat64    bool // 64-bit mdat header
	tailMdat  bool // a second mdat after moov holding the audio chunks
	noVideo   bool
}

// buildMP4 returns a file with a video track (3 chunks) and an audio track
// (2 chunks), and the chunk offsets in file order (video then audio).
func buildMP4(o mp4Opts) ([]byte, []uint64) {
	if o.codec == "" {
		o.codec = "avc1"
	}
	if o.w == 0 {
		o.w, o.h = 1280, 720
	}
	data := payload(5)
	mdatHdr := 8
	if o.mdat64 {
		mdatHdr = 16
	}
	mkmdat := func(p []byte) []byte {
		if o.mdat64 {
			return mkbox64("mdat", p)
		}
		return mkbox("mdat", p)
	}
	// moov size does not depend on the offset values, so build once to size it.
	layout := func(moovLen int) (vid, aud []uint64) {
		base := len(ftyp)
		if o.moovFirst {
			base += moovLen
		}
		start := uint64(base + mdatHdr)
		for i := range 3 {
			vid = append(vid, start+uint64(i*chunk))
		}
		if o.tailMdat {
			tail := uint64(len(ftyp) + mdatHdr + 3*chunk + moovLen + mdatHdr)
			aud = []uint64{tail, tail + chunk}
		} else {
			aud = []uint64{start + 3*chunk, start + 4*chunk}
		}
		return
	}
	mk := func(vid, aud []uint64) []byte {
		v := track{handler: "vide", codec: o.codec, w: o.w, h: o.h, offsets: vid, co64: o.co64}
		a := track{handler: "soun", codec: "mp4a", offsets: aud, co64: o.co64}
		if o.noVideo {
			v.handler = "soun"
		}
		return mkmoov(v, a)
	}
	vid, aud := layout(len(mk(layout(0))))
	moov := mk(vid, aud)
	var file []byte
	switch {
	case o.moovFirst:
		file = cat(ftyp, moov, mkmdat(data))
	case o.tailMdat:
		file = cat(ftyp, mkmdat(data[:3*chunk]), moov, mkmdat(data[3*chunk:]))
	default:
		file = cat(ftyp, mkmdat(data), moov)
	}
	return file, append(vid, aud...)
}

// offsetsIn returns all chunk offsets in file order, and whether each
// table was co64.
func offsetsIn(t *testing.T, file []byte) ([]uint64, []string) {
	t.Helper()
	r := bytes.NewReader(file)
	boxes, err := readBoxes(r, 0, int64(len(file)))
	if err != nil {
		t.Fatal(err)
	}
	moov, _, err := readMoov(r, boxes)
	if err != nil {
		t.Fatal(err)
	}
	var all []uint64
	var kinds []string
	moov.walk(func(n *node) {
		if n.typ == "stco" || n.typ == "co64" {
			o, err := chunkOffsets(n)
			if err != nil {
				t.Fatal(err)
			}
			all = append(all, o...)
			kinds = append(kinds, n.typ)
		}
	})
	return all, kinds
}

// chunksAt returns the chunk bytes at each offset.
func chunksAt(file []byte, offs []uint64) [][]byte {
	out := make([][]byte, len(offs))
	for i, o := range offs {
		out[i] = file[o : o+chunk]
	}
	return out
}

func TestSyntheticMP4Sane(t *testing.T) {
	// The builder's own offsets must point at the payload chunks in order.
	for _, o := range []mp4Opts{{}, {moovFirst: true}, {tailMdat: true}, {mdat64: true}, {co64: true}} {
		file, offs := buildMP4(o)
		got, _ := offsetsIn(t, file)
		data := payload(5)
		for i, c := range chunksAt(file, got) {
			if got[i] != offs[i] || !bytes.Equal(c, data[i*chunk:(i+1)*chunk]) {
				t.Fatalf("%+v: chunk %d wrong", o, i)
			}
		}
	}
}

func writeMP4(t *testing.T, name string, file []byte) string {
	t.Helper()
	return writeFile(t, name, file)
}

func readFileT(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
