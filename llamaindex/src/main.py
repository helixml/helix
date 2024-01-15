from flask import Flask, request, jsonify
import os
import tempfile
import requests
from sql import getEngine

####################
#
# APP ROUTES
#
####################

app = Flask(__name__)
engine = getEngine()

@app.route('/api/v1/test', methods=['GET'])
def test():
  import pprint; pprint.pprint(request.json)
  return jsonify({
    "ok": True,
  }), 200

if __name__ == '__main__':
  app.run(debug=True, port=6000, host='0.0.0.0')
