package controller

import (
	"context"

	"github.com/bacalhau-project/lilysaas/api/pkg/bacalhau"
	"github.com/bacalhau-project/lilysaas/api/pkg/store"
	"github.com/bacalhau-project/lilysaas/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// artists that have arrived in the database we should check exist in the
// smart contract - if the answer is no and the artist is > 24 hours old
// then we delete it
func (c *Controller) checkForNewArtists(ctx context.Context) error {
	// load the artists where bacalhau state is "Created"
	// for each artist, check if it exists in the smart contract
	// if no - then check the creation time
	// if created > 24 hours then delete
	// if yes it's in the smart contract then trigger the bacalhau training job
	// and record the id of the job in the database
	// and update the bacalhau state to "Running"
	newArtists, err := c.Store.ListArtists(ctx, store.ListArtistsQuery{
		OnlyNew: true,
	})
	if err != nil {
		return err
	}
	if len(newArtists) == 0 {
		return nil
	}
	log.Info().Msgf("Found %d new artists", len(newArtists))
	contractIDs, err := c.Contract.GetArtistIDs(ctx)
	if err != nil {
		return err
	}
	for _, artist := range newArtists {
		// check if the artist.ID exists in the contractIDs array
		if containsString(contractIDs, artist.ID) {
			// trigger the artist training
			log.Info().Msgf("Found new artist in contract: %s - triggering training", artist.ID)
			jobID, err := c.trainArtist(ctx, artist)
			// if we got an error trying to train then let's flag the artist as errored
			// which will end up with their fee being refunded in the smart contract
			if err != nil {
				log.Info().Msgf("Error in training artist: %s %s", artist.ID, err.Error())
				c.artistBacalhauError(ctx, artist.ID, err)
				continue
			}
			log.Info().Msgf("Bacalhau Job created: %s", jobID)
			c.artistBacalhauRunning(ctx, artist.ID, jobID)
		} else {
			// check if the creation time is > 24 hours
			// and delete the artist if it is
			if isOlderThan24Hours(artist.Created) {
				log.Info().Msgf("Artist not found in contract > 24 hours later: %s - triggering delete", artist.ID)
				err = c.Store.DeleteArtist(ctx, artist.ID)
				if err != nil {
					continue
				}
			}
		}
	}
	return nil
}

// we need to check on the artists that are currently training
// as bacalhau jobs and update the job state if the bacalhau job has finished
func (c *Controller) checkForRunningArtists(ctx context.Context) error {
	// load the artists where bacalhau state is "Running"
	// for each artist, check the status of the bacalhau job
	// if the job is still running then continue
	// if the job is errored then update the bacalhau state to "Error"
	// if the job is complete then update the bacalhau state to "Complete"
	// if we error getting the state of the job then contunue (and assume network flake)
	runningArtists, err := c.Store.ListArtists(ctx, store.ListArtistsQuery{
		OnlyRunning: true,
	})
	if err != nil {
		return err
	}
	if len(runningArtists) == 0 {
		return nil
	}
	for _, artist := range runningArtists {
		jobStatus, jobError, err := c.Bacalhau.GetJobStatus(ctx, artist.BacalhauTrainingID)
		if err != nil {
			log.Info().Msgf("Error checking job status for artist: %s, job: %s, error: %s", artist.ID, artist.BacalhauTrainingID, err.Error())
			continue
		}
		// the job has errored
		if jobStatus == types.BacalhauStateError {
			log.Error().Msgf("Training Job has error - artist: %s, job: %s, error: %s", artist.ID, artist.BacalhauTrainingID, artist.Error)
			c.artistBacalhauError(ctx, artist.ID, jobError)
		} else if jobStatus == types.BacalhauStateComplete {
			log.Info().Msgf("Training Job is complete: %s, job: %s", artist.ID, artist.BacalhauTrainingID)
			c.artistBacalhauComplete(ctx, artist.ID)
		}
	}
	return nil
}

// check for artists that have finished the bacalhau job but have not updated
// the smart contract
func (c *Controller) checkForFinishedArtists(ctx context.Context) error {
	// load the artists where the bacalhau state is "Complete" or "Error"
	// and the contract state is "None"
	// if the bacalhau state is "Complete" then call the "ArtistComplete" function
	// if the bacalhau state is "Error" then call the "ArtistError" function
	finishedArtists, err := c.Store.ListArtists(ctx, store.ListArtistsQuery{
		OnlyFinished: true,
	})
	if err != nil {
		return err
	}
	if len(finishedArtists) == 0 {
		return nil
	}
	for _, artist := range finishedArtists {
		if artist.BacalhauState == types.BacalhauStateComplete {
			err = c.Contract.ArtistComplete(ctx, artist.ID)
			if err != nil {
				log.Error().Msgf("Error calling ArtistComplete on smart contract: %s, error: %s", artist.ID, err.Error())
				c.artistContractError(ctx, artist.ID, err)
				continue
			}
			c.artistContractComplete(ctx, artist.ID)
		} else if artist.BacalhauState == types.BacalhauStateError {
			err = c.Contract.ArtistCancelled(ctx, artist.ID)
			if err != nil {
				log.Error().Msgf("Error calling ArtistCancelled on smart contract: %s, error: %s", artist.ID, err.Error())
				c.artistContractError(ctx, artist.ID, err)
				continue
			}
			c.artistContractComplete(ctx, artist.ID)
		}
	}
	return nil
}

// check for new images that are in the smart contract and not in the database
// trigger the bacalhau job for any new images found
func (c *Controller) checkForNewImages(ctx context.Context) error {
	// load the image IDs from both the database and the smart contract
	// check if any exist in the smart contract and not in the database
	// insert any of these into the database
	// trigger the bacalhau inference job
	// and record the id of the job in the database
	// and update the bacalhau state to "Running"
	// TODO: this is not very efficient
	allImages, err := c.Store.ListImages(ctx, store.ListImagesQuery{})
	if err != nil {
		return err
	}
	databaseIDs := []int{}
	// build a map of the database IDs
	for _, image := range allImages {
		databaseIDs = append(databaseIDs, image.ID)
	}
	contractIDs, err := c.Contract.GetImageIDs(ctx)
	if err != nil {
		return err
	}

	// create a map of the database IDs so we can easily see if the contract ID exists
	databaseIDMap := make(map[int]bool)
	for _, id := range databaseIDs {
		databaseIDMap[id] = true
	}

	// find which ids in the contractIDs array are not in the databaseIDMap
	newImageIDs := []int{}
	for _, id := range contractIDs {
		if _, ok := databaseIDMap[id]; !ok {
			newImageIDs = append(newImageIDs, id)
		}
	}

	for _, id := range newImageIDs {
		contractImage, err := c.Contract.GetImage(ctx, id)
		if err != nil {
			return err
		}
		err = c.Store.AddImage(ctx, types.Image{
			ID:     id,
			Artist: contractImage.Artist,
			Prompt: contractImage.Prompt,
		})
		if err != nil {
			return err
		}
		image, err := c.Store.GetImage(ctx, id)
		if err != nil {
			return err
		}
		log.Info().Msgf("Found new image in contract: %d, artist: %s, prompt: %s - triggering inference", image.ID, image.Artist, image.Prompt)
		jobID, err := c.runInference(ctx, image)
		if err != nil {
			return err
		}
		log.Info().Msgf("Bacalhau Job created: %s", jobID)
		c.imageBacalhauRunning(ctx, image.ID, jobID)
	}

	return nil
}

// check for images that are running on bacalhau
// update the image state on either failure or success
func (c *Controller) checkForRunningImages(ctx context.Context) error {
	// load the images where bacalhau state is "Running"
	// for each artist, check the status of the bacalhau job
	// if the job is still running then continue
	// if the job is errored then update the bacalhau state to "Error"
	// if the job is complete then update the bacalhau state to "Complete"
	// if we error getting the state of the job then contunue (and assume network flake)
	runningImages, err := c.Store.ListImages(ctx, store.ListImagesQuery{
		OnlyRunning: true,
	})
	if err != nil {
		return err
	}
	if len(runningImages) == 0 {
		return nil
	}
	for _, image := range runningImages {
		jobStatus, jobError, err := c.Bacalhau.GetJobStatus(ctx, image.BacalhauInferenceID)
		if err != nil {
			log.Info().Msgf("Error checking job status for image: %s, job: %s, error: %s", image.Artist, image.BacalhauInferenceID, err.Error())
			continue
		}
		// the job has errored
		if jobStatus == types.BacalhauStateError {
			log.Error().Msgf("Image Job has error - artist: %s, job: %s, error: %s", image.Artist, image.BacalhauInferenceID, image.Error)
			c.imageBacalhauError(ctx, image.ID, jobError)
		} else if jobStatus == types.BacalhauStateComplete {
			log.Error().Msgf("Image Job is complete: %s, job: %s", image.Artist, image.BacalhauInferenceID)
			c.imageBacalhauComplete(ctx, image.ID)
		}
	}
	return nil
}

// check for images that have finished the bacalhau job but have not updated
// the smart contract
func (c *Controller) checkForFinishedImages(ctx context.Context) error {
	// load the images where the bacalhau state is "Complete" or "Error"
	// and the contract state is "None"
	// if the bacalhau state is "Complete" then call the "ImageComplete" function
	// if the bacalhau state is "Error" then call the "ImageCancelled" function
	finishedImages, err := c.Store.ListImages(ctx, store.ListImagesQuery{
		OnlyFinished: true,
	})
	if err != nil {
		return err
	}
	if len(finishedImages) == 0 {
		return nil
	}
	for _, image := range finishedImages {
		if image.BacalhauState == types.BacalhauStateComplete {
			err = c.Contract.ImageComplete(ctx, image.ID, "")
			if err != nil {
				log.Error().Msgf("Error calling ImageComplete on smart contract: %s, error: %s", image.ID, err.Error())
				c.imageContractError(ctx, image.ID, err)
				continue
			}
			c.imageContractComplete(ctx, image.ID)
		} else if image.BacalhauState == types.BacalhauStateError {
			err = c.Contract.ImageCancelled(ctx, image.ID, image.Error)
			if err != nil {
				log.Error().Msgf("Error calling ImageCancelled on smart contract: %s, error: %s", image.ID, err.Error())
				c.imageContractError(ctx, image.ID, err)
				continue
			}
			c.imageContractComplete(ctx, image.ID)
		}
	}
	return nil
}

func (c *Controller) updateArtist(
	ctx context.Context,
	id string,
	handler func(*types.Artist),
) {
	artist, err := c.Store.GetArtist(ctx, id)
	if err != nil {
		log.Error().Msgf("Error getting artist from database: %s %s", id, err.Error())
		return
	}
	handler(artist)

	err = c.Store.UpdateArtist(ctx, *artist)
	if err != nil {
		log.Error().Msgf("Error updating artist: %s %s", id, err.Error())
		return
	}
}

func (c *Controller) updateImage(
	ctx context.Context,
	id int,
	handler func(*types.Image),
) {
	image, err := c.Store.GetImage(ctx, id)
	if err != nil {
		log.Error().Msgf("Error getting image from database: %s %s", id, err.Error())
		return
	}
	handler(image)

	err = c.Store.UpdateImage(ctx, *image)
	if err != nil {
		log.Error().Msgf("Error updating image: %s %s", id, err.Error())
		return
	}
}

func (c *Controller) artistBacalhauRunning(
	ctx context.Context,
	id string,
	jobID string,
) {
	c.updateArtist(ctx, id, func(artist *types.Artist) {
		artist.BacalhauState = types.BacalhauStateRunning
		artist.BacalhauTrainingID = jobID
	})
}

func (c *Controller) artistBacalhauComplete(
	ctx context.Context,
	id string,
) {
	c.updateArtist(ctx, id, func(artist *types.Artist) {
		artist.BacalhauState = types.BacalhauStateComplete
	})
}

func (c *Controller) artistBacalhauError(
	ctx context.Context,
	id string,
	err error,
) {
	c.updateArtist(ctx, id, func(artist *types.Artist) {
		artist.BacalhauState = types.BacalhauStateError
		artist.Error = err.Error()
	})
}

func (c *Controller) artistContractComplete(
	ctx context.Context,
	id string,
) {
	c.updateArtist(ctx, id, func(artist *types.Artist) {
		artist.ContractState = types.ContractStateComplete
	})
}

func (c *Controller) artistContractError(
	ctx context.Context,
	id string,
	err error,
) {
	c.updateArtist(ctx, id, func(artist *types.Artist) {
		artist.ContractState = types.ContractStateError
		artist.Error = err.Error()
	})
}

func (c *Controller) imageBacalhauRunning(
	ctx context.Context,
	id int,
	jobID string,
) {
	c.updateImage(ctx, id, func(image *types.Image) {
		image.BacalhauState = types.BacalhauStateRunning
		image.BacalhauInferenceID = jobID
	})
}

func (c *Controller) imageBacalhauComplete(
	ctx context.Context,
	id int,
) {
	c.updateImage(ctx, id, func(image *types.Image) {
		image.BacalhauState = types.BacalhauStateComplete
	})
}

func (c *Controller) imageBacalhauError(
	ctx context.Context,
	id int,
	err error,
) {
	c.updateImage(ctx, id, func(image *types.Image) {
		image.BacalhauState = types.BacalhauStateError
		image.Error = err.Error()
	})
}

func (c *Controller) imageContractComplete(
	ctx context.Context,
	id int,
) {
	c.updateImage(ctx, id, func(image *types.Image) {
		image.ContractState = types.ContractStateComplete
	})
}

func (c *Controller) imageContractError(
	ctx context.Context,
	id int,
	err error,
) {
	c.updateImage(ctx, id, func(image *types.Image) {
		image.ContractState = types.ContractStateError
		image.Error = err.Error()
	})
}

func (c *Controller) trainArtist(ctx context.Context, artist *types.Artist) (string, error) {
	// this is the ImagesDownloadURL for the training job
	imagesURL := c.getArtistTrainingImagesDownloadURL(artist.ID)
	weightsUploadUrl := c.getArtistWeightUploadURL(artist.ID)
	spec := bacalhau.GetTrainingSpec(bacalhau.TrainingSpecOptions{
		ArtistID:          artist.ID,
		ImagesDownloadURL: imagesURL,
		WeightsUploadURL:  weightsUploadUrl,
	})
	log.Info().Msgf("Generated bacalhau job spec for artist training: %s", artist.ID)
	dumpObject(spec)
	job, err := c.Bacalhau.CreateJob(ctx, spec, []string{})
	if err != nil {
		return "", err
	}
	return job.ID(), nil
}

func (c *Controller) runInference(ctx context.Context, image *types.Image) (string, error) {
	weightsURL := c.getArtistWeightsDownloadURL(image.Artist)
	imagesURL := c.getImageUploadURL(image.ID)
	spec := bacalhau.GetInferenceSpec(bacalhau.InferenceSpecOptions{
		ArtistID:           image.Artist,
		Prompt:             image.Prompt,
		ImageID:            image.ID,
		WeightsDownloadURL: weightsURL,
		ImagesUploadURL:    imagesURL,
	})
	log.Info().Msgf("Generated bacalhau job spec for image inference: %s", image.ID)
	dumpObject(spec)
	job, err := c.Bacalhau.CreateJob(ctx, spec, []string{})
	if err != nil {
		return "", err
	}
	return job.ID(), nil
}
