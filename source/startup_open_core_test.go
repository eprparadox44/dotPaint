package main

import (
	"strings"
	"testing"
)

func TestStartupDocumentPathFromExplorer(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		want      string
		errorWant bool
	}{
		{name: "plain launch"},
		{name: "png Korean spaces", args: []string{`C:\Users\사용자\내 그림\별 사진.png`}, want: `C:\Users\사용자\내 그림\별 사진.png`},
		{name: "bmp", args: []string{`D:\Images\art.bmp`}, want: `D:\Images\art.bmp`},
		{name: "jpg uppercase", args: []string{`D:\Images\a.JPG`}, want: `D:\Images\a.JPG`},
		{name: "jpeg", args: []string{`D:\Images\a.jpeg`}, want: `D:\Images\a.jpeg`},
		{name: "gif", args: []string{`D:\Images\a.gif`}, want: `D:\Images\a.gif`},
		{name: "DejaVu IMG", args: []string{`D:\FQ4\dj\기관실\P2-2.IMG`}, want: `D:\FQ4\dj\기관실\P2-2.IMG`},
		{name: "multiple file args first only", args: []string{`C:\a.png`, `D:\b.png`}, want: `C:\a.png`},
		{name: "empty arg", args: []string{`  `}, errorWant: true},
		{name: "unrecognized file", args: []string{`C:\a.txt`}, errorWant: true},
		{name: "missing extension", args: []string{`C:\a`}, errorWant: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := startupDocumentPath(tc.args)
			if (err != nil) != tc.errorWant {
				t.Fatalf("err = %v, expectedErr = %v", err, tc.errorWant)
			}
			if got != tc.want {
				t.Fatalf("path = %q, expected %q", got, tc.want)
			}
		})
	}
}

func TestStartupDocumentPathDoesNotTrimOrReparseShellArgument(t *testing.T) {
	// Windows passes a Unicode path containing spaces as a single os.Args item.
	path := `C:\path with spaces\one [2] (final).PNG`
	got, err := startupDocumentPath([]string{path})
	if err != nil || got != path || strings.Contains(got, `"`) {
		t.Fatalf("got %q, err %v", got, err)
	}
}
