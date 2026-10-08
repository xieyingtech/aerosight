package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type S3Config struct {
	Endpoint, Region, Bucket, AccessKeyID, SecretAccessKey, SessionToken string
	ForcePathStyle                                                       bool
}

func LoadS3Config() (S3Config, error) {
	c := S3Config{Endpoint: strings.TrimSpace(os.Getenv("S3_ENDPOINT")), Region: strings.TrimSpace(os.Getenv("S3_REGION")), Bucket: strings.TrimSpace(os.Getenv("S3_BUCKET")), AccessKeyID: strings.TrimSpace(os.Getenv("S3_ACCESS_KEY_ID")), SecretAccessKey: os.Getenv("S3_SECRET_ACCESS_KEY"), SessionToken: os.Getenv("S3_SESSION_TOKEN"), ForcePathStyle: true}
	if raw := os.Getenv("S3_FORCE_PATH_STYLE"); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return c, errors.New("S3_FORCE_PATH_STYLE must be true or false")
		}
		c.ForcePathStyle = value
	}
	configured := c.Endpoint != "" || c.Bucket != "" || c.AccessKeyID != "" || c.SecretAccessKey != "" || c.Region != "" || c.SessionToken != ""
	if !configured {
		return c, nil
	}
	if c.Endpoint == "" || c.Bucket == "" || c.AccessKeyID == "" || c.SecretAccessKey == "" {
		return c, errors.New("S3_ENDPOINT, S3_BUCKET, S3_ACCESS_KEY_ID and S3_SECRET_ACCESS_KEY must be configured together")
	}
	u, err := url.Parse(c.Endpoint)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return c, errors.New("S3_ENDPOINT must be an HTTP(S) origin")
	}
	if c.Region == "" {
		c.Region = "us-east-1"
		if strings.HasPrefix(u.Hostname(), "oss-") && strings.HasSuffix(u.Hostname(), ".aliyuncs.com") {
			c.Region = strings.TrimSuffix(strings.TrimPrefix(u.Hostname(), "oss-"), ".aliyuncs.com")
		}
	}
	return c, nil
}

type S3ObjectStorage struct {
	client    *minio.Client
	bucket    string
	ossBucket *oss.Bucket
}

func NewConfiguredObjectStorage(root string, cfg S3Config) (ObjectStorage, error) {
	if cfg.Endpoint != "" {
		remote, err := NewS3ObjectStorage(cfg)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(root) == "" {
			return remote, nil
		}
		local, err := NewLocalObjectStorage(root)
		if err != nil {
			return nil, err
		}
		return &s3WithLocalFallback{S3ObjectStorage: remote, local: local, root: root}, nil
	}
	return NewLocalObjectStorage(root)
}

// Existing files remain readable when an installation switches to S3. All new
// writes go to S3; only a missing remote object falls back to the local volume.
type s3WithLocalFallback struct {
	*S3ObjectStorage
	local *LocalObjectStorage
	root  string
}

func missingS3Object(err error) bool {
	response := minio.ToErrorResponse(err)
	return response.Code == "NoSuchKey" || response.Code == "NoSuchObject"
}

func (s *s3WithLocalFallback) GetObject(ctx context.Context, key string) (Object, error) {
	object, err := s.S3ObjectStorage.GetObject(ctx, key)
	if err != nil && missingS3Object(err) {
		return s.local.GetObject(ctx, key)
	}
	return object, err
}

func (s *s3WithLocalFallback) OpenObject(ctx context.Context, key string) (io.ReadSeekCloser, error) {
	object, err := s.S3ObjectStorage.OpenObject(ctx, key)
	if err != nil && missingS3Object(err) {
		if !validObjectKey(key) {
			return nil, errors.New("invalid project object key")
		}
		parts := strings.Split(key, "/")
		pid, parseErr := strconv.ParseInt(parts[1], 10, 32)
		if parseErr != nil || pid <= 0 {
			return nil, errors.New("invalid project object key")
		}
		return OpenProjectObject(s.root, int32(pid), key)
	}
	return object, err
}

func NewS3ObjectStorage(cfg S3Config) (*S3ObjectStorage, error) {
	u, err := url.Parse(cfg.Endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || cfg.Bucket == "" || cfg.AccessKeyID == "" || cfg.SecretAccessKey == "" {
		return nil, errors.New("invalid S3 configuration")
	}
	lookup := minio.BucketLookupDNS
	if cfg.ForcePathStyle {
		lookup = minio.BucketLookupPath
	}
	options := &minio.Options{Creds: credentials.NewStaticV4(cfg.AccessKeyID, cfg.SecretAccessKey, cfg.SessionToken), Secure: u.Scheme == "https", Region: cfg.Region, BucketLookup: lookup}
	if strings.HasSuffix(u.Hostname(), ".aliyuncs.com") {
		options.BucketLookup = minio.BucketLookupDNS
		transport, transportErr := minio.DefaultTransport(u.Scheme == "https")
		if transportErr != nil {
			return nil, transportErr
		}
		options.Transport = ossCompatTransport{transport}
	}
	client, err := minio.New(u.Host, options)
	if err != nil {
		return nil, errors.New("invalid S3 configuration")
	}
	store := &S3ObjectStorage{client: client, bucket: cfg.Bucket}
	if strings.HasSuffix(u.Hostname(), ".aliyuncs.com") {
		ossClient, err := oss.New(cfg.Endpoint, cfg.AccessKeyID, cfg.SecretAccessKey, oss.SecurityToken(cfg.SessionToken))
		if err != nil {
			return nil, err
		}
		store.ossBucket, err = ossClient.Bucket(cfg.Bucket)
		if err != nil {
			return nil, err
		}
	}
	return store, nil
}

// Browser requests cannot attach the OSS S3 compatibility header. Use OSS's
// native URL signer there, and standard S3 query signing for other providers.
func (s *S3ObjectStorage) PresignRead(ctx context.Context, key string, ttl time.Duration) (*Access, error) {
	if !validObjectKey(key) || ttl < time.Second || ttl > 24*time.Hour {
		return nil, errors.New("invalid signed object access")
	}
	if _, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{}); err != nil {
		return nil, err
	}
	expires := time.Now().Add(ttl)
	var signed string
	var err error
	if s.ossBucket != nil {
		signed, err = s.ossBucket.SignURL(key, oss.HTTPGet, int64(ttl/time.Second))
	} else {
		var result *url.URL
		result, err = s.client.PresignedGetObject(ctx, s.bucket, key, ttl, nil)
		if err == nil {
			signed = result.String()
		}
	}
	if err != nil {
		return nil, err
	}
	return &Access{URL: signed, ExpiresAt: expires.UTC().Format("2006-01-02T15:04:05.000Z")}, nil
}

func (s *s3WithLocalFallback) PresignRead(ctx context.Context, key string, ttl time.Duration) (*Access, error) {
	access, err := s.S3ObjectStorage.PresignRead(ctx, key, ttl)
	if err != nil && missingS3Object(err) {
		return nil, nil
	}
	return access, err
}

type ossCompatTransport struct{ base http.RoundTripper }

func (t ossCompatTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	clone := r.Clone(r.Context())
	clone.Header = r.Header.Clone()
	clone.Header.Set("x-oss-s3-compat", "true")
	return t.base.RoundTrip(clone)
}

func validObjectKey(key string) bool {
	return strings.HasPrefix(key, "projects/") && path.Clean(key) == key && !strings.ContainsAny(key, "\\\x00")
}

func (s *S3ObjectStorage) OpenObject(ctx context.Context, key string) (io.ReadSeekCloser, error) {
	if !validObjectKey(key) {
		return nil, errors.New("invalid project object key")
	}
	object, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	if _, err = object.Stat(); err != nil {
		object.Close()
		return nil, err
	}
	return object, nil
}

func (s *S3ObjectStorage) GetObject(ctx context.Context, key string) (Object, error) {
	reader, err := s.OpenObject(ctx, key)
	if err != nil {
		return Object{}, err
	}
	defer reader.Close()
	body, err := io.ReadAll(io.LimitReader(reader, (128<<20)+1))
	if err != nil {
		return Object{}, err
	}
	if len(body) > 128<<20 {
		return Object{}, errors.New("object exceeds in-memory processing limit")
	}
	info, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return Object{}, err
	}
	return Object{Key: key, Body: body, ContentType: info.ContentType, ChecksumSHA256: checksum(body), VersionID: info.VersionID}, nil
}

func (s *S3ObjectStorage) PutObject(ctx context.Context, key string, reader io.Reader, contentType string) (Object, error) {
	if !validObjectKey(key) {
		return Object{}, errors.New("invalid project object key")
	}
	// Spool to disk so video uploads neither buffer entire files nor require a
	// caller-owned seekable stream. A known length also works with S3 providers
	// that do not support unsigned/chunked payloads.
	file, err := os.CreateTemp("", "aerosight-s3-*")
	if err != nil {
		return Object{}, err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(file, hash), reader)
	if err != nil {
		return Object{}, err
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return Object{}, err
	}
	info, err := s.client.PutObject(ctx, s.bucket, key, file, size, minio.PutObjectOptions{ContentType: contentType, DisableContentSha256: true, UserMetadata: map[string]string{"sha256": hex.EncodeToString(hash.Sum(nil))}})
	if err != nil {
		return Object{}, err
	}
	return Object{Key: key, ContentType: contentType, ChecksumSHA256: hex.EncodeToString(hash.Sum(nil)), VersionID: info.VersionID}, nil
}

// OpenStoredProjectObject keeps the same project boundary for S3 and local
// readers. S3's seekable reader serves HTTP ranges without loading a video.
func OpenStoredProjectObject(ctx context.Context, storage ObjectStorage, root string, pid int32, key string) (io.ReadSeekCloser, error) {
	if !validObjectKey(key) || !strings.HasPrefix(key, fmt.Sprintf("projects/%d/", pid)) {
		return nil, errors.New("invalid project object key")
	}
	if remote, ok := storage.(interface {
		OpenObject(context.Context, string) (io.ReadSeekCloser, error)
	}); ok {
		return remote.OpenObject(ctx, key)
	}
	return OpenProjectObject(root, pid, key)
}
