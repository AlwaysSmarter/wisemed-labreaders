package siuihistory

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type objectStore interface {
	Put(context.Context, string, io.Reader, int64) error
	Open(context.Context, string, int64) (io.ReadCloser, error)
	Delete(context.Context, string) error
}
type localStore string

func (s localStore) path(k string) (string, error) {
	if strings.Contains(k, "\\") || filepath.IsAbs(k) || strings.Contains(k, "..") {
		return "", errors.New("invalid object key")
	}
	return filepath.Join(string(s), filepath.FromSlash(k)), nil
}
func (s localStore) Put(ctx context.Context, k string, r io.Reader, n int64) error {
	p, e := s.path(k)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(p), 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(p), ".upload-")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	written, e := io.Copy(f, r)
	if e == nil && written != n {
		e = io.ErrUnexpectedEOF
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	return os.Rename(f.Name(), p)
}
func (s localStore) Open(ctx context.Context, k string, offset int64) (io.ReadCloser, error) {
	p, e := s.path(k)
	if e != nil {
		return nil, e
	}
	f, e := os.Open(p)
	if e != nil {
		return nil, e
	}
	if _, e = f.Seek(offset, io.SeekStart); e != nil {
		f.Close()
		return nil, e
	}
	return f, nil
}
func (s localStore) Delete(ctx context.Context, k string) error {
	p, e := s.path(k)
	if e != nil {
		return e
	}
	return os.Remove(p)
}

type s3Store struct {
	client         *minio.Client
	bucket, prefix string
}

func (s s3Store) Put(ctx context.Context, k string, r io.Reader, n int64) error {
	_, e := s.client.PutObject(ctx, s.bucket, s.prefix+k, r, n, minio.PutObjectOptions{ContentType: "application/octet-stream"})
	return e
}
func (s s3Store) Open(ctx context.Context, k string, offset int64) (io.ReadCloser, error) {
	opts := minio.GetObjectOptions{}
	if offset > 0 {
		if e := opts.SetRange(offset, 0); e != nil {
			return nil, e
		}
	}
	o, e := s.client.GetObject(ctx, s.bucket, s.prefix+k, opts)
	if e != nil {
		return nil, e
	}
	if _, e = o.Stat(); e != nil {
		o.Close()
		return nil, e
	}
	return o, nil
}
func (s s3Store) Delete(ctx context.Context, k string) error {
	return s.client.RemoveObject(ctx, s.bucket, s.prefix+k, minio.RemoveObjectOptions{})
}
func (m *Module) store() (objectStore, error) {
	c := m.cfg
	if c.Storage == "local" {
		if c.BasePath == "" {
			return nil, errors.New("local base path required")
		}
		return localStore(m.rt.ResolvePath(c.BasePath)), nil
	}
	if c.Storage != "s3" || c.Bucket == "" {
		return nil, errors.New("configure local or s3 storage")
	}
	endpoint := c.Endpoint
	if endpoint == "" {
		endpoint = "s3.amazonaws.com"
	}
	client, e := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(c.AccessKey, c.SecretKey, c.SessionToken), Secure: !c.S3Insecure, Region: c.Region})
	if e != nil {
		return nil, e
	}
	prefix := strings.Trim(c.Prefix, "/")
	if prefix != "" {
		prefix += "/"
	}
	return s3Store{client, c.Bucket, prefix}, nil
}
