# GDPR Compliance Analyzer — Demo Video Script

## Opening (15 sec)

"Today I'm going to show you how to build an AI-powered compliance analyzer using Helix's RAG capabilities. We'll upload a real privacy policy and automatically evaluate it against GDPR requirements — all powered by a Helix app with knowledge-backed RAG."

## Part 1: What is Helix RAG? (30 sec)

"Helix lets you create AI apps that have access to your own documents through RAG — Retrieval-Augmented Generation. You upload documents to a knowledge store, Helix chunks and indexes them, and then when your app gets a question, it automatically searches the relevant documents and includes that context in the AI's prompt. The AI answers based on your actual data, not just its training."

## Part 2: The Helix App Setup (show Helix UI) (45 sec)

"Let me show you how this works in the Helix dashboard."

**[Show Helix UI — App creation page]**

"Here I've created a Helix app. It has an assistant configured with a knowledge source — a filestore where we upload privacy policy documents. When we make API calls to this app, Helix automatically searches this knowledge store and enriches the prompt with relevant chunks from the documents."

**[Show the knowledge source configuration]**

"The knowledge source is configured with RAG settings — how many results to return, chunk size — all tunable depending on your use case."

## Part 3: The Demo App (show browser) (30 sec)

"Now let me show you the compliance analyzer. This is a React app that talks to the Helix API."

**[Show the Setup page]**

"We connect to our Helix instance with an API key. The app automatically creates a fresh Helix app for each analysis — so each document gets its own isolated knowledge store. No cross-contamination between analyses."

## Part 4: Upload & Index (show upload flow) (30 sec)

**[Upload a privacy policy PDF]**

"We upload a privacy policy — this can be a PDF, Markdown, or plain text. The app uploads it to the Helix filestore, triggers indexing, and we can watch the progress. Helix is chunking the document, generating embeddings, and storing them for retrieval."

**[Show indexing progress completing]**

"Once indexing is complete, we're ready to analyze."

## Part 5: The Analysis (show analysis running) (45 sec)

**[Click "Run GDPR Analysis"]**

"Now the app evaluates 10 core GDPR requirements against the privacy policy. It fires these off in parallel — grouped by category — so we get results fast. Each evaluation is a separate API call to Helix. The app sends the GDPR requirement, Helix searches the indexed privacy policy via RAG, and the AI evaluates whether the requirement is met."

**[Show results trickling in as categories complete]**

"You can see results coming in as each category finishes — lawful basis, transparency, data subject rights, security."

## Part 6: Results Walkthrough (show results page) (60 sec)

**[Show the results matrix]**

"Here's our compliance matrix. Each tile is a GDPR article. Green means the policy clearly covers it. Amber means it's partially addressed — something is vague or incomplete. Red means it's a gap — the policy doesn't address it at all."

**[Show the classification legend]**

"The legend explains exactly how these classifications are determined."

**[Click on a specific control tile — e.g., Art.17 Right to Erasure]**

"Let's drill into one. Article 17 — Right to Erasure. The AI found that the privacy policy mentions data deletion but doesn't specify the circumstances where the right applies or the exceptions. That's flagged as partial coverage."

**[Click on another — e.g., Art.33 Data Breach Notification]**

"And here — breach notification. The AI found no mention of the 72-hour supervisory authority notification requirement. That's a gap."

## Part 7: Analyze Another Document (15 sec)

**[Click "Analyze Another Document"]**

"And because each analysis creates its own isolated app, we can immediately analyze a different privacy policy. The previous results don't interfere — clean slate, fresh RAG index."

## Closing (15 sec)

"That's Helix RAG in action — document ingestion, intelligent retrieval, and AI-powered analysis, all through a simple API. You can build apps like this for compliance, contract review, policy auditing — anything where AI needs to reason over your specific documents."
