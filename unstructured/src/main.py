from flask import Flask, request, jsonify
import os
# from unstructured.partition.auto import partition
import tempfile

app = Flask(__name__)

# User-defined function
def parse_document(filename):
  elements = partition(filename=filename)
  print("\n\n".join([str(el) for el in elements]))
  # Implement the parsing logic here
  # This is just a placeholder function for demonstration purposes
  print(f"Parsing document: {file_path}")

@app.route('/api/v1/convert', methods=['POST'])
def upload_files():
  # Create a temporary directory
  temp_dir = tempfile.mkdtemp()

  file_paths = []

  # Check if the post request has the file part
  if 'documents' not in request.files:
    return jsonify({"error": "No files part in the request"}), 400

  files = request.files.getlist('documents')

  # Save each file in the temporary directory
  for file in files:
    if file.filename == '':
      return jsonify({"error": "No selected file"}), 400

    if file:
      filename = secure_filename(file.filename)
      file_path = os.path.join(temp_dir, filename)
      file.save(file_path)
      file_paths.append(file_path)

      # Call user defined function
      parse_document(file_path)

  return jsonify({"message": "Files successfully uploaded and parsed", "file_paths": file_paths}), 200

@app.route('/api/v1/hello', methods=['GET'])
def hello():
  return jsonify({"message": "Hello2"}), 200

if __name__ == '__main__':
  app.run(debug=True, port=5000, host='0.0.0.0')
