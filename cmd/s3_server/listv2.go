package main

import (
	"encoding/xml"
	"net/http"
	"strconv"
	"strings"
)

type ListBucketResult struct {
	XMLName               xml.Name       `xml:"ListBucketResult"`
	Name                  string         `xml:"Name"`
	Prefix                string         `xml:"Prefix,omitempty"`
	Delimiter             string         `xml:"Delimiter,omitempty"`
	MaxKeys               int            `xml:"MaxKeys"`
	IsTruncated           bool           `xml:"IsTruncated"`
	Contents              []Contents     `xml:"Contents"`
	CommonPrefixes        []CommonPrefix `xml:"CommonPrefixes,omitempty"`
	KeyCount              int            `xml:"KeyCount"`
	ContinuationToken     string         `xml:"ContinuationToken,omitempty"`
	NextContinuationToken string         `xml:"NextContinuationToken,omitempty"`
	StartAfter            string         `xml:"StartAfter,omitempty"`
}

type Contents struct {
	Key          string `xml:"Key"`
	LastModified string `xml:"LastModified"`
	ETag         string `xml:"ETag"`
	Size         int64  `xml:"Size"`
	StorageClass string `xml:"StorageClass"`
}

type CommonPrefix struct {
	Prefix string `xml:"Prefix"`
}

func (s *GatewayServer) handleListObjectsV2(w http.ResponseWriter, r *http.Request, bucket string) {
	query := r.URL.Query()
	prefix := query.Get("prefix")
	delimiter := query.Get("delimiter")
	maxKeysStr := query.Get("max-keys")
	continuationToken := query.Get("continuation-token")
	startAfter := query.Get("start-after")

	maxKeys := 1000
	if maxKeysStr != "" {
		if mk, err := strconv.Atoi(maxKeysStr); err == nil && mk > 0 {
			maxKeys = mk
		}
	}

	marker := continuationToken
	if marker == "" {
		marker = startAfter
	}

	// List from backend
	objects, err := s.backend.List(r.Context(), bucket, prefix, marker, maxKeys+1)
	if err != nil {
		s.logger.Error("list failed", "bucket", bucket, "error", err)
		s.writeS3Error(w, r, "InternalError", err.Error(), http.StatusInternalServerError)
		return
	}

	// Process results
	isTruncated := len(objects) > maxKeys
	if isTruncated {
		objects = objects[:maxKeys]
	}

	var contents []Contents
	var commonPrefixes []CommonPrefix
	prefixSet := make(map[string]bool)

	for _, obj := range objects {
		if delimiter != "" {
			// Check if key has delimiter after prefix
			remainder := strings.TrimPrefix(obj.Key, prefix)
			if idx := strings.Index(remainder, delimiter); idx >= 0 {
				// This is a "directory"
				commonPrefix := prefix + remainder[:idx+1]
				if !prefixSet[commonPrefix] {
					prefixSet[commonPrefix] = true
					commonPrefixes = append(commonPrefixes, CommonPrefix{Prefix: commonPrefix})
				}
				continue
			}
		}

		contents = append(contents, Contents{
			Key:          obj.Key,
			LastModified: obj.LastModified,
			ETag:         obj.ETag,
			Size:         obj.Size,
			StorageClass: "STANDARD",
		})
	}

	nextToken := ""
	if isTruncated && len(objects) > 0 {
		nextToken = objects[len(objects)-1].Key
	}

	result := ListBucketResult{
		Name:                  bucket,
		Prefix:                prefix,
		Delimiter:             delimiter,
		MaxKeys:               maxKeys,
		IsTruncated:           isTruncated,
		Contents:              contents,
		CommonPrefixes:        commonPrefixes,
		KeyCount:              len(contents) + len(commonPrefixes),
		ContinuationToken:     continuationToken,
		NextContinuationToken: nextToken,
		StartAfter:            startAfter,
	}

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	xml.NewEncoder(w).Encode(result)
}
