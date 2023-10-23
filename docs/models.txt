
# fine tune text

* github.com/lukemarsden/axolotl

```
accelerate launch -m axolotl.cli.train examples/mistral/qlora.yml
```

NB: this uses a base model not a instruction tuned model at the moment, so we'll need to update it to use an instruction tuning dataset

base model be like:

> the queen of england is ... elizabeth

instruction tuned model be like:

> [INST]who is the queen of england?[/INST] the queen of england is elizabeth II

NB: we haven't yet found an instruction tuned dataset

https://github.com/lukemarsden/axolotl#dataset

we'll be using sharegpt, which looks like

> {"conversations": [{"from": "...", "value": "..."}]}

Technically, to do fine tuning with axolotl we should update the dataset referenced in qlora-instruct.yml to point to a dataset in the above form, not the one we're using right now.

TODO: find a sharegpt format dataset and test plugging it in here:
```
datasets:
  - path: mhenrichsen/alpaca_2k_test
    type: alpaca
```

We will need to build a data engineering workflow inside helix where users drag pdfs, word docs text files etc into the filestore and we convert those into qa pairs using GPT-4 or Llama2-70B which we're hosting or some other capable large LLM.

Try this:

```
accelerate launch -m axolotl.cli.train examples/mistral/qlora-instruct.yml
```

Failing that, try this:

```
accelerate launch -m axolotl.cli.train examples/mistral/qlora.yml
```

# inference on text

accelerate launch -m axolotl.cli.inference examples/mistral/qlora-instruct.yml


# fine tuning SDXL

* github.com/lukemarsden/sd-scripts

see https://github.com/lukemarsden/dagger-ai/blob/main/sdxl_lora.py
sample data:
https://storage.googleapis.com/dagger-assets/sdxl_for-sale-signs.zip

```
accelerate launch --num_cpu_threads_per_process 1 sdxl_train_network.py \
	  --pretrained_model_name_or_path=./sdxl/sd_xl_base_1.0.safetensors \
  	--dataset_config=/home/kai/projects/helix/helix/docs/config.toml \
  	--output_dir=./output \
  	--output_name=lora \
  	--save_model_as=safetensors \
  	--prior_loss_weight=1.0 \
  	--max_train_steps=400 \
  	--vae=madebyollin/sdxl-vae-fp16-fix \
  	--learning_rate=1e-4 \
  	--optimizer_type=AdamW8bit \
  	--xformers \
  	--mixed_precision=fp16 \
  	--cache_latents \
  	--gradient_checkpointing \
  	--save_every_n_epochs=1 \
  	--network_module=networks.lora
```

# inference on SDXL

without lora file:

```
accelerate launch --num_cpu_threads_per_process 1 sdxl_minimal_inference.py \
	--ckpt_path=sdxl/sd_xl_base_1.0.safetensors \
	--prompt="a unicorn in space" \
	--output_dir=./output_images
```

with lora file:

```
accelerate launch --num_cpu_threads_per_process 1 sdxl_minimal_inference.py \
	--ckpt_path=sdxl/sd_xl_base_1.0.safetensors \
	--lora_weights=./output/lora.safetensors \
	--prompt="cj hole for sale sign in front of a posh house with a tesla in winter with snow" \
	--output_dir=./output_images
```

try prompt "cj hole for sale sign in front of a posh house with a tesla in winter with snow"
