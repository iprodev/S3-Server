package miniobject

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
)

// ClusterBackend manages multiple backends with replication
type ClusterBackend struct {
	ring     *HashRing
	backends map[string]Backend
	replicas int
	w        int // write quorum
	r        int // read quorum
}

// NewClusterBackend creates a cluster backend
func NewClusterBackend(nodeURLs []string, replicas, w, r int, authToken string) (*ClusterBackend, error) {
	if w+r <= replicas {
		return nil, errors.New("write + read quorum must be > replicas for consistency")
	}

	backends := make(map[string]Backend)
	for _, url := range nodeURLs {
		backends[url] = NewHTTPBackend(url, authToken)
	}

	ring := NewHashRing(nodeURLs, replicas)

	return &ClusterBackend{
		ring:     ring,
		backends: backends,
		replicas: replicas,
		w:        w,
		r:        r,
	}, nil
}

// Put replicates object to N nodes, waits for W successes
func (c *ClusterBackend) Put(ctx context.Context, bucket, key string, r io.Reader, contentType, contentMD5 string) (string, error) {
	// Read all data into memory for replication
	data, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}

	nodes := c.ring.GetNodes(bucket+"/"+key, c.replicas)
	if len(nodes) == 0 {
		return "", errors.New("no nodes available")
	}

	type result struct {
		node string
		etag string
		err  error
	}

	results := make(chan result, len(nodes))
	var wg sync.WaitGroup

	for _, node := range nodes {
		wg.Add(1)
		go func(n string) {
			defer wg.Done()
			backend := c.backends[n]
			etag, err := backend.Put(ctx, bucket, key, bytes.NewReader(data), contentType, contentMD5)
			results <- result{node: n, etag: etag, err: err}
		}(node)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	successes := 0
	var firstETag string
	for res := range results {
		if res.err == nil {
			successes++
			if firstETag == "" {
				firstETag = res.etag
			}
		}
	}

	if successes < c.w {
		return "", fmt.Errorf("write quorum not met: %d/%d", successes, c.w)
	}

	return firstETag, nil
}

// Get reads from R nodes and returns first success
func (c *ClusterBackend) Get(ctx context.Context, bucket, key string, rangeSpec string) (io.ReadCloser, string, string, int64, int, error) {
	nodes := c.ring.GetNodes(bucket+"/"+key, c.replicas)
	if len(nodes) == 0 {
		return nil, "", "", 0, 500, errors.New("no nodes available")
	}

	type result struct {
		rc          io.ReadCloser
		contentType string
		etag        string
		size        int64
		status      int
		err         error
	}

	results := make(chan result, len(nodes))
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	for _, node := range nodes {
		go func(n string) {
			backend := c.backends[n]
			rc, ct, etag, size, status, err := backend.Get(ctx, bucket, key, rangeSpec)
			results <- result{rc: rc, contentType: ct, etag: etag, size: size, status: status, err: err}
		}(node)
	}

	// Return first successful read
	for i := 0; i < len(nodes); i++ {
		res := <-results
		if res.err == nil && res.status != 404 {
			cancel() // Cancel other requests
			return res.rc, res.contentType, res.etag, res.size, res.status, nil
		}
		if res.rc != nil {
			res.rc.Close()
		}
	}

	return nil, "", "", 0, 404, errors.New("NoSuchKey")
}

// Head queries nodes for metadata
func (c *ClusterBackend) Head(ctx context.Context, bucket, key string) (string, string, int64, bool, error) {
	nodes := c.ring.GetNodes(bucket+"/"+key, c.replicas)
	if len(nodes) == 0 {
		return "", "", 0, false, errors.New("no nodes available")
	}

	for _, node := range nodes {
		backend := c.backends[node]
		ct, etag, size, exists, err := backend.Head(ctx, bucket, key)
		if err == nil && exists {
			return ct, etag, size, true, nil
		}
	}

	return "", "", 0, false, nil
}

// Delete removes from all replica nodes
func (c *ClusterBackend) Delete(ctx context.Context, bucket, key string) error {
	nodes := c.ring.GetNodes(bucket+"/"+key, c.replicas)

	var wg sync.WaitGroup
	for _, node := range nodes {
		wg.Add(1)
		go func(n string) {
			defer wg.Done()
			backend := c.backends[n]
			backend.Delete(ctx, bucket, key)
		}(node)
	}
	wg.Wait()

	return nil
}

// List queries first available node
func (c *ClusterBackend) List(ctx context.Context, bucket, prefix, marker string, limit int) ([]ObjectInfo, error) {
	nodes := c.ring.AllNodes()
	if len(nodes) == 0 {
		return nil, errors.New("no nodes available")
	}

	backend := c.backends[nodes[0]]
	return backend.List(ctx, bucket, prefix, marker, limit)
}

// RepairObject attempts to replicate object to all nodes
func (c *ClusterBackend) RepairObject(ctx context.Context, bucket, key string) error {
	nodes := c.ring.GetNodes(bucket+"/"+key, c.replicas)

	// Find a node that has the object
	var sourceNode string
	var sourceData []byte
	var contentType string

	for _, node := range nodes {
		backend := c.backends[node]
		rc, ct, _, _, status, err := backend.Get(ctx, bucket, key, "")
		if err == nil && status == 200 {
			defer rc.Close()
			data, err := io.ReadAll(rc)
			if err == nil {
				sourceNode = node
				sourceData = data
				contentType = ct
				break
			}
		}
	}

	if sourceNode == "" {
		return errors.New("no source node found")
	}

	// Replicate to missing nodes
	for _, node := range nodes {
		if node == sourceNode {
			continue
		}

		backend := c.backends[node]
		_, _, _, exists, _ := backend.Head(ctx, bucket, key)
		if !exists {
			backend.Put(ctx, bucket, key, bytes.NewReader(sourceData), contentType, "")
		}
	}

	return nil
}
