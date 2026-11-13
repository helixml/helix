// Package websearch is the `helix websearch` command group: the
// Exa-compatible web search API from the command line.
package websearch

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/websearch"
)

func New() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "websearch",
		Short: "Search the web, read pages and get cited answers",
	}
	cmd.AddCommand(newSearchCommand(), newContentsCommand(), newAnswerCommand(), newEvalCommand())
	return cmd
}

func newSearchCommand() *cobra.Command {
	var (
		numResults int
		searchType string
		category   string
		textChars  int
		jsonOutput bool
	)
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search the web",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			apiClient, err := client.NewClientFromEnv()
			if err != nil {
				return err
			}
			req := &websearch.SearchRequest{
				Query:      strings.Join(args, " "),
				NumResults: numResults,
				Type:       searchType,
				Category:   category,
			}
			if textChars > 0 {
				req.Contents = &websearch.ContentsOptions{Text: &websearch.TextOption{
					Enabled: true, TextOptions: websearch.TextOptions{MaxCharacters: textChars},
				}}
			}
			resp, err := apiClient.WebSearch(cmd.Context(), req)
			if err != nil {
				return err
			}
			if jsonOutput {
				return printJSON(cmd.OutOrStdout(), resp)
			}
			printResults(cmd.OutOrStdout(), resp.Results)
			return nil
		},
	}
	cmd.Flags().IntVarP(&numResults, "num-results", "n", 10, "Number of results")
	cmd.Flags().StringVar(&searchType, "type", "", "Search type: auto, lite, deep-lite or deep")
	cmd.Flags().StringVar(&category, "category", "", "Category, e.g. news, company, people")
	cmd.Flags().IntVar(&textChars, "text", 0, "Include up to this many characters of each page's text")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Print the full JSON response")
	return cmd
}

func newContentsCommand() *cobra.Command {
	var (
		maxChars   int
		jsonOutput bool
	)
	cmd := &cobra.Command{
		Use:   "contents <url-or-id>...",
		Short: "Fetch the text of web pages by URL or search result id",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			apiClient, err := client.NewClientFromEnv()
			if err != nil {
				return err
			}
			req := &websearch.ContentsRequest{ContentsOptions: websearch.ContentsOptions{
				Text: &websearch.TextOption{Enabled: true, TextOptions: websearch.TextOptions{MaxCharacters: maxChars}},
			}}
			// The API takes either ids or urls, never both.
			if strings.HasPrefix(args[0], "http://") || strings.HasPrefix(args[0], "https://") {
				req.URLs = args
			} else {
				req.IDs = args
			}
			resp, err := apiClient.WebSearchContents(cmd.Context(), req)
			if err != nil {
				return err
			}
			if jsonOutput {
				return printJSON(cmd.OutOrStdout(), resp)
			}
			printResults(cmd.OutOrStdout(), resp.Results)
			for _, status := range resp.Statuses {
				if status.Error != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "%s: %s\n", status.ID, status.Error.Tag)
				}
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&maxChars, "max-chars", 0, "Truncate each page's text to this many characters (0: no limit)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Print the full JSON response")
	return cmd
}

func newAnswerCommand() *cobra.Command {
	var (
		stream     bool
		jsonOutput bool
	)
	cmd := &cobra.Command{
		Use:   "answer <question>",
		Short: "Answer a question from web search results, with citations",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if stream && jsonOutput {
				return fmt.Errorf("--stream and --json cannot be combined")
			}
			apiClient, err := client.NewClientFromEnv()
			if err != nil {
				return err
			}
			req := &websearch.AnswerRequest{Query: strings.Join(args, " ")}
			out := cmd.OutOrStdout()
			if stream {
				var citations []websearch.Result
				err := apiClient.StreamWebSearchAnswer(cmd.Context(), req, func(ev websearch.AnswerEvent) error {
					for _, choice := range ev.Choices {
						fmt.Fprint(out, choice.Delta.Content)
					}
					citations = append(citations, ev.Citations...)
					return nil
				})
				if err != nil {
					return err
				}
				fmt.Fprintln(out)
				printCitations(out, citations)
				return nil
			}
			resp, err := apiClient.WebSearchAnswer(cmd.Context(), req)
			if err != nil {
				return err
			}
			if jsonOutput {
				return printJSON(out, resp)
			}
			if text, ok := resp.Answer.(string); ok {
				fmt.Fprintln(out, text)
			} else if err := printJSON(out, resp.Answer); err != nil {
				return err
			}
			printCitations(out, resp.Citations)
			return nil
		},
	}
	cmd.Flags().BoolVar(&stream, "stream", false, "Print the answer as it is generated")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Print the full JSON response")
	return cmd
}

func printResults(w io.Writer, results []websearch.Result) {
	for i, r := range results {
		fmt.Fprintf(w, "%d. %s\n   %s\n", i+1, r.Title, r.URL)
		if r.Text != nil && *r.Text != "" {
			fmt.Fprintf(w, "\n%s\n", *r.Text)
		}
		fmt.Fprintln(w)
	}
}

func printCitations(w io.Writer, citations []websearch.Result) {
	if len(citations) == 0 {
		return
	}
	fmt.Fprintln(w, "\nSources:")
	for i, c := range citations {
		fmt.Fprintf(w, "[%d] %s\n    %s\n", i+1, c.Title, c.URL)
	}
}

func printJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
