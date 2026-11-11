package prepare

import (
	"fmt"
	"io"
	"math"
)

// stcoLimit is the largest offset an stco entry holds. It is a variable so
// tests can force the co64 upgrade without a 4 GB file.
var stcoLimit uint64 = math.MaxUint32

// needsFastStart reports whether moov comes after the first mdat, which
// makes browsers download the whole file before playing.
func needsFastStart(boxes []box) bool {
	for _, b := range boxes {
		switch b.typ {
		case "moov":
			return false
		case "mdat":
			return true
		}
	}
	return false
}

// fastStart writes the file in r to w with moov moved in front of the first
// mdat, and every chunk offset in stco/co64 changed to point at the same
// bytes in the new layout. If an offset no longer fits in stco, all stco
// boxes become co64. The other boxes are copied unchanged and in order.
func fastStart(r io.ReaderAt, size int64, w io.Writer) error {
	boxes, err := readBoxes(r, 0, size)
	if err != nil {
		return err
	}
	moov, moovBox, err := readMoov(r, boxes)
	if err != nil {
		return err
	}
	firstMdat := -1
	for i, b := range boxes {
		if b.typ == "mdat" {
			firstMdat = i
			break
		}
	}
	if firstMdat < 0 {
		return fmt.Errorf("%w: no mdat box", errBadMP4)
	}

	// Output order: boxes before the first mdat, moov, then the rest.
	var order []box
	order = append(order, boxes[:firstMdat]...)
	order = append(order, moovBox)
	for _, b := range boxes[firstMdat:] {
		if b.typ != "moov" {
			order = append(order, b)
		}
	}

	var tables []*node
	moov.walk(func(n *node) {
		if n.typ == "stco" || n.typ == "co64" {
			tables = append(tables, n)
		}
	})
	orig := make([][]uint64, len(tables))
	for i, t := range tables {
		if orig[i], err = chunkOffsets(t); err != nil {
			return err
		}
	}

	// Rewrite offsets for the current moov size. If an stco overflows,
	// upgrade every stco to co64 (which grows moov) and do it once more.
	for pass := 0; ; pass++ {
		newOff := layout(order, moovBox, moov.size())
		overflow := false
		for i, t := range tables {
			moved := make([]uint64, len(orig[i]))
			for j, o := range orig[i] {
				if moved[j], err = remap(o, boxes, moovBox, newOff); err != nil {
					return err
				}
				if t.typ == "stco" && moved[j] > stcoLimit {
					overflow = true
				}
			}
			setChunkOffsets(t, moved, t.typ == "co64")
		}
		if !overflow {
			break
		}
		if pass > 0 {
			return fmt.Errorf("%w: chunk offsets overflow after co64 upgrade", errBadMP4)
		}
		for i, t := range tables {
			setChunkOffsets(t, orig[i], true)
		}
	}

	moovBytes := moov.encode(nil)
	for _, b := range order {
		if b.typ == "moov" && b.off == moovBox.off {
			if _, err := w.Write(moovBytes); err != nil {
				return err
			}
			continue
		}
		if _, err := io.Copy(w, io.NewSectionReader(r, b.off, b.size)); err != nil {
			return err
		}
	}
	return nil
}

// layout returns the new start offset of every box in order (keyed by its
// old offset), given the new moov size.
func layout(order []box, moovBox box, moovSize int64) map[int64]int64 {
	out := make(map[int64]int64, len(order))
	var pos int64
	for _, b := range order {
		out[b.off] = pos
		if b.off == moovBox.off {
			pos += moovSize
		} else {
			pos += b.size
		}
	}
	return out
}

// remap moves one chunk offset from the old layout to the new one. The
// offset must point inside a box other than moov.
func remap(o uint64, boxes []box, moovBox box, newOff map[int64]int64) (uint64, error) {
	for _, b := range boxes {
		if b.off == moovBox.off {
			continue
		}
		if o >= uint64(b.off) && o < uint64(b.off+b.size) {
			return o - uint64(b.off) + uint64(newOff[b.off]), nil
		}
	}
	return 0, fmt.Errorf("%w: chunk offset %d is outside the media data", errBadMP4, o)
}
