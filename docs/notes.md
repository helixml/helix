# notes

## install gpu node

First install [nvidia drivers](https://docs.nvidia.com/datacenter/tesla/tesla-installation-notes/index.html)

```bash
sudo apt-get update
sudo apt-get -y install cuda-drivers
sudo apt-get -y install nvidia-cuda-toolkit
```

Then container toolkit:

```bash
curl -fsSL https://nvidia.github.io/libnvidia-container/gpgkey | sudo gpg --dearmor -o /usr/share/keyrings/nvidia-container-toolkit-keyring.gpg \
  && curl -s -L https://nvidia.github.io/libnvidia-container/stable/deb/nvidia-container-toolkit.list | \
    sed 's#deb https://#deb [signed-by=/usr/share/keyrings/nvidia-container-toolkit-keyring.gpg] https://#g' | \
    sudo tee /etc/apt/sources.list.d/nvidia-container-toolkit.list \
  && \
    sudo apt-get update
sudo apt-get install -y nvidia-container-toolkit
```

Then configure docker:

```bash
sudo nvidia-ctk runtime configure --runtime=docker
sudo systemctl restart docker
```

Now reboot machine.

Then test:

```bash
sudo nvidia-smi
sudo docker run --rm --runtime=nvidia --gpus all nvidia/cuda:11.6.2-base-ubuntu20.04 nvidia-smi
```

## example docker jobs

examples of manual docker containers

### mistral

```bash
echo "[INST]i really like you[/INST]" |docker run --gpus all -i quay.io/lukemarsden/axolotl:v0.0.1 python -u -m axolotl.cli.inference examples/mistral/qlora-instruct.yml
```

### sdxl

```bash
docker run --gpus all --workdir /app/sd-scripts -ti quay.io/lukemarsden/sd-scripts:v0.0.1 accelerate launch --num_cpu_threads_per_process 1 sdxl_minimal_inference.py --ckpt_path=sdxl/sd_xl_base_1.0.safetensors --prompt="a beautiful sunset on a distant planet with two suns and green fields, 8k, cinematic, photorealistic"
```

## design notes

### job scheduling

We have three types of request:

 * start a new chat session
 * continue an existing chat session
 * fine tune a model

#### new session

Start a new chat session needs to be a queue - if we do not have the GPUs right now - you WILL get latency because you've arrived and the shop is full.

We will try to grow and shrink the size of the shop but for those that arrive early in a spike there will be a "wait time".

#### continue session

These are vital that they respond quickly - someone is already in a chat and their experience of the product is massively impacted by latency at this point.

So - once a new session has been scheduled to a GPU - we need some kind of "multi-tenancy" coefficient that we can use to decide if we can schedule another job on the same GPU.

If this coefficient is 1 - then a single session will occupy a single GPU and when the user is not typing, the GPU has 0% utilisation.

We can then play with what the correct coefficient is for different GPUs and different models.

So the data structure is:

 * GPU
    * currently active sessions scheduled to GPU

^ this relies on the ability to multiplex conversations via context windows to a long running LLM running inside a container

QUESTION: Kai just doesn't quite yet understand the api to these containers so is the design above even possible?

#### fine tune a model

We will always need to keep some GPU's free so that folks can run batch jobs (i.e. fine tuning) on them.


### long running servers

We need inference models to start up, load the model weights into memory, and then somehow wait until new requests arrive.

What we need is a wrapper process that will use HTTP (either websockets or short polling) to wait for new jobs.  Upon initialisation, it should load the model weights and then be ready to pipe new requests into the model.

#### mistral

This will change depending on what the model is and how it works but here is the example for Mistral.

We are using axolotl to wrap the model here is the [entrypoint](https://github.com/lukemarsden/axolotl/blob/main/src/axolotl/cli/inference.py) of the code.

We will add a http entrypoint that will have the equivalent of do_inference but will be long running.

**Testing**

First up - let's get ourselves into a container and run an inference manually:

```bash
docker run --gpus all -ti --entrypoint bash quay.io/lukemarsden/axolotl:v0.0.1
echo "[INST]how are you feeling?[/INST]" | python -u -m axolotl.cli.inference examples/mistral/qlora-instruct.yml
```

So - we need to duplicate `do_inference` and change the `get_multi_line_input` function (which reads from stdin)

Whilst iterating I just ran the container above and then used poor mans git:

```bash
cd axolotl/src/axolotl/cli
cat __init__.py | ssh kai@beefy bash -c 'cat | docker exec -i 8b9a8531e242 bash -c "cat > src/axolotl/cli/__init__.py"'
```

Now you can change the do_inference to start the http client loop and iterate.

Then - we just run the axolotl inference again but giving it a http url to ask for jobs.

The http URL is a pointing back at the Helix api server from wherever the container is running.

Our container will only run Mode=create, Type=text and ModelName=mistralai/Mistral-7B-Instruct-v0.1

So we pass those filters to the URL to ensure we get the correct type of task:

```bash
export HELIX_GET_JOB_URL='http://192.168.86.24/api/v1/worker/task?mode=Create&type=Text&model_name=mistralai/Mistral-7B-Instruct-v0.1'
python -u -m axolotl.cli.inference examples/mistral/qlora-instruct.yml
```

Now the container will get jobs targeted to it.