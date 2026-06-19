package storage

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Client struct {
	mc       *minio.Client
	bucket   string
	endpoint string
}

// Stats holds aggregate information about the storage bucket.
type Stats struct {
	Endpoint    string
	Bucket      string
	Connected   bool
	ObjectCount int64
	TotalBytes  int64
}

func New(endpoint, accessKey, secretKey, bucket string) (*Client, error) {
	mc, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: false,
	})
	if err != nil {
		return nil, fmt.Errorf("minio client: %w", err)
	}
	return &Client{mc: mc, bucket: bucket, endpoint: endpoint}, nil
}

// BucketStats returns connection status and aggregate object count/size.
func (c *Client) BucketStats(ctx context.Context) Stats {
	s := Stats{Endpoint: c.endpoint, Bucket: c.bucket}
	exists, err := c.mc.BucketExists(ctx, c.bucket)
	if err != nil || !exists {
		return s
	}
	s.Connected = true
	for obj := range c.mc.ListObjects(ctx, c.bucket, minio.ListObjectsOptions{Recursive: true}) {
		if obj.Err != nil {
			break
		}
		s.ObjectCount++
		s.TotalBytes += obj.Size
	}
	return s
}

func (c *Client) EnsureBucket(ctx context.Context) error {
	exists, err := c.mc.BucketExists(ctx, c.bucket)
	if err != nil {
		return fmt.Errorf("bucket check: %w", err)
	}
	if exists {
		return nil
	}
	return c.mc.MakeBucket(ctx, c.bucket, minio.MakeBucketOptions{})
}

func (c *Client) Upload(ctx context.Context, objectKey string, reader io.Reader, size int64, contentType string) error {
	_, err := c.mc.PutObject(ctx, c.bucket, objectKey, reader, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	return err
}

func (c *Client) Delete(ctx context.Context, objectKey string) error {
	return c.mc.RemoveObject(ctx, c.bucket, objectKey, minio.RemoveObjectOptions{})
}

// PresignedURL returns a time-limited URL for the client to download an object directly.
func (c *Client) PresignedURL(ctx context.Context, objectKey string) (string, error) {
	u, err := c.mc.PresignedGetObject(ctx, c.bucket, objectKey, 15*time.Minute, nil)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}
