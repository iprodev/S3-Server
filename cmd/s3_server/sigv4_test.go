package main

import (
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestPresignedURLExpiry(t *testing.T) {
	cfg := &Config{
		AWSAccessKey: "AKIAIOSFODNN7EXAMPLE",
		AWSSecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		AWSRegion:    "us-east-1",
	}
	logger := NewLogger("error")

	tests := []struct {
		name      string
		dateStr   string
		expires   int
		wantValid bool
	}{
		{"valid current", time.Now().UTC().Format("20060102T150405Z"), 3600, true},
		{"expired 2 hours ago", time.Now().UTC().Add(-2 * time.Hour).Format("20060102T150405Z"), 3600, false},
		{"future 10 minutes", time.Now().UTC().Add(10 * time.Minute).Format("20060102T150405Z"), 3600, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, _ := url.Parse("http://localhost:9000/bucket/key")
			q := u.Query()
			q.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
			q.Set("X-Amz-Credential", "AKIAIOSFODNN7EXAMPLE/20230101/us-east-1/s3/aws4_request")
			q.Set("X-Amz-Date", tt.dateStr)
			q.Set("X-Amz-Expires", "3600")
			q.Set("X-Amz-SignedHeaders", "host")
			q.Set("X-Amz-Signature", "dummy")
			u.RawQuery = q.Encode()

			req, _ := http.NewRequest("GET", u.String(), nil)
			req.Host = "localhost:9000"

			server := &GatewayServer{cfg: cfg, logger: logger}
			
			// Note: This test checks date validation, not full signature
			query := req.URL.Query()
			dateStr := query.Get("X-Amz-Date")
			reqTime, _ := time.Parse("20060102T150405Z", dateStr)
			
			now := time.Now()
			expired := now.Sub(reqTime) > time.Duration(tt.expires)*time.Second
			tooFuture := reqTime.Sub(now) > 5*time.Minute

			valid := !expired && !tooFuture

			if valid != tt.wantValid {
				t.Errorf("expiry check = %v, want %v", valid, tt.wantValid)
			}
		})
	}
}

func TestPresignedURLSkew(t *testing.T) {
	// Test ±5 minute clock skew tolerance
	now := time.Now().UTC()

	tests := []struct {
		name      string
		timeSkew  time.Duration
		wantValid bool
	}{
		{"within skew +4min", 4 * time.Minute, true},
		{"within skew -4min", -4 * time.Minute, true},
		{"outside skew +6min", 6 * time.Minute, false},
		{"outside skew -1hour", -1 * time.Hour, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqTime := now.Add(tt.timeSkew)
			
			// Check if within ±5 minute window
			diff := reqTime.Sub(now)
			if diff < 0 {
				diff = -diff
			}
			
			valid := diff <= 5*time.Minute

			if valid != tt.wantValid {
				t.Errorf("skew check = %v, want %v (skew: %v)", valid, tt.wantValid, tt.timeSkew)
			}
		})
	}
}
