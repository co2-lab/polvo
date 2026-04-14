//go:build !darwin && !linux

package filelock

import "sync"

// newGlobalRegistry falls back to the in-process FileLockRegistry on platforms
// that do not support flock(2) (e.g., Windows, plan9).
// The crossProcess flag is silently ignored on these platforms.
func newGlobalRegistry(_ bool) Registry {
	r := &FileLockRegistry{}
	for i := range r.buckets {
		r.buckets[i].locks = make(map[string]*sync.RWMutex)
	}
	return r
}
