package prepare

import (
	"bytes"
	"errors"
	"testing"
)

// runFastStart rewrites file and checks that moov now precedes every mdat
// and that every chunk offset points at the same bytes as before.
func runFastStart(t *testing.T, file []byte) ([]byte, []string) {
	t.Helper()
	before, _ := offsetsIn(t, file)
	want := chunksAt(file, before)

	var out bytes.Buffer
	if err := fastStart(bytes.NewReader(file), int64(len(file)), &out); err != nil {
		t.Fatal(err)
	}
	res := out.Bytes()
	boxes, err := readBoxes(bytes.NewReader(res), 0, int64(len(res)))
	if err != nil {
		t.Fatal(err)
	}
	if needsFastStart(boxes) {
		t.Fatal("moov still after mdat")
	}
	after, kinds := offsetsIn(t, res)
	if len(after) != len(before) {
		t.Fatalf("%d offsets, want %d", len(after), len(before))
	}
	for i, c := range chunksAt(res, after) {
		if !bytes.Equal(c, want[i]) {
			t.Fatalf("chunk %d moved: offset %d -> %d", i, before[i], after[i])
		}
	}
	return res, kinds
}

func TestFastStartMovesMoov(t *testing.T) {
	file, _ := buildMP4(mp4Opts{})
	boxes, _ := readBoxes(bytes.NewReader(file), 0, int64(len(file)))
	if !needsFastStart(boxes) {
		t.Fatal("moov-after-mdat not detected")
	}
	res, kinds := runFastStart(t, file)
	if len(res) != len(file) || kinds[0] != "stco" {
		t.Fatalf("size %d -> %d, kinds %v", len(file), len(res), kinds)
	}
}

func TestFastStartVariants(t *testing.T) {
	for name, o := range map[string]mp4Opts{
		"tail mdat after moov": {tailMdat: true},
		"64-bit mdat header":   {mdat64: true},
		"co64 input":           {co64: true},
	} {
		t.Run(name, func(t *testing.T) {
			file, _ := buildMP4(o)
			runFastStart(t, file)
		})
	}
}

func TestFastStartUpgradesToCo64(t *testing.T) {
	old := stcoLimit
	t.Cleanup(func() { stcoLimit = old })
	file, _ := buildMP4(mp4Opts{})
	stcoLimit = uint64(len(file)) / 2 // new offsets (past moov) no longer fit
	_, kinds := runFastStart(t, file)
	for _, k := range kinds {
		if k != "co64" {
			t.Fatalf("tables %v, want all co64", kinds)
		}
	}
}

func TestFastStartAlreadyFast(t *testing.T) {
	file, _ := buildMP4(mp4Opts{moovFirst: true})
	boxes, _ := readBoxes(bytes.NewReader(file), 0, int64(len(file)))
	if needsFastStart(boxes) {
		t.Fatal("fast-start file flagged")
	}
	if needsFastStart(nil) {
		t.Fatal("empty list flagged")
	}
}

func TestFastStartErrors(t *testing.T) {
	noMdat := cat(ftyp, mkmoov(track{handler: "vide", codec: "avc1"}))
	badOffset := cat(ftyp, mkbox("mdat", payload(1)), mkmoov(track{handler: "vide", codec: "avc1", offsets: []uint64{1 << 30}}))
	noMoov := cat(ftyp, mkbox("mdat", payload(1)))
	truncated := cat(ftyp, u32(100), []byte("mdat"))
	for name, f := range map[string][]byte{"no mdat": noMdat, "bad offset": badOffset, "no moov": noMoov, "truncated": truncated} {
		var out bytes.Buffer
		if err := fastStart(bytes.NewReader(f), int64(len(f)), &out); !errors.Is(err, errBadMP4) {
			t.Errorf("%s: want errBadMP4, got %v", name, err)
		}
	}
}

func TestReadBoxesErrors(t *testing.T) {
	cases := map[string][]byte{
		"stray bytes":   cat(ftyp, []byte{0, 0, 0}),
		"size too big":  cat(u32(1000), []byte("free")),
		"size too smol": cat(u32(4), []byte("free")),
		"short 64-bit":  cat(u32(1), []byte("mdat")),
	}
	for name, f := range cases {
		if _, err := readBoxes(bytes.NewReader(f), 0, int64(len(f))); !errors.Is(err, errBadMP4) {
			t.Errorf("%s: want errBadMP4, got %v", name, err)
		}
	}
	// size 0 means "to the end of the file".
	f := cat(ftyp, u32(0), []byte("mdat"), payload(1))
	boxes, err := readBoxes(bytes.NewReader(f), 0, int64(len(f)))
	if err != nil || len(boxes) != 2 || boxes[1].size != int64(8+chunk) {
		t.Fatalf("size-0 box: %v %+v", err, boxes)
	}
}

func TestParseNodeErrors(t *testing.T) {
	cases := map[string][]byte{
		"stray":     {0, 0, 0},
		"bad size":  cat(u32(100), []byte("trak")),
		"short 64":  cat(u32(1), []byte("trak")),
		"deep bad":  mkbox("trak", cat(u32(100), []byte("mdia"))),
		"too small": cat(u32(3), []byte("trak")),
	}
	for name, p := range cases {
		if _, err := parseNode("moov", p); !errors.Is(err, errBadMP4) {
			t.Errorf("%s: want errBadMP4, got %v", name, err)
		}
	}
	// 64-bit and size-0 children parse.
	p := cat(mkbox64("free", []byte{1, 2}), u32(0), []byte("skip"), []byte{3})
	n, err := parseNode("moov", p)
	if err != nil || len(n.kids) != 2 || len(n.kids[1].data) != 1 {
		t.Fatalf("got %v %+v", err, n)
	}
	if _, err := chunkOffsets(&node{typ: "stco", data: []byte{0}}); err == nil {
		t.Fatal("short stco accepted")
	}
	if _, err := chunkOffsets(&node{typ: "stco", data: full(0, u32(5))}); err == nil {
		t.Fatal("stco with missing entries accepted")
	}
}
