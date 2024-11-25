import os
import sys
import requests
import time
import json


def do_inference():
    getJobURL = os.environ.get("HELIX_NEXT_TASK_URL", None)
    readSessionURL = os.environ.get("HELIX_INITIAL_SESSION_URL", "")
    mockError = os.environ.get("HELIX_MOCK_ERROR", "")
    mockDelay = os.environ.get("HELIX_MOCK_DELAY", "")

    if getJobURL is None:
        sys.exit("HELIX_NEXT_TASK_URL is not set")

    if readSessionURL == "":
        sys.exit("HELIX_INITIAL_SESSION_URL is not set")

    waiting_for_initial_session = True

    while waiting_for_initial_session:
        response = requests.get(readSessionURL)
        if response.status_code != 200:
            time.sleep(0.1)
            continue

        waiting_for_initial_session = False

    session_id = ""

    while True:
        currentJobData = ""

        response = requests.get(getJobURL)

        if response.status_code != 200:
            time.sleep(0.1)
            continue

        currentJobData = response.content

        # print out the response content to stdout
        print("🟣🟣🟣 SDXL Finetune --------------------------------------------------")
        print("🟣🟣🟣 SDXL Finetune --------------------------------------------------")
        print("🟣🟣🟣 SDXL Finetune --------------------------------------------------")
        print("🟣🟣🟣 SDXL Finetune --------------------------------------------------")
        print(currentJobData)

        if mockError != "":
            sys.exit(f"Mock error {mockError}")

        if mockDelay != "":
            time.sleep(int(mockDelay))

        task = json.loads(currentJobData)
        session_id = task["session_id"]
        dir_path = os.getcwd() + "/runner/fixtures"
        print(f" [SESSION_START]session_id={session_id} ", file=sys.stdout, flush=True)
        print("steps:\n")
        for i in range(1, 101):
            print(f"{i}%|\n")
            time.sleep(0.1)

        print(
            f" [SESSION_END_LORA_DIR]lora_dir={dir_path} ", file=sys.stdout, flush=True
        )


if __name__ == "__main__":
    do_inference()
