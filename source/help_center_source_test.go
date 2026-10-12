package main

import (
	"os"
	"strings"
	"testing"
)

func TestA233HelpCenterSourceMarkers(t *testing.T) {
	mainData, err := os.ReadFile("main_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	mainText := string(mainData)
	for _, marker := range []string{
		`const appVersion = "A237-DARK-LIBRARIES-HELP-ABOUT-BACKGROUND"`,
		`case ID_HELP_TOPICS:`,
		`a.showHelpCenter()`,
		`if vk == VK_F1`,
		`helpRefreshTheme(s)`,
	} {
		if !strings.Contains(mainText, marker) {
			t.Fatalf("main_windows.go missing A233 help marker %q", marker)
		}
	}

	helpData, err := os.ReadFile("help_window_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	helpText := string(helpData)
	for _, marker := range []string{
		`helpWindowClassName = "dotPaintHelpCenterA233"`,
		`func dotPaintHelpTopics() []helpTopic`,
		`Title: "1. 시작하기와 화면 구성"`,
		`Title: fmt.Sprintf("%d. 버전 정보와 정식 1.3", versionNumber)`,
		`func (a *App) showHelpCenter()`,
		`handleDarkEditorNonClient`,
		`drawThemedOwnerDrawPushButton`,
		`openDiagnosticLog(hwnd)`,
	} {
		if !strings.Contains(helpText, marker) {
			t.Fatalf("help_window_windows.go missing marker %q", marker)
		}
	}
}

func TestA233HelpTopicsAreOrganized(t *testing.T) {
	data, err := os.ReadFile("help_window_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	// Nineteen common topics plus the development-only diagnostics topic are
	// static declarations. The final version topic is appended with a dynamic
	// number so the no-log release has 20 topics and the development build 21.
	if got := strings.Count(text, "\t\t\tTitle:"); got != 20 {
		t.Fatalf("static help topic declarations=%d want 20", got)
	}
	if strings.Contains(text, `Body: ""`) {
		t.Fatal("empty help topic body found")
	}
}
