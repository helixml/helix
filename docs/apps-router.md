## apps router

An app is the entrypoint for a human onto a Helix AI.

You send some text to an app and it just knows what to do.

In the background there are multiple assistants - each an expert in a specific domain.

The trick is for the app to route the text entered by the human to the correct assistant.

Each assistant has:

 * name
 * description
 * model
 * apis
   * name
   * description
   * apispec
 * gptscripts
   * name
   * description
   * code
 * RAG source & config
 * finetune data entity
 * system prompt

We currently have a similar concept with tools - when a new session is created, the active list of tools is interrogated to see if any of them can handle the input.

It's the same thing but now we have multiple AIs behind - because we have many assistants.

```yaml
name: My Marketing App
description: A marketing app that helps you with your marketing needs
assistants:
  # a copy writer that has been fine tuned on lots of emails etc
  - name: Copywriter
    description: An assistant that can write marketing material and general copy
    model: llama3:instruct
    finetune: bob-the-salesman-finetune-1.2
  # a graphic designer that has been fine tuned on corporate style
  - name: Graphic Designer
    description: An designer that can make logos, banners and other graphics
    model: stabilityai/stable-diffusion-xl-base-1.0
    finetune: corporate-imagery-finetune-1.1
  # an analyst that knows how to speak to current sales figures database
  - name: Sales Analyst
    description: An analyst that connects to realtime sales data and answers questions about it
    model: llama3:instruct
    system_prompt: |
      You are an analyst that will answer questions about sales data
      Only answer questions about sales data and nothing else
    apis:
     - name: sales-reporter-api
       description: An API that connects to the sales database
       apispec: https://api.example.com/sales-reporter-api
  # an event organiser that knows how to speak to the event management system
  - name: Event Organiser
    description: An organiser that can create and manage events
    gptscripts:
     - name: create-event
       description: A script that books and event and emails the attendees
       code: scripts/event_booker.gpt
  # a friendly onboarding expert that knows all about corporate onboarding policy
  - name: Onboarding 
    description: A friendly assistant that helps with onboarding new employees
    rag: onboarding-policy-documents-collection-2.3
```

Now you can ask:

> write me a marketing email

> design me a logo

> what were the sales figures for last month

> book a meeting for next week

> what is the onboarding policy for new employees