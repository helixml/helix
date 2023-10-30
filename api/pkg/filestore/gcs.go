package filestore

import (
	"context"
	"fmt"
	"io"
	"strings"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

type GCSStorage struct {
	client *storage.Client
	bucket *storage.BucketHandle
}

func NewGCSStorage(ctx context.Context, bucketName, serviceAccountKeyFile string) (*GCSStorage, error) {
	client, err := storage.NewClient(ctx, option.WithCredentialsFile(serviceAccountKeyFile))
	if err != nil {
		return nil, fmt.Errorf("failed to create storage client: %w", err)
	}

	return &GCSStorage{
		client: client,
		bucket: client.Bucket(bucketName),
	}, nil
}

func (s *GCSStorage) List(ctx context.Context, prefix string) ([]FileStoreItem, error) {
	it := s.bucket.Objects(ctx, &storage.Query{Prefix: prefix})
	items := []FileStoreItem{}

	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error iterating over GCS objects: %w", err)
		}

		item := FileStoreItem{
			Directory: strings.HasSuffix(attrs.Name, "/"),
			Name:      attrs.Name,
			Path:      attrs.Name,
			URL:       attrs.MediaLink,
			Created:   attrs.Created.Unix(),
			Size:      attrs.Size,
		}
		items = append(items, item)
	}

	return items, nil
}

func (s *GCSStorage) Get(ctx context.Context, path string) (FileStoreItem, error) {
	attrs, err := s.bucket.Object(path).Attrs(ctx)
	if err != nil {
		return FileStoreItem{}, fmt.Errorf("error fetching GCS object attributes: %w", err)
	}

	return FileStoreItem{
		Directory: strings.HasSuffix(attrs.Name, "/"),
		Name:      attrs.Name,
		Path:      attrs.Name,
		URL:       attrs.MediaLink,
		Created:   attrs.Created.Unix(),
		Size:      attrs.Size,
	}, nil
}

func (s *GCSStorage) Upload(ctx context.Context, path string, r io.Reader) (FileStoreItem, error) {
	obj := s.bucket.Object(path)
	writer := obj.NewWriter(ctx)
	if _, err := io.Copy(writer, r); err != nil {
		return FileStoreItem{}, fmt.Errorf("failed to copy content to GCS: %w", err)
	}
	if err := writer.Close(); err != nil {
		return FileStoreItem{}, fmt.Errorf("failed to finalize GCS object upload: %w", err)
	}
	attrs, err := obj.Attrs(ctx)
	if err != nil {
		return FileStoreItem{}, fmt.Errorf("error fetching GCS object attributes after upload: %w", err)
	}
	return FileStoreItem{
		Directory: strings.HasSuffix(attrs.Name, "/"),
		Name:      attrs.Name,
		Path:      attrs.Name,
		URL:       attrs.MediaLink,
		Created:   attrs.Created.Unix(),
		Size:      attrs.Size,
	}, nil
}

func (s *GCSStorage) Download(ctx context.Context, path string) (io.Reader, error) {
	obj := s.bucket.Object(path)
	reader, err := obj.NewReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCS object reader: %w", err)
	}
	return reader, nil
}

func (s *GCSStorage) Rename(ctx context.Context, path string, newPath string) (FileStoreItem, error) {
	src := s.bucket.Object(path)
	dst := s.bucket.Object(newPath)

	// For directories, iterate over each item and copy then delete
	if strings.HasSuffix(path, "/") {
		it := s.bucket.Objects(ctx, &storage.Query{Prefix: path})
		for {
			attrs, err := it.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return FileStoreItem{}, fmt.Errorf("error iterating over GCS objects during rename: %w", err)
			}
			newObjPath := strings.Replace(attrs.Name, path, newPath, 1)
			_, err = s.bucket.Object(newObjPath).CopierFrom(src).Run(ctx)
			if err != nil {
				return FileStoreItem{}, fmt.Errorf("error copying GCS object during rename: %w", err)
			}
			if err := src.Delete(ctx); err != nil {
				return FileStoreItem{}, fmt.Errorf("error deleting original GCS object post rename: %w", err)
			}
		}
	} else { // For single objects
		if _, err := dst.CopierFrom(src).Run(ctx); err != nil {
			return FileStoreItem{}, fmt.Errorf("failed to rename GCS object: %w", err)
		}
		if err := src.Delete(ctx); err != nil {
			return FileStoreItem{}, fmt.Errorf("failed to delete original GCS object after renaming: %w", err)
		}
	}
	return s.Get(ctx, newPath)
}

func (s *GCSStorage) Delete(ctx context.Context, path string) error {
	if strings.HasSuffix(path, "/") { // If it's a directory
		it := s.bucket.Objects(ctx, &storage.Query{Prefix: path})
		for {
			attrs, err := it.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return fmt.Errorf("error iterating over GCS objects during delete: %w", err)
			}
			if err := s.bucket.Object(attrs.Name).Delete(ctx); err != nil {
				return fmt.Errorf("error deleting GCS object: %w", err)
			}
		}
	} else { // For single objects
		if err := s.bucket.Object(path).Delete(ctx); err != nil {
			return fmt.Errorf("failed to delete GCS object: %w", err)
		}
	}
	return nil
}

func (s *GCSStorage) CreateFolder(ctx context.Context, path string) (FileStoreItem, error) {
	obj := s.bucket.Object(path + "/")
	if _, err := obj.NewWriter(ctx).Write([]byte("")); err != nil {
		// Check if the error is due to the folder already existing
		if strings.Contains(err.Error(), "googleapi: Error 409: Conflict") {
			attrs, err := obj.Attrs(ctx)
			if err != nil {
				return FileStoreItem{}, fmt.Errorf("error fetching GCS object attributes after folder creation: %w", err)
			}
			return FileStoreItem{
				Directory: strings.HasSuffix(attrs.Name, "/"),
				Name:      attrs.Name,
				Path:      attrs.Name,
				URL:       attrs.MediaLink,
				Created:   attrs.Created.Unix(),
				Size:      attrs.Size,
			}, nil
		}
		return FileStoreItem{}, fmt.Errorf("failed to create GCS folder: %w", err)
	}
	attrs, err := obj.Attrs(ctx)
	if err != nil {
		return FileStoreItem{}, fmt.Errorf("error fetching GCS object attributes after folder creation: %w", err)
	}
	return FileStoreItem{
		Directory: strings.HasSuffix(attrs.Name, "/"),
		Name:      attrs.Name,
		Path:      attrs.Name,
		URL:       attrs.MediaLink,
		Created:   attrs.Created.Unix(),
		Size:      attrs.Size,
	}, nil
}

// Compile-time interface check:
var _ FileStore = (*GCSStorage)(nil)
