# The simplest possible model runner.
# It doesn't actually run any models.

import requests
import time

# Sample response data
responses = ["Hello,", "how", "can", "I", "assist", "you", "today?"]
session_id = "123"  # replace with actual session ID

def simulate_worker():
    url = 'http://localhost/api/v1/worker/response'  # replace with actual server URL
    action = 'begin'

    for word in responses:
        message = word
        payload = {
            "action": action,
            "session_id": session_id,
            "message": message
        }

        # Post a word to the /worker/response endpoint
        print("Posting:", payload)
        r = requests.post(url, json=payload)
        print(r.text)

        action = 'continue'
        time.sleep(1)

    payload["action"] = 'end'
    print("Posting:", payload)
    r = requests.post(url, json=payload)
    print(r.text)


# Call the simulate_worker function to start the simulation
simulate_worker()
