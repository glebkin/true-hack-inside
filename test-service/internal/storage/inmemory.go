package storage

import (
	"maps"
	"slices"
	"sync"
)

type InMemory[K comparable, V any] struct {
	data map[K]V
	mx   sync.RWMutex
}

func NewInMemory[K comparable, V any]() *InMemory[K, V] {
	return &InMemory[K, V]{
		data: make(map[K]V),
	}
}

func (s *InMemory[K, V]) Put(k K, v V) {
	s.mx.Lock()
	defer s.mx.Unlock()
	s.data[k] = v
}

func (s *InMemory[K, V]) Get(k K) (V, bool) {
	s.mx.RLock()
	defer s.mx.RUnlock()
	v, ok := s.data[k]
	return v, ok
}

func (s *InMemory[K, V]) GetAll() []V {
	s.mx.RLock()
	defer s.mx.RUnlock()
	return slices.Collect(maps.Values(s.data))
}

func (s *InMemory[K, V]) Remove(k K) {
	s.mx.Lock()
	defer s.mx.Unlock()
	delete(s.data, k)
}

func (s *InMemory[K, V]) RemoveAll() {
	s.mx.Lock()
	defer s.mx.Unlock()
	s.data = make(map[K]V)
}
