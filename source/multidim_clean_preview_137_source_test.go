package main

import (
	"os"
	"strings"
	"testing"
)

// A clean workspace should not spend render-loop time assembling explanatory
// paragraphs, and it should give that freed horizontal space to the preview.
func TestMultidimCleanPreview137(t *testing.T) {
	data, err := os.ReadFile("multidim_shape_editor_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	for _, text := range []string{
		"s.infoEdit = multidimCreateChild",
		"s.descEdit = multidimCreateChild",
		"multidimMoveWindow(s.infoEdit",
		"multidimMoveWindow(s.descEdit",
		"multidimSetText(s.infoEdit",
		"multidimSetText(s.descEdit",
	} {
		if strings.Contains(src, text) {
			t.Errorf("removed informational pane is still active: %s", text)
		}
	}
	for _, text := range []string{
		"previewX := margin",
		"previewW := width - 2*margin",
		"multidimMoveWindow(s.previewBox, previewX, contentTop, previewW, contentH, true)",
		"multidimEditorRefreshValueLabels(s)",
		"multidimInvalidatePreview(s)",
		"multidimMoveWindow(s.statusLabel, margin",
	} {
		if !strings.Contains(src, text) {
			t.Errorf("essential preview behavior/layout missing: %s", text)
		}
	}
}
