package miniobject

import (
	"crypto/md5"
	"encoding/binary"
	"sort"
	"sync"
)

const virtualNodes = 150

// HashRing implements consistent hashing with virtual nodes
type HashRing struct {
	mu       sync.RWMutex
	nodes    []string
	ring     []uint32
	nodeMap  map[uint32]string
	replicas int
}

// NewHashRing creates a new hash ring
func NewHashRing(nodes []string, replicas int) *HashRing {
	hr := &HashRing{
		nodes:    make([]string, len(nodes)),
		nodeMap:  make(map[uint32]string),
		replicas: replicas,
	}
	copy(hr.nodes, nodes)
	hr.rebuild()
	return hr
}

func (hr *HashRing) rebuild() {
	hr.ring = nil
	hr.nodeMap = make(map[uint32]string)

	for _, node := range hr.nodes {
		for i := 0; i < virtualNodes; i++ {
			hash := hr.hashKey(node, i)
			hr.ring = append(hr.ring, hash)
			hr.nodeMap[hash] = node
		}
	}
	sort.Slice(hr.ring, func(i, j int) bool {
		return hr.ring[i] < hr.ring[j]
	})
}

func (hr *HashRing) hashKey(key string, index int) uint32 {
	h := md5.Sum([]byte(key + string(rune(index))))
	return binary.BigEndian.Uint32(h[:4])
}

// GetNodes returns N unique nodes for the given key
func (hr *HashRing) GetNodes(key string, n int) []string {
	hr.mu.RLock()
	defer hr.mu.RUnlock()

	if len(hr.nodes) == 0 {
		return nil
	}

	if n > len(hr.nodes) {
		n = len(hr.nodes)
	}

	hash := hr.hashKey(key, 0)
	idx := sort.Search(len(hr.ring), func(i int) bool {
		return hr.ring[i] >= hash
	})

	if idx >= len(hr.ring) {
		idx = 0
	}

	seen := make(map[string]bool)
	result := make([]string, 0, n)

	for i := 0; i < len(hr.ring) && len(result) < n; i++ {
		ringIdx := (idx + i) % len(hr.ring)
		node := hr.nodeMap[hr.ring[ringIdx]]
		if !seen[node] {
			seen[node] = true
			result = append(result, node)
		}
	}

	return result
}

// AllNodes returns all nodes
func (hr *HashRing) AllNodes() []string {
	hr.mu.RLock()
	defer hr.mu.RUnlock()
	result := make([]string, len(hr.nodes))
	copy(result, hr.nodes)
	return result
}
