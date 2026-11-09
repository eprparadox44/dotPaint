package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// DOP v1 is a ZIP container containing a versioned JSON scene and exact BGRA
// pixel buffers. Editable objects (including inverted and locked layers) are
// serialized in their original Z-order. No object is rasterized by saving.
const (
	dopFormatID         = "dotPaint/DOP"
	dopFormatVersion    = 1
	dopMaxSide          = 20000
	dopMaxPixelBytes    = 1 << 30 // guard against malformed archives on load
	dopMaxObjects       = 20000
	dopMaxManifestBytes = 64 << 20
)

type DOPProject struct {
	Canvas                     *Canvas
	Objects                    []MovableObject
	Palette                    []uint32
	IndexedColors              [16]uint32
	IndexedColors256           []uint32
	StandardPalette            []uint32
	CustomColors               [16]uint32
	IndexedMode                bool
	IndexedSize                int
	IndexedCount               int
	IndexedPixels              []byte
	Foreground, Background     uint32
	ForegroundOp, BackgroundOp PaintOperation
	NextGroupID                uint64
}

type dopObjectRecord struct {
	MovableObject
	PixFile         string `json:"pixFile,omitempty"`
	ReplaceFile     string `json:"replaceFile,omitempty"`
	GlyphInvertFile string `json:"glyphInvertFile,omitempty"`
}

type dopManifest struct {
	Format            string            `json:"format"`
	Version           int               `json:"version"`
	Width             int               `json:"width"`
	Height            int               `json:"height"`
	Palette           []uint32          `json:"palette"`
	IndexedColors     [16]uint32        `json:"indexedColors16"`
	IndexedColors256  []uint32          `json:"indexedColors256,omitempty"`
	StandardPalette   []uint32          `json:"standardPalette,omitempty"`
	CustomColors      [16]uint32        `json:"customColors"`
	IndexedMode       bool              `json:"indexedMode"`
	IndexedSize       int               `json:"indexedSize"`
	IndexedCount      int               `json:"indexedCount"`
	IndexedPixelsFile string            `json:"indexedPixelsFile,omitempty"`
	Foreground        uint32            `json:"foreground"`
	Background        uint32            `json:"background"`
	ForegroundOp      PaintOperation    `json:"foregroundOperation"`
	BackgroundOp      PaintOperation    `json:"backgroundOperation"`
	NextGroupID       uint64            `json:"nextGroupID"`
	Objects           []dopObjectRecord `json:"objects"`
}

func dopPixelByteCount(w, h int) (int, error) {
	if w < 1 || h < 1 || w > dopMaxSide || h > dopMaxSide || int64(w)*int64(h)*4 > dopMaxPixelBytes {
		return 0, fmt.Errorf("DOP 그림 크기가 지원 범위를 벗어났습니다: %d×%d", w, h)
	}
	return w * h * 4, nil
}

func hasEditableDOPObjects(objects []MovableObject) bool {
	for _, o := range objects {
		if !o.Rasterized {
			return true
		}
	}
	return false
}

func dopValidateProject(p *DOPProject) error {
	if p == nil || p.Canvas == nil {
		return errors.New("빈 DOP 그림입니다")
	}
	n, err := dopPixelByteCount(p.Canvas.W, p.Canvas.H)
	if err != nil {
		return err
	}
	if len(p.Canvas.Pix) != n {
		return fmt.Errorf("DOP 기본 픽셀 길이 오류: %d != %d", len(p.Canvas.Pix), n)
	}
	if len(p.Palette) < 1 || len(p.Palette) > 256 {
		return errors.New("DOP 팔레트 크기가 잘못되었습니다")
	}
	if len(p.Objects) > dopMaxObjects {
		return fmt.Errorf("DOP 객체 개수가 너무 많습니다: %d", len(p.Objects))
	}
	for i := range p.Objects {
		o := &p.Objects[i]
		if o.W < 0 || o.H < 0 || o.SpriteW < 0 || o.SpriteH < 0 || o.W > dopMaxSide || o.H > dopMaxSide || o.SpriteW > dopMaxSide || o.SpriteH > dopMaxSide {
			return fmt.Errorf("DOP 객체 %d의 크기가 잘못되었습니다", i)
		}
		if len(o.Pix) != 0 {
			n, err := dopPixelByteCount(o.SpriteW, o.SpriteH)
			if err != nil || len(o.Pix) != n {
				return fmt.Errorf("DOP 객체 %d의 픽셀 크기가 잘못되었습니다", i)
			}
		}
		if len(o.ReplaceMask) > 0 && (o.SpriteW < 1 || o.SpriteH < 1 || len(o.ReplaceMask) > (o.SpriteW*o.SpriteH+7)/8) {
			return fmt.Errorf("DOP 객체 %d의 가림 마스크가 잘못되었습니다", i)
		}
		if len(o.GlyphInvertMask) > 0 && len(o.GlyphInvertMask) != o.SpriteW*o.SpriteH {
			return fmt.Errorf("DOP 객체 %d의 글자 반전 마스크가 잘못되었습니다", i)
		}
		if len(o.Path) > 1000000 || len(o.Styles) > 1000000 {
			return fmt.Errorf("DOP 객체 %d의 데이터가 너무 큽니다", i)
		}
		if o.Operation > PaintInvert || o.BackgroundOperation > PaintInvert {
			return fmt.Errorf("DOP 객체 %d의 색상 연산이 잘못되었습니다", i)
		}
	}
	if p.ForegroundOp > PaintInvert || p.BackgroundOp > PaintInvert {
		return errors.New("DOP 색상 연산이 잘못되었습니다")
	}
	if p.IndexedMode {
		if (p.IndexedSize != 16 && p.IndexedSize != 256) || len(p.Palette) != p.IndexedSize || p.IndexedCount < 1 || p.IndexedCount > p.IndexedSize {
			return errors.New("DOP 인덱스 팔레트의 형식이 잘못되었습니다")
		}
		if len(p.IndexedPixels) != p.Canvas.W*p.Canvas.H {
			return errors.New("DOP 인덱스 픽셀 정보가 불완전합니다")
		}
		for _, idx := range p.IndexedPixels {
			if int(idx) >= p.IndexedCount {
				return errors.New("DOP 팔레트 범위를 벗어나는 픽셀 인덱스가 있습니다")
			}
		}
	} else if len(p.IndexedPixels) != 0 {
		return errors.New("RGB 문서에 인덱스 픽셀이 있습니다")
	}
	return nil
}

func dopAddEntry(w *zip.Writer, name string, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	wr, err := w.Create(name)
	if err != nil {
		return err
	}
	_, err = wr.Write(data)
	return err
}

// WriteDOPProject writes a complete independent file to a same-directory temp,
// then replaces the destination only after the ZIP has been closed and synced.
// preview is optional and is never used to restore editable pixels.
func WriteDOPProject(path string, p *DOPProject, preview *Canvas) error {
	if err := dopValidateProject(p); err != nil {
		return err
	}
	if strings.ToLower(filepath.Ext(path)) != ".dop" {
		return errors.New("DOP 프로젝트 파일에는 .dop 확장자가 필요합니다")
	}
	if preview != nil && (preview.W != p.Canvas.W || preview.H != p.Canvas.H || len(preview.Pix) != len(p.Canvas.Pix)) {
		return errors.New("DOP 미리보기 크기가 잘못되었습니다")
	}
	m := dopManifest{Format: dopFormatID, Version: dopFormatVersion, Width: p.Canvas.W, Height: p.Canvas.H, Palette: append([]uint32(nil), p.Palette...), IndexedColors: p.IndexedColors, IndexedColors256: append([]uint32(nil), p.IndexedColors256...), StandardPalette: append([]uint32(nil), p.StandardPalette...), CustomColors: p.CustomColors, IndexedMode: p.IndexedMode, IndexedSize: p.IndexedSize, IndexedCount: p.IndexedCount, Foreground: p.Foreground, Background: p.Background, ForegroundOp: p.ForegroundOp, BackgroundOp: p.BackgroundOp, NextGroupID: p.NextGroupID}
	if p.IndexedMode {
		m.IndexedPixelsFile = "palette/indices.bin"
	}
	for i, src := range p.Objects {
		o := src
		entry := dopObjectRecord{MovableObject: o}
		if len(o.Pix) > 0 {
			entry.PixFile = fmt.Sprintf("objects/%06d.bgra", i)
		}
		if len(o.ReplaceMask) > 0 {
			entry.ReplaceFile = fmt.Sprintf("objects/%06d.replace", i)
		}
		if len(o.GlyphInvertMask) > 0 {
			entry.GlyphInvertFile = fmt.Sprintf("objects/%06d.invert", i)
		}
		entry.Pix = nil
		entry.ReplaceMask = nil
		entry.GlyphInvertMask = nil
		m.Objects = append(m.Objects, entry)
	}
	manifest, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if len(manifest) > dopMaxManifestBytes {
		return errors.New("DOP 객체 정보가 너무 큽니다")
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".dotpaint-save-*.tmp")
	if err != nil {
		return err
	}
	temp := f.Name()
	defer os.Remove(temp)
	zw := zip.NewWriter(f)
	writeErr := dopAddEntry(zw, "manifest.json", manifest)
	if writeErr == nil {
		writeErr = dopAddEntry(zw, "canvas.bgra", p.Canvas.Pix)
	}
	if writeErr == nil && p.IndexedMode {
		writeErr = dopAddEntry(zw, m.IndexedPixelsFile, p.IndexedPixels)
	}
	for i, o := range p.Objects {
		if writeErr != nil {
			break
		}
		e := m.Objects[i]
		if len(o.Pix) > 0 {
			writeErr = dopAddEntry(zw, e.PixFile, o.Pix)
		}
		if writeErr == nil && len(o.ReplaceMask) > 0 {
			writeErr = dopAddEntry(zw, e.ReplaceFile, o.ReplaceMask)
		}
		if writeErr == nil && len(o.GlyphInvertMask) > 0 {
			writeErr = dopAddEntry(zw, e.GlyphInvertFile, o.GlyphInvertMask)
		}
	}
	if writeErr == nil && preview != nil {
		var pw io.Writer
		pw, writeErr = zw.Create("preview.png")
		if writeErr == nil {
			writeErr = png.Encode(pw, preview.toRGBA())
		}
	}
	closeErr := zw.Close()
	if writeErr == nil {
		writeErr = closeErr
	}
	if writeErr == nil {
		writeErr = f.Sync()
	}
	fCloseErr := f.Close()
	if writeErr == nil {
		writeErr = fCloseErr
	}
	if writeErr != nil {
		return writeErr
	}
	return os.Rename(temp, path)
}

func dopReadEntry(files map[string]*zip.File, name string, max int) ([]byte, error) {
	z := files[name]
	if z == nil {
		return nil, fmt.Errorf("DOP 필수 데이터가 없습니다: %s", name)
	}
	if z.UncompressedSize64 > uint64(max) {
		return nil, fmt.Errorf("DOP 데이터가 너무 큽니다: %s", name)
	}
	r, err := z.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	// io.LimitReader rejects a ZIP declaring a short length but producing more.
	b, err := io.ReadAll(io.LimitReader(r, int64(max)+1))
	if err != nil {
		return nil, err
	}
	if len(b) > max {
		return nil, fmt.Errorf("DOP 데이터 제한 초과: %s", name)
	}
	return b, nil
}

func ReadDOPProject(path string) (*DOPProject, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	files := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		if _, ok := files[f.Name]; ok {
			return nil, fmt.Errorf("DOP 중복 항목: %s", f.Name)
		}
		files[f.Name] = f
	}
	mb, err := dopReadEntry(files, "manifest.json", dopMaxManifestBytes)
	if err != nil {
		return nil, err
	}
	var m dopManifest
	if err = json.Unmarshal(mb, &m); err != nil {
		return nil, fmt.Errorf("DOP 정보 파일 오류: %w", err)
	}
	if m.Format != dopFormatID {
		return nil, errors.New("dotPaint DOP 프로젝트가 아닙니다")
	}
	if m.Version != dopFormatVersion {
		return nil, fmt.Errorf("지원하지 않는 DOP 형식 버전: %d", m.Version)
	}
	n, err := dopPixelByteCount(m.Width, m.Height)
	if err != nil {
		return nil, err
	}
	pixels, err := dopReadEntry(files, "canvas.bgra", n)
	if err != nil {
		return nil, err
	}
	if len(pixels) != n {
		return nil, errors.New("DOP 캔버스 픽셀이 불완전합니다")
	}
	p := &DOPProject{Canvas: &Canvas{W: m.Width, H: m.Height, Pix: pixels}, Palette: m.Palette, IndexedColors: m.IndexedColors, IndexedColors256: m.IndexedColors256, StandardPalette: m.StandardPalette, CustomColors: m.CustomColors, IndexedMode: m.IndexedMode, IndexedSize: m.IndexedSize, IndexedCount: m.IndexedCount, Foreground: m.Foreground, Background: m.Background, ForegroundOp: m.ForegroundOp, BackgroundOp: m.BackgroundOp, NextGroupID: m.NextGroupID}
	if m.IndexedMode {
		p.IndexedPixels, err = dopReadEntry(files, m.IndexedPixelsFile, m.Width*m.Height)
		if err != nil {
			return nil, err
		}
	}
	if len(m.Objects) > dopMaxObjects {
		return nil, errors.New("DOP 객체 개수 제한 초과")
	}
	for i, e := range m.Objects {
		o := e.MovableObject
		if e.PixFile != "" {
			pixN, err := dopPixelByteCount(o.SpriteW, o.SpriteH)
			if err != nil {
				return nil, err
			}
			o.Pix, err = dopReadEntry(files, e.PixFile, pixN)
			if err != nil {
				return nil, err
			}
			if len(o.Pix) != pixN {
				return nil, fmt.Errorf("DOP 객체 %d의 픽셀 데이터가 잘렸습니다", i)
			}
		}
		if e.ReplaceFile != "" {
			o.ReplaceMask, err = dopReadEntry(files, e.ReplaceFile, (o.SpriteW*o.SpriteH+7)/8)
			if err != nil {
				return nil, err
			}
		}
		if e.GlyphInvertFile != "" {
			o.GlyphInvertMask, err = dopReadEntry(files, e.GlyphInvertFile, o.SpriteW*o.SpriteH)
			if err != nil {
				return nil, err
			}
		}
		p.Objects = append(p.Objects, o)
	}
	if err = dopValidateProject(p); err != nil {
		return nil, err
	}
	return p, nil
}

// Prevent dependent code from accidentally treating DOP as a compressed PNG.
func dopHasSignature(data []byte) bool {
	return len(data) >= 4 && bytes.Equal(data[:4], []byte{'P', 'K', 3, 4})
}
