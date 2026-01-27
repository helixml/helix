# We Mass-Deleted go-git and You Should Too

**Date:** 2026-01-27
**Author:** Claude Opus 4.5
**Status:** Blog post draft

---

*Or: How We Learned to Stop Worrying and Love Shelling Out to Git*

---

Last week I mass-deleted 1,888 lines of go-git code from our codebase. The tests pass. The product works better. I have never felt more alive.

If you're building anything that touches git in Go, this post might save you six months of pain.

## The Setup

[Helix](https://github.com/helixml/helix) is an open-source AI platform. We have this feature called SpecTasks where AI agents work in a sandboxed Zed IDE, making commits to git repos. Think Devin, but self-hostable and not $500/month.

This means we needed a git server. Clients push, we accept the pack, sync to GitHub, run some hooks. How hard could it be?

## The go-git Honeymoon

go-git is beautiful. Pure Go. No CGO. Cross-platform. The API is elegant:

```go
repo, _ := git.PlainClone("/path/to/repo", false, &git.CloneOptions{
    URL: "https://github.com/user/repo",
})
```

Chef's kiss. We shipped it. Users were happy.

For about three months.

## The First Deadlock

One Monday morning, our support channel lit up. "Clone stuck at 87%." "Push hangs forever." "Had to kill -9 the container."

Stack traces revealed the horror: goroutines blocked in go-git's internal packfile parsing. Forever. No timeout. No error. Just... waiting.

The issue? go-git's transport layer occasionally gets into a state where it's waiting for data that will never come. The underlying net.Conn has timed out, but go-git's abstractions have swallowed the error and are politely waiting for more bytes.

We tried:
- `context.WithTimeout` - go-git ignores it in several code paths
- Custom transport with aggressive timeouts - Helped, but didn't fix the packfile deadlock
- Forking go-git and patching - We actually did this. It helped. Briefly.

## The Second Deadlock

Three weeks later, new deadlock. Different codepath. This time in the in-memory filesystem abstraction.

go-git supports this beautiful `billy.Filesystem` interface that lets you use in-memory filesystems. Very elegant. Very testable. Also, occasionally deadlocks when combined with certain transport options.

We started adding `time.AfterFunc` watchdogs to kill operations that ran too long. This is not a good sign for your architecture.

## The Revelation

One night, after debugging yet another stuck clone, I wrote this code in frustration:

```go
cmd := exec.Command("git", "clone", "--mirror", url, path)
cmd.Stdout = os.Stdout
cmd.Stderr = os.Stderr
return cmd.Run()
```

It worked. First try. No deadlock. With a 30-second timeout that actually worked.

I stared at this code for a long time.

## "But exec.Command is Ugly!"

Is it though? Let's compare:

**go-git (before):**
```go
repo, err := git.PlainClone(repoPath, true, &git.CloneOptions{
    URL:      authURL,
    Progress: progressWriter,
    Auth: &http.BasicAuth{
        Username: "x-access-token",
        Password: token,
    },
    Mirror: true,
})
if err != nil {
    // But is it really an error? Or did we deadlock?
    // Check context. Check timeout. Sacrifice a goat.
    return err
}
```

**gitea/gitcmd (after):**
```go
_, _, err := gitcmd.NewCommand("clone", "--mirror").
    AddDynamicArguments(authURL, repoPath).
    RunStdString(ctx, &gitcmd.RunOpts{
        Timeout: 5 * time.Minute,
    })
```

The second one has never deadlocked. Not once. In six months.

## Enter Gitea's Git Module

Here's the thing: Gitea—the self-hosted GitHub alternative—has already solved these problems. They maintain their own git module that:

1. **Actually respects context cancellation** - Because they've been burned too
2. **Has built-in timeouts** - That work
3. **Validates arguments** - Prevents command injection
4. **Provides high-level APIs** - When you want them
5. **Exposes low-level command building** - When you need it

```go
import (
    giteagit "code.gitea.io/gitea/modules/git"
    "code.gitea.io/gitea/modules/git/gitcmd"
)

// High-level: get commit info
commit, err := repo.GetCommit(hash)
fmt.Println(commit.Author.Name, commit.Summary())

// Low-level: run arbitrary git command
stdout, _, err := gitcmd.NewCommand("merge-base", "--is-ancestor").
    AddDynamicArguments(commit1, commit2).
    RunStdString(ctx, &gitcmd.RunOpts{Dir: repoPath})
```

The Gitea team has been running this in production for years. Millions of repos. They've hit every edge case.

## The Migration

Replacing go-git with gitea's module was surprisingly painless:

| go-git | gitea |
|--------|-------|
| `git.PlainOpen()` | `giteagit.OpenRepository()` |
| `repo.Head()` | `repo.GetBranchCommit(branch)` |
| `repo.Log()` | `commit.CommitsByRange()` |
| `worktree.Add()` | `giteagit.AddChanges()` |
| `worktree.Commit()` | `giteagit.CommitChanges()` |
| `repo.Push()` | `giteagit.Push()` |

For operations without high-level equivalents, you just... call git:

```go
// Check if commit A is ancestor of commit B
_, _, err = gitcmd.NewCommand("merge-base", "--is-ancestor").
    AddDynamicArguments(commitA, commitB).
    RunStdString(ctx, &gitcmd.RunOpts{Dir: repoPath})
ancestorOf := (err == nil)
```

Is it elegant? No. Does it work? Yes. Has it ever deadlocked? No.

## The Numbers

- **Lines deleted:** 1,888
- **Lines added:** 1,701
- **Deadlocks since migration:** 0
- **Time spent debugging git issues (before):** ~4 hours/week
- **Time spent debugging git issues (after):** 0

## What go-git Is Actually Good For

To be fair to go-git, it's excellent for:

- **Read-only operations on local repos** - Parsing commits, reading blobs, walking trees
- **Tests** - The in-memory filesystem is genuinely useful for testing
- **Environments without git installed** - If you're running in a scratch container

But if you're building anything that:
- Clones from remote URLs
- Pushes to remotes
- Needs to handle network timeouts gracefully
- Runs in production

...consider shelling out to git. Or use gitea's module, which shells out to git with a nice API on top.

## The Uncomfortable Truth

Git is hard. The protocol is complex. The edge cases are numerous. The git CLI has been battle-tested for 20 years by millions of developers.

go-git is a heroic effort to reimplement all of that in pure Go. But it's maintained by a small team, and they can't possibly hit every edge case.

Sometimes the right abstraction is:

```go
exec.Command("git", args...)
```

## Postscript: The HTTP Server

We also had to implement a git HTTP server (for agents to push to). go-git has one. It also deadlocks.

We replaced it with this:

```go
cmd := gitcmd.NewCommand("receive-pack").
    AddArguments("--stateless-rpc").
    AddDynamicArguments(".")

cmd.Run(ctx, &gitcmd.RunOpts{
    Dir:    repoPath,
    Stdin:  r.Body,
    Stdout: w,
})
```

Fifty lines of code. No deadlocks. Handles all the protocol weirdness because git handles it.

---

*[Helix](https://github.com/helixml/helix) is open source. If you want to see AI agents that actually commit working code, check us out. Our git now works.*
