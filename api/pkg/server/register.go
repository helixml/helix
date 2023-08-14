package server

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/bacalhau-project/lilysaas/api/pkg/types"
	"github.com/rs/zerolog/log"
)

func (apiServer *WaterlilyAPIServer) copyImages(artistid string, folder string, fieldname string, req *http.Request) ([]string, error) {
	subPath := filepath.Join("artists", artistid, folder)
	uploadPath, err := apiServer.ensureFilestorePath(subPath)
	if err != nil {
		return nil, err
	}
	files := req.MultipartForm.File[fieldname]
	filenames := []string{}
	for _, fileHeader := range files {
		file, err := fileHeader.Open()
		if err != nil {
			return nil, err
		}
		defer file.Close()
		log.Ctx(req.Context()).Info().Msgf("uploading %s file: %s to %s", folder, fileHeader.Filename, uploadPath)

		f, err := os.OpenFile(filepath.Join(uploadPath, fileHeader.Filename), os.O_WRONLY|os.O_CREATE, 0666)
		if err != nil {
			return nil, err
		}
		defer f.Close()

		// Write the file to the disk
		_, err = io.Copy(f, file)
		if err != nil {
			return nil, err
		}
		filenames = append(filenames, filepath.Join(subPath, fileHeader.Filename))
	}

	return filenames, nil
}

func (apiServer *WaterlilyAPIServer) register(res http.ResponseWriter, req *http.Request) {
	res.Header().Set("Access-Control-Allow-Origin", "*")

	err := func() error {
		// Parse the multipart form request
		err := req.ParseMultipartForm(200 << 20) // 200MB is the maximum memory allocated to store the files
		if err != nil {
			return err
		}

		artistid := req.FormValue("artistid")

		_, err = apiServer.copyImages(artistid, "images", "images", req)
		if err != nil {
			return err
		}
		thumbnails, err := apiServer.copyImages(artistid, "thumbnails", "thumbnails", req)
		if err != nil {
			return err
		}
		avatars, err := apiServer.copyImages(artistid, "avatar", "avatar", req)
		if err != nil {
			return err
		}

		err = tarGzipFolder(
			apiServer.getFilestorePath(filepath.Join("artists", artistid, "images")),
			apiServer.getFilestorePath(filepath.Join("artists", artistid, "images.tar.gz")),
		)
		if err != nil {
			return fmt.Errorf("There was an error compressing the uploaded images: %s", err.Error())
		}

		originalArt, err := strconv.ParseBool(req.FormValue("originalArt"))
		if err != nil {
			return fmt.Errorf("We could not parse the originalArt field: %s", err.Error())
		}
		trainingConsent, err := strconv.ParseBool(req.FormValue("trainingConsent"))
		if err != nil {
			return fmt.Errorf("We could not parse the trainingConsent field: %s", err.Error())
		}
		legalContent, err := strconv.ParseBool(req.FormValue("legalContent"))
		if err != nil {
			return fmt.Errorf("We could not parse the legalContent field: %s", err.Error())
		}

		avatar := ""

		if len(avatars) > 0 {
			avatar = avatars[0]
		}

		err = apiServer.Controller.Store.AddArtist(req.Context(), types.Artist{
			ID:            artistid,
			BacalhauState: types.BacalhauStateCreated,
			ContractState: types.ContractStateNone,
			Data: types.ArtistData{
				Period:          req.FormValue("period"),
				Name:            req.FormValue("name"),
				Email:           req.FormValue("email"),
				WalletAddress:   req.FormValue("walletAddress"),
				Nationality:     req.FormValue("nationality"),
				Biography:       req.FormValue("biography"),
				Category:        req.FormValue("category"),
				Style:           req.FormValue("style"),
				Tags:            req.FormValue("tags"),
				Portfolio:       req.FormValue("portfolio"),
				OriginalArt:     originalArt,
				TrainingConsent: trainingConsent,
				LegalContent:    legalContent,
				ArtistType:      req.FormValue("artistType"),
				Thumbnails:      thumbnails,
				Avatar:          avatar,
			},
		})
		if err != nil {
			return err
		}

		artist, err := apiServer.Controller.Store.GetArtist(req.Context(), artistid)
		if err != nil {
			return err
		}

		err = json.NewEncoder(res).Encode(artist)
		if err != nil {
			return err
		}

		return nil
	}()

	if err != nil {
		log.Ctx(req.Context()).Error().Msgf("error for register route: %s", err.Error())
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}
}

func tarGzipFolder(dir string, tarGz string) error {
	// Create the tar.gz file
	file, err := os.Create(tarGz)
	if err != nil {
		fmt.Printf("step1\n")
		return err
	}
	defer file.Close()

	// Create a gzip writer
	gw := gzip.NewWriter(file)
	defer gw.Close()

	// Create a tar writer
	tw := tar.NewWriter(gw)
	defer tw.Close()

	// Walk the directory tree and add files to the tar archive
	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Printf("step2\n")
			return err
		}

		// Get the file header
		header, err := tar.FileInfoHeader(info, info.Name())
		if err != nil {
			return err
		}

		// Set the header name to the relative path of the file
		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		header.Name = relPath

		// Write the header and file content to the tar archive
		err = tw.WriteHeader(header)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()
			_, err = io.Copy(tw, file)
			if err != nil {
				return err
			}
		}
		return nil
	})
	return err
}
