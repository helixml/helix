package knowledge

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

func (r *Reconciler) startCron(ctx context.Context) error {
	// start the scheduler
	r.cron.Start()

	// Block until the context is done
	<-ctx.Done()

	// when you're done, shut it down
	err := r.cron.Shutdown()
	if err != nil {
		return fmt.Errorf("failed to shutdown scheduler: %w", err)
	}

	return nil
}

func (r *Reconciler) reconcileCronJobs(ctx context.Context) error {
	knowledges, err := r.listKnowledge(ctx)
	if err != nil {
		return fmt.Errorf("failed to list knowledges: %w", err)
	}
	jobs := r.cron.Jobs()

	knowledgesMap := make(map[string]*types.Knowledge) // knowledge id to knowledge
	jobsMap := make(map[string]gocron.Job)             // knowledge id to job

	for _, knowledge := range knowledges {
		knowledgesMap[knowledge.ID] = knowledge
	}

	for _, job := range jobs {
		jobsMap[job.Name()] = job

		// If the job is not in the knowledges list, remove it
		if _, ok := knowledgesMap[job.Name()]; !ok {
			log.Info().
				Str("job_id", job.ID().String()).
				Strs("job_tags", job.Tags()).
				Msg("removing job")

			err := r.cron.RemoveJob(job.ID())
			if err != nil {
				return fmt.Errorf("failed to remove job: %w", err)
			}
		}
	}

	for _, knowledge := range knowledges {
		job, ok := jobsMap[knowledge.ID]
		if !ok {
			log.Info().
				Str("knowledge_id", knowledge.ID).
				Str("knowledge_name", knowledge.Name).
				Str("knowledge_refresh_schedule", knowledge.RefreshSchedule).
				Msg("adding cron job to the scheduler")

			// job doesn't exist, create it
			_, err := r.cron.NewJob(
				gocron.CronJob(knowledge.RefreshSchedule, true),
				r.getCronTask(ctx, knowledge.ID),
				r.getCronJobOptions(knowledge)...,
			)
			if err != nil {
				log.Error().
					Err(err).
					Str("knowledge_id", knowledge.ID).
					Str("knowledge_name", knowledge.Name).
					Str("knowledge_refresh_schedule", knowledge.RefreshSchedule).
					Msg("failed to create job")
			}
		} else {
			// Job exists, check schedule and update if needed
			currentSchedule := getJobSchedule(job)

			if currentSchedule != knowledge.RefreshSchedule {
				log.Info().
					Str("knowledge_id", knowledge.ID).
					Str("knowledge_name", knowledge.Name).
					Str("knowledge_refresh_schedule", knowledge.RefreshSchedule).
					Str("current_schedule", currentSchedule).
					Msg("updating cron job schedule")

				_, err := r.cron.Update(
					job.ID(),
					gocron.CronJob(knowledge.RefreshSchedule, true),
					r.getCronTask(ctx, knowledge.ID),
					r.getCronJobOptions(knowledge)...,
				)
				if err != nil {
					return fmt.Errorf("failed to remove job: %w", err)
				}
			}
		}
	}

	return nil
}

func (r *Reconciler) getCronTask(ctx context.Context, knowledgeID string) gocron.Task {
	return gocron.NewTask(func() {
		// TODO:
		fmt.Println(time.Now().Format("2006-01-02 15:04:05") + " running job for knowledge " + knowledgeID)

		knowledge, err := r.store.GetKnowledge(ctx, knowledgeID)
		if err != nil {
			log.Error().
				Err(err).
				Str("knowledge_id", knowledgeID).
				Msg("failed to get knowledge")
			return
		}

		// If knowledge is indexing or pending, skip it
		if knowledge.State == types.KnowledgeStateIndexing || knowledge.State == types.KnowledgeStatePending {
			return
		}

		// Generate a new version ID
		version := system.GenerateVersion()

		err = r.indexKnowledge(ctx, knowledge, version)
		if err != nil {
			log.Error().
				Err(err).
				Str("knowledge_id", knowledgeID).
				Msg("failed to index knowledge")

			knowledge.State = types.KnowledgeStateError
			knowledge.Message = err.Error()
			_, _ = r.store.UpdateKnowledge(ctx, knowledge)

			// Create a failed version too just for logs
			_, _ = r.store.CreateKnowledgeVersion(ctx, &types.KnowledgeVersion{
				KnowledgeID: knowledge.ID,
				Version:     version,
				Size:        knowledge.Size,
				State:       types.KnowledgeStateError,
				Message:     err.Error(),
			})
		}
	})
}

func (r *Reconciler) getCronJobOptions(knowledge *types.Knowledge) []gocron.JobOption {
	return []gocron.JobOption{
		gocron.WithName(knowledge.ID),
		gocron.WithTags(fmt.Sprintf("schedule:%s", knowledge.RefreshSchedule)),
	}
}

func getJobSchedule(job gocron.Job) string {
	tags := job.Tags()

	// current schedule
	var currentSchedule string
	for _, tag := range tags {
		if strings.HasPrefix(tag, "schedule:") {
			currentSchedule = strings.TrimPrefix(tag, "schedule:")
			return currentSchedule
		}
	}

	return currentSchedule
}

func (r *Reconciler) listKnowledge(ctx context.Context) ([]*types.Knowledge, error) {
	knowledges, err := r.store.ListKnowledge(ctx, &store.ListKnowledgeQuery{})
	if err != nil {
		return nil, fmt.Errorf("failed to list knowledges: %w", err)
	}

	var filtered []*types.Knowledge

	for _, knowledge := range knowledges {
		if !knowledge.RefreshEnabled {
			continue
		}

		if knowledge.RefreshSchedule == "" {
			continue
		}

		filtered = append(filtered, knowledge)
	}

	return filtered, nil
}
