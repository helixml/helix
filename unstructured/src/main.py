from flask import Flask, request, jsonify
import os
from unstructured.partition.auto import partition
from unstructured.documents.elements import NarrativeText
from unstructured.chunking.title import chunk_by_title
import tempfile

app = Flask(__name__)

def parse_document(url):
  elements = partition(url=url)
  text = ""
  for element in elements:
    if isinstance(element, NarrativeText):
      text += element.text + "\n"
  return text
  # if we want unstructured to do the splitting then we mess with this
  # chunks = chunk_by_title(
  #   elements=elements,
  # )
  # texts = [element.text for element in chunks]
  # return texts

@app.route('/api/v1/extract', methods=['POST'])
def extract_file():
  if 'url' not in request.json:
    return jsonify({"error": "No 'url' field in the request"}), 400
  
  url = request.json['url']

  print("-------------------------------------------")
  print(f"converting URL: {url}")
  text = parse_document(url)
  print("-------------------------------------------")
  print(f"converted URL: {url} - length: {len(text)}")
  
  return jsonify({
    "text": text,
  }), 200

if __name__ == '__main__':
  app.run(debug=True, port=5000, host='0.0.0.0')
