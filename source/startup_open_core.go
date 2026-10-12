package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

// startupDocumentPath accepts the first image path passed by the Windows shell
// (for example via Explorer > Open With). The shell has already parsed quoted
// command-line paths into arguments; do not strip path quotes or split on spaces.
// Additional arguments are ignored; dotPaint edits one document per window.
// An empty result with nil error means the program was started normally.
func startupDocumentPath(args []string) (string, error) {
	if len(args) == 0 {
		return "", nil
	}
	path := args[0]
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("파일 경로가 비어 있습니다")
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".bmp", ".jpg", ".jpeg", ".gif", ".img":
		return path, nil
	default:
		return "", fmt.Errorf("지원하지 않는 그림 파일입니다: %s (PNG, BMP, JPG, JPEG, GIF, DejaVu IMG 지원)", filepath.Ext(path))
	}
}
