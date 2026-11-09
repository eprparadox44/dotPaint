package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"math"
	"strings"
)

// SVG is an optional interoperable object container. Each dotPaint object
// becomes its own independently selectable <g>/<image> element. Raster
// previews are used inside these elements to maintain exact authored pixels;
// native editability (control points, inversion) is carried only by the
// dotPaint-specific clipboard format, not by generic SVG.
// visual must have the same ordering as p.Objects; dynamic inversion can be
// baked against the source document before calling this function.
// A small subset of ordinary editable shapes can be represented as true SVG
// geometry, so applications accepting SVG may edit them as vector shapes.
// Complex objects and exact pixel-art renderings remain per-object PNGs.
func objectSVGPrimitive(o MovableObject, atX, atY int) string {
	if o.Operation != PaintRGB || o.Transparency != 0 || o.ShowCenter || len(o.ReplaceMask) > 0 || len(o.GlyphInvertMask) > 0 {
		return ""
	}
	col := fmt.Sprintf("#%02x%02x%02x", byte(o.FG), byte(o.FG>>8), byte(o.FG>>16))
	stroke := math.Max(1, float64(o.Stroke))
	if (o.Kind == ToolRectangle || o.Kind == ToolEllipse) && o.ShapeW > 0 && o.ShapeH > 0 {
		// Shape endpoints are stored in the original sprite coordinate system.
		// Apply live resize scaling without discarding the vector geometry.
		x0 := float64(atX) + float64(o.ShapeX0)*float64(o.W)/float64(o.ShapeW)
		y0 := float64(atY) + float64(o.ShapeY0)*float64(o.H)/float64(o.ShapeH)
		x1 := float64(atX) + float64(o.ShapeX1)*float64(o.W)/float64(o.ShapeW)
		y1 := float64(atY) + float64(o.ShapeY1)*float64(o.H)/float64(o.ShapeH)
		l, t, r, b := math.Min(x0, x1), math.Min(y0, y1), math.Max(x0, x1), math.Max(y0, y1)
		if o.Kind == ToolRectangle {
			return fmt.Sprintf(`<rect x="%.2f" y="%.2f" width="%.2f" height="%.2f" fill="none" stroke="%s" stroke-width="%.2f"/>`, l, t, r-l, b-t, col, stroke)
		}
		return fmt.Sprintf(`<ellipse cx="%.2f" cy="%.2f" rx="%.2f" ry="%.2f" fill="none" stroke="%s" stroke-width="%.2f"/>`, (l+r)/2, (t+b)/2, (r-l)/2, (b-t)/2, col, stroke)
	}
	if o.Kind == ToolLine && o.Vector.Count == 2 && !o.Vector.Constrained {
		x0, y0 := o.Vector.Points[0].X+atX, o.Vector.Points[0].Y+atY
		x1, y1 := o.Vector.Points[1].X+atX, o.Vector.Points[1].Y+atY
		return fmt.Sprintf(`<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="%s" stroke-width="%.2f" stroke-linecap="round"/>`, x0, y0, x1, y1, col, stroke)
	}
	return ""
}

func buildObjectClipboardSVG(p objectClipboardPacket, visual []MovableObject) ([]byte, error) {
	if err := validateObjectClipboardPacket(&p); err != nil {
		return nil, err
	}
	if len(visual) != len(p.Objects) {
		return nil, fmt.Errorf("SVG 객체 배열 길이가 다릅니다")
	}
	var out strings.Builder
	fmt.Fprintf(&out, `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="%d" height="%d" viewBox="0 0 %d %d">`+"\n", p.Width, p.Height, p.Width, p.Height)
	for i, o := range visual {
		sw, sh := o.spriteSize()
		if !validObjectSize(sw, sh) || len(o.Pix) != sw*sh*4 {
			return nil, fmt.Errorf("SVG 객체 %d 픽셀 오류", i)
		}
		if primitive := objectSVGPrimitive(p.Objects[i], p.Objects[i].X, p.Objects[i].Y); primitive != "" {
			fmt.Fprintf(&out, `<g id="dotpaint-object-%d">%s</g>`+"\n", i+1, primitive)
			continue
		}
		// Draw every complex object as a separate image element; external SVG
		// editors can still move or resize each object independently.
		pngBytes, err := buildTransparentClipboardPNG(sw, sh, o.Pix)
		if err != nil {
			return nil, err
		}
		var encoded bytes.Buffer
		enc := base64.NewEncoder(base64.StdEncoding, &encoded)
		if _, err = enc.Write(pngBytes); err != nil {
			return nil, err
		}
		if err = enc.Close(); err != nil {
			return nil, err
		}
		fmt.Fprintf(&out, `<g id="dotpaint-object-%d"><image x="%d" y="%d" width="%d" height="%d" preserveAspectRatio="none" xlink:href="data:image/png;base64,%s"/></g>`+"\n", i+1, p.Objects[i].X, p.Objects[i].Y, p.Objects[i].W, p.Objects[i].H, encoded.String())
	}
	out.WriteString("</svg>\n")
	return []byte(out.String()), nil
}
