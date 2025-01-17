# helix-github-issues

A Helix-powered app that enables natural language interaction with GitHub Issues. Talk to your GitHub issues using simple, conversational language and get structured, clear responses.

## Step-by-Step Setup Guide

### Step 1: Prerequisites

You'll need:
- GitHub Account
- Helix Account (sign up at app.tryhelix.ai)
- Terminal/Command Line access

### Step 2: Get Your Access Tokens

**GitHub Token:**
1. Go to GitHub.com → Settings → Developer Settings
2. Select "Personal Access Tokens" → "Tokens (classic)"
3. Generate new token with 'repo' permissions
4. Copy and save the token

**Helix API Key:**
1. Log into app.tryhelix.ai
2. Click on the three dots beside your name
3. Click on Account & API
4. Copy Helix API Key

### Step 3: Set Up Environment

Open terminal and run these commands:
bash

export GITHUB_OWNER=your_github_username
export GITHUB_API_TOKEN=your_github_token
export HELIX_API_KEY=your_helix_api_key

Verify they're set:
echo $GITHUB_OWNER
echo $GITHUB_API_TOKEN
echo $HELIX_API_KEY

### Step 4: Create Project

bash

Create directory

mkdir github-assistant

cd github-assistant

Create helix.yaml file

touch helix.yaml


### Step 5: Add Configuration

Copy this into helix.yaml:
piVersion: app.aispec.org/v1alpha1
kind: AIApp
metadata:
name: Github-issues-Assistant
spec:
assistants:
model: llama3.1:8b-instruct-q8_0
type: text
system_prompt: |
You are a GitHub Issues expert. Present issues clearly and include:
Issue title and number
Current status (open/closed)
Assignees and labels
apis:
name: GitHub Issues API
description: List repository issues
schema: |-
openapi: 3.0.0
info:
title: GitHub Issues API
description: List repository issues
version: "1.0.0"
servers:
url: https://api.github.com
paths:
/repos/{owner}/{repo}/issues:
get:
operationId: listIssues
parameters:
name: owner
in: path
required: true
schema:
type: string
name: repo
in: path
required: true
schema:
type: string
name: state
in: query
schema:
type: string
enum: [open, closed, all]
responses:
'200':
description: List of issues
url: https://api.github.com


### Step 6: Test Configuration

bash

Run test

helix test -f helix.yaml

You should see test results and get an app ID


### Step 7: Try It Out

1. Go to app.tryhelix.ai
2. Find your GitHub Issues assistant
3. Try these queries:
   - "Show me open issues in helix repository"
   - "List closed issues from last week"
   - "Show issues labeled bug"

## Troubleshooting Tips

If you get errors:
1. Check environment variables are set correctly
2. Verify GitHub token has 'repo' access
3. Make sure repository names are correct
4. Check Helix API key is valid

## Common Commands

bash

Test configuration

helix test -f helix.yaml

View test results

cat summary_latest.md

Check environment

echo $GITHUB_OWNER
