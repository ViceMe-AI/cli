package s3publish

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

type objectMeta struct {
	Size         int64
	ETag         string
	CacheControl string
	ContentType  string
	HasHeaders   bool
}

type listing struct {
	available bool
	objects   map[string]objectMeta
}

type objectStore interface {
	Head(ctx context.Context, bucket, key string) (objectMeta, bool, error)
	Get(ctx context.Context, bucket, key string) ([]byte, error)
	Put(ctx context.Context, bucket, key string, body []byte, cacheControl, contentType string) error
	Delete(ctx context.Context, bucket, key string) error
	List(ctx context.Context, bucket, prefix string) (listing, error)
}

type awsStore struct {
	client *s3.Client
	label  string
}

func (s *awsStore) Head(ctx context.Context, bucket, key string) (objectMeta, bool, error) {
	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isNotFound(err) {
			return objectMeta{}, false, nil
		}
		return objectMeta{}, false, storageErr(s.label, "head", key, err)
	}
	meta := objectMeta{HasHeaders: true}
	if out.ContentLength != nil {
		meta.Size = *out.ContentLength
	}
	if out.ETag != nil {
		meta.ETag = *out.ETag
	}
	if out.CacheControl != nil {
		meta.CacheControl = *out.CacheControl
	}
	if out.ContentType != nil {
		meta.ContentType = *out.ContentType
	}
	return meta, true, nil
}

func (s *awsStore) Get(ctx context.Context, bucket, key string) ([]byte, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isNotFound(err) {
			return nil, errNotFound
		}
		return nil, storageErr(s.label, "get", key, err)
	}
	defer out.Body.Close()
	data, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, storageErr(s.label, "get", key, err)
	}
	return data, nil
}

func (s *awsStore) Put(ctx context.Context, bucket, key string, body []byte, cacheControl, contentType string) error {
	input := &s3.PutObjectInput{
		Bucket:        aws.String(bucket),
		Key:           aws.String(key),
		Body:          bytes.NewReader(body),
		ContentLength: aws.Int64(int64(len(body))),
		CacheControl:  aws.String(cacheControl),
	}
	if contentType != "" {
		input.ContentType = aws.String(contentType)
	}
	_, err := s.client.PutObject(ctx, input)
	if err != nil {
		return storageErr(s.label, "put", key, err)
	}
	return nil
}

func (s *awsStore) Delete(ctx context.Context, bucket, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return storageErr(s.label, "delete", key, err)
	}
	return nil
}

func (s *awsStore) List(ctx context.Context, bucket, prefix string) (listing, error) {
	objects := make(map[string]objectMeta)
	var token *string
	for {
		out, err := s.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(bucket),
			Prefix:            aws.String(prefix),
			ContinuationToken: token,
		})
		if err != nil {
			if isAccessDenied(err) {
				return listing{available: false}, nil
			}
			return listing{}, storageErr(s.label, "list", prefix, err)
		}
		for _, item := range out.Contents {
			if item.Key == nil {
				continue
			}
			meta := objectMeta{}
			if item.Size != nil {
				meta.Size = *item.Size
			}
			if item.ETag != nil {
				meta.ETag = *item.ETag
			}
			objects[*item.Key] = meta
		}
		if aws.ToBool(out.IsTruncated) && out.NextContinuationToken != nil {
			token = out.NextContinuationToken
			continue
		}
		return listing{available: true, objects: objects}, nil
	}
}

var errNotFound = errors.New("object not found")

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errNotFound) {
		return true
	}
	var notFound *types.NotFound
	if errors.As(err, &notFound) {
		return true
	}
	var noSuchKey *types.NoSuchKey
	if errors.As(err, &noSuchKey) {
		return true
	}
	var api smithy.APIError
	if errors.As(err, &api) {
		switch api.ErrorCode() {
		case "NotFound", "NoSuchKey", "404":
			return true
		}
	}
	return httpStatus(err) == http.StatusNotFound
}

func isAccessDenied(err error) bool {
	if err == nil {
		return false
	}
	var api smithy.APIError
	if errors.As(err, &api) {
		switch api.ErrorCode() {
		case "AccessDenied", "Forbidden", "AllAccessDisabled":
			return true
		}
	}
	return httpStatus(err) == http.StatusForbidden
}

func httpStatus(err error) int {
	var response *awshttp.ResponseError
	if errors.As(err, &response) {
		return response.HTTPStatusCode()
	}
	return 0
}

func errorCode(err error) string {
	var api smithy.APIError
	if errors.As(err, &api) && api.ErrorCode() != "" {
		return api.ErrorCode()
	}
	if status := httpStatus(err); status != 0 {
		return fmt.Sprintf("http %d", status)
	}
	return "request failed"
}

func storageErr(label, op, key string, err error) error {
	return fmt.Errorf("%s %s %s failed: %s", label, op, key, errorCode(err))
}

func etagMatches(etag string, body []byte) bool {
	normalized := strings.TrimSpace(strings.ToLower(strings.Trim(etag, `"`)))
	if normalized == "" || strings.Contains(normalized, "-") {
		return false
	}
	sum := md5.Sum(body)
	return normalized == hex.EncodeToString(sum[:])
}

func headersMatch(meta objectMeta, item upload) bool {
	if !meta.HasHeaders {
		return false
	}
	return cacheControlMatch(meta.CacheControl, item.Cache) && contentTypeMatch(meta.ContentType, item.ContentType)
}

func cacheControlMatch(got, want string) bool {
	return canonicalHeader(got) == canonicalHeader(want)
}

func contentTypeMatch(got, want string) bool {
	got = canonicalHeader(got)
	want = canonicalHeader(want)
	if want == "" {
		return got == "" || got == "application/octet-stream" || got == "binary/octet-stream"
	}
	return got == want
}

func canonicalHeader(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.ReplaceAll(value, " ", "")
}
