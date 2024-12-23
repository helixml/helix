package filestore

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

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

func (s *GCSStorage) List(ctx context.Context, prefix string) ([]Item, error) {
	it := s.bucket.Objects(ctx, &storage.Query{Prefix: prefix})
	items := []Item{}

	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return []Item{}, nil
		}

		item := Item{
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

func (s *GCSStorage) Get(ctx context.Context, path string) (Item, error) {
	attrs, err := s.bucket.Object(path).Attrs(ctx)
	if err != nil {
		return Item{}, fmt.Errorf("error fetching GCS object attributes: %w", err)
	}

	return Item{
		Directory: strings.HasSuffix(attrs.Name, "/"),
		Name:      attrs.Name,
		Path:      attrs.Name,
		URL:       attrs.MediaLink,
		Created:   attrs.Created.Unix(),
		Size:      attrs.Size,
	}, nil
}

func (s *GCSStorage) SignedURL(_ context.Context, path string) (string, error) {
	return s.bucket.SignedURL(path, &storage.SignedURLOptions{
		Expires: time.Now().Add(20 * time.Minute),
		Method:  http.MethodGet,
	})
}

func (s *GCSStorage) WriteFile(ctx context.Context, path string, r io.Reader) (Item, error) {
	obj := s.bucket.Object(path)
	writer := obj.NewWriter(ctx)
	if _, err := io.Copy(writer, r); err != nil {
		return Item{}, fmt.Errorf("failed to copy content to GCS: %w", err)
	}
	if err := writer.Close(); err != nil {
		return Item{}, fmt.Errorf("failed to finalize GCS object upload: %w", err)
	}
	attrs, err := obj.Attrs(ctx)
	if err != nil {
		return Item{}, fmt.Errorf("error fetching GCS object attributes after upload: %w", err)
	}
	return Item{
		Directory: strings.HasSuffix(attrs.Name, "/"),
		Name:      attrs.Name,
		Path:      attrs.Name,
		URL:       attrs.MediaLink,
		Created:   attrs.Created.Unix(),
		Size:      attrs.Size,
	}, nil
}

func (s *GCSStorage) OpenFile(ctx context.Context, path string) (io.ReadCloser, error) {
	obj := s.bucket.Object(path)
	reader, err := obj.NewReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCS object reader: %w", err)
	}
	return reader, nil
}

func (s *GCSStorage) DownloadFolder(ctx context.Context, path string) (io.Reader, error) {
	var buf bytes.Buffer
	tarWriter := tar.NewWriter(&buf)
	defer tarWriter.Close()

	it := s.bucket.Objects(ctx, &storage.Query{Prefix: path})
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}

		// Read the object
		obj := s.bucket.Object(attrs.Name)
		reader, err := obj.NewReader(ctx)
		if err != nil {
			return nil, err
		}

		// Write the file header to the tar
		if err := tarWriter.WriteHeader(&tar.Header{
			Name: attrs.Name,
			Mode: 0600,
			Size: attrs.Size,
		}); err != nil {
			return nil, err
		}

		// Copy file content to tar
		if _, err := io.Copy(tarWriter, reader); err != nil {
			return nil, err
		}
		reader.Close()
	}

	return &buf, nil
}

func (s *GCSStorage) UploadFolder(ctx context.Context, path string, r io.Reader) error {
	tarReader := tar.NewReader(r)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("error reading tar header: %w", err)
		}

		// Skip directories
		if header.Typeflag == tar.TypeDir {
			continue
		}

		// Create the object path by appending the file name to the given path
		objPath := path + "/" + header.Name

		// Create the object in GCS
		obj := s.bucket.Object(objPath)
		writer := obj.NewWriter(ctx)
		if _, err := io.Copy(writer, tarReader); err != nil {
			writer.Close()
			return fmt.Errorf("failed to copy content to GCS: %w", err)
		}
		if err := writer.Close(); err != nil {
			return fmt.Errorf("failed to finalize GCS object upload: %w", err)
		}

		// Fetch the object attributes
		_, err = obj.Attrs(ctx)
		if err != nil {
			return fmt.Errorf("error fetching GCS object attributes after upload: %w", err)
		}
	}

	return nil
}

func (s *GCSStorage) Rename(ctx context.Context, path string, newPath string) (Item, error) {
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
				return Item{}, fmt.Errorf("error iterating over GCS objects during rename: %w", err)
			}
			newObjPath := strings.Replace(attrs.Name, path, newPath, 1)
			_, err = s.bucket.Object(newObjPath).CopierFrom(src).Run(ctx)
			if err != nil {
				return Item{}, fmt.Errorf("error copying GCS object during rename: %w", err)
			}
			if err := src.Delete(ctx); err != nil {
				return Item{}, fmt.Errorf("error deleting original GCS object post rename: %w", err)
			}
		}
	} else { // For single objects
		if _, err := dst.CopierFrom(src).Run(ctx); err != nil {
			return Item{}, fmt.Errorf("failed to rename GCS object: %w", err)
		}
		if err := src.Delete(ctx); err != nil {
			return Item{}, fmt.Errorf("failed to delete original GCS object after renaming: %w", err)
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

func (s *GCSStorage) CreateFolder(ctx context.Context, path string) (Item, error) {
	obj := s.bucket.Object(path + "/")
	if _, err := obj.NewWriter(ctx).Write([]byte("")); err != nil {
		// Check if the error is due to the folder already existing
		if strings.Contains(err.Error(), "googleapi: Error 409: Conflict") {
			attrs, err := obj.Attrs(ctx)
			if err != nil {
				return Item{}, fmt.Errorf("error fetching GCS object attributes after folder creation: %w", err)
			}
			return Item{
				Directory: strings.HasSuffix(attrs.Name, "/"),
				Name:      attrs.Name,
				Path:      attrs.Name,
				URL:       attrs.MediaLink,
				Created:   attrs.Created.Unix(),
				Size:      attrs.Size,
			}, nil
		}
		return Item{}, fmt.Errorf("failed to create GCS folder: %w", err)
	}
	attrs, err := obj.Attrs(ctx)
	if err != nil {
		return Item{}, fmt.Errorf("error fetching GCS object attributes after folder creation: %w", err)
	}
	return Item{
		Directory: strings.HasSuffix(attrs.Name, "/"),
		Name:      attrs.Name,
		Path:      attrs.Name,
		URL:       attrs.MediaLink,
		Created:   attrs.Created.Unix(),
		Size:      attrs.Size,
	}, nil
}

func (s *GCSStorage) CopyFile(ctx context.Context, fromPath string, toPath string) error {
	// Check if the fromPath exists
	_, err := s.Get(ctx, fromPath)
	if err != nil {
		return fmt.Errorf("failed to get source file: %w", err)
	}

	// Create the folder for the toPath if it doesn't exist
	toFolder := filepath.Dir(toPath)
	_, err = s.CreateFolder(ctx, toFolder)
	if err != nil {
		return fmt.Errorf("failed to create destination folder: %w", err)
	}

	// Copy the file
	fromReader, err := s.OpenFile(ctx, fromPath)
	if err != nil {
		return fmt.Errorf("failed to download source file: %w", err)
	}
	defer fromReader.Close()

	toWriter := s.bucket.Object(toPath).NewWriter(ctx)
	defer toWriter.Close()

	if _, err := io.Copy(toWriter, fromReader); err != nil {
		return fmt.Errorf("failed to copy file: %w", err)
	}

	return nil
}

// Compile-time interface check:
var _ FileStore = (*GCSStorage)(nil)
