# Weekly Project Report: Jan 30 - Feb 6, 2026

**168 commits. 3 repos. One big week.**

This was one of those weeks where you look at the diff and realize the product moved. Not incrementally — *structurally*. We shipped a new auth architecture, gave agents real-world tool access, made the mobile experience actually usable, and laid the groundwork for measuring how good our coding agent really is.

Here's what happened.

---

## Helix Got a New Security Model

The biggest change this week isn't a feature — it's a foundation.

We designed and implemented a **Backend-for-Frontend (BFF) authentication pattern**, replacing the old approach where auth tokens sat in the browser's local storage. That's the pattern most SPAs use, and it works fine until you're in an enterprise security review and someone asks "so where do you store your access tokens?" and the answer is "in JavaScript-accessible storage."

Now the server manages sessions. Tokens live server-side in secure HTTP-only cookies that JavaScript can't touch. CSRF protection is built in. Token refresh happens silently in the background. WebSocket auth flows through cookies instead of query parameters.

What this means in practice: a security-conscious team evaluating Helix for internal use no longer has to file an exception for client-side token storage. The auth model is what their security team expects to see.

The migration is clean — the old token management code was fully removed from the frontend, not just deprecated. No dead code, no feature flags, no "we'll clean this up later."

---

## Agents Can Actually Do Things Now

### From Chat to Action

This was the week MCP skills went from "interesting demo" to "genuinely useful."

We shipped **project-level MCP support**, meaning agents inherit tools scoped to their project. Then we added **stdio transport** so agents can spin up local tool servers as child processes. Then we wired in two built-in skills: **Drone CI** and **GitHub**.

Picture this: a developer pushes code, a CI build fails, and they ask their Helix agent "what happened?" The agent queries the Drone CI MCP server, pulls the failing step's logs (with smart truncation so it doesn't choke on 50MB of output), identifies the error, and suggests a fix. All inside the chat.

The setup experience got simpler too — adding a local MCP tool is now a single command input instead of a multi-field form.

### GitHub OAuth

We landed native GitHub OAuth integration. Connect your GitHub account and Helix can access your private repos directly. No more copying personal access tokens around. This is the connective tissue between "Helix as a chat tool" and "Helix as something that works with your actual code."

---

## A Frontend That Feels Like a Product

44 commits went into the frontend this week, and collectively they cross a threshold. Helix's UI went from "functional" to something that feels like a product people would choose to use.

**Global search** is live. You can find projects, specs, tasks — anything — from a single search bar. It sounds basic, but if you've been manually clicking through project lists to find something, this changes your daily workflow.

**Notifications** landed. The system tells you when things happen — tasks pending review, status changes, completed agent work. It's the difference between having to go check on things and being told when things need your attention.

**The backlog view** got a proper table layout with inline-editable priority and automatic re-sorting. Change a task's priority and it slides into its new position. Product managers and team leads will live in this view.

And then there's **mobile**. Not "responsive design" in the sense of "it technically renders on a phone." Actual mobile work: PWA install support so you can add Helix to your home screen, swipe gestures for natural navigation, a sidebar that works on small screens, and scroll behavior that doesn't fight with you. This is a mobile experience someone might actually choose to use — check your backlog on the train, review an agent's work from the couch, get a notification that a spec task is done while you're away from your desk.

---

## Multi-Tenancy That Works for Teams

A series of fixes that matter most when you're not a solo user.

**Organization domain auto-join** means users with matching email domains (say, `@acme.com`) automatically land in the right org when they sign up. No admin has to manually invite every new hire.

**Project-to-org migration** got smart. When you move a personal project into an organization, the system now warns you about agents that might lose access and repos that will need re-scoping. Before, you'd move the project and then discover things were broken.

**Project secrets** are now gated by real role-based access checks, not just project ownership. If you're managing API keys across a team, the access model is what you'd expect.

**Google OIDC** got fixed properly — logout actually logs you out (it wasn't before), the account picker shows up, and refresh tokens work when offline access is enabled. The kind of fixes that seem small but were blocking real deployments.

---

## Dev Containers: Boring but Important

If you run Helix dev containers, this week fixed the kind of issues that make you lose an afternoon.

Multiple dev containers now **share a single BuildKit cache**, so rebuilding an image doesn't start from zero because a different container built it last time. When BuildKit is unavailable, you get an **immediate error** instead of hanging for minutes before timing out. Cache directory permissions work for **non-root containers** now — a classic Docker gotcha, finally handled. And Docker bridge networking for sandbox containers is stable, which means the intermittent failures in Helix-in-Helix mode are gone.

---

## Kodit: Benchmarks and Apple Silicon

Four commits, but they set important direction.

**ARM64 Docker support** means Kodit now runs natively on Apple Silicon. No more Rosetta emulation layer eating performance during local development. If you're on an M-series Mac, builds and runs are noticeably faster.

**SWE-bench benchmarking** is the bigger strategic move. We set up a harness to evaluate Kodit against the industry-standard SWE-bench dataset — real GitHub issues from real repositories. This is how you stop guessing whether your coding agent is improving and start measuring it. We also fixed an issue where Kodit tried to index git submodules, which was causing it to choke on repos with large vendored dependencies.

---

## Launchpad: Deploy Without the Pain

17 commits went into making Helix easier to deploy on Kubernetes, specifically for teams that want production-grade setups.

Every major cloud now has **specific, tested TLS instructions** — GKE, EKS, AKS, bare metal. On Google Cloud, **ManagedCertificate support** means you can get automatic HTTPS with zero cert-manager configuration. The deployment wizard now **validates your domain** before generating Helm values, catching typos before they become failed deployments. And there's **static IP reservation guidance** so your DNS records don't break when pods reschedule.

The marketing pages also got an **enterprise visual redesign** — cleaner, more polished, and pitched at the buyers who care about how a product looks before they care about how it works.

---

## Performance

Two changes worth calling out.

The frontend build toolchain jumped from **Vite 4.5 to 7.3** with SWC replacing Babel. Dev server startup is faster, hot reload is faster, production builds are faster. This is one of those invisible changes that makes every developer's day slightly better.

Git repos in workspace setup now **clone in parallel**. If your project has five repos, setup takes the time of the slowest clone, not the sum of all five.

---

## What It All Adds Up To

A week ago, Helix stored auth tokens in the browser, agents couldn't talk to your CI system, there was no search, no notifications, no mobile experience worth using, and deploying on Kubernetes required reading between the lines of sparse docs.

Now it doesn't.

That's the kind of week you want to have.
