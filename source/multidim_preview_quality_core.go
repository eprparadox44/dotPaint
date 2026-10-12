package main

// mdShapeUsesInteractiveFastAA identifies the shapes whose rendering cost is
// high enough to justify a 1x working preview while the user is manipulating
// them.  Every other shape keeps the final 6x AA path even during interaction.
func mdShapeUsesInteractiveFastAA(name string) bool {
	switch name {
	case "포물면/쌍곡면",
		"말안장면",
		"시공간: 블랙홀",
		"쌍원뿔과 원뿔곡선",
		"원환면(토러스)",
		"뫼비우스 띠",
		"2-스타인메츠 입체",
		"3-스타인메츠 입체",
		"정다포체: 120-셀",
		"정다포체: 600-셀",
		"4차원 곡면: 클라인 병":
		return true
	default:
		return false
	}
}

// mdPreviewSupersamplePolicy is the single source of truth for the smooth
// preview scale.  Smooth-off is always pixel-exact 1x.  Smooth-on is 6x for
// every shape at rest; only the explicitly expensive shapes temporarily use
// 1x while an interaction is in progress.
func mdPreviewSupersamplePolicy(name string, smooth, interactive bool) int {
	if !smooth {
		return 1
	}
	if interactive && mdShapeUsesInteractiveFastAA(name) {
		return 1
	}
	return 6
}
