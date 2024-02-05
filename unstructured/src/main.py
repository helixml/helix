from flask import Flask, request, jsonify
import os
from unstructured.partition.auto import partition
from unstructured.documents.elements import NarrativeText
from unstructured.chunking.title import chunk_by_title
import tempfile
import requests
from bs4 import BeautifulSoup
import html2text

app = Flask(__name__)

class HttpException(Exception):
  def __init__(self, message, status_code):
    super().__init__(message)
    self.status_code = status_code

def download_url(url):
  response = requests.get(url)
  if response.status_code == 200:
    temp_file = tempfile.NamedTemporaryFile(delete=False)
    temp_file.write(response.content)
    temp_file.close()
    return temp_file.name, response.headers.get('Content-Type')
  else:
    raise HttpException(f"Download failed with {url} {response.status_code}: {response.text}", response.status_code)

# set to false to use html2text
USE_BEAUTIFUL_SOUP = False

def parse_document(url):

  # download url to temporary location
  fname, mimeType = download_url(url)

  print(f"Got mimeType {mimeType}")
  if mimeType.startswith("text/html"):
    if USE_BEAUTIFUL_SOUP:
      # beautiful soup does a better job than unstructured on html
      gfg = BeautifulSoup(open(fname).read())

      maybeArticle = gfg.find('article')
      if maybeArticle:
        # Extracting data for article section
        bodyHtml = maybeArticle
      else:
        bodyHtml = gfg
  
      # Calculating result
      res = bodyHtml.get_text()

      os.unlink(fname)
      return res
    else:
      # but html2text does an even better job (and outputs markdown which LLMs
      # like)
      h = html2text.HTML2Text()
      h.ignore_links = True
      h.body_width = 0
      h.images_to_alt = True
      return h.handle(open(fname).read())


  # otherwise fall back to unstructured
  elements = partition(filename=fname)
  text = ""
  for element in elements:
    if hasattr(element, "text"):
      text += element.text + "\n"

  os.unlink(fname)
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
