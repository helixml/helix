package org

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	orgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
	"github.com/helixml/helix/api/pkg/org/interfaces/jsonapi"
	"github.com/spf13/cobra"
)

func newProcessorsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "processors",
		Short:   "List helix-org processors",
		Aliases: []string{"processor", "proc"},
	}
	cmd.AddCommand(newProcessorsListCmd())
	return cmd
}

func newProcessorsListCmd() *cobra.Command {
	var orgFlag string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List processors in an organization",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := newHTTPClient()
			if err != nil {
				return err
			}
			orgID, err := c.resolveOrg(cmd.Context(), orgFlag)
			if err != nil {
				return err
			}
			// JSON:API document — data is []Resource with ProcessorAttributes.
			var doc jsonapi.Document
			if err := c.doJSON(cmd.Context(), http.MethodGet, "/orgs/"+orgID+"/processors", nil, &doc, 30*time.Second); err != nil {
				return err
			}
			if jsonOut {
				return printJSON(doc)
			}
			resources, err := decodeProcessorResources(doc.Data)
			if err != nil {
				return err
			}
			fmt.Printf("%-28s %-16s %s\n", "ID", "KIND", "NAME")
			for _, r := range resources {
				attrs, err := processorAttrs(r)
				if err != nil {
					return err
				}
				fmt.Printf("%-28s %-16s %s\n", r.ID, attrs.Kind, attrs.Name)
			}
			if len(resources) == 0 {
				fmt.Println("(none)")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&orgFlag, "org", "", "Organization id or name (or $HELIX_ORG)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output")
	return cmd
}

func decodeProcessorResources(data any) ([]jsonapi.Resource, error) {
	if data == nil {
		return nil, nil
	}
	// data arrives as []any / map after json.Unmarshal into Document.Data (any).
	// Re-marshal into the typed Resource slice.
	bts, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	var resources []jsonapi.Resource
	if err := json.Unmarshal(bts, &resources); err != nil {
		// Single resource form.
		var one jsonapi.Resource
		if err2 := json.Unmarshal(bts, &one); err2 != nil {
			return nil, fmt.Errorf("decode processors data: %w", err)
		}
		if one.ID == "" {
			return nil, nil
		}
		return []jsonapi.Resource{one}, nil
	}
	return resources, nil
}

func processorAttrs(r jsonapi.Resource) (orgapi.ProcessorAttributes, error) {
	var attrs orgapi.ProcessorAttributes
	if r.Attributes == nil {
		return attrs, nil
	}
	// Attributes may already be ProcessorAttributes or a map[string]any.
	bts, err := json.Marshal(r.Attributes)
	if err != nil {
		return attrs, err
	}
	if err := json.Unmarshal(bts, &attrs); err != nil {
		return attrs, fmt.Errorf("decode processor attributes for %s: %w", r.ID, err)
	}
	return attrs, nil
}
