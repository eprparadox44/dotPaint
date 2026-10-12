package main

import (
	"runtime"
	"testing"
)

func TestFieldStreamlineWorkerCount129(t *testing.T) {
	if got := fieldStreamlineWorkerCount(0); got != 0 {
		t.Fatalf("zero tasks workers=%d", got)
	}
	if got := fieldStreamlineWorkerCount(1); got != 1 {
		t.Fatalf("one task workers=%d", got)
	}
	got := fieldStreamlineWorkerCount(100)
	wantMax := runtime.GOMAXPROCS(0)
	if wantMax > 8 {
		wantMax = 8
	}
	if got < 1 || got > wantMax {
		t.Fatalf("workers=%d want 1..%d", got, wantMax)
	}
}

func TestParallelMagneticSeedTracePreservesTaskOrder129(t *testing.T) {
	eval := func(p fieldVec3) fieldVec3 { return fieldVec3{X: 1} }
	policy := fieldDefaultStreamlinePolicy()
	policy.MaxSteps = 4
	policy.MaxArcLength = .2
	policy.InitialStep = .03
	policy.MaxStep = .03
	tasks := make([]fieldMagneticSeedTask, 16)
	for i := range tasks {
		tasks[i] = fieldMagneticSeedTask{Index: i, Group: 100 + i, Seed: fieldVec3{Y: float64(i)}}
	}
	results, workers := fieldTraceMagneticSeedsParallel(eval, policy, tasks)
	if workers < 1 || len(results) != len(tasks) {
		t.Fatalf("workers=%d results=%d", workers, len(results))
	}
	for i, result := range results {
		if result.Accepted {
			if result.Line.Group != tasks[i].Group {
				t.Fatalf("result %d group=%d want=%d", i, result.Line.Group, tasks[i].Group)
			}
		}
	}
}
