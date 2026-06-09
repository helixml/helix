package services

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// stageAttachmentsAndBuildPromptSection loads the task's attachments, commits any
// un-staged ones into the helix-specs branch under design/tasks/<taskDir>/attachments/,
// and returns the markdown section that tells the agent where to find them.
//
// Idempotent: attachments whose CommittedSHA is already set are skipped (so re-running
// after an API restart or retry doesn't re-commit). Failures to stage individual files
// are logged but do not block the prompt — the listing in the prompt still references
// the files so the agent can see what was attempted, even if a re-stage is needed.
func (s *SpecDrivenTaskService) stageAttachmentsAndBuildPromptSection(
	ctx context.Context,
	task *types.SpecTask,
	project *types.Project,
) (string, error) {
	attachments, err := s.store.ListSpecTaskAttachments(ctx, task.ID)
	if err != nil {
		return "", fmt.Errorf("list attachments: %w", err)
	}
	if len(attachments) == 0 {
		return "", nil
	}

	// Determine the taskDirName used in the prompt section. Falls back to task.ID for
	// backwards compatibility (matches BuildPlanningPrompt's own fallback).
	taskDirName := task.DesignDocPath
	if taskDirName == "" {
		taskDirName = task.ID
	}

	// Stage files into helix-specs if we have a repo configured. Without a repo (e.g.
	// internal-only tasks), the attachments still get listed in the prompt — the agent
	// will have to deal with their absence, but the user-facing UI still shows them.
	if project != nil && project.DefaultRepoID != "" && s.gitRepositoryService != nil {
		if err := s.commitAttachmentsToHelixSpecs(ctx, task, project, attachments); err != nil {
			log.Warn().
				Err(err).
				Str("task_id", task.ID).
				Msg("Failed to commit attachments to helix-specs (agent may not see them in workspace)")
		}
	}

	return BuildAttachmentsSection(attachments, taskDirName), nil
}

// commitAttachmentsToHelixSpecs writes each un-staged attachment as a blob in the
// helix-specs branch and updates its CommittedSHA. Uses a single WithExternalRepoWrite
// session so all attachments land in one push.
func (s *SpecDrivenTaskService) commitAttachmentsToHelixSpecs(
	ctx context.Context,
	task *types.SpecTask,
	project *types.Project,
	attachments []*types.SpecTaskAttachment,
) error {
	// Filter to only un-staged attachments.
	pending := make([]*types.SpecTaskAttachment, 0, len(attachments))
	for _, a := range attachments {
		if a.CommittedSHA == "" {
			pending = append(pending, a)
		}
	}
	if len(pending) == 0 {
		return nil
	}

	repo, err := s.store.GetGitRepository(ctx, project.DefaultRepoID)
	if err != nil {
		return fmt.Errorf("get default repository: %w", err)
	}
	if repo.LocalPath == "" {
		return fmt.Errorf("repository has no local path")
	}

	user, err := s.store.GetUser(ctx, &store.GetUserQuery{ID: task.CreatedBy})
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}
	authorName := user.FullName
	if authorName == "" {
		authorName = "Helix"
	}
	authorEmail := user.Email
	if authorEmail == "" {
		authorEmail = "helix@helix.ml"
	}

	taskDirName := task.DesignDocPath
	if taskDirName == "" {
		taskDirName = task.ID
	}
	basePath := fmt.Sprintf("design/tasks/%s/attachments", taskDirName)

	if s.ReadAttachmentBlob == nil {
		return fmt.Errorf("ReadAttachmentBlob callback not configured")
	}

	// Read all blobs out of the filestore up-front, then commit them inside the write
	// session. Reading inside the write callback works too, but keeping IO outside
	// lets us bail early if a file is missing.
	type stagedBlob struct {
		attachment *types.SpecTaskAttachment
		content    []byte
	}
	staged := make([]stagedBlob, 0, len(pending))
	for _, a := range pending {
		buf, err := s.ReadAttachmentBlob(ctx, a.FilestorePath)
		if err != nil {
			log.Warn().Err(err).Str("attachment_id", a.ID).Str("path", a.FilestorePath).Msg("Failed to read attachment blob")
			continue
		}
		staged = append(staged, stagedBlob{attachment: a, content: buf})
	}
	if len(staged) == 0 {
		return fmt.Errorf("no attachments could be read from filestore")
	}

	committedShas := make(map[string]string, len(staged)) // attachmentID -> commit SHA

	writeErr := s.gitRepositoryService.WithExternalRepoWrite(
		ctx,
		repo,
		ExternalRepoWriteOptions{
			Branch:          SpecsBranchName,
			FailOnSyncError: true,
			FailOnPushError: false, // staging is best-effort; logs warn if push failed
		},
		func() error {
			for _, sb := range staged {
				filePath := fmt.Sprintf("%s/%s", basePath, sb.attachment.Filename)
				commitSHA, _, err := CommitFileToBareBranch(
					ctx,
					repo.LocalPath,
					SpecsBranchName,
					filePath,
					sb.content,
					authorName,
					authorEmail,
					fmt.Sprintf("chore(specs): attach %s for %s", sb.attachment.Filename, task.Name),
				)
				if err != nil {
					log.Warn().Err(err).Str("attachment_id", sb.attachment.ID).Str("file", filePath).Msg("Failed to commit attachment")
					continue
				}
				committedShas[sb.attachment.ID] = commitSHA
			}
			return nil
		},
	)
	if writeErr != nil {
		return writeErr
	}

	// Persist the commit SHAs so future calls skip these attachments (idempotency).
	for attachmentID, sha := range committedShas {
		for _, a := range pending {
			if a.ID == attachmentID {
				a.CommittedSHA = sha
				if err := s.store.UpdateSpecTaskAttachment(ctx, a); err != nil {
					log.Warn().Err(err).Str("attachment_id", a.ID).Msg("Failed to persist committed_sha for attachment")
				}
				break
			}
		}
	}

	log.Info().
		Str("task_id", task.ID).
		Int("staged", len(committedShas)).
		Msg("Committed attachments to helix-specs branch")
	return nil
}

// cloneAttachmentsInRepo copies attachments from the source task (task.ClonedFromID)
// into the cloned task's helix-specs directory in the same commit window as the
// cloned specs. Also creates SpecTaskAttachment rows for the new task so they appear
// in its prompt and detail view. The caller must already hold a WithExternalRepoWrite
// session for `repo` on the helix-specs branch.
func (s *SpecDrivenTaskService) cloneAttachmentsInRepo(
	ctx context.Context,
	task *types.SpecTask,
	repo *types.GitRepository,
	authorName, authorEmail string,
) error {
	if task.ClonedFromID == "" {
		return nil
	}
	srcAttachments, err := s.store.ListSpecTaskAttachments(ctx, task.ClonedFromID)
	if err != nil {
		return fmt.Errorf("list source attachments: %w", err)
	}
	if len(srcAttachments) == 0 {
		return nil
	}
	if s.ReadAttachmentBlob == nil {
		return fmt.Errorf("ReadAttachmentBlob callback not configured")
	}

	taskDirName := task.DesignDocPath
	if taskDirName == "" {
		taskDirName = task.ID
	}
	basePath := fmt.Sprintf("design/tasks/%s/attachments", taskDirName)

	for _, src := range srcAttachments {
		buf, err := s.ReadAttachmentBlob(ctx, src.FilestorePath)
		if err != nil {
			log.Warn().Err(err).Str("attachment_id", src.ID).Msg("Failed to read source attachment — skipping in clone")
			continue
		}
		filePath := fmt.Sprintf("%s/%s", basePath, src.Filename)
		commitSHA, _, err := CommitFileToBareBranch(
			ctx,
			repo.LocalPath,
			SpecsBranchName,
			filePath,
			buf,
			authorName,
			authorEmail,
			fmt.Sprintf("chore(specs): clone attachment %s for %s", src.Filename, task.Name),
		)
		if err != nil {
			log.Warn().Err(err).Str("file", filePath).Msg("Failed to commit cloned attachment")
			continue
		}
		// Note: we set CommittedSHA so the staging step skips re-committing the same bytes.
		// FilestorePath is intentionally left equal to the source's path (a soft reference)
		// so we don't duplicate storage. The agent reads from git, not from filestore, so
		// this is fine. If the user later replaces the file via the UI, the new upload
		// creates a fresh blob and overwrites the row.
		row := &types.SpecTaskAttachment{
			ID:            system.GenerateSpecTaskAttachmentID(),
			SpecTaskID:    task.ID,
			ProjectID:     task.ProjectID,
			UserID:        task.CreatedBy,
			Filename:      src.Filename,
			MimeType:      src.MimeType,
			SizeBytes:     src.SizeBytes,
			Caption:       src.Caption,
			FilestorePath: src.FilestorePath,
			CommittedSHA:  commitSHA,
		}
		if err := s.store.CreateSpecTaskAttachment(ctx, row); err != nil {
			log.Warn().Err(err).Str("attachment_id", row.ID).Msg("Failed to create cloned attachment row")
		}
	}
	return nil
}
