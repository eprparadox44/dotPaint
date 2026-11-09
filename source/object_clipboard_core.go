package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
)

// dotPaint clipboard v1 preserves the exact native editable scene alongside
// CF_DIB/PNG for programs which cannot read dotPaint objects.
const (
	objectClipboardFormatName = "dotPaint.EditableObjects.v1"
	objectClipboardSignature  = "dotPaint/object-clipboard"
	objectClipboardVersion    = 1
	objectClipboardMaxPacked  = 64 << 20
	objectClipboardMaxDecoded = 256 << 20
	objectClipboardMaxObjects = 10000
)

type objectClipboardPacket struct {
	Format  string          `json:"format"`
	Version int             `json:"version"`
	Width   int             `json:"width"`
	Height  int             `json:"height"`
	Objects []MovableObject `json:"objects"`
}

// The selected objects are copied in Z-order, normalized to local coordinates.
// Every object attribute, including inversion/individual character styles,
// geometry, replacement masks and original pixel caches, remains untouched.
func makeObjectClipboardPacket(scene []MovableObject, selected []int) (objectClipboardPacket, error) {
	if len(selected) == 0 || len(selected) > objectClipboardMaxObjects {
		return objectClipboardPacket{}, errors.New("선택된 객체가 없거나 너무 많습니다")
	}
	indices := append([]int(nil), selected...)
	sort.Ints(indices)
	for i, v := range indices {
		if v < 0 || v >= len(scene) || scene[v].Rasterized || (i > 0 && indices[i-1] == v) {
			return objectClipboardPacket{}, errors.New("객체 선택에 잘못된 항목이 있습니다")
		}
	}
	bounds, ok := selectedGroupBounds(scene, indices)
	if !ok || !validObjectSize(bounds.W, bounds.H) {
		return objectClipboardPacket{}, errors.New("객체 복사 영역의 크기가 올바르지 않습니다")
	}
	out := objectClipboardPacket{Format: objectClipboardSignature, Version: objectClipboardVersion, Width: bounds.W, Height: bounds.H}
	for _, i := range indices {
		o := scene[i]
		o.X -= bounds.X
		o.Y -= bounds.Y
		out.Objects = append(out.Objects, o)
	}
	if err := validateObjectClipboardPacket(&out); err != nil {
		return objectClipboardPacket{}, err
	}
	return out, nil
}

func validateObjectClipboardPacket(p *objectClipboardPacket) error {
	if p == nil || p.Format != objectClipboardSignature || p.Version != objectClipboardVersion {
		return errors.New("지원되지 않는 dotPaint 객체 클립보드 형식입니다")
	}
	if !validObjectSize(p.Width, p.Height) || len(p.Objects) == 0 || len(p.Objects) > objectClipboardMaxObjects {
		return errors.New("클립보드 객체 수 또는 복사 영역이 올바르지 않습니다")
	}
	for i := range p.Objects {
		o := &p.Objects[i]
		switch o.Kind {
		case ToolRectangle, ToolEllipse, ToolRightTriangle, ToolText, ToolLine, ToolCurve, ToolArrow:
		default:
			return fmt.Errorf("클립보드 객체 %d는 지원되지 않는 도형 형식입니다", i)
		}
		if o.Rasterized || !validObjectSize(o.W, o.H) || o.X < 0 || o.Y < 0 || int64(o.X)+int64(o.W) > int64(p.Width) || int64(o.Y)+int64(o.H) > int64(p.Height) {
			return fmt.Errorf("클립보드 객체 %d의 위치 또는 크기가 잘못되었습니다", i)
		}
		sw, sh := o.spriteSize()
		if !validObjectSize(sw, sh) || int64(len(o.Pix)) != int64(sw)*int64(sh)*4 {
			return fmt.Errorf("클립보드 객체 %d의 원본 픽셀 크기가 잘못되었습니다", i)
		}
		if len(o.ReplaceMask) > 0 && int64(len(o.ReplaceMask)) > (int64(sw)*int64(sh)+7)/8 {
			return fmt.Errorf("클립보드 객체 %d의 픽셀 마스크가 잘못되었습니다", i)
		}
		if len(o.GlyphInvertMask) > 0 && int64(len(o.GlyphInvertMask)) != int64(sw)*int64(sh) {
			return fmt.Errorf("클립보드 객체 %d의 반전 마스크가 잘못되었습니다", i)
		}
		if len(o.Styles) > 1000000 || len(o.Path) > 1000000 || len(o.Text) > 8<<20 || o.Vector.Count < 0 || o.Vector.Count > 6 || o.Operation > PaintInvert || o.BackgroundOperation > PaintInvert {
			return fmt.Errorf("클립보드 객체 %d의 속성이 올바르지 않습니다", i)
		}
	}
	return nil
}

func encodeObjectClipboard(p objectClipboardPacket) ([]byte, error) {
	if err := validateObjectClipboardPacket(&p); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	zw, err := gzip.NewWriterLevel(&out, gzip.BestSpeed)
	if err != nil {
		return nil, err
	}
	if err = json.NewEncoder(zw).Encode(p); err != nil {
		zw.Close()
		return nil, err
	}
	if err = zw.Close(); err != nil {
		return nil, err
	}
	if out.Len() > objectClipboardMaxPacked {
		return nil, errors.New("객체 클립보드 데이터가 너무 큽니다")
	}
	return out.Bytes(), nil
}

func decodeObjectClipboard(data []byte) (objectClipboardPacket, error) {
	if len(data) == 0 || len(data) > objectClipboardMaxPacked {
		return objectClipboardPacket{}, errors.New("클립보드 객체 데이터 크기가 올바르지 않습니다")
	}
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return objectClipboardPacket{}, err
	}
	defer zr.Close()
	decoded, err := io.ReadAll(io.LimitReader(zr, objectClipboardMaxDecoded+1))
	if err != nil {
		return objectClipboardPacket{}, err
	}
	if len(decoded) > objectClipboardMaxDecoded {
		return objectClipboardPacket{}, errors.New("클립보드 객체 압축 해제 크기 제한 초과")
	}
	var p objectClipboardPacket
	if err = json.Unmarshal(decoded, &p); err != nil {
		return objectClipboardPacket{}, err
	}
	if err = validateObjectClipboardPacket(&p); err != nil {
		return objectClipboardPacket{}, err
	}
	return p, nil
}

// Remap every nonzero group ID to a fresh destination ID. If only one member
// of an existing group was copied, the pasted object becomes independent.
func pasteObjectsFromPacket(p objectClipboardPacket, x, y int, previous []MovableObject, nextGroupID uint64) ([]MovableObject, []int, uint64, error) {
	if err := validateObjectClipboardPacket(&p); err != nil {
		return nil, nil, nextGroupID, err
	}
	for _, o := range previous {
		if o.GroupID >= nextGroupID {
			nextGroupID = o.GroupID + 1
		}
	}
	counts := make(map[uint64]int)
	for _, o := range p.Objects {
		if o.GroupID != 0 {
			counts[o.GroupID]++
		}
	}
	groups := make(map[uint64]uint64)
	out := copyObjects(previous)
	ids := make([]int, 0, len(p.Objects))
	for _, original := range p.Objects {
		o := original
		o.X += x
		o.Y += y
		if original.GroupID != 0 {
			if counts[original.GroupID] < 2 {
				o.GroupID = 0
			} else {
				id, ok := groups[original.GroupID]
				if !ok {
					nextGroupID++
					id = nextGroupID
					groups[original.GroupID] = id
				}
				o.GroupID = id
			}
		}
		ids = append(ids, len(out))
		out = append(out, o)
	}
	return out, ids, nextGroupID, nil
}
