from flask import Flask, request, jsonify
import os
import tempfile
import requests
import pprint
from sql import getEngine, checkDocumentChunkData, insertData, getRow
from embedding import getEmbedding

app = Flask(__name__)
engine = getEngine()

# curl -X POST -H "Content-Type: application/json" -d '{
#   "session_id": "123",
#   "interaction_id": "456",
#   "filename": "test.txt",
#   "offset": 0,
#   "text": "hello world"
# }' http://localhost:6000/api/v1/chunk
@app.route('/api/v1/chunk', methods=['POST'])
def test():
  data = request.json
  pprint.pprint(data)
  checkDocumentChunkData(data)
  data["embedding"] = getEmbedding(data["text"])
  pprint.pprint(data["embedding"])
  id = insertData(engine, data)
  pprint.pprint(id)
  result = getRow(engine, id)
  return jsonify(result), 200

if __name__ == '__main__':
  app.run(debug=True, port=6000, host='0.0.0.0')
