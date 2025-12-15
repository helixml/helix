# Microsoft Teams Bot Architecture Overview

## Communication Flow

When a user sends a message to a Teams bot, the message does NOT go directly from the Teams client to your bot. Instead, it passes through Microsoft's Bot Framework Service.

```
┌─────────────────┐     ┌─────────────────────┐     ┌─────────────────┐
│  Teams Desktop  │────▶│  Microsoft Bot      │────▶│  Your Azure Bot │
│  Client         │     │  Framework Service  │     │  (Messaging     │
│                 │◀────│  (Azure cloud)      │◀────│   Endpoint)     │
└─────────────────┘     └─────────────────────┘     └─────────────────┘
        │                        │                          │
        │                        │                          │
   User types                Routes messages            Processes
   message in             to registered bots           messages and
   Teams chat              via HTTPS POST              sends responses
```

### Step-by-Step Message Flow

1. **User → Teams Client**: User types a message (or @mentions the bot) in Teams
2. **Teams Client → Bot Framework Service**: Teams sends the message to Microsoft's cloud-hosted Bot Framework Service
3. **Bot Framework Service → Your Messaging Endpoint**: Microsoft looks up your bot's registered messaging endpoint and sends an HTTPS POST request containing the message activity
4. **Your Bot → Bot Framework Service**: Your bot processes the message and sends a response back via the Bot Framework API
5. **Bot Framework Service → Teams Client**: Microsoft relays the response to the user's Teams client

## Key Components

### 1. Azure Bot Resource (Azure Portal)

The Azure Bot is registered in the Azure Portal and contains:

| Setting | Description |
|---------|-------------|
| **Microsoft App ID** | Unique identifier for your bot (from Entra/AAD) |
| **App Password** | Secret used for authentication (client secret) |
| **Messaging Endpoint** | Public HTTPS URL where Microsoft sends messages |

### 2. Entra Application (Azure AD)

The Entra (Azure AD) application provides:
- OAuth2 authentication for the bot
- App ID and App Password credentials
- Token validation for incoming webhook requests

### 3. Teams App Manifest

The manifest (`teamsapp/manifest.json`) defines:
- Bot capabilities and scopes (personal, team, groupchat)
- App icons and metadata
- The Microsoft App ID linking to your Azure Bot

### 4. Your Bot's Messaging Endpoint

Your bot must expose a publicly accessible HTTPS endpoint that:
- Receives POST requests from the Bot Framework Service
- Validates the JWT token in the Authorization header
- Processes the incoming activity (message, event, etc.)
- Returns responses via the Bot Framework API

## Internet Accessibility Requirement

**Yes, your Azure Bot's messaging endpoint MUST be internet-accessible.**

Microsoft's Bot Framework Service needs to reach your endpoint via HTTPS. Options include:

| Hosting Option | Use Case |
|----------------|----------|
| Azure App Service | Production hosting with managed SSL |
| Azure Functions | Serverless, pay-per-execution |
| Your own server | Self-hosted with valid SSL certificate |
| ngrok (dev only) | Local development tunneling |

## Authentication Flow

```
Bot Framework Service                    Your Messaging Endpoint
         │                                         │
         │  POST /api/messages                     │
         │  Authorization: Bearer <JWT>            │
         │────────────────────────────────────────▶│
         │                                         │
         │                              Validate JWT using
         │                              Microsoft's public keys
         │                                         │
         │            200 OK                       │
         │◀────────────────────────────────────────│
```

The JWT token contains:
- Issuer: `https://login.microsoftonline.com/{tenantId}/v2.0`
- Audience: Your Microsoft App ID
- Service URL for sending responses

## Cross-Tenant Setup

In this setup:
- **Azure Bot + Entra App**: Registered in your Azure/Entra tenant
- **Teams App**: Installed as custom app in a different Office 365 tenant

This works because:
1. The Teams manifest references your Azure Bot's App ID
2. Microsoft's Bot Framework Service routes messages regardless of tenant
3. Your bot validates tokens and responds through the Bot Framework API

## Related Documentation

- [Microsoft Bot Framework Overview](https://learn.microsoft.com/en-us/azure/bot-service/bot-service-overview)
- [Teams Bot Documentation](https://learn.microsoft.com/en-us/microsoftteams/platform/bots/what-are-bots)
- [Bot Framework Authentication](https://learn.microsoft.com/en-us/azure/bot-service/rest-api/bot-framework-rest-connector-authentication)
