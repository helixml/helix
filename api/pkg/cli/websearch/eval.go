package websearch

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/websearch/eval"
	"github.com/helixml/helix/api/pkg/websearch/exa"
)

const exaBaseURL = "https://api.exa.ai"

func newEvalCommand() *cobra.Command {
	var (
		dataDir          string
		outDir           string
		runFile          string
		judgeURL         string
		judgeModel       string
		judgeConcurrency int
		captureReference bool
	)
	cmd := &cobra.Command{
		Use:   "eval",
		Short: "Score Helix search relevance against a frozen Exa reference",
		Long: `Runs the frozen query set against Helix (HELIX_URL, HELIX_API_KEY), grades
every result with an LLM judge (JUDGE_API_KEY) and reports Helix beside the
frozen Exa reference. --capture-reference instead records the Exa reference
once (EXA_API_KEY). See design/websearch-eval/README.md.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			queries, err := eval.LoadQueries(filepath.Join(dataDir, eval.QueriesFile))
			if err != nil {
				return err
			}
			refPath := filepath.Join(dataDir, eval.ReferenceFile)
			if captureReference {
				provider, err := exa.New(os.Getenv("EXA_API_KEY"), exaBaseURL)
				if err != nil {
					return err
				}
				ref := &eval.Run{Provider: "exa", CapturedAt: time.Now().UTC()}
				if _, err := os.Stat(refPath); err == nil {
					if ref, err = eval.LoadRun(refPath); err != nil {
						return err
					}
				}
				added, err := eval.ExtendReference(ctx, ref, queries, provider.Search)
				if err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "captured %d new queries; existing reference entries unchanged\n", added)
				if added == 0 {
					return nil
				}
				return eval.SaveRun(refPath, ref)
			}

			if outDir == "" {
				return errors.New("--out is required")
			}
			reference, err := eval.LoadRun(refPath)
			if err != nil {
				return err
			}
			judge, err := eval.NewLLMJudge(judgeURL, os.Getenv("JUDGE_API_KEY"), judgeModel)
			if err != nil {
				return err
			}
			judgmentsPath := filepath.Join(dataDir, eval.JudgmentsFile)
			judgments, err := eval.LoadJudgments(judgmentsPath, judgeModel)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(outDir, 0o755); err != nil {
				return err
			}

			var candidate *eval.Run
			if runFile != "" {
				candidate, err = eval.LoadRun(runFile)
			} else {
				candidate, err = searchHelix(cmd, queries, filepath.Join(outDir, "helix_run.json"))
			}
			if err != nil {
				return err
			}

			pools, err := eval.Pools(queries, candidate, reference)
			if err != nil {
				return err
			}
			judged, judgeErr := eval.JudgeAll(ctx, judge, judgments, queries, pools, judgeConcurrency, func(graded int) error {
				fmt.Fprintf(cmd.ErrOrStderr(), "judged %d pages, saving %s\n", graded, judgmentsPath)
				return judgments.Save(judgmentsPath)
			})
			if err := judgments.Save(judgmentsPath); err != nil {
				return err
			}
			if judgeErr != nil {
				return judgeErr
			}

			report, err := eval.BuildReport(queries, judgments, candidate, reference)
			if err != nil {
				return err
			}
			report.NewJudgments = judged
			if err := eval.WriteReportFiles(outDir, report); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%d new judgments; report in %s\n", judged, outDir)
			return nil
		},
	}
	cmd.Flags().StringVar(&dataDir, "data-dir", "api/pkg/websearch/eval/testdata", "Directory with the frozen queries, Exa reference and judgment cache")
	cmd.Flags().StringVar(&outDir, "out", "", "Directory for report.md, report.json and helix_run.json")
	cmd.Flags().StringVar(&runFile, "run", "", "Score a saved helix_run.json instead of searching Helix again")
	cmd.Flags().StringVar(&judgeURL, "judge-url", "http://localhost:8080/v1", "OpenAI-compatible base URL of the judge")
	cmd.Flags().StringVar(&judgeModel, "judge-model", "glm-5.3-flash", "Judge model; the judgment cache is bound to it")
	cmd.Flags().IntVar(&judgeConcurrency, "judge-concurrency", 8, "Parallel judge calls")
	cmd.Flags().BoolVar(&captureReference, "capture-reference", false, "Capture the Exa reference for queries it has none for (existing entries stay frozen) and exit")
	return cmd
}

func searchHelix(cmd *cobra.Command, queries []eval.Query, savePath string) (*eval.Run, error) {
	apiClient, err := client.NewClientFromEnv()
	if err != nil {
		return nil, err
	}
	run, err := eval.Collect(cmd.Context(), "helix", queries, apiClient.WebSearch)
	if err != nil {
		return nil, err
	}
	return run, eval.SaveRun(savePath, run)
}
