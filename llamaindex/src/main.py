from flask import Flask, request, jsonify
import os
import tempfile
import requests
import pprint
import sql
from embedding import getEmbedding

app = Flask(__name__)
engine = sql.getEngine()

# curl -X POST -H "Content-Type: application/json" -d '{
#   "session_id": "123",
#   "interaction_id": "456",
#   "filename": "test.txt",
#   "document_id": "abc",
#   "document_group_id": "def",
#   "offset": 0,
#   "text": "hello world"
# }' http://localhost:6000/api/v1/chunk
# this route will convert the text chunk into an embedding and then store it in the database
@app.route('/api/v1/chunk', methods=['POST'])
def test():
  data = request.json
  sql.checkDocumentChunkData(data)
  data["embedding"] = getEmbedding(data["text"])
  id = sql.insertData(engine, data)
  result = sql.getRow(engine, id)
  pprint.pprint(result)
  return jsonify(result), 200

# curl -X POST -H "Content-Type: application/json" -d '{
#   "session_id": "123",
#   "prompt": "hello world"
# }' http://localhost:6000/api/v1/prompt
# this will
#  * convert the prompt
#  * conduct a search on matching records (for that session)
#  * formulate a prompt that contains the context of the matching records
#  * return the prompt alongside the matching records (so we can show provenance of what was matched in the UI) 
@app.route('/api/v1/prompt', methods=['POST'])
def test():
  data = request.json
  prompt = data["prompt"]
  session_id = data["session_id"]
  if prompt is None or len(prompt) == 0:
    return jsonify({"error": "missing prompt"}), 400
  if session_id is None or len(session_id) == 0:
    return jsonify({"error": "missing session_id"}), 400
  promptEmbedding = getEmbedding(prompt)
  results = sql.queryPrompt(engine, session_id, promptEmbedding)
  pprint.pprint(results)
  return jsonify({
    "ok": True,
  }), 200

if __name__ == '__main__':
  app.run(debug=True, port=6000, host='0.0.0.0')
