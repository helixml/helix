from flask import Flask, request, jsonify
import pprint
import sql
from embedding import getEmbedding
from chunks import parse_document
from utils import HttpException

app = Flask(__name__)

# curl -X POST -H "Content-Type: application/json" -d '{
#   "session_id": "123",
#   "interaction_id": "456",
#   "filename": "test.txt",
#   "document_id": "abc",
#   "document_group_id": "def",
#   "offset": 0,
#   "text": "hello world"
# }' http://localhost:5000/api/v1/rag/chunk
# this route will convert the text chunk into an embedding and then store it in the database
@app.route('/api/v1/rag/chunk', methods=['POST'])
def rag_insert_chunk():
  data = request.json
  sql.checkDocumentChunkData(data)
  data["embedding"] = getEmbedding(data["content"])
  id = sql.insertData(data)
  result = sql.getRow(id)
  pprint.pprint(result)
  return jsonify(result), 200

# curl -X POST -H "Content-Type: application/json" -d '{
#   "session_id": "123",
#   "prompt": "hello world",
#   "distance_function": "cosine",
#   "distance_threshold": 0.1,
#   "max_results": 5
# }' http://localhost:5000/api/v1/rag/prompt
# this will
#  * convert the prompt
#  * conduct a search on matching records (for that session)
#  * formulate a prompt that contains the context of the matching records
#  * return the prompt alongside the matching records (so we can show provenance of what was matched in the UI) 
@app.route('/api/v1/rag/query', methods=['POST'])
def rag_query():
  data = request.json
  prompt = data["prompt"]
  session_id = data["session_id"]
  distance_threshold = data["distance_threshold"]
  distance_function = data["distance_function"]
  max_results = data["max_results"]

  if prompt is None or len(prompt) == 0:
    return jsonify({"error": "missing prompt"}), 400
  if session_id is None or len(session_id) == 0:
    return jsonify({"error": "missing session_id"}), 400
  if distance_function is None or len(distance_function) == 0:
    return jsonify({"error": "missing distance_function"}), 400
  if distance_function not in ["l2", "inner_product", "cosine"]:
    return jsonify({"error": "distance_function must be one of 'l2', 'inner_product', or 'cosine'"}), 400
  if distance_threshold is None:
    return jsonify({"error": "missing distance threshold (between 0 and 2)"}), 400
  if isinstance(distance_threshold, (int, float)) == False:
    return jsonify({"error": "distance threshold must be a number"}), 400
  if max_results is None:
    return jsonify({"error": "missing max_results"}), 400
  if isinstance(max_results, (int, float)) == False:
    return jsonify({"error": "max_results must be a number"}), 400
  promptEmbedding = getEmbedding(prompt)
  results = sql.queryPrompt(session_id, promptEmbedding, distance_function, distance_threshold, max_results)
  return jsonify({
    "ok": True,
    "results": results,
  }), 200

@app.route('/api/v1/extract', methods=['POST'])
def extract_file():
  if 'url' not in request.json:
    return jsonify({"error": "No 'url' field in the request"}), 400
  
  url = request.json['url']

  print("-------------------------------------------")
  print(f"converting URL: {url}")
  try:
    text = parse_document(url)
    print("-------------------------------------------")
    print(f"converted URL: {url} - length: {len(text)}")

    return jsonify({
      "text": text,
    }), 200
  
  except HttpException as e:
    print("-------------------------------------------")
    print(f"error URL: {url} - {str(e)}")
    return str(e), e.status_code
  except Exception as e:
    print("-------------------------------------------")
    print(f"error URL: {url} - {str(e)}")
    return str(e), 500

if __name__ == '__main__':
  app.run(debug=True, port=5000, host='0.0.0.0')
