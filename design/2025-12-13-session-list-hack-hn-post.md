# We Got Impatient and Implemented ACP Session List In Our Zed Fork

**Draft HN Post - 2025-12-13**

---

## The Problem: Zed Doesn't Remember Your AI Sessions

We're building [Helix](https://helix.ml), an open-source AI dev platform. Part of it involves running AI coding agents (think Claude Code, but with any model) inside sandboxed desktop containers. We use Zed as the IDE because it's fast, Rust-based, and has a nice agent panel.

One problem: when you restart the container, all your AI conversation history disappears.

Well, not *really* disappears. The sessions are still there on disk. Zed just doesn't know how to find them again.

## ACP: The Protocol That Almost Works

Zed has this thing called ACP (Agent Client Protocol) - basically LSP but for AI agents. Your agent (Claude Code, our qwen-code fork, whatever) speaks ACP over stdin/stdout to Zed. Messages flow back and forth. Tool calls happen. Files get edited.

ACP even has `session/list` and `session/load` endpoints. In theory, you can:
1. Ask the agent "what sessions do you have?"
2. Pick one
3. Resume it

In practice? Zed only calls `list_sessions` *after* you've already started a new thread. The sessions are right there! But you can't see them when Zed starts up because nobody asks for them.

## "Surely Someone Will Fix This"

We waited. We filed issues. We made sad faces in Discord.

Zed is moving fast on agent stuff, but they're focused on their native agent, not external ACP agents. Our use case - ephemeral sandboxes that restart constantly - isn't really on their radar.

After three days of debugging why session resume wasn't working, we realized the problem wasn't in our agent. It wasn't in the session storage. It was in Zed's `agent_panel.rs` - the code just... didn't ask.

## The Fix: 90 Lines of Rust

```rust
/// Load ACP sessions from all configured external agents at startup.
fn load_acp_sessions_from_agents(&self, cx: &mut Context<Self>) {
    let external_agent_names: Vec<SharedString> = project.read(cx)
        .agent_server_store()
        .read(cx)
        .external_agents()
        .map(|name| name.0.clone())
        .collect();

    for agent_name in external_agent_names {
        cx.spawn(async move |_this, mut cx| {
            Self::load_sessions_from_agent(
                agent_name, fs, project, history_store, root_dir, &mut cx
            ).await
        }).detach();
    }
}
```

Called from `AgentPanel::new()`. When Zed starts, it now queries each external agent for their session list. Sessions show up in the history. Click one, and `load_thread_from_agent()` resumes it.

## The Other Bug We Found Along The Way

This is the fun part. While debugging why sessions weren't loading, we discovered our `settings-sync-daemon` was generating broken JSON:

```json
{
  "agent_servers": {
    "qwen": {
      "command": "qwen",
      "args": ["--experimental-acp"]
    }
  }
}
```

Zed deserializes this with a tagged enum that requires `"type": "custom"`. Without it, serde silently fails, and `external_agents()` returns an empty iterator. Our entire session loading code was working perfectly - it just had nothing to iterate over.

One line fix: `"type": "custom"`.

Three days of debugging.

## Should You Do This?

Probably not. We're maintaining a fork of Zed now. Every time they push an update, we have to merge.

But for our use case - AI coding in ephemeral sandboxes - it's worth it. Users expect their conversations to persist. When the sandbox restarts (and it will restart), they need to pick up where they left off.

The alternative was telling users "sorry, just start over" every time their container recycled. That's not a product.

## What We Actually Wanted

Honestly? We wish Zed had this out of the box:

1. **Session list at startup** - Query all configured ACP agents for their sessions
2. **Session resume on reconnect** - When a thread ID comes in that's not in memory, try loading it from the agent
3. **Unified history** - Show native Zed threads and external agent sessions in the same list

We've implemented all three in our fork. It's ~200 lines of Rust total.

If you're using Zed with external ACP agents and want session persistence, you can grab our changes from `helixml/zed`. Or wait for upstream to maybe implement it someday.

Or, you know, just use Helix and let us deal with it.

---

**Repo:** https://github.com/helixml/zed (fork with session fixes)

**Commits:**
- `feat(agent_panel): load ACP sessions from external agents at startup`
- `fix: Load session from agent when thread not in registry`

**The real lesson:** If you're building on top of fast-moving open source projects, budget time for "the feature that's almost there but not quite."
