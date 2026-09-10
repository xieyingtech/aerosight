package flighthub

import (
	"context"
	"errors"
	"io"
	"net/url"
	"strings"

	"github.com/minio/minio-go/v7"
	miniocredentials "github.com/minio/minio-go/v7/pkg/credentials"
)

type MinIOWaylineObjectUploader struct{}

func NewMinIOWaylineObjectUploader() *MinIOWaylineObjectUploader {
	return &MinIOWaylineObjectUploader{}
}

func (uploader *MinIOWaylineObjectUploader) Upload(
	ctx context.Context,
	sts StorageSTS,
	objectKey string,
	body io.Reader,
	size int64,
	contentType string,
) error {
	if uploader == nil || body == nil || size <= 0 || strings.TrimSpace(sts.Bucket) == "" ||
		strings.TrimSpace(sts.Region) == "" || strings.TrimSpace(objectKey) == "" ||
		!strings.HasPrefix(objectKey, strings.TrimSuffix(strings.TrimSpace(sts.ObjectKeyPrefix), "/")+"/") {
		return errors.New("FlightHub controlled object upload input is invalid")
	}
	endpoint, err := url.Parse(strings.TrimSpace(sts.Endpoint))
	if err != nil || endpoint.Scheme != "https" || endpoint.Host == "" || endpoint.User != nil ||
		endpoint.Path != "" || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return errors.New("FlightHub object storage endpoint is invalid")
	}
	client, err := minio.New(endpoint.Host, &minio.Options{
		Creds: miniocredentials.NewStaticV4(
			sts.Credentials.AccessKeyID,
			sts.Credentials.AccessKeySecret,
			sts.Credentials.SecurityToken,
		),
		Secure: true,
		Region: sts.Region,
	})
	if err != nil {
		return errors.New("FlightHub object storage client initialization failed")
	}
	_, err = client.PutObject(ctx, sts.Bucket, objectKey, body, size, minio.PutObjectOptions{
		ContentType: strings.TrimSpace(contentType),
	})
	if err != nil {
		return errors.New("FlightHub controlled object upload failed")
	}
	return nil
}
