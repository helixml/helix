## unstructured

This is a quick way for us to process any document into plain text.

We use the hosted api at: https://unstructured-io.github.io/unstructured/api.html

We can always host our own version at some point.

```bash
cd ~/Downloads
curl -X 'POST' \
'https://api.unstructured.io/general/v0/general' \
-H 'accept: application/json' \
-H 'Content-Type: multipart/form-data' \
-H "unstructured-api-key: $UNSTRUCTURED_API_KEY" \
-F 'files=@./shakespeare/hamlet_PDF_FolgerShakespeare.pdf'
```

## python

```bash
cd worker
source ./venv/bin/activate
python main.py --file ~/Downloads//shakespeare/hamlet_PDF_FolgerShakespeare.pdf
```