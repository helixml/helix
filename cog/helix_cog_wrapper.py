"""
Adapt between the helix.ml runner API and replicate.ai's cog.
Initially, cog-sdxl in particular.
"""

import json
import os
import shutil
import sys
import tempfile
import time
import zipfile
from pathlib import Path
from typing import List, Optional, Union

import requests
from fastapi import FastAPI, HTTPException
from predict import Predictor
from pydantic import BaseModel

# we get copied into the cog-sdxl folder so assume these modules are available
# TODO: parse dynamically these entrypoints from any cog yaml
from train import train

app = FastAPI()

# Define the input model based on predictor.predict parameters
class PredictionInput(BaseModel):
    prompt: str
    negative_prompt: Optional[str] = ""
    image: Optional[str] = None
    mask: Optional[str] = None
    width: Optional[int] = 1024
    height: Optional[int] = 1024
    num_outputs: Optional[int] = 1
    scheduler: Optional[str] = "K_EULER"
    num_inference_steps: Optional[int] = 50
    guidance_scale: Optional[float] = 7.5
    prompt_strength: Optional[float] = 0.8
    seed: Optional[int] = 42
    refine: Optional[str] = "base_image_refiner"
    high_noise_frac: Optional[float] = 0.8
    refine_steps: Optional[int] = None
    apply_watermark: Optional[bool] = False
    lora_scale: Optional[float] = 0.7
    replicate_weights: Optional[str] = None
    disable_safety_checker: Optional[bool] = True

# Modify CogInference to be accessible as a singleton
class CogInferenceSingleton:
    _instance = None

    @classmethod
    def get_instance(cls, getJobURL=None, readSessionURL=None):
        if cls._instance is None:
            cls._instance = CogInference(getJobURL, readSessionURL)
            cls._instance.predictor.setup()
        return cls._instance



@app.get("/healthz")
async def healthz():
    return {"status": "ok"}


# Add API endpoints
@app.post("/predictions/{session_id}")
async def create_prediction(session_id: str, input_data: PredictionInput):
    try:
        predictor = CogInferenceSingleton.get_instance().predictor
        
        # Convert input model to dict and remove None values
        input_dict = {k: v for k, v in input_data.dict().items()}
        print(input_dict)
        
        # Call predict and get image paths
        image_paths = predictor.predict(**input_dict)
        print(image_paths)
        
        timestamp = time.time()
        for i, ip in enumerate(image_paths):
            image_paths[i] = ip.rename(ip.parent / f"image_{session_id}_{timestamp:.4f}_{i:03d}.png")

        image_paths = [str(path) for path in image_paths]  # Convert paths to strings

        return {
            "status": "succeeded",
            "output": image_paths,
        }
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))

def create_zip_file(directory, output_file):
    with zipfile.ZipFile(output_file, 'w', zipfile.ZIP_DEFLATED) as zipf:
        for root, dirs, files in os.walk(directory):
            for file in files:
                file_path = os.path.join(root, file)
                # don't add the file we're writing to the zip file, or we'll get
                # into an infinite loop
                if file_path != output_file:
                    zipf.write(file_path, os.path.relpath(file_path, directory))


class CogTrainer:
    """
    A one-shot finetune.
    """
    def __init__(self, getJobURL, readSessionURL):
        self.getJobURL = getJobURL
        self.readSessionURL = readSessionURL

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
        training_dir = f"{base_dir}/training_dir"

        Path(training_dir).mkdir(parents=True, exist_ok=True)

        # TODO: eliminate captions in the UI for now (or show the ones the system generates)

        os.chdir(training_dir)

        print(f" [SESSION_START]session_id={session_id} ", file=sys.stdout, flush=True)

        input_file = str(Path(dataset_dir) / "images.zip")
        
        print("🟡 SDXL Inputs --------------------------------------------------\n")
        print(f"dataset_dir={dataset_dir}")
        print(f"input_file={input_file}")

        print("🟡 SDXL All Outputs --------------------------------------------------\n")
        print(training_dir)

        create_zip_file(dataset_dir, input_file)

        # write output into session directory
        # it's ok to do this because we're single threaded
        # TODO: would be nicer to pass the output path to cog but it just writes
        # to the cwd for training
        os.chdir(Path("/tmp/helix/results") / session_id)

        output = train(
            input_images=input_file,
            seed=42,
            resolution=768,
            train_batch_size=4,
            num_train_epochs=4000,
            max_train_steps=1000, # default
            # max_train_steps=10, # just for fast development iterations
            is_lora=True,
            unet_learning_rate=1e-6,
            ti_lr=3e-4,
            lora_lr=1e-4,
            lora_rank=32,
            lr_scheduler="constant",
            lr_warmup_steps=100,
            token_string="TOK",
            caption_prefix="a photo of TOK, ",
            mask_target_prompts=None,
            crop_based_on_salience=True,
            use_face_detection_instead=True,
            clipseg_temperature=1.0,
            verbose=True,
            checkpointing_steps=9999999,
            input_images_filetype="zip",
        )
        # TODO: do something with output

        print(f"--------------- OUTPUT ------------------")
        import pprint; pprint.pprint(output)
        print(f"-----------------------------------------")

        # move result into ./training_dir
        Path("./trained_model.tar").rename(f"{training_dir}/trained_model.tar")

        # for testing you can return the lora from a previous finetune
        # shutil.copy(f"/tmp/helix/results/e627fb41-048b-41d9-8090-e867d0e858fc/final_tensors/{lora_filename}", f"{final_tensors_dir}/{lora_filename}")

        print(f" [SESSION_END_LORA_DIR]lora_dir={training_dir} ", file=sys.stdout, flush=True)



class CogInference:
    """
    A long-running inference instance.
    """
    def __init__(self, getJobURL, readSessionURL):
        self.getJobURL = getJobURL
        self.readSessionURL = readSessionURL
        self.lora_weights = None

        # XXX: predictor.predict() assumes it can always write to /tmp/out-0.png
        # This will break when we have multiple concurrent instances of this
        # running in parallel inside the same container. Fix this by patching
        # cog-sdxl!

        self.predictor = Predictor()
        self.predictor.setup()



    def run(self):
        # TODO: modify the predictor so it takes the lora file as an argument
        # rather than assuming a hard-coded location
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
            print("🟡 GOT SESSION --------------------------------------------------\n")
            import pprint; pprint.pprint(session)
            # pick out the latest interaction with a lora_dir - that is the path
            # we need in the filestore starting with 'dev/...'
            lora_api_path = None
            for itx in session["interactions"]:
                if "lora_dir" in itx and itx["lora_dir"] != "":
                    lora_api_path = itx["lora_dir"]
            waiting_for_initial_session = False
            if lora_api_path:
                # Cog likes these weights as a URL, so construct one for it.
                # TODO: this is wasteful because the runner has already gone to
                # the bother of downloading this file for us, probably.
                # TODO: improve this, by making cog just read the weights from
                # the filesystem, rather than downloading them
                apiToken = os.getenv("API_TOKEN")
                if apiToken is None:
                    sys.exit("API_TOKEN is not set")
                if apiToken == "":
                    sys.exit("API_TOKEN is not set")
                apiHost = os.getenv("API_HOST")
                if apiHost.endswith("/"):
                    apiHost = apiHost[:-1]
                # needs to be like:
                # http://localhost/api/v1/filestore/viewer/dev/users
                #     /568a0236-b855-4615-9ecc-945a3350ea1a
                #     /sessions/6af9dcfc-a431-4331-8aca-8ddde090cf30
                #     /lora/bb9e6395-0df6-4073-8064-0ae759075b2f/trained_model.tar
                # XXX TODO: maybe we can construct url from session instead, e.g. user etc
                self.lora_weights = f"{apiHost}/api/v1/filestore/viewer/{lora_api_path}/trained_model.tar?access_token={apiToken}"

        print("🟡 Lora weights URL --------------------------------------------------\n")
        print(self.lora_weights)

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

            print(f" [SESSION_START]session_id={session_id} ", file=sys.stdout, flush=True)

            # TODO: Seems like you can pass the lora weights as a URL either in
            # setup() or at predict() time. Given the latter, which we use here,
            # we could send LoRA requests to non-LoRA instances of cog-sdxl,
            # which could be a performance/GPU memory improvement.

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
                lora_scale=0.7,
                replicate_weights=self.lora_weights,
                disable_safety_checker=True,
            )

            # TODO: rename files per f"image_{session_id}_{timestamp}_{i:03d}.png"
            timestamp = time.time()
            for i, ip in enumerate(image_paths):
                image_paths[i] = ip.rename(ip.parent / f"image_{session_id}_{timestamp:.4f}_{i:03d}.png")
    
            image_paths = [str(path) for path in image_paths]  # Convert paths to strings

            print(f" [SESSION_END_IMAGES]images={json.dumps(image_paths)} ", file=sys.stdout, flush=True)
            print("🟡 SDXL Result --------------------------------------------------\n")
            print(image_paths)


if __name__ == "__main__":
    print("Greetings from Helix-Cog adapter.")

    getJobURL = os.environ.get("HELIX_NEXT_TASK_URL")
    readSessionURL = os.environ.get("HELIX_INITIAL_SESSION_URL")

    if getJobURL is None:
        sys.exit("HELIX_GET_JOB_URL is not set")

    if readSessionURL is None:
        sys.exit("HELIX_INITIAL_SESSION_URL is not set")

    print(f"🟡 HELIX_NEXT_TASK_URL {getJobURL} --------------------------------------------------\n")
    print(f"🟡 HELIX_INITIAL_SESSION_URL {readSessionURL} --------------------------------------------------\n")

    if sys.argv[1] == "inference":
        # clean up after a previous run (XXX won't be safe to run concurrently with itself)
        os.system("rm -rf /src/weights-cache")
        c = CogInference(getJobURL, readSessionURL)
        c.run()
    if sys.argv[1] == "finetune":
        c = CogTrainer(getJobURL, readSessionURL)
        c.run()

# TODO: write tests