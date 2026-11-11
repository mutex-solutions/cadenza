package prepare

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// This file reads the ISO base media (MP4) box structure: just enough to
// find the codec and size of the video track and to move moov in front of
// mdat. Only the boxes on the path to tkhd, hdlr, stsd and stco/co64 are
// parsed; every other box is kept as opaque bytes.

var errBadMP4 = errors.New("not a valid MP4 box structure")

// maxMoovBytes caps how much of moov is read into memory. Real moov boxes
// are a few MB even for long videos.
const maxMoovBytes = 256 << 20

// box is one top-level box in a file.
type box struct {
	typ  string
	off  int64 // where the box header starts
	size int64 // header plus payload
	hdr  int64 // header length: 8, or 16 with a 64-bit size
}

// readBoxes lists the boxes in r between start and end.
func readBoxes(r io.ReaderAt, start, end int64) ([]box, error) {
	var out []box
	var h [16]byte
	for off := start; off < end; {
		if end-off < 8 {
			return nil, fmt.Errorf("%w: %d stray bytes at %d", errBadMP4, end-off, off)
		}
		if _, err := r.ReadAt(h[:8], off); err != nil {
			return nil, fmt.Errorf("%w: %v", errBadMP4, err)
		}
		b := box{typ: string(h[4:8]), off: off, size: int64(binary.BigEndian.Uint32(h[:4])), hdr: 8}
		switch b.size {
		case 0: // box runs to the end
			b.size = end - off
		case 1: // 64-bit size follows
			if _, err := r.ReadAt(h[8:16], off+8); err != nil {
				return nil, fmt.Errorf("%w: %v", errBadMP4, err)
			}
			b.size = int64(binary.BigEndian.Uint64(h[8:16]))
			b.hdr = 16
		}
		if b.size < b.hdr || b.size > end-off {
			return nil, fmt.Errorf("%w: box %q at %d has bad size %d", errBadMP4, b.typ, off, b.size)
		}
		out = append(out, b)
		off += b.size
	}
	return out, nil
}

// node is a parsed box inside moov. Containers have kids; leaves keep their
// payload as raw bytes.
type node struct {
	typ  string
	data []byte
	kids []*node
}

// containers are the boxes parse descends into.
var containers = map[string]bool{"moov": true, "trak": true, "mdia": true, "minf": true, "stbl": true}

// parseNode parses a box payload of the given type.
func parseNode(typ string, payload []byte) (*node, error) {
	n := &node{typ: typ}
	if !containers[typ] {
		n.data = payload
		return n, nil
	}
	for p := payload; len(p) > 0; {
		if len(p) < 8 {
			return nil, fmt.Errorf("%w: stray bytes in %s", errBadMP4, typ)
		}
		size := uint64(binary.BigEndian.Uint32(p[:4]))
		kt := string(p[4:8])
		hdr := uint64(8)
		switch size {
		case 0:
			size = uint64(len(p))
		case 1:
			if len(p) < 16 {
				return nil, fmt.Errorf("%w: short header in %s", errBadMP4, typ)
			}
			size, hdr = binary.BigEndian.Uint64(p[8:16]), 16
		}
		if size < hdr || size > uint64(len(p)) {
			return nil, fmt.Errorf("%w: box %q in %s has bad size", errBadMP4, kt, typ)
		}
		kid, err := parseNode(kt, p[hdr:size])
		if err != nil {
			return nil, err
		}
		n.kids = append(n.kids, kid)
		p = p[size:]
	}
	return n, nil
}

// child returns the first direct child of type typ, or nil.
func (n *node) child(typ string) *node {
	for _, k := range n.kids {
		if k.typ == typ {
			return k
		}
	}
	return nil
}

// path follows a chain of child types, returning nil if any is missing.
func (n *node) path(types ...string) *node {
	for _, t := range types {
		if n == nil {
			return nil
		}
		n = n.child(t)
	}
	return n
}

// walk calls fn for n and every box below it.
func (n *node) walk(fn func(*node)) {
	fn(n)
	for _, k := range n.kids {
		k.walk(fn)
	}
}

// size is the encoded length of n, always with a 32-bit size header.
func (n *node) size() int64 {
	if n.kids == nil {
		return 8 + int64(len(n.data))
	}
	s := int64(8)
	for _, k := range n.kids {
		s += k.size()
	}
	return s
}

// encode appends the encoded box to b.
func (n *node) encode(b []byte) []byte {
	b = binary.BigEndian.AppendUint32(b, uint32(n.size()))
	b = append(b, n.typ...)
	if n.kids == nil {
		return append(b, n.data...)
	}
	for _, k := range n.kids {
		b = k.encode(b)
	}
	return b
}

// readMoov finds and parses the moov box among the top-level boxes.
func readMoov(r io.ReaderAt, boxes []box) (*node, box, error) {
	for _, b := range boxes {
		if b.typ != "moov" {
			continue
		}
		if b.size-b.hdr > maxMoovBytes {
			return nil, b, fmt.Errorf("%w: moov is %d bytes", errBadMP4, b.size)
		}
		payload := make([]byte, b.size-b.hdr)
		if _, err := r.ReadAt(payload, b.off+b.hdr); err != nil {
			return nil, b, fmt.Errorf("%w: %v", errBadMP4, err)
		}
		n, err := parseNode("moov", payload)
		return n, b, err
	}
	return nil, box{}, fmt.Errorf("%w: no moov box", errBadMP4)
}

// chunkOffsets returns the entries of an stco or co64 leaf.
func chunkOffsets(n *node) ([]uint64, error) {
	width := 4
	if n.typ == "co64" {
		width = 8
	}
	if len(n.data) < 8 {
		return nil, fmt.Errorf("%w: short %s", errBadMP4, n.typ)
	}
	count := int(binary.BigEndian.Uint32(n.data[4:8]))
	if count < 0 || len(n.data)-8 < count*width {
		return nil, fmt.Errorf("%w: %s entry count %d does not fit", errBadMP4, n.typ, count)
	}
	out := make([]uint64, count)
	for i := range out {
		p := n.data[8+i*width:]
		if width == 4 {
			out[i] = uint64(binary.BigEndian.Uint32(p))
		} else {
			out[i] = binary.BigEndian.Uint64(p)
		}
	}
	return out, nil
}

// setChunkOffsets rewrites n as stco (4-byte entries) or co64 (8-byte
// entries) holding offs. The version and flags are kept.
func setChunkOffsets(n *node, offs []uint64, co64 bool) {
	width := 4
	n.typ = "stco"
	if co64 {
		width, n.typ = 8, "co64"
	}
	data := make([]byte, 8, 8+len(offs)*width)
	copy(data[:4], n.data[:4])
	binary.BigEndian.PutUint32(data[4:8], uint32(len(offs)))
	for _, o := range offs {
		if co64 {
			data = binary.BigEndian.AppendUint64(data, o)
		} else {
			data = binary.BigEndian.AppendUint32(data, uint32(o))
		}
	}
	n.data = data
}
