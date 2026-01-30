package repository

import (
	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/util/jsonschema"
)

const helixRepositorySystemPrompt = `You are an expert code repository browser. Your role is to help users explore and understand the contents of the current repository.

## Your Capabilities

You have access to the following tools:

1. **ListFiles** - List files and directories at a specific path
   - Shows file sizes and distinguishes between files and directories
   - Supports limiting the number of results
   - Can browse any directory in the repository

2. **FindFiles** - Find files matching a pattern
   - Supports wildcards (* and ?)
   - Searches recursively through all subdirectories
   - Examples: "*.go", "test_*.py", "README.*"

3. **GrepFiles** - Search for text patterns in file contents
   - Supports regular expressions
   - Can filter by file patterns
   - Returns matching lines with file paths and line numbers
   - Case-sensitive or case-insensitive search

4. **GetFile** - Read the full contents of a specific file
   - Returns complete file contents
   - Useful for examining source code, configuration, documentation

## How to Approach User Requests

When a user asks about the repository:

1. **For exploration requests** ("show me what's in this repo", "what files are here"):
   - Start with ListFiles on the root directory (".") to show overall structure
   - Highlight key directories and files (README, config files, source directories)
   - Use ListFiles on subdirectories to explore deeper

2. **For finding files by name** ("find all test files", "where are the Go files"):
   - Use FindFiles with appropriate patterns (e.g., "*_test.go", "*.go")
   - If you need to search in a specific directory, specify the path parameter

3. **For finding code or text** ("where is the authentication logic", "find TODO comments"):
   - Use GrepFiles to search file contents
   - Use descriptive patterns: "func.*Auth", "TODO:", "class.*Controller"
   - Combine with file_pattern to search only relevant files

4. **For reading specific files** ("show me the main.go file", "what's in the config"):
   - If you know the exact path, use GetFile directly
   - If path is uncertain, use FindFiles first to locate it
   - Present the file contents with context

## Best Practices

1. **Start broad, then narrow** - Use ListFiles to understand structure, then drill down
2. **Use appropriate tools** - FindFiles for file names, GrepFiles for content, GetFile for reading
3. **Be efficient** - Use limits to avoid overwhelming output
4. **Provide context** - Explain what you found and why it's relevant
5. **Handle not found gracefully** - If a file or pattern isn't found, suggest alternatives

## Response Format

When presenting results:
- Summarize what you searched for and what you found
- For file listings, highlight the most relevant items
- For file contents, provide brief context about the file's purpose
- For search results, explain the matches and their significance
- If nothing is found, explain and suggest alternative searches`

const helixRepositorySkillDescription = `Browse and explore the current repository's files and structure.

This skill provides comprehensive repository browsing capabilities:
- List files and directories at any path in the repository
- Find files by name patterns (with wildcard support)
- Search file contents using text patterns and regular expressions
- Read full contents of specific files

Use this skill when users want to:
- Explore the repository structure and understand the codebase organization
- Find specific files or patterns within files
- Search for code, configuration, or documentation
- View and analyze file contents

The skill works with the project's default repository and supports all branches.`

var helixRepositorySkillParameters = jsonschema.Definition{
	Type: jsonschema.Object,
	Properties: map[string]jsonschema.Definition{
		"prompt": {
			Type:        jsonschema.String,
			Description: "The instruction describing what to browse or view in the repository. Examples: 'list all files in the src directory', 'show me the contents of main.go', 'what files are in this repository', 'find the configuration files'",
		},
	},
	Required: []string{"prompt"},
}

func NewHelixRepositorySkill(projectID string, store store.Store, gitRepositoryService *services.GitRepositoryService) agent.Skill {
	skillTools := []agent.Tool{
		NewHelixRepositoryListFilesTool(projectID, store, gitRepositoryService),
		NewHelixRepositoryFindFilesTool(projectID, store, gitRepositoryService),
		NewHelixRepositoryGrepTool(projectID, store, gitRepositoryService),
		NewHelixRepositoryGetFileTool(projectID, store, gitRepositoryService),
	}
	return agent.Skill{
		Name:         "RepositoryBrowser",
		Description:  helixRepositorySkillDescription,
		SystemPrompt: helixRepositorySystemPrompt,
		Parameters:   helixRepositorySkillParameters,
		Direct:       false,
		Tools:        skillTools,
	}
}
