package main

import (
	"os"
	"strings"
	"testing"
)

// Confirm that the About dialog exposes the actual repository license and
// keeps third-party notices distinct from project-author MIT permissions.
func TestAboutLicenseInformationAndLinks(t *testing.T) {
	b, err := os.ReadFile("about_window_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	for _, token := range []string{
		"라이선스: MIT License (프로젝트 자체 코드)",
		"Copyright © 2026 지너드 · G.nerd (eprparadox44)",
		"제3자 코드·이미지에는 별도 사용 조건이 적용될 수 있습니다.",
		"aboutLicenseButtonID", "aboutThirdPartyButtonID",
		"aboutOpenDocumentURL(hwnd, aboutLicenseURL)",
		"aboutOpenDocumentURL(hwnd, aboutThirdPartyNoticeURL)",
		"themeWindowHandles(s.owner.darkMode, s.hwnd, s.closeBtn, s.licenseBtn, s.thirdPartyBtn)",
	} {
		if !strings.Contains(src, token) {
			t.Errorf("About dialog missing %q", token)
		}
	}
}
