package knowledge

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-co-op/gocron/v2"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/store"
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

	jobsMap := make(map[string]gocron.Job)

	for _, job := range jobs {
		jobsMap[job.Name()] = job
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
			_, err := r.cron.NewJob(gocron.CronJob(knowledge.RefreshSchedule, true), gocron.NewTask(func() {
				fmt.Println("running job for knowledge", knowledge.ID)
			}),
				gocron.WithName(knowledge.ID),
				gocron.WithTags(fmt.Sprintf("schedule:%s", knowledge.RefreshSchedule)),
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
			tags := job.Tags()

			// current schedule
			var currentSchedule string
			for _, tag := range tags {
				if strings.HasPrefix(tag, "schedule:") {
					currentSchedule = strings.TrimPrefix(tag, "schedule:")
					break
				}
			}

			if currentSchedule != knowledge.RefreshSchedule {
				_, err := r.cron.Update(job.ID(), gocron.CronJob(knowledge.RefreshSchedule, true), gocron.NewTask(func() {
					fmt.Println("running job for knowledge", knowledge.ID)
				}), gocron.WithTags(fmt.Sprintf("schedule:%s", knowledge.RefreshSchedule)))
				if err != nil {
					return fmt.Errorf("failed to remove job: %w", err)
				}

			}
		}
	}

	return nil
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
