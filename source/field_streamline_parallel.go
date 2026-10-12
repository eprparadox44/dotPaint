package main

import (
	"runtime"
	"sync"
	"time"
)

type fieldMagneticSeedTask struct {
	Index   int
	Group   int
	Seed    fieldVec3
	Spacing float64
}

type fieldMagneticSeedResult struct {
	Line        fieldLine
	Accepted    bool
	Points      int
	Evaluations int
	Elapsed     time.Duration
}

func fieldStreamlineWorkerCount(tasks int) int {
	if tasks <= 1 {
		return max(0, tasks)
	}
	workers := runtime.GOMAXPROCS(0)
	if workers < 1 {
		workers = 1
	}
	// More than eight workers adds memory/cache pressure without helping the
	// relatively small 28..96 candidate batches used by the field editor.
	if workers > 8 {
		workers = 8
	}
	if workers > tasks {
		workers = tasks
	}
	return workers
}

func fieldTraceMagneticSeed(eval fieldVectorEvaluator, policy fieldStreamlinePolicy, task fieldMagneticSeedTask) fieldMagneticSeedResult {
	started := time.Now()
	result := fieldIntegrateBidirectionalStreamline(eval, task.Seed, policy)
	out := fieldMagneticSeedResult{
		Points:      len(result.Points),
		Evaluations: result.Evaluations,
		Elapsed:     time.Since(started),
	}
	if len(result.Points) < 7 {
		return out
	}
	report := fieldValidateStreamline(eval, result.Points, 1, .085, .84, result.Closed, policy.ClosureDistance*1.35, policy.ClosureDirectionCos)
	if report.NonFinite != 0 || report.LongJump != 0 || report.Misaligned != 0 || report.FalseClosure != 0 {
		return out
	}
	out.Accepted = true
	out.Line = fieldRefineMagneticLineSpacing(result.Points, result.Closed, task.Spacing)
	out.Line.Group = task.Group
	return out
}

func fieldTraceMagneticSeedsParallel(eval fieldVectorEvaluator, policy fieldStreamlinePolicy, tasks []fieldMagneticSeedTask) ([]fieldMagneticSeedResult, int) {
	workers := fieldStreamlineWorkerCount(len(tasks))
	results := make([]fieldMagneticSeedResult, len(tasks))
	if workers == 0 {
		return results, 0
	}
	if workers == 1 {
		for i, task := range tasks {
			results[i] = fieldTraceMagneticSeed(eval, policy, task)
		}
		return results, 1
	}

	jobs := make(chan int, workers*2)
	var wg sync.WaitGroup
	wg.Add(workers)
	for worker := 0; worker < workers; worker++ {
		go func() {
			defer wg.Done()
			for index := range jobs {
				results[index] = fieldTraceMagneticSeed(eval, policy, tasks[index])
			}
		}()
	}
	for index := range tasks {
		jobs <- index
	}
	close(jobs)
	wg.Wait()
	return results, workers
}
