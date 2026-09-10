package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// High-level Store methods on *Client. These let callers depend on the
// storage.Store interface so the same code works against AWS S3, MinIO,
// Cloudflare R2, Backblaze B2, Hetzner Object Storage, or the filesystem
// backend without conditional logic.
//
// The lower-level Client.GetObject / PutObject / etc. (via embedded *s3.Client)
// remain available for existing call sites that haven't been migrated yet.

func (c *Client) Name() string { return "s3" }

func (c *Client) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := c.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var nsk *types.NoSuchKey
		if errors.As(err, &nsk) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return out.Body, nil
}

func (c *Client) Put(ctx context.Context, key string, body io.Reader, contentType string) error {
	in := &s3.PutObjectInput{
		Bucket: aws.String(c.Bucket),
		Key:    aws.String(key),
		Body:   body,
	}
	if contentType != "" {
		in.ContentType = aws.String(contentType)
	}
	_, err := c.PutObject(ctx, in)
	return err
}

// PutPublic writes a public-read object with a long immutable cache and
// returns its public URL. Used for avatars and org logos.
func (c *Client) PutPublic(ctx context.Context, key string, body io.Reader, contentType string) (string, error) {
	in := &s3.PutObjectInput{
		Bucket:       aws.String(c.Bucket),
		Key:          aws.String(key),
		Body:         body,
		CacheControl: aws.String("public, max-age=31536000, immutable"),
		ACL:          types.ObjectCannedACLPublicRead,
	}
	if contentType != "" {
		in.ContentType = aws.String(contentType)
	}
	if _, err := c.PutObject(ctx, in); err != nil {
		return "", err
	}
	if c.PublicBaseURL != "" {
		return strings.TrimRight(c.PublicBaseURL, "/") + "/" + key, nil
	}
	return fmt.Sprintf("https://%s.s3.amazonaws.com/%s", c.Bucket, key), nil
}

func (c *Client) Delete(ctx context.Context, key string) error {
	_, err := c.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(c.Bucket),
		Key:    aws.String(key),
	})
	return err
}

func (c *Client) Has(ctx context.Context, key string) (bool, error) {
	return c.Exists(ctx, c.Bucket, key)
}

func (c *Client) PresignedGetURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	return c.PresignedURL(ctx, PresignOpGet, key, "", ttl)
}

// PresignedURL signs one operation on one key. This is what lets a worker hold
// no bucket credentials at all: the control plane signs, the node sends the
// bytes straight to the store, and nothing is proxied.
//
// A signature covers the verb, so a URL minted for a read cannot be used to
// overwrite.
func (c *Client) PresignedURL(ctx context.Context, op PresignOp, key, contentType string, ttl time.Duration) (string, error) {
	ps := s3.NewPresignClient(c.Client)
	expires := s3.WithPresignExpires(ttl)

	switch op {
	case PresignOpGet:
		out, err := ps.PresignGetObject(ctx, &s3.GetObjectInput{
			Bucket: aws.String(c.Bucket),
			Key:    aws.String(key),
		}, expires)
		if err != nil {
			return "", err
		}
		return out.URL, nil

	case PresignOpPut:
		in := &s3.PutObjectInput{
			Bucket: aws.String(c.Bucket),
			Key:    aws.String(key),
		}
		// Signed when present, so the sender must send the same header back.
		// Left unsigned when empty rather than defaulted, which would make
		// every unspecified upload fail the signature check.
		if contentType != "" {
			in.ContentType = aws.String(contentType)
		}
		out, err := ps.PresignPutObject(ctx, in, expires)
		if err != nil {
			return "", err
		}
		return out.URL, nil

	case PresignOpHead:
		out, err := ps.PresignHeadObject(ctx, &s3.HeadObjectInput{
			Bucket: aws.String(c.Bucket),
			Key:    aws.String(key),
		}, expires)
		if err != nil {
			return "", err
		}
		return out.URL, nil

	case PresignOpDelete:
		out, err := ps.PresignDeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(c.Bucket),
			Key:    aws.String(key),
		}, expires)
		if err != nil {
			return "", err
		}
		return out.URL, nil

	default:
		return "", fmt.Errorf("storage: cannot presign unknown op %q", op)
	}
}
