package main

import "testing"

func TestObjectGroupSelectionFlattenAndBounds(t *testing.T) {
	objects := []MovableObject{
		{X: 10, Y: 10, W: 10, H: 15, GroupID: 5},
		{X: 40, Y: 20, W: 15, H: 15, GroupID: 5},
		{X: 3, Y: 5, W: 10, H: 10},
	}
	ids := expandGroupSelection(objects, 0, 0)
	if len(ids) != 2 || ids[0] != 0 || ids[1] != 1 {
		t.Fatalf("expanded group: %v", ids)
	}
	if xs := expandGroupSelection(objects, 0, 5); len(xs) != 1 || xs[0] != 0 {
		t.Fatalf("isolated member: %v", xs)
	}
	if !assignObjectGroup(objects, []int{0, 1, 2}, 7) {
		t.Fatal("merge failed")
	}
	if objects[2].GroupID != 7 || len(expandGroupSelection(objects, 2, 0)) != 3 {
		t.Fatal("group merge failed")
	}
	b, ok := selectedGroupBounds(objects, []int{0, 1, 2})
	if !ok || b != (ObjectBounds{3, 5, 52, 30}) {
		t.Fatalf("bounds: %+v", b)
	}
	if !dissolveObjectGroup(objects, 7) || objects[0].GroupID != 0 {
		t.Fatal("ungroup failed")
	}
}

func TestObjectGroupProportionalResize(t *testing.T) {
	old := ObjectBounds{X: 20, Y: 30, W: 40, H: 20}
	group := ObjectBounds{X: 10, Y: 20, W: 100, H: 100}
	next := ObjectBounds{X: 110, Y: 120, W: 200, H: 150}
	b, ok := scaleObjectBoundsWithinGroup(old, group, next)
	if !ok || b != (ObjectBounds{X: 130, Y: 135, W: 80, H: 30}) {
		t.Fatalf("scaled=%+v", b)
	}
}

func TestObjectGroupIDIsHistoryRelevant(t *testing.T) {
	a := []MovableObject{{Kind: ToolRectangle, GroupID: 3}}
	b := copyObjects(a)
	if !objectsEqual(a, b) {
		t.Fatal("copy lost group ID")
	}
	b[0].GroupID++
	if objectsEqual(a, b) {
		t.Fatal("group change missing from history comparator")
	}
}
