package system

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func WriteFile(path string, data []byte) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(data)
	if err != nil {
		return err
	}
	return nil
}

func GetTarBuffer(localPath string) (*bytes.Buffer, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	err := filepath.Walk(localPath, func(file string, fi os.FileInfo, err error) error {
		// Handle errors
		if err != nil {
			return err
		}

		// Create tar header
		header, err := tar.FileInfoHeader(fi, file)
		if err != nil {
			return err
		}

		// Set header.Name to relative path
		relPath, err := filepath.Rel(localPath, file)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(relPath)

		// Write header
		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		// If it's a directory, there's no content to write, return.
		if fi.Mode().IsDir() {
			return nil
		}

		// Write file content
		data, err := os.Open(file)
		if err != nil {
			return err
		}
		defer data.Close()
		if _, err := io.Copy(tw, data); err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	if err := tw.Close(); err != nil {
		return nil, err
	}

	return &buf, nil
}

func GetTarStream(localPath string) (io.Reader, error) {
	buffer, err := GetTarBuffer(localPath)
	if err != nil {
		return nil, err
	}
	return buffer, nil
}

func ExpandTarBuffer(buf *bytes.Buffer, localPath string) error {
	err := os.RemoveAll(localPath)
	if err != nil {
		return err
	}
	// Create a new tar reader
	tr := tar.NewReader(buf)

	// Iterate through tar headers (files)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break // End of archive
		}
		if err != nil {
			return err
		}

		// Prepare file path and create directories if needed
		target := filepath.Join(localPath, header.Name)
		dir, _ := filepath.Split(target)
		if err := os.MkdirAll(dir, os.ModePerm); err != nil {
			return err
		}

		// Check for file type
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.Mkdir(target, os.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			// Open the file
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			defer f.Close()

			// Copy file content
			if _, err := io.Copy(f, tr); err != nil {
				return err
			}
		}
	}
	return nil
}

func ConcatenateFiles(outputFile string, inputFiles []string, delimiter string) error {
	// Open the output file in append mode
	out, err := os.OpenFile(outputFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer out.Close()

	// Loop over the input files
	for i, inputFile := range inputFiles {
		// Open each input file
		in, err := os.Open(inputFile)
		if err != nil {
			return err
		}
		defer in.Close()

		// Copy the contents of the input file to the output file
		_, err = io.Copy(out, in)
		if err != nil {
			return err
		}

		// Write the delimiter after each file, except for the last file
		if i < len(inputFiles)-1 {
			_, err = out.WriteString(delimiter)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func ReadTextFile(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return "", err
	}

	if stat.IsDir() {
		return "", fmt.Errorf("expected a file, got a directory")
	}

	jsBytes, err := io.ReadAll(file)
	if err != nil {
		return "", err
	}

	return string(jsBytes), nil
}
