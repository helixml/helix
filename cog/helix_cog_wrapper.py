"""
Adapt between the helix.ml runner API and replicate.ai's cog.
Initially, cog-sdxl in particular.
"""

import os
import sys
import requests
import time
import json
import shutil
import tempfile
from pathlib import Path

# we get copied into the cog-sdxl folder so assume these modules are available
from train import train
from predict import Predictor


class CogTrainer:
    """
    A one-shot finetune.
    """
    def __init__(self, getJobURL, readSessionURL, appFolder):
        self.getJobURL = getJobURL
        self.readSessionURL = readSessionURL
        self.appFolder = appFolder

        # TODO: poll the local helix runner API for our training job, do it and then
        # exit


    def run(self):
        response = requests.get(getJobURL)
        if response.status_code != 200:
            time.sleep(0.1)
            # TODO: should we retry here?
            return

        waitLoops = 0
        last_seen_progress = 0

        task = json.loads(response.content)

        print("🟡 SDXL Finetine Job --------------------------------------------------\n")
        print(task)

        session_id = task["session_id"]
        dataset_dir = task["dataset_dir"]

        base_dir = f"/tmp/helix/results/{session_id}"
        all_tensors_dir = f"{base_dir}/all_tensors"
        final_tensors_dir = f"{base_dir}/final_tensors"
        lora_filename = "lora.safetensors"

        Path(all_tensors_dir).mkdir(parents=True, exist_ok=True)
        Path(final_tensors_dir).mkdir(parents=True, exist_ok=True)

        with tempfile.NamedTemporaryFile(suffix=".toml", delete=False) as temp:
            config_path = temp.name
        
        values = {
            'dataset_path': dataset_dir
        }
        # TODO: do something with dataset_path

        # TODO: do stuff with captions here, writing them to a csv file (or
        # eliminate captions)

        # filled_template = toml_template.format(**values)
        # with open(config_path, 'w') as f:
        #     f.write(filled_template)

        print("🟡 SDXL Config File --------------------------------------------------\n")
        print(config_path)

        # print("🟡 SDXL Config --------------------------------------------------\n")
        # print(filled_template)

        print("🟡 SDXL Inputs --------------------------------------------------\n")
        print(dataset_dir)

        print("🟡 SDXL All Outputs --------------------------------------------------\n")
        print(all_tensors_dir)

        # cliArgs.dataset_config = config_path
        # cliArgs.output_dir = all_tensors_dir

        # args = train_util.read_config_from_file(cliArgs, parser)

        print(f"[SESSION_START]session_id={session_id}", file=sys.stdout)

        # TODO: cog wants a zip file?
        output = train(input_images=dataset_dir)
        # TODO: do something with output

        shutil.move(f"{all_tensors_dir}/{lora_filename}", f"{final_tensors_dir}/{lora_filename}")
        shutil.rmtree(all_tensors_dir)

        # for testing you can return the lora from a previous finetune
        # shutil.copy(f"/tmp/helix/results/e627fb41-048b-41d9-8090-e867d0e858fc/final_tensors/{lora_filename}", f"{final_tensors_dir}/{lora_filename}")

        print(f"[SESSION_END_LORA_DIR]lora_dir={final_tensors_dir}", file=sys.stdout)



class CogInference:
    """
    A long-running inference instance.
    """
    def __init__(self, getJobURL, readSessionURL, appFolder):
        self.getJobURL = getJobURL
        self.readSessionURL = readSessionURL
        self.appFolder = appFolder

        self.predictor = Predictor()
        self.predictor.setup()



    def run(self):
        # TODO: modify the predictor so it takes the lora file as an argument
        # rather than assuming a hard-coded location
        lora_weights = []
        waiting_for_initial_session = True

        # we need to load the first task to know what the Lora weights are
        # perhaps there are no lora weights in which case we will skip
        # this step - we are not popping the task from the queue
        # rather waiting until it appears so we can know what lora weights to
        # load (if any)
        while waiting_for_initial_session:
            response = requests.get(self.readSessionURL)
            if response.status_code != 200:
                time.sleep(0.1)
                continue
            
            session = json.loads(response.content)
            waiting_for_initial_session = False
            lora_dir = session["lora_dir"]
            if lora_dir != "":
                lora_weights = [f"{lora_dir}/lora.safetensors"]

        print("🟡 Lora weights --------------------------------------------------\n")
        print(lora_weights)

        self.mainLoop()


    def mainLoop(self):
        # TODO: poll the local helix runner API for jobs
        while True:
            response = requests.get(self.getJobURL)
            if response.status_code != 200:
                time.sleep(0.1)
                continue

            # TODO: report on generation progress somehow
            last_seen_progress = 0

            task = json.loads(response.content)
            session_id = task["session_id"]

            print("🟡 SDXL Job --------------------------------------------------\n")
            print(task)

            print(f"[SESSION_START]session_id={session_id}", file=sys.stdout)

            image_paths = self.predictor.predict(
                prompt=task["prompt"],
                negative_prompt="",
                image=None,
                mask=None,
                width=1024,
                height=1024,
                num_outputs=1,
                scheduler="K_EULER",
                num_inference_steps=50,
                guidance_scale=7.5,
                prompt_strength=0.8,
                seed=42,
                refine="base_image_refiner",
                high_noise_frac=0.8,
                refine_steps=None,
                apply_watermark=False,
                lora_scale=0.6,
                replicate_weights=None,
                disable_safety_checker=True,
            )

            # TODO: rename files per f"image_{session_id}_{timestamp}_{i:03d}.png"
            timestamp = time.time()
            for i, ip in enumerate(image_paths):
                image_paths[i] = ip.rename(ip.parent / f"image_{session_id}_{timestamp:.4f}_{i:03d}.png")
    
            image_paths = [str(path) for path in image_paths]  # Convert paths to strings

            print(f"[SESSION_END_IMAGES]images={json.dumps(image_paths)}", file=sys.stdout)
            print("🟡 SDXL Result --------------------------------------------------\n")
            print(image_paths)


if __name__ == "__main__":
    print("Greetings from Helix-Cog adapter.")

    getJobURL = os.environ.get("HELIX_NEXT_TASK_URL")
    readSessionURL = os.environ.get("HELIX_INITIAL_SESSION_URL")
    
    # this points at the axolotl or cog-sdxl repo in a relative way
    # to where the helix runner is active
    appFolder = os.environ.get("APP_FOLDER")

    if getJobURL is None:
        sys.exit("HELIX_GET_JOB_URL is not set")

    if readSessionURL is None:
        sys.exit("HELIX_INITIAL_SESSION_URL is not set")

    if appFolder is None:
        sys.exit("APP_FOLDER is not set")

    print(f"🟡 HELIX_NEXT_TASK_URL {getJobURL} --------------------------------------------------\n")
    print(f"🟡 HELIX_INITIAL_SESSION_URL {readSessionURL} --------------------------------------------------\n")
    print(f"🟡 APP_FOLDER {appFolder} --------------------------------------------------\n")

    if sys.argv[1] == "inference":
        c = CogInference(getJobURL, readSessionURL, appFolder)
        c.run()
    if sys.argv[1] == "finetune":
        c = CogTrainer(getJobURL, readSessionURL, appFolder)
        c.run()

# TODO: write tests