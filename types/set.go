package types

import (
	"sync"
)

type Set struct {
	cache map[string]Void
	// mu
	mu sync.RWMutex
}

func NewSet(keys ...string) *Set {
	s := &Set{cache: map[string]Void{}}
	s.Add(keys...)
	return s
}

func (s *Set) Add(keys ...string) bool {
	if len(keys) == 0 {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, key := range keys {
		s.cache[key] = NULL
	}
	return true
}

func (s *Set) Delete(keys ...string) bool {
	if len(keys) == 0 {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, key := range keys {
		delete(s.cache, key)
	}
	return true
}

func (s *Set) Clear() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cache = map[string]Void{}
	return true
}

func (s *Set) Has(key string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, exists := s.cache[key]
	return exists
}

func (s *Set) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.cache)
}

func (s *Set) All() map[string]Void {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_tmp := map[string]Void{}

	for k := range s.cache {
		_tmp[k] = NULL
	}

	return _tmp
}

func (s *Set) Keys() (list []string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for k := range s.cache {
		list = append(list, k)
	}

	return list
}
