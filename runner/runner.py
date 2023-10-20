import json
import time
import os
import base64
import subprocess
from datetime import datetime
import requests

def do_loop():
    # the url of where we ask for new jobs
    # as soon as we have finished the current job, we will ask for another one
    # if this fails - it means there are no jobs so wait 1 second then ask again
    getJobURL = os.environ.get("HELIX_GET_JOB_URL", None)
    respondJobURL = os.environ.get("HELIX_RESPOND_JOB_URL", None)
    
    if getJobURL is None:
        sys.exit("HELIX_GET_JOB_URL is not set")

    if respondJobURL is None:
        sys.exit("HELIX_RESPOND_JOB_URL is not set")

    waitLoops = 0

    while True:
        response = requests.get(getJobURL)

        if response.status_code != 200:
            time.sleep(0.1)
            waitLoops = waitLoops + 1
            if waitLoops % 10 == 0:
                print("--------------------------------------------------\n")
                current_timestamp = datetime.now().strftime('%Y-%m-%d %H:%M:%S')
                print(f"{current_timestamp} top level waiting for next job")
            continue

        waitLoops = 0

        # print out the response content to stdout
        print("--------------------------------------------------\n")
        print(response.content)
        print("--------------------------------------------------\n")

        task = json.loads(response.content)
        
        # copy the initial job into the environment
        # each of the scripts will run this right away before asking for more jobs
        env = os.environ.copy()
        env["HELIX_INITIAL_JOB_DATA_BASE64"] = base64.b64encode(response.content.decode().encode("utf-8"))
        
        if task["mode"] == "Create" and task["type"] == "Text":
            print("TEXT JOB")
            env["APP_FOLDER"] = "../../axolotl"
            proc = subprocess.Popen(["bash", "venv_command.sh", "python", "-u", "-m", "axolotl.cli.inference", "examples/mistral/qlora-instruct.yml"], env=env)
            proc.wait()
        elif task["mode"] == "Create" and task["type"] == "Image":
            print("SDXL JOB")

if __name__ == "__main__":
    do_loop()