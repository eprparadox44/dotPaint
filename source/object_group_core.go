package main

// Selection state is intentionally not serialized into history. The group ID
// stored on each object is serialized by copyObjects and compared by objectsEqual.
func expandGroupSelection(objects []MovableObject, index int, isolatedGroup uint64) []int {
	if index < 0 || index >= len(objects) || objects[index].Rasterized {
		return nil
	}
	id := objects[index].GroupID
	if id == 0 || id == isolatedGroup {
		return []int{index}
	}
	out := make([]int, 0, 4)
	for i := range objects {
		if !objects[i].Rasterized && objects[i].GroupID == id {
			out = append(out, i)
		}
	}
	return out
}

func selectedGroupBounds(objects []MovableObject, indices []int) (ObjectBounds, bool) {
	if len(indices) == 0 {
		return ObjectBounds{}, false
	}
	valid := false
	left, top, right, bottom := 0, 0, 0, 0
	for _, i := range indices {
		if i < 0 || i >= len(objects) || objects[i].Rasterized {
			continue
		}
		o := objects[i]
		if !valid {
			left, top, right, bottom = o.X, o.Y, o.X+o.W, o.Y+o.H
			valid = true
			continue
		}
		left, top = min(left, o.X), min(top, o.Y)
		right, bottom = max(right, o.X+o.W), max(bottom, o.Y+o.H)
	}
	return ObjectBounds{left, top, right - left, bottom - top}, valid
}

func scaleObjectBoundsWithinGroup(old, groupBefore, groupAfter ObjectBounds) (ObjectBounds, bool) {
	if groupBefore.W <= 0 || groupBefore.H <= 0 || !validObjectSize(groupAfter.W, groupAfter.H) {
		return ObjectBounds{}, false
	}
	// Calculate boundaries rather than independent widths, so neighbouring
	// members remain aligned after repeated relative scaling.
	scaleX := func(v int) int {
		return groupAfter.X + int((int64(v-groupBefore.X)*int64(groupAfter.W)+int64(groupBefore.W)/2)/int64(groupBefore.W))
	}
	scaleY := func(v int) int {
		return groupAfter.Y + int((int64(v-groupBefore.Y)*int64(groupAfter.H)+int64(groupBefore.H)/2)/int64(groupBefore.H))
	}
	x0, y0 := scaleX(old.X), scaleY(old.Y)
	x1, y1 := scaleX(old.X+old.W), scaleY(old.Y+old.H)
	out := ObjectBounds{x0, y0, max(1, x1-x0), max(1, y1-y0)}
	return out, validObjectSize(out.W, out.H)
}

func assignObjectGroup(objects []MovableObject, indices []int, id uint64) bool {
	if len(indices) < 2 || id == 0 {
		return false
	}
	changed := false
	for _, i := range indices {
		if i < 0 || i >= len(objects) || objects[i].Rasterized {
			continue
		}
		if objects[i].GroupID != id {
			objects[i].GroupID = id
			changed = true
		}
	}
	return changed
}

func dissolveObjectGroup(objects []MovableObject, id uint64) bool {
	if id == 0 {
		return false
	}
	changed := false
	for i := range objects {
		if objects[i].GroupID == id {
			objects[i].GroupID = 0
			changed = true
		}
	}
	return changed
}
