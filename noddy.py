# The simplest possible model runner.
# It doesn't actually run any models.
import requests
import time
import json

API_SERVER = "http://localhost/api/v1"

# Sample response data
responses = ["Hello,", "how", "can", "I", "assist", "you", "today?"]

def get_task(url):
    response = requests.get(url)
    if response.status_code == 200:
        return response.json()
    else:
        print(f"Failed to fetch tasks: {response.status_code}")
        return None

def simulate_worker(task):
    response_url = API_SERVER + '/worker/response'  # replace with the actual server URL
    session_id = task['SessionID']
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
        r = requests.post(response_url, json=payload)
        print(r.text)

        action = 'continue'
        time.sleep(1)

    payload["action"] = 'end'
    print("Posting:", payload)
    r = requests.post(response_url, json=payload)
    print(r.text)


# Continuous loop to poll tasks and process them
task_url = API_SERVER + '/worker/task'  # replace with the actual server URL
while True:
    task = get_task(task_url)
    if task:
        print("Fetched Task:", json.dumps(task, indent=4))
        simulate_worker(task)

    print("Waiting for the next task...")
