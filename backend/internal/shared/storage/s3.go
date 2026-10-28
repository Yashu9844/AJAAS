package storage

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

// S3Config configures the S3-compatible adapter (AWS S3, MinIO, and others).
type S3Config struct {
	Endpoint  string `mapstructure:"endpoint"` // empty for AWS; http://localhost:9000 for MinIO
	Region    string `mapstructure:"region"`
	Bucket    string `mapstructure:"bucket"`
	AccessKey string `mapstructure:"access_key"`
	SecretKey string `mapstructure:"secret_key"`
	PathStyle bool   `mapstructure:"path_style"` // required by MinIO
}

// S3 is the Store backed by an S3-compatible service.
type S3 struct {
	bucket  string
	client  *s3.Client
	presign *s3.PresignClient
}

// NewS3 builds the adapter. It does not contact the service; call EnsureBucket in development.
func NewS3(ctx context.Context, c S3Config) (*S3, error) {
	if c.Bucket == "" {
		return nil, errors.New("storage: bucket is required")
	}
	if c.Region == "" {
		c.Region = "us-east-1"
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(c.Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(c.AccessKey, c.SecretKey, "")))
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		if c.Endpoint != "" {
			o.BaseEndpoint = aws.String(c.Endpoint)
		}
		o.UsePathStyle = c.PathStyle
	})
	return &S3{bucket: c.Bucket, client: client, presign: s3.NewPresignClient(client)}, nil
}

// EnsureBucket creates the bucket when it is missing (development and tests; production buckets are provisioned).
func (s *S3) EnsureBucket(ctx context.Context) error {
	if _, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: &s.bucket}); err == nil {
		return nil
	}
	_, err := s.client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: &s.bucket})
	return err
}

func (s *S3) PresignPut(ctx context.Context, key, contentType string, size int64, sum string, ttl time.Duration) (PutTarget, error) {
	in := &s3.PutObjectInput{Bucket: &s.bucket, Key: &key, ContentType: &contentType, ContentLength: &size}
	headers := map[string]string{"Content-Type": contentType}
	if sum != "" {
		in.Metadata = map[string]string{"sha256": sum}
		headers["x-amz-meta-sha256"] = sum
	}
	r, err := s.presign.PresignPutObject(ctx, in, s3.WithPresignExpires(ttl))
	if err != nil {
		return PutTarget{}, fmt.Errorf("storage: presign put: %w", err)
	}
	return PutTarget{URL: r.URL, Method: r.Method, Headers: headers, Expires: time.Now().Add(ttl)}, nil
}

func (s *S3) PresignGet(ctx context.Context, key, filename string, ttl time.Duration) (string, error) {
	disp := fmt.Sprintf("attachment; filename=%q", filename)
	r, err := s.presign.PresignGetObject(ctx, &s3.GetObjectInput{Bucket: &s.bucket, Key: &key, ResponseContentDisposition: &disp}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", fmt.Errorf("storage: presign get: %w", err)
	}
	return r.URL, nil
}

func (s *S3) Head(ctx context.Context, key string) (ObjectInfo, error) {
	o, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: &s.bucket, Key: &key})
	if err != nil {
		var ae smithy.APIError
		if errors.As(err, &ae) && (ae.ErrorCode() == "NotFound" || ae.ErrorCode() == "NoSuchKey") {
			return ObjectInfo{}, ErrNotFound
		}
		var re interface{ HTTPStatusCode() int }
		if errors.As(err, &re) && re.HTTPStatusCode() == http.StatusNotFound {
			return ObjectInfo{}, ErrNotFound
		}
		return ObjectInfo{}, err
	}
	return ObjectInfo{Size: aws.ToInt64(o.ContentLength), ContentType: aws.ToString(o.ContentType), SHA256: o.Metadata["sha256"]}, nil
}

func (s *S3) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: &s.bucket, Key: &key})
	return err
}
