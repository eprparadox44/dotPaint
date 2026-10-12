package main

import "testing"

func TestMDInteractiveFastAAExactShapeSet(t *testing.T) {
	want := map[string]bool{
		"포물면/쌍곡면":       true,
		"말안장면":          true,
		"시공간: 블랙홀":      true,
		"쌍원뿔과 원뿔곡선":     true,
		"원환면(토러스)":      true,
		"뫼비우스 띠":        true,
		"2-스타인메츠 입체":    true,
		"3-스타인메츠 입체":    true,
		"정다포체: 120-셀":   true,
		"정다포체: 600-셀":   true,
		"4차원 곡면: 클라인 병": true,
	}
	all := []string{
		"원판", "평면", "좌표평면", "포물면/쌍곡면", "말안장면", "시공간: 블랙홀",
		"쌍원뿔과 원뿔곡선", "구", "타원체", "원기둥", "원뿔", "다각뿔",
		"원환면(토러스)", "뫼비우스 띠", "정사면체", "정육면체", "정팔면체",
		"정십이면체", "정이십면체", "풀러렌(C60)", "2-스타인메츠 입체",
		"3-스타인메츠 입체", "정다포체: 5-셀", "정다포체: 테서렉트",
		"정다포체: 16-셀", "정다포체: 24-셀", "정다포체: 120-셀",
		"정다포체: 600-셀", "4차원 곡면: 클라인 병",
	}
	for _, name := range all {
		if got := mdShapeUsesInteractiveFastAA(name); got != want[name] {
			t.Errorf("shape %q fast-AA=%v want %v", name, got, want[name])
		}
	}
}

func TestMDPreviewSupersamplePolicy(t *testing.T) {
	for _, name := range []string{"포물면/쌍곡면", "시공간: 블랙홀", "정다포체: 120-셀", "4차원 곡면: 클라인 병"} {
		if got := mdPreviewSupersamplePolicy(name, true, true); got != 1 {
			t.Errorf("expensive interactive shape %q got %dx want 1x", name, got)
		}
		if got := mdPreviewSupersamplePolicy(name, true, false); got != 6 {
			t.Errorf("expensive resting shape %q got %dx want 6x", name, got)
		}
	}
	for _, name := range []string{"원판", "구", "정육면체", "정다포체: 테서렉트"} {
		if got := mdPreviewSupersamplePolicy(name, true, true); got != 6 {
			t.Errorf("ordinary interactive shape %q got %dx want 6x", name, got)
		}
		if got := mdPreviewSupersamplePolicy(name, true, false); got != 6 {
			t.Errorf("ordinary resting shape %q got %dx want 6x", name, got)
		}
	}
	for _, interactive := range []bool{false, true} {
		if got := mdPreviewSupersamplePolicy("정다포체: 600-셀", false, interactive); got != 1 {
			t.Errorf("smooth-off interactive=%v got %dx want 1x", interactive, got)
		}
	}
}
