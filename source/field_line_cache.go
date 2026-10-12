package main

import "sync"

type fieldPhysicalLineCacheEntry struct {
	Key   uint64
	Valid bool
	Lines []fieldLine
}

var fieldPhysicalLineCache struct {
	sync.Mutex
	Electric fieldPhysicalLineCacheEntry
	Magnetic fieldPhysicalLineCacheEntry
	Gravity  fieldPhysicalLineCacheEntry
}

func fieldLineSceneKey(scene fieldScene, samples []fieldChargeSample, kind string) uint64 {
	// The physical vector hash already excludes camera, background and AA while
	// tracking every object and (for electric lines) the solved charge samples.
	key := fieldVectorSceneKey(scene, samples, false, fieldVectorGridSteps(scene))
	key ^= uint64(uint32(scene.Density)) * 0x9e3779b185ebca87
	key ^= uint64(uint32(scene.Dimension)+1) * 0xc2b2ae3d27d4eb4f
	if scene.precise3DLines {
		key ^= 0x8f3f73b5cf1c9ade
	}
	switch kind {
	case "electric":
		key ^= 0x454c454354524943
	case "gravity":
		key ^= 0x4752415649545921
	}
	return key
}

func fieldElectricLineSceneKey(scene fieldScene, samples []fieldChargeSample) uint64 {
	return fieldLineSceneKey(scene, samples, "electric")
}

func fieldGravityLineSceneKey(scene fieldScene) uint64 {
	return fieldLineSceneKey(scene, nil, "gravity")
}

func fieldMagneticLineSceneKey(scene fieldScene) uint64 {
	// Reuse the physical vector-field hash, then mix in the parameters that
	// change streamline seeding but not the E/B grid itself. Camera rotation,
	// display mode and antialiasing remain deliberately excluded.
	key := fieldVectorSceneKey(scene, nil, true, fieldVectorGridSteps(scene))
	key ^= uint64(uint32(scene.Density)) * 0x9e3779b185ebca87
	key ^= uint64(uint32(scene.Dimension)+1) * 0xc2b2ae3d27d4eb4f
	if scene.precise3DLines {
		key ^= 0x8f3f73b5cf1c9ade
	}
	return key
}

func fieldCloneLineSlice(lines []fieldLine) []fieldLine {
	if len(lines) == 0 {
		return nil
	}
	out := make([]fieldLine, len(lines))
	copy(out, lines)
	return out
}

func fieldCachedPhysicalLines(kind string, key uint64) ([]fieldLine, bool) {
	fieldPhysicalLineCache.Lock()
	defer fieldPhysicalLineCache.Unlock()
	entry := fieldPhysicalLineCache.Electric
	switch kind {
	case "magnetic":
		entry = fieldPhysicalLineCache.Magnetic
	case "gravity":
		entry = fieldPhysicalLineCache.Gravity
	}
	if !entry.Valid || entry.Key != key {
		return nil, false
	}
	return fieldCloneLineSlice(entry.Lines), true
}

func fieldStorePhysicalLines(kind string, key uint64, lines []fieldLine) {
	entry := fieldPhysicalLineCacheEntry{Key: key, Valid: true, Lines: fieldCloneLineSlice(lines)}
	fieldPhysicalLineCache.Lock()
	switch kind {
	case "magnetic":
		fieldPhysicalLineCache.Magnetic = entry
	case "gravity":
		fieldPhysicalLineCache.Gravity = entry
	default:
		fieldPhysicalLineCache.Electric = entry
	}
	fieldPhysicalLineCache.Unlock()
}

func fieldCachedMagneticLines(key uint64) ([]fieldLine, bool) {
	return fieldCachedPhysicalLines("magnetic", key)
}

func fieldStoreMagneticLines(key uint64, lines []fieldLine) {
	fieldStorePhysicalLines("magnetic", key, lines)
}
