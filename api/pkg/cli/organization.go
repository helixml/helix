package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

func LookupOrganization(ctx context.Context, apiClient client.Client, orgRef string) (*types.Organization, error) {
	// If the reference doesn't start with org_ prefix, assume it's a name
	if strings.HasPrefix(orgRef, system.OrganizationPrefix) {

		// Get by ID
		organization, err := apiClient.GetOrganization(ctx, orgRef)
		if err != nil {
			return nil, fmt.Errorf("failed to get organization: %w", err)
		}
		return organization, nil
	}

	// List organizations to find the one with matching name
	organizations, err := apiClient.ListOrganizations(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list organizations: %w", err)
	}

	for _, o := range organizations {
		if o.Name == orgRef || o.ID == orgRef {
			return o, nil
		}
	}

	return nil, fmt.Errorf("organization not found: %s", orgRef)
}

// ResolveOrganization resolves an organization flag to an org ID.
// If orgFlag is provided, it looks up the org by name or ID.
// If orgFlag is empty, it defaults to the user's first organization.
// Returns empty string if no org is found (backward-compatible).
func ResolveOrganization(ctx context.Context, apiClient client.Client, orgFlag string) (string, error) {
	if orgFlag != "" {
		org, err := LookupOrganization(ctx, apiClient, orgFlag)
		if err != nil {
			return "", err
		}
		return org.ID, nil
	}

	orgs, err := apiClient.ListOrganizations(ctx)
	if err != nil {
		return "", nil
	}
	if len(orgs) > 0 {
		return orgs[0].ID, nil
	}

	return "", nil
}

// ResolveOrganizationInteractive resolves an organization for interactive CLI use.
// Rules:
//   - If orgFlag is provided, look it up by name or ID (same as ResolveOrganization).
//   - If orgFlag is empty and the user belongs to exactly one org, use it and print a note.
//   - If orgFlag is empty and the user belongs to multiple orgs, prompt interactively.
//   - If orgFlag is empty and the user has no orgs, return an error.
func ResolveOrganizationInteractive(ctx context.Context, apiClient client.Client, orgFlag string) (string, error) {
	if orgFlag != "" {
		org, err := LookupOrganization(ctx, apiClient, orgFlag)
		if err != nil {
			return "", err
		}
		return org.ID, nil
	}

	orgs, err := apiClient.ListOrganizations(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to list organizations: %w", err)
	}

	switch len(orgs) {
	case 0:
		return "", fmt.Errorf("no organizations found; create one in the Helix UI first")
	case 1:
		fmt.Fprintf(os.Stderr, "Using organization: %s\n", orgs[0].Name)
		return orgs[0].ID, nil
	default:
		return promptSelectOrganization(orgs)
	}
}

func promptSelectOrganization(orgs []*types.Organization) (string, error) {
	fmt.Fprintln(os.Stderr, "Multiple organizations found. Select one:")
	for i, o := range orgs {
		fmt.Fprintf(os.Stderr, "  [%d] %s (%s)\n", i+1, o.Name, o.ID)
	}
	fmt.Fprint(os.Stderr, "Enter number: ")

	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read selection: %w", err)
	}

	n, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || n < 1 || n > len(orgs) {
		return "", fmt.Errorf("invalid selection %q — enter a number between 1 and %d", strings.TrimSpace(line), len(orgs))
	}

	selected := orgs[n-1]
	fmt.Fprintf(os.Stderr, "Using organization: %s\n", selected.Name)
	return selected.ID, nil
}

// LookupTeam looks up a team by name or ID in an organization
func LookupTeam(ctx context.Context, apiClient *client.HelixClient, orgID, ref string) (*types.Team, error) {
	if strings.HasPrefix(ref, system.TeamPrefix) {
		team, err := apiClient.GetTeam(ctx, orgID, ref)
		if err != nil {
			return nil, fmt.Errorf("failed to get team: %w", err)
		}
		return team, nil
	}

	teams, err := apiClient.ListTeams(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to list teams: %w", err)
	}

	for _, team := range teams {
		if team.Name == ref {
			return team, nil
		}
	}

	return nil, fmt.Errorf("team not found: %s", ref)
}
