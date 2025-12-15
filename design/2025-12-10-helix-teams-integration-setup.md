# Helix Microsoft Teams Integration Setup Guide

This guide walks you through connecting a Helix AI agent to Microsoft Teams, allowing users to interact with your agent directly from Teams channels and chats.

## Prerequisites

- A Helix deployment (cloud or self-hosted)
- An Azure account with permissions to create resources
- A Microsoft 365 tenant with Microsoft Teams
- Admin access to install custom Teams apps (or permission from your IT admin)

## Architecture Overview

```
┌─────────────────┐     ┌─────────────────────┐     ┌─────────────────┐
│  Teams Desktop  │────▶│  Microsoft Bot      │────▶│  Helix API      │
│  Client         │     │  Framework Service  │     │  (Webhook       │
│                 │◀────│  (Azure cloud)      │◀────│   Endpoint)     │
└─────────────────┘     └─────────────────────┘     └─────────────────┘
```

When a user messages your bot in Teams:
1. The message goes to Microsoft's Bot Framework Service
2. Microsoft forwards it to your Helix deployment's webhook endpoint
3. Helix processes the message using your configured agent
4. The response is sent back through Microsoft to the Teams client

---

## Step 1: Deploy Helix

Your Helix deployment must be accessible from the internet via HTTPS for Microsoft's Bot Framework to send messages to it.

### Option A: Helix Cloud

If you're using Helix Cloud, your deployment is already internet-accessible. Skip to Step 2.

### Option B: Self-Hosted Deployment

Follow the official Helix private deployment guide:

**[Helix Private Deployment Documentation](https://docs.helixml.tech/helix/private-deployment/)**

Key requirements for Teams integration:
- **Public HTTPS endpoint**: Your Helix API must be accessible via a public URL with a valid SSL certificate
- **Webhook URL format**: `https://your-helix-domain.com/api/v1/teams/webhook/{helix-app-id}`

For local development, you can use [ngrok](https://ngrok.com) to create a tunnel:

```bash
# Start ngrok tunnel to your local Helix API
ngrok http 8080

# Use the ngrok URL as your messaging endpoint
# Example: https://abc123.ngrok.io/api/v1/teams/webhook/{helix-app-id}
```

---

## Step 2: Create a Helix App/Agent

Before configuring Teams, you need a Helix app that will respond to messages.

### 2.1 Create a New App

1. Log in to your Helix dashboard
2. Navigate to **Apps** in the sidebar
3. Click **Create App**
4. Give your app a name (e.g., "Teams Support Bot")

### 2.2 Configure Your Agent

1. In your app settings, configure the agent behavior:
   - **System Prompt**: Define how your agent should respond
   - **Model**: Select the AI model to use
   - **Knowledge Base** (optional): Add documents for RAG

2. **Save** your app configuration

### 2.3 Note Your Helix App ID

Your Helix App ID is shown in the URL when editing your app:
```
https://your-helix.com/apps/{helix-app-id}
```

You'll need this ID for the webhook URL in Step 4.

---

## Step 3: Create Azure Resources

You need two Azure resources: an Entra ID (Azure AD) App Registration and an Azure Bot.

### 3.1 Create an Entra ID App Registration

1. Go to the [Azure Portal - App Registrations](https://portal.azure.com/#view/Microsoft_AAD_RegisteredApps/ApplicationsListBlade)

2. Click **New registration**

3. Configure the registration:
   - **Name**: `helix-teams-bot` (or your preferred name)
   - **Supported account types**: Select "Accounts in this organizational directory only" (single tenant) or "Accounts in any organizational directory" (multi-tenant) based on your needs
   - **Redirect URI**: Leave blank

4. Click **Register**

5. **Record these values** (you'll need them later):
   - **Application (client) ID** → This is your **Microsoft App ID**
   - **Directory (tenant) ID** → This is your **Tenant ID**

### 3.2 Create a Client Secret

1. In your app registration, go to **Certificates & secrets**

2. Click **New client secret**

3. Configure the secret:
   - **Description**: `helix-teams-secret`
   - **Expires**: Choose an appropriate expiration period

4. Click **Add**

5. **IMPORTANT**: Copy the **Value** column immediately!
   - This is your **App Password**
   - The value is only shown once - you cannot retrieve it later
   - Do NOT copy the "Secret ID" - that's not the password

### 3.3 Create an Azure Bot Resource

1. Go to [Create Azure Bot](https://portal.azure.com/#create/Microsoft.AzureBot)

2. Configure the bot:
   - **Bot handle**: `helix-agent-bot` (must be globally unique)
   - **Subscription**: Select your Azure subscription
   - **Resource group**: Create new or use existing
   - **Pricing tier**: Choose based on your needs (F0 is free)
   - **Microsoft App ID**: Select **"Use existing app registration"**
   - **App ID**: Enter the Application (client) ID from Step 3.1
   - **App tenant ID**: Enter the Directory (tenant) ID from Step 3.1

3. Click **Review + create**, then **Create**

4. Wait for deployment to complete

---

## Step 4: Configure the Messaging Endpoint

The messaging endpoint tells Microsoft where to send messages for your bot.

### 4.1 Construct Your Webhook URL

Your Helix webhook URL follows this format:
```
https://{your-helix-domain}/api/v1/teams/webhook/{helix-app-id}
```

**Examples:**
- Helix Cloud: `https://app.tryhelix.ai/api/v1/teams/webhook/app_abc123xyz`
- Self-hosted: `https://helix.yourcompany.com/api/v1/teams/webhook/app_abc123xyz`
- Local dev (ngrok): `https://abc123.ngrok.io/api/v1/teams/webhook/app_abc123xyz`

### 4.2 Set the Messaging Endpoint in Azure

1. Go to your Azure Bot resource in the Azure Portal

2. Navigate to **Settings** → **Configuration**

3. In the **Messaging endpoint** field, paste your webhook URL

4. Click **Apply** to save

---

## Step 5: Enable the Teams Channel

1. In your Azure Bot resource, go to **Settings** → **Channels**

2. Click **Microsoft Teams** (or the Teams icon)

3. Read and accept the Terms of Service

4. Configure channel settings:
   - **Messaging**: Enable (required)
   - **Calling**: Optional - enable if you want voice capabilities
   - **Meeting**: Optional - enable for meeting integrations

5. Click **Apply**

---

## Step 6: Create and Install the Teams App

### 6.1 Create a Teams App in Developer Portal

1. Go to [Teams Developer Portal](https://dev.teams.microsoft.com/apps)

2. Click **New app**

3. Fill in the basic information:
   - **Short name**: Your bot's display name (e.g., "Helix Assistant")
   - **Full name**: Full name of your bot
   - **Short description**: Brief description
   - **Full description**: Detailed description
   - **Developer name**: Your name or organization
   - **Website**: Your website URL
   - **Privacy policy**: Link to your privacy policy
   - **Terms of use**: Link to your terms of use

4. Click **Save**

### 6.2 Add Bot Capability

1. In your Teams app, go to **Configure** → **App features**

2. Click **Bot**

3. Configure the bot:
   - Select **Enter a bot ID**
   - **Bot ID**: Paste your **Microsoft App ID** (from Step 3.1)
   - **What can your bot do?**: Check the boxes for capabilities you want
   - **Select the scopes**: Choose where the bot can be used:
     - **Personal**: 1:1 chats with users
     - **Team**: Team channels
     - **Group chat**: Group conversations

4. Click **Save**

### 6.3 Add App Icons

1. Go to **Configure** → **Basic information**

2. Upload icons:
   - **Color icon**: 192x192 PNG (full color)
   - **Outline icon**: 32x32 PNG (transparent with white outline)

### 6.4 Install the App

**Option A: Publish to Your Organization**

1. Go to **Publish** → **Publish to org**
2. Click **Publish your app**
3. An admin must approve the app in the Teams Admin Center
4. Once approved, users can find it in the Teams app store under "Built for your org"

**Option B: Install Manually (for testing)**

1. Go to **Publish** → **Download app package**
2. This downloads a `.zip` file
3. In Teams, click **Apps** → **Manage your apps** → **Upload an app**
4. Select **Upload a custom app** and choose the `.zip` file
5. Click **Add** to install

---

## Step 7: Configure Helix Teams Trigger

Now connect everything in Helix.

### 7.1 Enable Teams Integration

1. In Helix, go to your app's settings

2. Navigate to the **Triggers** section

3. Find **Microsoft Teams** and toggle it **ON**

### 7.2 Enter Your Credentials

Enter the values from Step 3:

| Field | Value |
|-------|-------|
| **Microsoft App ID** | Application (client) ID from Entra app registration |
| **App Password** | Client secret Value (not Secret ID) |
| **Tenant ID** | Directory (tenant) ID from Entra app registration |

### 7.3 Save Configuration

1. Click **Save** to save your app configuration

2. The status indicator should show green when the integration is active

---

## Step 8: Test Your Bot

### 8.1 Find Your Bot in Teams

1. Open Microsoft Teams

2. Click **Chat** → **New chat**

3. Search for your bot by name

4. Select your bot to start a conversation

### 8.2 Send a Test Message

1. Type a message and press Enter

2. Your Helix agent should respond

### 8.3 Test in a Channel (if enabled)

1. Go to a Team where the bot is installed

2. In a channel, type `@YourBotName` followed by your message

3. The bot should respond in a thread

---

## Troubleshooting

### Bot Not Responding

1. **Check Helix logs**: Look for incoming webhook requests
   ```bash
   docker compose logs api | grep teams
   ```

2. **Verify messaging endpoint**: Ensure the URL in Azure Bot Configuration matches your Helix webhook URL

3. **Check credentials**: Verify App ID, App Password, and Tenant ID are correct in Helix

4. **Test endpoint accessibility**: Your Helix deployment must be reachable from the internet
   ```bash
   curl -I https://your-helix-domain.com/api/v1/teams/webhook/your-app-id
   ```

### Authentication Errors

- **"Unauthorized" errors**: The App Password may be incorrect or expired
- **"Invalid tenant" errors**: Check the Tenant ID matches your Entra app registration

### Bot Appears Offline

- Ensure the Teams trigger is enabled in Helix
- Verify the Azure Bot's Teams channel is enabled
- Check that the app is properly installed in Teams

### Messages Not Threading

- Helix maintains conversation context automatically
- If responses appear as new messages instead of replies, check the Helix API logs for errors

---

## Security Considerations

1. **Credential Storage**: App Password is stored securely in Helix's database

2. **Token Validation**: Helix validates JWT tokens from Microsoft's Bot Framework

3. **Tenant Restriction**: Use the Tenant ID field to restrict the bot to your organization only

4. **HTTPS Required**: The messaging endpoint must use HTTPS with a valid certificate

---

## Multi-Tenant Setup

You can install the Teams app in a different Microsoft 365 tenant than where the Azure Bot is registered:

1. When creating the Entra app registration, select **"Accounts in any organizational directory"** (multi-tenant)

2. Create the Teams app manifest with your Azure Bot's App ID

3. Install the Teams app in the target tenant as a custom app

4. The Bot Framework routes messages regardless of which tenant the user is in

---

## Next Steps

- **Add Knowledge Base**: Upload documents to your Helix app for RAG-powered responses
- **Configure Tools**: Add API integrations and tools your agent can use
- **Monitor Usage**: Check Helix analytics for conversation metrics
- **Customize Behavior**: Refine your agent's system prompt based on user interactions

---

## References

- [Helix Documentation](https://docs.helixml.tech)
- [Microsoft Bot Framework Documentation](https://learn.microsoft.com/en-us/azure/bot-service/)
- [Teams Bot Development](https://learn.microsoft.com/en-us/microsoftteams/platform/bots/what-are-bots)
- [Teams Developer Portal](https://dev.teams.microsoft.com)
