package artifact

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	appconfig "github.com/gongahkia/norbot/internal/config"
)

const MaxObjectBytes = 10 << 20

type Object struct {
	Key         string
	ContentType string
	Size        int64
	Digest      string
}

type Store interface {
	Put(context.Context, Object, []byte) (Object, error)
	Open(context.Context, string) (io.ReadCloser, Object, error)
	Delete(context.Context, string) error
}

type S3Store struct {
	client *s3.Client
	bucket string
}

func New(cfg appconfig.ArtifactStore) (Store, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	accessKey := strings.TrimSpace(os.Getenv(cfg.AccessKeyEnv))
	secretKey := strings.TrimSpace(os.Getenv(cfg.SecretKeyEnv))
	if accessKey == "" || secretKey == "" {
		return nil, fmt.Errorf("artifact credentials are unavailable from configured environment references")
	}
	region := cfg.Region
	if region == "" {
		region = "us-east-1"
	}
	loaded, err := awsconfig.LoadDefaultConfig(context.Background(), awsconfig.WithRegion(region), awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")))
	if err != nil {
		return nil, fmt.Errorf("load S3 configuration: %w", err)
	}
	client := s3.NewFromConfig(loaded, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(cfg.Endpoint)
		options.UsePathStyle = cfg.ForcePathStyle
	})
	return &S3Store{client: client, bucket: cfg.Bucket}, nil
}

func (s *S3Store) Put(ctx context.Context, object Object, content []byte) (Object, error) {
	if err := validObject(object, content); err != nil {
		return Object{}, err
	}
	digest := sha256.Sum256(content)
	object.Size = int64(len(content))
	object.Digest = fmt.Sprintf("sha256:%x", digest)
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(object.Key), Body: bytes.NewReader(content), ContentLength: aws.Int64(object.Size), ContentType: aws.String(object.ContentType)})
	if err != nil {
		return Object{}, fmt.Errorf("put artifact object: %w", err)
	}
	return object, nil
}

func (s *S3Store) Open(ctx context.Context, key string) (io.ReadCloser, Object, error) {
	if err := validKey(key); err != nil {
		return nil, Object{}, err
	}
	output, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	if err != nil {
		return nil, Object{}, fmt.Errorf("get artifact object: %w", err)
	}
	contentType := "application/octet-stream"
	if output.ContentType != nil && *output.ContentType != "" {
		contentType = *output.ContentType
	}
	return output.Body, Object{Key: key, ContentType: contentType, Size: aws.ToInt64(output.ContentLength)}, nil
}

func (s *S3Store) Delete(ctx context.Context, key string) error {
	if err := validKey(key); err != nil {
		return err
	}
	if _, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)}); err != nil {
		return fmt.Errorf("delete artifact object: %w", err)
	}
	return nil
}

func Key(parts ...string) (string, error) {
	clean := make([]string, 0, len(parts)+1)
	clean = append(clean, "norbot")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "." || part == ".." || strings.Contains(part, "/") || strings.Contains(part, "\\") || strings.Contains(part, "\x00") {
			return "", fmt.Errorf("invalid artifact key component")
		}
		clean = append(clean, part)
	}
	key := path.Join(clean...)
	if err := validKey(key); err != nil {
		return "", err
	}
	return key, nil
}

func validObject(object Object, content []byte) error {
	if err := validKey(object.Key); err != nil {
		return err
	}
	if len(content) > MaxObjectBytes {
		return fmt.Errorf("artifact exceeds 10 MiB")
	}
	if strings.TrimSpace(object.ContentType) == "" || strings.ContainsAny(object.ContentType, "\r\n") {
		return fmt.Errorf("invalid artifact content type")
	}
	return nil
}

func validKey(key string) error {
	if key == "" || strings.HasPrefix(key, "/") || strings.Contains(key, "\\") || strings.Contains(key, "\x00") || path.Clean(key) != key || !strings.HasPrefix(key, "norbot/") {
		return fmt.Errorf("invalid artifact key")
	}
	return nil
}
