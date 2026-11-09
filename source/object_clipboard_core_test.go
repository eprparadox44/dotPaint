package main

import (
	"bytes"
	"encoding/xml"
	"testing"
)

func clipboardTestScene() []MovableObject {
	return []MovableObject{
		{Kind: ToolRectangle, X: 4, Y: 4, W: 2, H: 1, SpriteW: 2, SpriteH: 1, Pix: []byte{1, 2, 3, 255, 4, 5, 6, 128}, GroupID: 12, Operation: PaintInvert, Stroke: 2},
		{Kind: ToolText, X: 8, Y: 5, W: 1, H: 1, SpriteW: 1, SpriteH: 1, Pix: []byte{12, 23, 34, 255}, GroupID: 12, Text: "한A", FontName: "맑은 고딕", Styles: []TextCharStyle{{ColorOp: PaintInvert, ColorSet: true}}, GlyphInvertMask: []byte{255}},
		{Kind: ToolCurve, X: 6, Y: 7, W: 2, H: 1, SpriteW: 2, SpriteH: 1, Pix: []byte{11, 21, 31, 255, 41, 51, 61, 255}, Vector: ObjectVector{Count: 3, Points: [6]ObjectPoint{{0, 0}, {1, 0}, {2, 0}}}, Path: []ObjectPoint{{0, 0}, {1, 0}}, ReplaceMask: []byte{3}},
		{Kind: ToolRectangle, X: 100, Y: 100, W: 1, H: 1, SpriteW: 1, SpriteH: 1, Pix: []byte{0, 0, 0, 255}, Rasterized: true},
	}
}

func TestObjectClipboardNativeRoundTripPreservesInvertStylesAndSelectedGroup(t *testing.T) {
	src := clipboardTestScene()
	p, err := makeObjectClipboardPacket(src, []int{2, 0, 1})
	if err != nil {
		t.Fatal(err)
	}
	if p.Width != 5 || p.Height != 4 || len(p.Objects) != 3 || p.Objects[0].X != 0 || p.Objects[1].X != 4 || p.Objects[2].X != 2 {
		t.Fatalf("bounds/z order wrong: %+v", p)
	}
	data, err := encodeObjectClipboard(p)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeObjectClipboard(data)
	if err != nil {
		t.Fatal(err)
	}
	if !objectsEqual(got.Objects, p.Objects) {
		t.Fatal("roundtrip lost native object data")
	}
	if got.Objects[0].Operation != PaintInvert || got.Objects[1].Styles[0].ColorOp != PaintInvert || got.Objects[2].Vector.Count != 3 {
		t.Fatal("invert/text/vector lost")
	}
	prev := []MovableObject{{GroupID: 12}, {GroupID: 14}}
	pasted, ids, gid, err := pasteObjectsFromPacket(got, 20, 40, prev, 14)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 3 || pasted[ids[0]].X != 20 || pasted[ids[1]].Y != 41 || pasted[ids[2]].Y != 43 {
		t.Fatal("paste coordinate offset wrong")
	}
	if pasted[ids[0]].GroupID == 12 || pasted[ids[0]].GroupID == 0 || pasted[ids[0]].GroupID != pasted[ids[1]].GroupID || pasted[ids[2]].GroupID != 0 || gid < 15 {
		t.Fatal("group collision or individual-group promotion")
	}
	if !objectsEqual(pasted[:2], prev) {
		t.Fatal("existing scene modified")
	}
	if !bytes.Equal(pasted[ids[1]].GlyphInvertMask, []byte{255}) {
		t.Fatal("invert mask lost")
	}
}

func TestObjectClipboardRejectsInvalidAndOversizedData(t *testing.T) {
	src := clipboardTestScene()
	if _, err := makeObjectClipboardPacket(src, []int{0, 0}); err == nil {
		t.Fatal("duplicate selected object accepted")
	}
	if _, err := makeObjectClipboardPacket(src, []int{3}); err == nil {
		t.Fatal("rasterized object accepted")
	}
	if _, err := decodeObjectClipboard([]byte("invalid")); err == nil {
		t.Fatal("malformed clipboard accepted")
	}
	p, _ := makeObjectClipboardPacket(src, []int{0})
	p.Version++
	if _, err := encodeObjectClipboard(p); err == nil {
		t.Fatal("future version accepted")
	}
	p.Version--
	p.Objects[0].Pix = p.Objects[0].Pix[:2]
	if _, err := encodeObjectClipboard(p); err == nil {
		t.Fatal("truncated pixel data accepted")
	}
}

func TestObjectClipboardSVGHoldsSeparateImageElements(t *testing.T) {
	src := clipboardTestScene()
	packet, err := makeObjectClipboardPacket(src, []int{0, 1})
	if err != nil {
		t.Fatal(err)
	}
	svg, err := buildObjectClipboardSVG(packet, packet.Objects)
	if err != nil {
		t.Fatal(err)
	}
	var root struct {
		XMLName xml.Name `xml:"svg"`
		Groups  []struct {
			ID     string `xml:"id,attr"`
			Images []struct {
				Href string `xml:"http://www.w3.org/1999/xlink href,attr"`
			} `xml:"image"`
		} `xml:"g"`
	}
	if err := xml.Unmarshal(svg, &root); err != nil {
		t.Fatal("invalid SVG", err)
	}
	if len(root.Groups) != 2 || len(root.Groups[0].Images) != 1 || len(root.Groups[1].Images) != 1 {
		t.Fatal("independent objects not retained")
	}
	if !bytes.Contains(svg, []byte("data:image/png;base64,")) {
		t.Fatal("fallback PNG missing")
	}
}

func TestObjectClipboardSVGTrueVectorPrimitiveWhenSupported(t *testing.T) {
	o := MovableObject{Kind: ToolRectangle, Operation: PaintRGB, X: 3, Y: 4, W: 9, H: 8, SpriteW: 9, SpriteH: 8, ShapeW: 9, ShapeH: 8, ShapeX0: 1, ShapeY0: 1, ShapeX1: 7, ShapeY1: 6, Stroke: 2, FG: 0x00A04020, Pix: make([]byte, 9*8*4)}
	p, err := makeObjectClipboardPacket([]MovableObject{o}, []int{0})
	if err != nil {
		t.Fatal(err)
	}
	svg, err := buildObjectClipboardSVG(p, p.Objects)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(svg, []byte("<rect ")) || bytes.Contains(svg, []byte("<image ")) {
		t.Fatal("supported vector primitive was rasterized")
	}
}
