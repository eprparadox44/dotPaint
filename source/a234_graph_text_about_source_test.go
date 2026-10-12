package main

import (
	"os"
	"strings"
	"testing"
)

func TestA234GraphInvertTextToolboxAndAbout(t *testing.T) {
	graphBytes, err := os.ReadFile("function_graph_editor_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	graph := string(graphBytes)
	for _, token := range []string{
		`invertX := x + 8`,
		`procMoveWindow.Call(s.invertBtn, uintptr(invertX), 40, invertW, 26, 1)`,
		`sampleX := max(w-samplePanelW-10, invertX+invertW+10)`,
		`procMoveWindow.Call(s.proportionalCheck, uintptr(proportionalX)`,
	} {
		if !strings.Contains(graph, token) {
			t.Fatalf("graph layout missing %q", token)
		}
	}

	mainBytes, err := os.ReadFile("main_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	mainSrc := string(mainBytes)
	for _, token := range []string{
		`const appVersion = "A237-DARK-LIBRARIES-HELP-ABOUT-BACKGROUND"`,
		`BS_OWNERDRAW`,
		`themeWindowHandles(a.darkMode, a.textTool.FontCombo`,
		`if drawThemedOwnerDrawPushButton(dis)`,
		`a.showAboutWindow()`,
	} {
		if !strings.Contains(mainSrc, token) {
			t.Fatalf("main source missing %q", token)
		}
	}
	aboutBytes, err := os.ReadFile("about_window_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	aboutSrc := string(aboutBytes)
	for _, token := range []string{
		`fmt.Sprintf("dotPaint %s", appVersion)`,
		`만든 이 지너드`,
		`넣고 싶은 기능을 잡탕으로 모두 넣은 그리기 툴`,
		`제작자는 프로그래머/개발자가 아닙니다.`,
		`버그 제보는 받지만 수정된다고 장담하지 못합니다 ㅋㅋ`,
	} {
		if !strings.Contains(aboutSrc, token) {
			t.Fatalf("about source missing %q", token)
		}
	}
}
