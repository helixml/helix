# AgentPod Design Spec

## 1. Introduction

AgentPod is an AI agent framework built around a session-based interaction model. Each user message initiates a new session that manages the entire interaction lifecycle - from input processing and context management to agent coordination and response delivery.

The framework is heavily inspired by and built upon OpenAI's [model spec](https://model-spec.openai.com/2025-02-12.html). AgentPod assumes that the user of this framework uses OpenAI models (that follow this specification). While using another model is not discouraged and it is technicaly compatibile, it may introduce behavioral considerations that developers should be aware of, including potential security implications. As of this writing, OpenAI is the only provider with a published model spec that their models are trained against, so we recommend using OpenAI models exclusively.

Here's a simple example of AgentPod initialization and usage:

```go
// initialization
agent := agentpod.NewAgent(AgentMainPrompt, []agentpod.Skill{
    skills.KeywordResearchSkill(keywordsPlace),
})
memory := agentpod.NewMem0()
storage := agentpod.NewPostgresql()
llm := agentpod.NewLLM(
    cfg.AI.KeywordsAIAPIKey, 
    cfg.AI.KeywordsAIBaseURL,
    "o1",  // reasoning model
    "gpt-4o",  // generation model
    "o1-mini",  // small reasoning model
    "gpt-4o-mini"  // small generation model
)

meta := agentpod.Meta{
    CustomerID: orgID,
    SessionID: sessionID,
    Extra: map[string]string{"user_id": userID},
}

session := agentpod.NewSession(ctx, llm, memory, agent, storage, meta)
session.In("Hey there")
msg := session.Out()
```

## 2. High-Level Architecture

The system consists of three primary layers that represent both functional components and a hierarchical organization of entities:

### Session
The Session Layer handles incoming user messages and serves as the entry point for all interactions. It performs initial LLM calls to determine next actions based on user input, updates memory with relevant information, and constructs the appropriate context for processing. Though orthogonal to the agent object itself, the session takes the agent object and invokes it to process input requests while providing necessary context. Once prepared, it invokes the agent with this enriched context to generate responses. A key feature of the Session Layer is its ability to maintain session state for pause/resume capabilities. This enables the system to handle long-running operations without keeping an active session process in memory. When a task requires extended processing time, the system can save the session state and resume from that saved state when new information becomes available, allowing for efficient resource utilization.

### Agent
The Agent Layer provides the structural framework for organizing capabilities within the system. Agents are configured with system prompts and skills that define their behavior and accessible functionality. For any given task, an agent "chooses which hat to wear" by selecting the appropriate skill for the current context. The agent's high-level run method determines if a task requires one or multiple skills. When multiple skills are needed, the agent creates separate instances for each skill context, potentially passing outputs from one skill instance to another, and merges the results together. This orchestration is controlled by the session runner. Rather than being complex entities themselves, agents primarily serve as organized containers for the skills and tools hierarchy, with the ability to coordinate between different skill contexts to accomplish complex tasks.

### Skill and Tool
The Skill and Tool Layer implements a hierarchical approach to functionality organization. Skills are defined as groups of related tools with associated system prompts that specify their domain or functionality. When an agent operates within a specific skill context (wearing a particular "hat"), it only has access to the tools that belong to that skill. This design allows developers to expose only relevant tools when needed, rather than overwhelming the LLM with all available options. Instead of implementing multiple isolated agents each with a single task, the framework enables a hierarchical organization of capabilities through skills. Each skill encapsulates a specific system prompt and a collection of tools that can be grouped logically, making the system more modular and easier to extend. An agent cannot wear different skill hats simultaneously but can switch between them as orchestrated by its high-level run method.


## FAQ

> Why don't we use "send_message" or something similar to send the messages back to the user?

As all the models becoming reasoning models, internal monologue is becoming a different thing at the model level. With that, all the non-thinking tokens the model generates must sent to the user. 

> Why do we have skills instead of sub agents?

Organizing different capabilities as skills or sub agents is just a matter of semantics. Technically, on the low level, they don't differ. It's always a call to LLM with some context, conversation history and tools. We decided to call it skills because it resonated with the idea of one agent able to do multiple things. On the technical level, there is a very minor nece-essity which is that the agent information in the prompt doesn't need to be repeated. The main prompt has things like "You are an agent and your name is Berlin" etc.

> Why the agent handoff happen at the root level instead of giving each sub agents (skilled agent execution) the ability to call other agents (skills).

This is probably a side effect of organizing capabilities as skills instead of sub agents. Although, we can make the skilled agents call other skills but the semantics doesn't make sense. Although, we are not against implementing it if that make the agent better. Why we didn't do it is because, having the handoff on each level put the pressure on the developer to figure out which skill (sub agent) is needed at what point (or they have to setup custom solution to expose all the agents all the time). In our design, the top level execution offloads the task to a Skill (sub agent) which expects the agent with that skill to go and does all the internal loops and come back with just one message to the conversation history. Then the main loop can decide if it should call another skill or can it stop the execution. The main loop never sends the conversation to the end user (except for the first fake message to keep the user engaged). The agentic execution with each skill can send the resonses to the user. 

