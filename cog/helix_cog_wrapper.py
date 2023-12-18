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
import zipfile

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

        # cog wants a tar file?

        output_file = str(Path(dataset_dir) / "images.zip")

        create_zip_file(dataset_dir, output_file)

        print("!!!!!!!!!!!!!!!!!!!!!!!!")
        print(f"dataset_dir={dataset_dir}")
        print(f"output_file={output_file}")
        print("!!!!!!!!!!!!!!!!!!!!!!!!")

        output = train(
            input_images=output_file,
            seed=42,
            resolution=768,
            train_batch_size=4,
            num_train_epochs=4000,
            # max_train_steps=1000, # default
            max_train_steps=10, # just for fast development iterations
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

        shutil.move(f"{all_tensors_dir}/{lora_filename}", f"{final_tensors_dir}/{lora_filename}")
        shutil.rmtree(all_tensors_dir)

        # for testing you can return the lora from a previous finetune
        # shutil.copy(f"/tmp/helix/results/e627fb41-048b-41d9-8090-e867d0e858fc/final_tensors/{lora_filename}", f"{final_tensors_dir}/{lora_filename}")

        print(f"[SESSION_END_LORA_DIR]lora_dir={final_tensors_dir}", file=sys.stdout)



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
                # Cog likes these weights as a URL, so construct one for it.
                # TODO: when we start enforcing auth on the filestore, we'll
                # need to pass in our API_TOKEN as well or something. But see
                # below for something to do instead.
                # TODO: this is wasteful because the runner has already gone to
                # the bother of downloading this file for us, probably.
                # TODO: improve this, by making cog just read the weights from
                # the filesystem, rather than downloading them
                apiHost = os.getenv("API_HOST")
                # needs to be like:
                # http://localhost/api/v1/filestore/viewer/dev/users/568a0236-b855-4615-9ecc-945a3350ea1a/sessions/6af9dcfc-a431-4331-8aca-8ddde090cf30/inputs/143a79cc-2f5a-4efc-980d-8b07aae623d1/IMG_0004.jpg
                # XXX TODO: maybe we can construct url from session instead, e.g. user etc
                self.lora_weights = f"{apiHost}/{lora_dir}"

        print("🟡 Lora weights --------------------------------------------------\n")
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

            print(f"[SESSION_START]session_id={session_id}", file=sys.stdout)

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

    if getJobURL is None:
        sys.exit("HELIX_GET_JOB_URL is not set")

    if readSessionURL is None:
        sys.exit("HELIX_INITIAL_SESSION_URL is not set")

    print(f"🟡 HELIX_NEXT_TASK_URL {getJobURL} --------------------------------------------------\n")
    print(f"🟡 HELIX_INITIAL_SESSION_URL {readSessionURL} --------------------------------------------------\n")

    # cog fine tuner writes files to current working directory.
    # we might be running concurrently with ourselves, so switch to a new random directory

    # Switch to a new random temporary directory
    with tempfile.TemporaryDirectory() as tmpdir:
        os.chdir(tmpdir)

        if sys.argv[1] == "inference":
            c = CogInference(getJobURL, readSessionURL)
            c.run()
        if sys.argv[1] == "finetune":
            c = CogTrainer(getJobURL, readSessionURL)
            c.run()

# TODO: write tests