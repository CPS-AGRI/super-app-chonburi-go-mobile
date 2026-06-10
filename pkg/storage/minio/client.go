package minio

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	appconfig "super-app-chonburi-go-mobile/config"

	miniosdk "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Client struct {
	S3            *miniosdk.Client
	Bucket        string
	Region        string
	PublicRead    bool
	PublicBaseURL string
	PresignURLTTL time.Duration
}

func NewClient(cfg appconfig.MinIOConfig) (*Client, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("minio endpoint is required")
	}
	if cfg.AccessKey == "" {
		return nil, fmt.Errorf("minio access key is required")
	}
	if cfg.SecretKey == "" {
		return nil, fmt.Errorf("minio secret key is required")
	}
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("minio bucket is required")
	}
	if cfg.PresignURLTTL <= 0 {
		cfg.PresignURLTTL = 3600
	}

	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	s3, err := miniosdk.New(cfg.Endpoint, &miniosdk.Options{
		Creds:     credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure:    cfg.Secure,
		Region:    cfg.Region,
		Transport: transport,
	})
	if err != nil {
		return nil, err
	}

	return &Client{
		S3:            s3,
		Bucket:        cfg.Bucket,
		Region:        cfg.Region,
		PublicRead:    cfg.PublicRead,
		PublicBaseURL: strings.TrimRight(cfg.PublicBaseURL, "/"),
		PresignURLTTL: time.Duration(cfg.PresignURLTTL) * time.Second,
	}, nil
}

func (c *Client) BucketExists(ctx context.Context) (bool, error) {
	return c.S3.BucketExists(ctx, c.Bucket)
}

func (c *Client) PutObject(ctx context.Context, objectKey string, reader io.Reader, size int64, contentType string) (miniosdk.UploadInfo, error) {
	return c.S3.PutObject(ctx, c.Bucket, objectKey, reader, size, miniosdk.PutObjectOptions{
		ContentType: contentType,
	})
}

func (c *Client) PresignedGetURL(ctx context.Context, objectKey string) (string, error) {
	values := make(url.Values)
	presignedURL, err := c.S3.PresignedGetObject(ctx, c.Bucket, objectKey, c.PresignURLTTL, values)
	if err != nil {
		return "", err
	}

	return presignedURL.String(), nil
}

func (c *Client) ObjectURL(ctx context.Context, objectKey string) (string, error) {
	if c.PublicRead && c.PublicBaseURL != "" {
		publicURL, err := url.JoinPath(c.PublicBaseURL, c.Bucket, objectKey)
		if err != nil {
			return "", err
		}

		return publicURL, nil
	}

	return c.PresignedGetURL(ctx, objectKey)
}
