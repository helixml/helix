import os
import sys
import requests
import time
import json
import builtins

def print(*args, **kwargs):
    kwargs['flush'] = True
    return builtins.print(*args, **kwargs)

def do_inference():
    getJobURL = os.environ.get("HELIX_NEXT_TASK_URL", None)
    readSessionURL = os.environ.get("HELIX_INITIAL_SESSION_URL", "")
    mockError = os.environ.get("HELIX_MOCK_ERROR", "")
    mockDelay = os.environ.get("HELIX_MOCK_DELAY", "")

    if getJobURL is None:
        sys.exit("HELIX_NEXT_TASK_URL is not set")

    if readSessionURL == "":
        sys.exit("HELIX_INITIAL_SESSION_URL is not set")
    
    lora_dir = ""
    waiting_for_initial_session = True

    while waiting_for_initial_session:
        response = requests.get(readSessionURL)
        if response.status_code != 200:
            time.sleep(0.1)
            continue
        
        session = json.loads(response.content)
        waiting_for_initial_session = False
        lora_dir = session["lora_dir"]

    if lora_dir != "":
        print("🟡🟡🟡 SDXL Lora dir --------------------------------------------------\n")
        print(lora_dir)

    session_id = ""
    
    while True:
        currentJobData = ""

        response = requests.get(getJobURL)

        if response.status_code != 200:
            time.sleep(0.1)
            continue

        currentJobData = response.content

        # print out the response content to stdout
        print("🟣🟣🟣 SDXL Job --------------------------------------------------")
        print(currentJobData)

        if mockError != "":
            sys.exit(f"Mock error {mockError}")

        if mockDelay != "":
            time.sleep(int(mockDelay))

        task = json.loads(currentJobData)
        instruction: str = task["prompt"]
        session_id = task["session_id"]
        image_path = os.getcwd() + "/runner/fixtures/image.png"
        print(f" [SESSION_START]session_id={session_id} ", file=sys.stdout)

        for i in range(1, 101):
          print(f"{i}%|\n")
          time.sleep(0.1)
        
        print(f" [SESSION_END_IMAGES]images=[\"{image_path}\"] ", file=sys.stdout)

if __name__ == "__main__":
    do_inference()