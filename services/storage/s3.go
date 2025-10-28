package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3API defines the S3 operations used by S3Storage
// This interface allows for easier testing with mock implementations
type S3API interface {
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
	ListObjectsV2(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
	DeleteObject(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
}

// S3Storage implements the Storage interface using AWS S3 or S3-compatible storage
type S3Storage struct {
	client S3API
	bucket string
	region string
}

// S3Config holds configuration for S3 storage backend
type S3Config struct {
	Bucket          string
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	Endpoint        string // Optional: for S3-compatible services like MinIO
}

// NewS3Storage creates a new S3 storage backend with connection validation
func NewS3Storage(cfg S3Config) (*S3Storage, error) {
	if cfg.Bucket == "" {
		return nil, errors.New("S3 bucket is required")
	}
	if cfg.AccessKeyID == "" {
		return nil, errors.New("S3 access key ID is required")
	}
	if cfg.SecretAccessKey == "" {
		return nil, errors.New("S3 secret access key is required")
	}

	ctx := context.Background()

	// Build AWS config with credentials
	awsCfg, err := buildAWSConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to build AWS config: %w", err)
	}

	// Create S3 client
	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		// Configure custom endpoint if provided (for MinIO, LocalStack, etc.)
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
			// Use path-style addressing for S3-compatible services
			o.UsePathStyle = true
		}
	})

	storage := &S3Storage{
		client: s3Client,
		bucket: cfg.Bucket,
		region: cfg.Region,
	}

	// Validate connection by attempting to list objects (head bucket operation)
	if err := storage.validateConnection(); err != nil {
		return nil, fmt.Errorf("failed to validate S3 connection: %w", err)
	}

	return storage, nil
}

// buildAWSConfig constructs AWS SDK configuration with credentials and retry policy
func buildAWSConfig(ctx context.Context, cfg S3Config) (aws.Config, error) {
	// Create credentials provider
	credsProvider := credentials.NewStaticCredentialsProvider(
		cfg.AccessKeyID,
		cfg.SecretAccessKey,
		"", // Session token (empty for static credentials)
	)

	// Load config with credentials and retry configuration
	var awsCfg aws.Config
	var err error

	if cfg.Region != "" {
		awsCfg, err = config.LoadDefaultConfig(ctx,
			config.WithRegion(cfg.Region),
			config.WithCredentialsProvider(credsProvider),
		)
	} else {
		// If no region specified, use a default (required by AWS SDK)
		// For S3-compatible services like MinIO, region is often not important
		awsCfg, err = config.LoadDefaultConfig(ctx,
			config.WithRegion("us-east-1"),
			config.WithCredentialsProvider(credsProvider),
		)
	}

	if err != nil {
		return aws.Config{}, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// AWS SDK v2 has built-in retry logic with exponential backoff
	// Default: 3 attempts with exponential backoff
	// We rely on this built-in mechanism for transient failure recovery

	return awsCfg, nil
}

// validateConnection validates that we can connect to the S3 bucket
func (s *S3Storage) validateConnection() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Attempt to list objects with a limit of 1 to verify bucket access
	_, err := s.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket:  aws.String(s.bucket),
		MaxKeys: aws.Int32(1),
	})

	if err != nil {
		return fmt.Errorf("S3 connection validation failed: %w", err)
	}

	return nil
}

// Create creates a new object in S3 with version metadata
func (s *S3Storage) Create(path string, content []byte) error {
	ctx := context.Background()

	// Check if object already exists
	exists, err := s.Exists(path)
	if err != nil {
		return fmt.Errorf("failed to check if object exists: %w", err)
	}
	if exists {
		return fmt.Errorf("object already exists: %s", path)
	}

	// Upload object with version metadata
	metadata := map[string]string{
		"version": "1",
	}

	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:   aws.String(s.bucket),
		Key:      aws.String(path),
		Body:     bytes.NewReader(content),
		Metadata: metadata,
	})

	if err != nil {
		return fmt.Errorf("failed to create object in S3: %w", err)
	}

	return nil
}

// Read retrieves the content of an object from S3
func (s *S3Storage) Read(path string) ([]byte, error) {
	ctx := context.Background()

	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(path),
	})

	if err != nil {
		var notFound *types.NoSuchKey
		if errors.As(err, &notFound) {
			return nil, fmt.Errorf("object not found: %s", path)
		}
		return nil, fmt.Errorf("failed to read object from S3: %w", err)
	}
	defer result.Body.Close()

	// Read object content
	data, err := io.ReadAll(result.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read object body: %w", err)
	}

	return data, nil
}

// Write updates an existing object in S3 with optimistic locking
func (s *S3Storage) Write(path string, content []byte, version int) error {
	ctx := context.Background()

	// Get current version from object metadata
	currentVersion, err := s.GetVersion(path)
	if err != nil {
		// If object doesn't exist, treat as version 0 (new object)
		var nsk *types.NoSuchKey
		if errors.As(err, &nsk) {
			return fmt.Errorf("failed to get current version: %w", err)
		}
		currentVersion = 0
	}

	// Validate version (optimistic locking)
	if version != currentVersion+1 {
		return fmt.Errorf("version mismatch: expected %d, got %d", currentVersion+1, version)
	}

	// Upload object with updated version metadata
	metadata := map[string]string{
		"version": strconv.Itoa(version),
	}

	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:   aws.String(s.bucket),
		Key:      aws.String(path),
		Body:     bytes.NewReader(content),
		Metadata: metadata,
	})

	if err != nil {
		return fmt.Errorf("failed to write object to S3: %w", err)
	}

	return nil
}

// Exists checks if an object exists in S3
func (s *S3Storage) Exists(path string) (bool, error) {
	ctx := context.Background()

	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(path),
	})

	if err != nil {
		var notFound *types.NotFound
		if errors.As(err, &notFound) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check object existence: %w", err)
	}

	return true, nil
}

// List returns a list of object keys with the given prefix
func (s *S3Storage) List(prefix string) ([]string, error) {
	ctx := context.Background()

	var results []string
	var continuationToken *string

	// S3 ListObjectsV2 returns paginated results
	for {
		input := &s3.ListObjectsV2Input{
			Bucket: aws.String(s.bucket),
		}

		if prefix != "" {
			input.Prefix = aws.String(prefix)
		}

		if continuationToken != nil {
			input.ContinuationToken = continuationToken
		}

		output, err := s.client.ListObjectsV2(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("failed to list objects in S3: %w", err)
		}

		// Collect object keys
		for _, obj := range output.Contents {
			if obj.Key != nil {
				// Skip version metadata files (not applicable in S3, but keep pattern consistent)
				if !strings.HasSuffix(*obj.Key, ".version") {
					results = append(results, *obj.Key)
				}
			}
		}

		// Check if there are more results
		if !aws.ToBool(output.IsTruncated) {
			break
		}

		continuationToken = output.NextContinuationToken
	}

	return results, nil
}

// getVersion retrieves the version metadata for an object
// GetVersion retrieves the current version of an object from S3 metadata
func (s *S3Storage) GetVersion(path string) (int, error) {
	ctx := context.Background()

	result, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(path),
	})

	if err != nil {
		return 0, err
	}

	versionStr, ok := result.Metadata["version"]
	if !ok {
		// If no version metadata, assume version 1 (backward compatibility)
		return 1, nil
	}

	version, err := strconv.Atoi(versionStr)
	if err != nil {
		return 0, fmt.Errorf("invalid version format: %w", err)
	}

	return version, nil
}

// Verify S3Storage implements Storage interface
var _ Storage = (*S3Storage)(nil)
