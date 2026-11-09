//go:build windows

package main

import "testing"

func TestAboutCaptionAndLicenseTranslations(t *testing.T) {
	old := dotPaintEdition
	defer func() { dotPaintEdition = old }()
	for _, tt := range []struct{ locale, caption, license, button, notice, copyright string }{
		{"ko", "dotPaint 2.0.1 정보", "라이선스: MIT License (프로젝트 자체 코드)", "MIT 라이선스 전문", "제3자 자료 고지", "Copyright © 2026 지너드 · G.nerd (eprparadox44)"},
		{"en", "About dotPaint 2.0.1", "License: MIT License (original project code)", "Full MIT License", "Third-Party Notices", "Copyright © 2026 G.nerd (eprparadox44)"},
	} {
		dotPaintEdition = tt.locale
		if got := aboutWindowCaption(); got != tt.caption {
			t.Errorf("%s caption %q != %q", tt.locale, got, tt.caption)
		}
		for _, item := range []struct{ input, expected string }{
			{"라이선스: MIT License (프로젝트 자체 코드)", tt.license},
			{"MIT 라이선스 전문", tt.button},
			{"제3자 자료 고지", tt.notice},
			{"Copyright © 2026 지너드 · G.nerd (eprparadox44)", tt.copyright},
		} {
			if got := localizeUI(item.input); got != item.expected {
				t.Errorf("%s localizeUI(%q) = %q, expected %q", tt.locale, item.input, got, item.expected)
			}
		}
	}
}
