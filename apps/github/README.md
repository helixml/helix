# GitHub Issues Assistant for Helix

This Helix app helps you manage GitHub Issues using natural language. You can interact with your GitHub repositories' issues through simple conversations.

## Features

- List and filter issues
- Create new issues
- Update existing issues
- Get issue details
- Manage labels and assignees

## Setup

1. Get your GitHub Personal Access Token:
   - Go to GitHub Settings -> Developer Settings -> Personal Access Tokens
   - Generate a new token with 'repo' scope

2. Configure environment variables:
```bash
cp .env.example .env
```

Edit .env and add:
```
GITHUB_API_TOKEN=your_github_token_here
GITHUB_OWNER=your_github_username_or_org
```

3. Deploy the app:
```bash
./deploy.sh
```

## Usage Examples

Try asking:
- "Show me all open issues in repository X"
- "Create a new issue titled 'Fix login bug' and assign it to @username"
- "List all issues assigned to me"
- "What's the status of issue #123?"
- "Show me all issues with the 'bug' label"

## Contributing

Feel free to submit issues and enhancement requests!
