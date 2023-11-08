from flask import Flask, request, jsonify
import os
from unstructured.partition.auto import partition
from unstructured.documents.elements import NarrativeText
from unstructured.partition.text_type import sentence_count
import tempfile

app = Flask(__name__)

def parse_document(filename):
  elements = partition(filename=filename)
  text = ""
  for element in elements:
    if isinstance(element, NarrativeText):
        text += element.text + "\n"
        print(element.text)
        print("\n")
  return text

@app.route('/api/v1/extract', methods=['POST'])
def extract_file():
  # Create a temporary directory
  temp_dir = tempfile.mkdtemp()

  results = []

  # Check if the post request has the file part
  if 'documents' not in request.files:
    return jsonify({"error": "No files part in the request"}), 400

  files = request.files.getlist('documents')

  # Save each file in the temporary directory
  for file in files:
    if file.filename == '':
      return jsonify({"error": "No selected file"}), 400

    if file:
      file_path = os.path.join(temp_dir, file.filename)
      file.save(file_path)
      file_content = parse_document(file_path)
      results.append({
        "name": file.filename,
        "content": file_content,
      })

  return jsonify(results), 200

if __name__ == '__main__':
  app.run(debug=True, port=5000, host='0.0.0.0')
