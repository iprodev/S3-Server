package miniobject

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// HTTPBackend implements Backend interface using HTTP calls to nodes
type HTTPBackend struct {
	baseURL   string
	authToken string
	client    *http.Client
}

// NewHTTPBackend creates a new HTTP backend
func NewHTTPBackend(baseURL, authToken string) *HTTPBackend {
	return &HTTPBackend{
		baseURL:   baseURL,
		authToken: authToken,
		client: &http.Client{
			Timeout: 60 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

func (h *HTTPBackend) Put(ctx context.Context, bucket, key string, r io.Reader, contentType, contentMD5 string) (string, error) {
	url := fmt.Sprintf("%s/%s/%s", h.baseURL, bucket, key)

	req, err := http.NewRequestWithContext(ctx, "PUT", url, r)
	if err != nil {
		return "", err
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if contentMD5 != "" {
		req.Header.Set("Content-MD5", contentMD5)
	}
	if h.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+h.authToken)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("PUT failed: %d %s", resp.StatusCode, string(body))
	}

	etag := resp.Header.Get("ETag")
	return etag, nil
}

func (h *HTTPBackend) Get(ctx context.Context, bucket, key string, rangeSpec string) (io.ReadCloser, string, string, int64, int, error) {
	url := fmt.Sprintf("%s/%s/%s", h.baseURL, bucket, key)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, "", "", 0, 500, err
	}

	if rangeSpec != "" {
		req.Header.Set("Range", rangeSpec)
	}
	if h.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+h.authToken)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, "", "", 0, 500, err
	}

	if resp.StatusCode == 404 {
		resp.Body.Close()
		return nil, "", "", 0, 404, errors.New("NoSuchKey")
	}

	if resp.StatusCode != 200 && resp.StatusCode != 206 {
		resp.Body.Close()
		return nil, "", "", 0, resp.StatusCode, fmt.Errorf("GET failed: %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	etag := resp.Header.Get("ETag")
	size := resp.ContentLength

	return resp.Body, contentType, etag, size, resp.StatusCode, nil
}

func (h *HTTPBackend) Head(ctx context.Context, bucket, key string) (string, string, int64, bool, error) {
	url := fmt.Sprintf("%s/%s/%s", h.baseURL, bucket, key)

	req, err := http.NewRequestWithContext(ctx, "HEAD", url, nil)
	if err != nil {
		return "", "", 0, false, err
	}

	if h.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+h.authToken)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return "", "", 0, false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return "", "", 0, false, nil
	}

	if resp.StatusCode != 200 {
		return "", "", 0, false, fmt.Errorf("HEAD failed: %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	etag := resp.Header.Get("ETag")
	size := resp.ContentLength

	return contentType, etag, size, true, nil
}

func (h *HTTPBackend) Delete(ctx context.Context, bucket, key string) error {
	url := fmt.Sprintf("%s/%s/%s", h.baseURL, bucket, key)

	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return err
	}

	if h.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+h.authToken)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 204 {
		return fmt.Errorf("DELETE failed: %d", resp.StatusCode)
	}

	return nil
}

func (h *HTTPBackend) List(ctx context.Context, bucket, prefix, marker string, limit int) ([]ObjectInfo, error) {
	url := fmt.Sprintf("%s/%s?list=1&prefix=%s&marker=%s&limit=%d",
		h.baseURL, bucket, prefix, marker, limit)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	if h.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+h.authToken)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("LIST failed: %d", resp.StatusCode)
	}

	var results []ObjectInfo
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, err
	}

	return results, nil
}
