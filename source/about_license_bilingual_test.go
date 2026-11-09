package main

import (
	"os"
	"strings"
	"testing"
)

// Platform-neutral source-level check: the same language preference used by
// the other dialogs must drive the About body and all new license controls.
func TestAboutLicenseBilingualWiring(t *testing.T) {
	read := func(path string) string {
		t.Helper()
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	about := read("about_window_windows.go")
	lang := read("ui_language_windows.go")
	switcher := read("language_switch_windows.go")
	for _, key := range []string{
		`localizeUI("만든 이 지너드")`,
		`localizeUI("넣고 싶은 기능을 잡탕으로 모두 넣은 그리기 툴")`,
		`localizeUI("제작자는 프로그래머/개발자가 아닙니다.")`,
		`localizeUI("버그 제보는 받지만 수정된다고 장담하지 못합니다 ㅋㅋ")`,
		`fmt.Sprintf(localizeUI("현재 빌드: %s"), appVersion)`,
		`localizeUI("라이선스: MIT License (프로젝트 자체 코드)")`,
		`localizeUI("Copyright © 2026 지너드 · G.nerd (eprparadox44)")`,
		`localizeUI("제3자 코드·이미지에는 별도 사용 조건이 적용될 수 있습니다.")`,
		`localizeUI("MIT 라이선스 전문")`,
		`localizeUI("제3자 자료 고지")`,
		`func aboutWindowCaption() string`,
		`func aboutRefreshLanguage(s *aboutWindowState)`,
		`localizeUI("문서를 열 수 없습니다.\n")`,
	} {
		if !strings.Contains(about, key) {
			t.Errorf("About locale wiring missing %q", key)
		}
	}
	for _, pair := range []string{
		`"라이선스: MIT License (프로젝트 자체 코드)":`,
		`"Copyright © 2026 지너드 · G.nerd (eprparadox44)":`,
		`"제3자 코드·이미지에는 별도 사용 조건이 적용될 수 있습니다.":`,
		`"MIT 라이선스 전문":`,
		`"제3자 자료 고지":`,
		`"dotPaint 라이선스":`,
		`"문서를 열 수 없습니다.\n":`,
	} {
		if !strings.Contains(lang, pair) {
			t.Errorf("English license translation missing %q", pair)
		}
	}
	if !strings.Contains(switcher, "aboutRefreshLanguage(s)") {
		t.Error("Live language switch does not refresh the open About dialog")
	}
}
