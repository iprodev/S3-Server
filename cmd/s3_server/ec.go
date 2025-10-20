package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/iProDev/S3-Server/miniobject"
	"github.com/klauspost/reedsolomon"
)

type ECBackend struct {
	nodes      []string
	backends   map[string]miniobject.Backend
	dataShards int
	parShards  int
	encoder    reedsolomon.Encoder
	tmpDir     string
	logger     *Logger
}

type ECManifest struct {
	DataShards   int    `json:"data_shards"`
	ParityShards int    `json:"parity_shards"`
	Size         int64  `json:"size"`
	ContentType  string `json:"content_type"`
	Checksum     string `json:"checksum"`
}

func NewECBackend(nodes []string, dataShards, parityShards int, authToken, tmpDir string, logger *Logger) (*ECBackend, error) {
	if len(nodes) < dataShards+parityShards {
		return nil, fmt.Errorf("not enough nodes: need %d, have %d", dataShards+parityShards, len(nodes))
	}

	encoder, err := reedsolomon.New(dataShards, parityShards)
	if err != nil {
		return nil, err
	}

	backends := make(map[string]miniobject.Backend)
	for _, url := range nodes {
		backends[url] = miniobject.NewHTTPBackend(url, authToken)
	}

	return &ECBackend{
		nodes:      nodes,
		backends:   backends,
		dataShards: dataShards,
		parShards:  parityShards,
		encoder:    encoder,
		tmpDir:     tmpDir,
		logger:     logger,
	}, nil
}

func (ec *ECBackend) Put(ctx context.Context, bucket, key string, r io.Reader, contentType, contentMD5 string) (string, error) {
	// Write to temp file first
	tmpFile := filepath.Join(ec.tmpDir, fmt.Sprintf("ec-put-%d", os.Getpid()))
	f, err := os.Create(tmpFile)
	if err != nil {
		return "", err
	}
	defer os.Remove(tmpFile)

	size, err := io.Copy(f, r)
	if err != nil {
		f.Close()
		return "", err
	}
	f.Close()

	// Read back and split into shards
	data, err := os.ReadFile(tmpFile)
	if err != nil {
		return "", err
	}

	shards, err := ec.encoder.Split(data)
	if err != nil {
		return "", err
	}

	if err := ec.encoder.Encode(shards); err != nil {
		return "", err
	}

	// Store manifest
	manifest := ECManifest{
		DataShards:   ec.dataShards,
		ParityShards: ec.parShards,
		Size:         size,
		ContentType:  contentType,
		Checksum:     contentMD5,
	}

	manifestData, _ := json.Marshal(manifest)

	// Upload shards and manifest
	var wg sync.WaitGroup
	errors := make(chan error, len(shards)+1)

	// Upload manifest to first node
	wg.Add(1)
	go func() {
		defer wg.Done()
		backend := ec.backends[ec.nodes[0]]
		_, err := backend.Put(ctx, bucket, key+".manifest", bytes.NewReader(manifestData), "application/json", "")
		if err != nil {
			errors <- err
		}
	}()

	// Upload shards
	for i, shard := range shards {
		if i >= len(ec.nodes) {
			break
		}

		wg.Add(1)
		go func(idx int, shardData []byte) {
			defer wg.Done()
			backend := ec.backends[ec.nodes[idx]]
			shardKey := fmt.Sprintf("%s.shard.%d", key, idx)
			_, err := backend.Put(ctx, bucket, shardKey, bytes.NewReader(shardData), "application/octet-stream", "")
			if err != nil {
				errors <- err
			}
		}(i, shard)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		if err != nil {
			return "", err
		}
	}

	return `"` + contentMD5 + `"`, nil
}

func (ec *ECBackend) Get(ctx context.Context, bucket, key string, rangeSpec string) (io.ReadCloser, string, string, int64, int, error) {
	// Load manifest
	backend := ec.backends[ec.nodes[0]]
	rc, _, _, _, status, err := backend.Get(ctx, bucket, key+".manifest", "")
	if err != nil || status != 200 {
		return nil, "", "", 0, 404, fmt.Errorf("NoSuchKey")
	}

	manifestData, _ := io.ReadAll(rc)
	rc.Close()

	var manifest ECManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return nil, "", "", 0, 500, err
	}

	// Download shards
	totalShards := ec.dataShards + ec.parShards
	shards := make([][]byte, totalShards)
	var wg sync.WaitGroup

	for i := 0; i < totalShards && i < len(ec.nodes); i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			backend := ec.backends[ec.nodes[idx]]
			shardKey := fmt.Sprintf("%s.shard.%d", key, idx)
			rc, _, _, _, status, err := backend.Get(ctx, bucket, shardKey, "")
			if err == nil && status == 200 {
				data, _ := io.ReadAll(rc)
				rc.Close()
				shards[idx] = data
			}
		}(i)
	}

	wg.Wait()

	// Reconstruct
	if err := ec.encoder.Reconstruct(shards); err != nil {
		return nil, "", "", 0, 500, err
	}

	// Join data shards
	var buf bytes.Buffer
	if err := ec.encoder.Join(&buf, shards, int(manifest.Size)); err != nil {
		return nil, "", "", 0, 500, err
	}

	data := buf.Bytes()

	// Handle range if requested
	if rangeSpec != "" {
		// For EC, reconstruct entire object then apply range
		// In production, implement smarter range handling
		ec.logger.Warn("range request on EC object requires full reconstruction")
	}

	return io.NopCloser(bytes.NewReader(data)), manifest.ContentType, `"` + manifest.Checksum + `"`, manifest.Size, 200, nil
}

func (ec *ECBackend) Head(ctx context.Context, bucket, key string) (string, string, int64, bool, error) {
	backend := ec.backends[ec.nodes[0]]
	rc, _, _, _, status, err := backend.Get(ctx, bucket, key+".manifest", "")
	if err != nil || status != 200 {
		return "", "", 0, false, nil
	}

	manifestData, _ := io.ReadAll(rc)
	rc.Close()

	var manifest ECManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return "", "", 0, false, err
	}

	return manifest.ContentType, `"` + manifest.Checksum + `"`, manifest.Size, true, nil
}

func (ec *ECBackend) Delete(ctx context.Context, bucket, key string) error {
	// Delete manifest
	backend := ec.backends[ec.nodes[0]]
	backend.Delete(ctx, bucket, key+".manifest")

	// Delete shards
	totalShards := ec.dataShards + ec.parShards
	for i := 0; i < totalShards && i < len(ec.nodes); i++ {
		go func(idx int) {
			backend := ec.backends[ec.nodes[idx]]
			shardKey := fmt.Sprintf("%s.shard.%d", key, idx)
			backend.Delete(ctx, bucket, shardKey)
		}(i)
	}

	return nil
}

func (ec *ECBackend) List(ctx context.Context, bucket, prefix, marker string, limit int) ([]miniobject.ObjectInfo, error) {
	backend := ec.backends[ec.nodes[0]]
	return backend.List(ctx, bucket, prefix, marker, limit)
}
