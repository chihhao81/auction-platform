package storage

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// S3 stores objects in an S3-compatible bucket such as Cloudflare R2 (10 GB
// free, no egress fees). Objects are served to browsers from PublicBaseURL
// (an R2 custom domain or r2.dev URL), so images never pass through the VM.
type S3 struct {
	client        *minio.Client
	bucket        string
	publicBaseURL string
}

// S3Config holds the bucket settings. For R2 the endpoint is
// "<account-id>.r2.cloudflarestorage.com" and the region "auto".
type S3Config struct {
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
	Bucket          string
	Region          string
	PublicBaseURL   string
	Insecure        bool // plain HTTP, tests only
}

func NewS3(c S3Config) (*S3, error) {
	if c.Endpoint == "" || c.AccessKeyID == "" || c.SecretAccessKey == "" || c.Bucket == "" || c.PublicBaseURL == "" {
		return nil, fmt.Errorf("s3 storage: endpoint, credentials, bucket and public base URL are required")
	}
	region := c.Region
	if region == "" {
		region = "auto"
	}
	client, err := minio.New(c.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(c.AccessKeyID, c.SecretAccessKey, ""),
		Secure: !c.Insecure,
		Region: region, // a fixed region avoids an extra bucket-location request
	})
	if err != nil {
		return nil, fmt.Errorf("s3 client: %w", err)
	}
	return &S3{client: client, bucket: c.Bucket, publicBaseURL: strings.TrimRight(c.PublicBaseURL, "/")}, nil
}

func (s *S3) Put(ctx context.Context, key string, data []byte, contentType string) error {
	if !ValidKey(key) {
		return fmt.Errorf("invalid storage key %q", key)
	}
	_, err := s.client.PutObject(ctx, s.bucket, key, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{
		ContentType:  contentType,
		CacheControl: "public, max-age=31536000, immutable", // keys are never reused
		// Plain single-request upload (no aws-chunked streaming signature):
		// widely supported by S3-compatible stores and cheaper on CPU. TLS
		// protects the payload in transit.
		DisableContentSha256: true,
	})
	if err != nil {
		return fmt.Errorf("s3 put %s: %w", key, err)
	}
	return nil
}

func (s *S3) Delete(ctx context.Context, key string) error {
	if !ValidKey(key) {
		return fmt.Errorf("invalid storage key %q", key)
	}
	if err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("s3 delete %s: %w", key, err)
	}
	return nil
}

func (s *S3) URL(key string) string { return s.publicBaseURL + "/" + key }
