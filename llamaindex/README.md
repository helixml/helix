## Testing

This code isn't setup for testing, I tried and I would have had to have rewritten it. So instead, I'm documenting stuff here.

### Development

Firstly, the docker-compose file started by ./stack start doesn't work as you'd expect. It's using a strange entrypoint env var that I can't find anywhere. So first step, comment out `# entrypoint: ${LLAMAINDEX_ENTRYPOINT:-tail -f /dev/null}` in `docker-compose.dev.yaml`.

Next, uncomment the ports definition so you can curl it later:

```
    ports:
      - 5000:5000
```

Then the database migrations are run by the code, so just restart the container and they will happen automatically.

### Tests

Writing manual tests here because there were no automated ones.

#### Bug Chunking long filenames

Context: https://mlops-community.slack.com/archives/C0675EX9V2Q/p1716475282798279

Failing test:

```bash
curl -X POST -H "Content-Type: application/json" -d '{
  "session_id": "123",
  "interaction_id": "456",
  "filename": "dev/users/f280387e-9a8b-4d21-9482-f09f145bf2a3/sessions/10e120a2-f9b7-4688-baab-56a9848fbb33/inputs/befde1e5-efb7-482d-823e-79bba64f46c9/winder.ai_a-comparison-of-reinforcement-learning-frameworks-dopamine-rllib-keras-rl-coach-trfl-tensorforce-coach-and-more_.md",
  "document_id": "abc",
  "document_group_id": "def",
  "content_offset": 0,
  "content": "hello world"
}' http://localhost:5000/api/v1/rag/chunk
```

Fix: see [llamaindex/src/migrations/versions/03_helix_document_chunk_filename.py](llamaindex/src/migrations/versions/03_helix_document_chunk_filename.py)
