import pprint
import signal
import time
import traceback
import uuid
from typing import List, Optional
import os

# Set default LOG_LEVEL if it doesn't exist to prevent axolotl from crashing
if os.environ["LOG_LEVEL"] == "":
    os.environ["LOG_LEVEL"] = "INFO"

import torch
import transformers
from axolotl.cli import (
    check_accelerate_default_config,
    check_user_token,
    load_cfg,
    load_datasets,
)
from axolotl.common.cli import TrainerCliArgs, load_model_and_tokenizer
from axolotl.core.trainer_builder import AxolotlTrainingArguments
from axolotl.train import train
from axolotl.utils import callbacks
from axolotl.utils.callbacks import mlflow_
from axolotl.utils.chat_templates import chat_templates
from axolotl.utils.config import normalize_config
from fastapi import BackgroundTasks, FastAPI, HTTPException
from pydantic import BaseModel
from transformers import GenerationConfig, TextStreamer

app = FastAPI()


class TrainingProgressReport(BaseModel):
    type: str = "training_progress_report"
    loss: float
    grad_norm: float
    learning_rate: float
    epoch: float
    progress: int


class Hyperparameters(BaseModel):
    n_epochs: Optional[int] = 4
    batch_size: Optional[int] = 1
    learning_rate_multiplier: Optional[float] = 1.0


class CreateFineTuningJobRequest(BaseModel):
    training_file: str
    model: str
    validation_file: Optional[str] = None
    hyperparameters: Optional[Hyperparameters] = Hyperparameters()
    suffix: str


class FineTuningJob(BaseModel):
    id: str
    model: str
    training_file: str
    validation_file: Optional[str]
    status: str
    created_at: int
    fine_tuned_model: Optional[str] = None
    hyperparameters: Hyperparameters
    trained_tokens: Optional[int] = 0
    result_files: Optional[List[str]] = []
    integrations: Optional[List[dict]] = []
    seed: Optional[int] = 0
    estimated_finish: Optional[int] = 0


class FineTuningEvent(BaseModel):
    id: str
    created_at: int
    level: str
    message: str
    data: Optional[dict] = None


class FineTuningEventList(BaseModel):
    object: str
    data: list[FineTuningEvent]
    has_more: bool


# In-memory storage to mock database
fine_tuning_jobs = {}
fine_tuning_events = {}


def generate_job_id():
    return f"ftjob-{uuid.uuid4()}"


def generate_model_id(model: str, org: str, suffix: str, job_id: str):
    return f"ft:{model}:{org}:{suffix}:{job_id}"


def suffix_from_model_id(job_id: str):
    s = job_id.split(":")
    if len(s) > 3:
        return s[3]
    return job_id


class HelixCallback(callbacks.TrainerCallback):
    def __init__(self, axolotl_config_path):
        self.axolotl_config_path = axolotl_config_path

    def on_step_end(
        self,
        args: AxolotlTrainingArguments,
        state: callbacks.TrainerState,
        control: callbacks.TrainerControl,
        **kwargs,
    ):
        loss, grad_norm, learning_rate, epoch, progress = 0.0, 0.0, 0.0, 0.0, 0
        epoch = state.epoch
        progress = int(100.0 * (state.epoch / state.num_train_epochs))
        if len(state.log_history) > 0:
            hist = state.log_history[-1]
            if "loss" in hist:
                loss = hist["loss"]
            if "grad_norm" in hist:
                grad_norm = hist["grad_norm"]
            if "learning_rate" in hist:
                learning_rate = hist["learning_rate"]

        report = TrainingProgressReport(
            epoch=epoch,
            progress=progress,
            loss=loss,
            grad_norm=grad_norm,
            learning_rate=learning_rate,
        )
        add_fine_tuning_event(args.run_name, "info", report.model_dump_json())


# Function to run fine-tuning using Axolotl
def run_fine_tuning(
    job_id: str, model: str, training_file: str, hyperparameters: Hyperparameters
):
    orig_signal = signal.signal
    # Override signal, required so axolotl doesn't try and use signals in a thread
    signal.signal = lambda sig, handler: True

    try:
        # Update job status to running
        fine_tuning_jobs[job_id].status = "running"
        add_fine_tuning_event(job_id, "info", "Fine-tuning job started.")

        parsed_cfg = unified_config(job_id, training_file, "", hyperparameters.n_epochs)

        cli_args = TrainerCliArgs()
        dataset_meta = load_datasets(cfg=parsed_cfg, cli_args=cli_args)

        train(cfg=parsed_cfg, cli_args=cli_args, dataset_meta=dataset_meta)

        # Update job status to succeeded
        fine_tuning_jobs[job_id].status = "succeeded"
        fine_tuning_jobs[job_id].fine_tuned_model = job_id
        fine_tuning_jobs[job_id].result_files = [parsed_cfg["output_dir"]]
        add_fine_tuning_event(job_id, "info", "Fine-tuning job completed successfully.")

    except Exception:
        # Handle any errors that occur during the fine-tuning process
        fine_tuning_jobs[job_id].status = "failed"
        add_fine_tuning_event(
            job_id,
            "error",
            f"Fine-tuning job failed: {traceback.format_exc()}.",
        )
        print(traceback.format_exc())

    signal.signal = orig_signal


def add_fine_tuning_event(job_id: str, level: str, message: str):
    event = FineTuningEvent(
        id=str(uuid.uuid4()), created_at=int(time.time()), level=level, message=message
    )
    if job_id not in fine_tuning_events:
        fine_tuning_events[job_id] = []
    fine_tuning_events[job_id].append(event)


# 1. Create Fine-tuning Job
@app.post("/v1/fine_tuning/jobs", response_model=FineTuningJob)
async def create_fine_tuning_job(
    request: CreateFineTuningJobRequest, background_tasks: BackgroundTasks
):
    job_id = generate_job_id()
    job = FineTuningJob(
        id=job_id,
        model=request.model,
        training_file=request.training_file,
        validation_file=request.validation_file,
        fine_tuned_model=generate_model_id(
            request.model, "helix", request.suffix, job_id
        ),
        status="queued",
        created_at=int(time.time()),
        hyperparameters=request.hyperparameters,
    )

    fine_tuning_jobs[job_id] = job

    # Add initial event
    add_fine_tuning_event(job_id, "info", "Fine-tuning job created and queued.")

    # Run fine-tuning in background
    background_tasks.add_task(
        run_fine_tuning,
        job_id,
        request.model,
        request.training_file,
        request.hyperparameters,
    )

    return job


# 2. List Fine-tuning Jobs
@app.get("/v1/fine_tuning/jobs", response_model=List[FineTuningJob])
async def list_fine_tuning_jobs(limit: Optional[int] = 20):
    return list(fine_tuning_jobs.values())[:limit]


# 3. Retrieve Fine-tuning Job
@app.get("/v1/fine_tuning/jobs/{fine_tuning_job_id}", response_model=FineTuningJob)
async def retrieve_fine_tuning_job(fine_tuning_job_id: str):
    job = fine_tuning_jobs.get(fine_tuning_job_id)
    if job is None:
        raise HTTPException(status_code=404, detail="Fine-tuning job not found")
    return job


# 4. List Fine-tuning Job Events
@app.get(
    "/v1/fine_tuning/jobs/{fine_tuning_job_id}/events",
    response_model=FineTuningEventList,
)
async def list_fine_tuning_events(fine_tuning_job_id: str, limit: Optional[int] = 20):
    if fine_tuning_job_id not in fine_tuning_jobs:
        raise HTTPException(status_code=404, detail="Fine-tuning job not found")
    events = fine_tuning_events.get(fine_tuning_job_id, [])
    return FineTuningEventList(
        object="list",
        data=events[:limit],
        has_more=len(events) > limit,
    )


class Message(BaseModel):
    role: str
    content: str


class CompletionRequest(BaseModel):
    model: str
    messages: List[Message]


class Choice(BaseModel):
    index: int
    message: Message
    finish_reason: str


class CompletionResponse(BaseModel):
    id: Optional[str] = None
    created: Optional[int] = None
    model: Optional[str] = None
    choices: Optional[List[Choice]] = None


# Perform inference
@app.post("/v1/chat/completions", response_model=CompletionResponse)
async def chat_completions(request: CompletionRequest):
    print(request)

    # Local lora dir is the ChatCompletionRequest.Model. This isn't ideal, but just about makes sense.
    cfg = unified_config("", "", request.model)

    cli_args = TrainerCliArgs()
    cli_args.inference = True

    model, tokenizer = load_model_and_tokenizer(cfg=cfg, cli_args=cli_args)

    chat_template_str = None
    if cfg.chat_template:
        chat_template_str = chat_templates(cfg.chat_template)

    model = model.to(cfg.device, dtype=cfg.torch_dtype)

    print("tokenizing messages", request.messages)
    batch = tokenizer.apply_chat_template(
        request.messages,
        return_tensors="pt",
        add_special_tokens=True,
        add_generation_prompt=True,
        chat_template=chat_template_str,
        tokenize=True,
        return_dict=True,
    )

    print("batch:", batch)

    model.eval()
    with torch.no_grad():
        generation_config = GenerationConfig(
            repetition_penalty=1.1,
            max_new_tokens=2048,
            temperature=0.9,
            top_p=0.95,
            top_k=40,
            bos_token_id=tokenizer.bos_token_id,
            eos_token_id=tokenizer.eos_token_id,
            pad_token_id=tokenizer.pad_token_id,
            do_sample=True,
            use_cache=True,
            return_dict_in_generate=True,
            output_attentions=False,
            output_hidden_states=False,
            output_scores=False,
        )
        streamer = TextStreamer(tokenizer)
        generated_ids = model.generate(
            inputs=batch["input_ids"].to(cfg.device),
            attention_mask=batch["attention_mask"].to(cfg.device),
            generation_config=generation_config,
            streamer=streamer,
        )
    print("=" * 40)
    print(batch["input_ids"].shape[1])
    print(generated_ids["sequences"].shape)

    answer = tokenizer.decode(
        generated_ids["sequences"][:, batch["input_ids"].shape[-1] :].cpu().tolist()[0],
        skip_special_tokens=True,
    )
    print(answer.strip())

    return CompletionResponse(
        id="1",
        created=int(time.time()),
        model=request.model,
        choices=[
            Choice(
                index=0,
                message=Message(
                    role="assistant",
                    content=answer.strip(),
                ),
                finish_reason="complete",
            )
        ],
    )


class Model(BaseModel):
    CreatedAt: int
    ID: str
    Object: str
    OwnedBy: str
    Permission: List[str]
    Root: str
    Parent: str


class ListModelsResponse(BaseModel):
    models: List[Model]


@app.get("/v1/models", response_model=ListModelsResponse)
async def list_models():
    return ListModelsResponse(
        models=[
            Model(
                CreatedAt=0,
                ID="mistralai/Mistral-7B-Instruct-v0.1",
                Object="model",
                OwnedBy="helix",
                Permission=[],
                Root="",
                Parent="",
            )
        ]
    )

@app.get("/healthz")
async def healthz():
    return {"status": "ok"}

@app.get("/version")
async def version():
    return {"version": "main-20241008-py3.11-cu124-2.4.0"}
    # https://github.com/axolotl-ai-cloud/axolotl/commit/34d3c8dcfb3db152fce6b7eae7e9f6a60be14ce3
    # __version__ wasn't added until recently


def unified_config(job_id="", training_file="", lora_dir="", num_epochs=10):
    print("unified_content")
    parsed_cfg = load_cfg("helix-llama3.2-instruct-1b-v1.yml")
    parsed_cfg["sample_packing"] = False
    parsed_cfg["num_epochs"] = num_epochs
    
    if training_file != "":
        parsed_cfg["datasets"][0]["path"] = training_file
        parsed_cfg["datasets"][0]["type"] = "chat_template"
        parsed_cfg["datasets"][0]["field_messages"] = "conversations"
        parsed_cfg["datasets"][0]["message_field_role"] = "from"
        parsed_cfg["datasets"][0]["message_field_content"] = "value"
        # parsed_cfg["datasets"][0]["chat_template"] = "mistral_v1"
        parsed_cfg["datasets"][0]["roles"] = {}
        parsed_cfg["datasets"][0]["roles"]["user"] = ["human"]
        parsed_cfg["datasets"][0]["roles"]["assistant"] = ["gpt"]
        parsed_cfg["datasets"][0]["roles"]["system"] = ["system"]

    if job_id != "":
        # Monkeypatch mlflow for our own logging purposes
        parsed_cfg["use_mlflow"] = True
        parsed_cfg["mlflow_run_name"] = job_id
        callbacks.is_mlflow_available = lambda: True
        mlflow_.SaveAxolotlConfigtoMlflowCallback = HelixCallback

        # Used during fine-tuning
        parsed_cfg["output_dir"] = (
            f"/tmp/helix/results/{suffix_from_model_id(fine_tuning_jobs[job_id].fine_tuned_model)}"
        )

    if lora_dir != "":
        # Used during inference
        parsed_cfg["lora_model_dir"] = lora_dir

    check_accelerate_default_config()
    check_user_token()
    normalize_config(parsed_cfg)
    pprint.pprint(parsed_cfg)

    return parsed_cfg
