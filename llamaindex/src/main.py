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
#   "document_id": "abc",
#   "document_group_id": "def",
#   "offset": 0,
#   "text": "hello world"
# }' http://localhost:6000/api/v1/chunk
# this route will convert the text chunk into an embedding and then store it in the database
@app.route('/api/v1/chunk', methods=['POST'])
def test():
  data = request.json
  checkDocumentChunkData(data)
  data["embedding"] = getEmbedding(data["text"])
  id = insertData(engine, data)
  result = getRow(engine, id)
  pprint.pprint(result)
  return jsonify(result), 200

if __name__ == '__main__':
  app.run(debug=True, port=6000, host='0.0.0.0')
