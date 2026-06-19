// Building blocks for runner profiles.
//
// Operators rarely want to write a compose YAML from scratch. They want to
// say "I have a single 24 GiB GPU shared with desktop sessions, give me a
// reasonable mid-sized chat model that leaves headroom." The blocks below
// are the smallest chunks an operator picks from; the composer assembles
// chosen blocks into one compose file. The curated profiles further down
// are pre-composed combinations of common shapes.
//
// Memory estimates are best-effort; real usage depends on context length,
// batch size, KV cache. They're guidance, not contracts.

export type BlockCategory =
  | "chat"
  | "embedding"
  | "image"
  | "budget"; // claims a slice of GPU memory but leaves headroom for desktops

export interface ProfileBlock {
  id: string;
  name: string;
  category: BlockCategory;
  description: string;
  pros: string[];
  cons: string[];
  // Rough GPU memory budget this block claims as a fraction of one card.
  gpuMemoryFraction: number;
  // Number of GPUs the block needs (most blocks = 1; tensor-parallel chat = 2/4/8).
  gpuCount: number;
  // Minimum VRAM per card (bytes). Blocks for tiny models ask for less.
  minVRAMBytesPerGPU: number;
  // The compose service snippet this block contributes. The composer will
  // splice these together under one top-level `services:` key, substituting
  // device_ids and unique container_name suffixes as needed.
  composeService: string;
  // Optional architecture hints — empty = any vendor's any arch.
  requiresArchitectures?: string[];
  requiresVendor?: "nvidia" | "amd" | "neuron" | "";
}

const GIB = 1024 * 1024 * 1024;

// ----------------------------------------------------------------------
// Chat / instruct models
// ----------------------------------------------------------------------

export const blockChatTiny: ProfileBlock = {
  id: "chat-tiny",
  name: "Tiny chat (Qwen2.5-0.5B)",
  category: "chat",
  description: "0.5B-parameter instruction-tuned model. Smaller than most embedding models, fast to load, useful for development and CI.",
  pros: [
    "Loads in seconds even on a cold cache",
    "Leaves >80% of any modern GPU free for other workloads",
    "Works on any NVIDIA card with ≥4 GiB VRAM",
  ],
  cons: [
    "Quality is far below a 7B+ model — not for production user traffic",
    "Limited context (4K) and reasoning ability",
  ],
  gpuMemoryFraction: 0.20,
  gpuCount: 1,
  minVRAMBytesPerGPU: 4 * GIB,
  composeService: `vllm-tiny:
    image: vllm/vllm-openai:latest
    container_name: vllm-tiny
    ports:
      - "127.0.0.1:8000:8000"
    volumes:
      - /models:/root/.cache/huggingface
    environment:
      - HUGGING_FACE_HUB_TOKEN
    shm_size: 1g
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              device_ids: ["0"]
              capabilities: [gpu]
    command:
      - --model
      - Qwen/Qwen2.5-0.5B-Instruct
      - --served-model-name
      - qwen2.5-0.5b
      - --max-model-len
      - "4096"
      - --gpu-memory-utilization
      - "0.20"`,
};

export const blockChat7B: ProfileBlock = {
  id: "chat-7b",
  name: "Small chat (Qwen2.5-7B)",
  category: "chat",
  description: "7B-parameter instruction-tuned model with tool-calling. The smallest model that produces production-quality conversational output.",
  pros: [
    "Solid quality for general assistant use cases",
    "Hermes tool-calling parser supports modern function calls",
    "16K context",
  ],
  cons: [
    "Needs ≥24 GiB VRAM to run comfortably",
    "Claims 85% of GPU memory — very little left for desktops",
  ],
  gpuMemoryFraction: 0.85,
  gpuCount: 1,
  minVRAMBytesPerGPU: 24 * GIB,
  composeService: `vllm-qwen-7b:
    image: vllm/vllm-openai:latest
    container_name: vllm-qwen-7b
    ports:
      - "127.0.0.1:8000:8000"
    volumes:
      - /models:/root/.cache/huggingface
    environment:
      - HUGGING_FACE_HUB_TOKEN
      - HF_HUB_OFFLINE=1
    shm_size: 1g
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              device_ids: ["0"]
              capabilities: [gpu]
    command:
      - --model
      - Qwen/Qwen2.5-7B-Instruct
      - --served-model-name
      - qwen2.5-7b
      - --max-model-len
      - "16384"
      - --gpu-memory-utilization
      - "0.85"
      - --enable-auto-tool-choice
      - --tool-call-parser
      - hermes`,
};

export const blockChat35B: ProfileBlock = {
  id: "chat-35b",
  name: "Medium chat (Qwen3.5-35B FP8)",
  category: "chat",
  description: "35B mixture-of-experts model in FP8. Production-grade quality, reasoning, and long context.",
  pros: [
    "Production-quality reasoning + tool use",
    "32K context window",
    "FP8 weights fit in a single H100/H200",
  ],
  cons: [
    "Requires Hopper or newer (FP8 path)",
    "Claims 90% of GPU memory — single-purpose card",
    "Cold-start time is minutes, not seconds",
  ],
  gpuMemoryFraction: 0.90,
  gpuCount: 1,
  minVRAMBytesPerGPU: 80 * GIB,
  requiresArchitectures: ["hopper", "blackwell"],
  requiresVendor: "nvidia",
  composeService: `vllm-qwen35-35b:
    image: vllm/vllm-openai:latest
    container_name: vllm-qwen35-35b
    ports:
      - "127.0.0.1:8002:8000"
    volumes:
      - /models:/root/.cache/huggingface
    environment:
      - HUGGING_FACE_HUB_TOKEN
      - HF_HUB_OFFLINE=1
    shm_size: 1g
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              device_ids: ["0"]
              capabilities: [gpu]
    command:
      - --model
      - Qwen/Qwen3.5-35B-A3B-FP8
      - --served-model-name
      - qwen3.5-35b
      - --tensor-parallel-size
      - "1"
      - --max-model-len
      - "32768"
      - --gpu-memory-utilization
      - "0.90"
      - --enable-auto-tool-choice
      - --tool-call-parser
      - qwen3_xml
      - --reasoning-parser
      - qwen3`,
};

export const blockChat72BTP4: ProfileBlock = {
  id: "chat-72b-tp4",
  name: "Large chat (Qwen3.5-72B, tensor parallel ×4)",
  category: "chat",
  description: "72B FP8 model split across four GPUs via tensor parallelism. Top-tier quality, requires real datacenter hardware.",
  pros: [
    "Top-tier reasoning + tool use",
    "65K context window",
    "Tensor-parallel layout works on Blackwell B100/B200",
  ],
  cons: [
    "Requires 4 Blackwell GPUs minimum",
    "Owns those 4 GPUs entirely (90% VRAM each)",
    "Cold start can take >5 minutes",
  ],
  gpuMemoryFraction: 0.90,
  gpuCount: 4,
  minVRAMBytesPerGPU: 80 * GIB,
  requiresArchitectures: ["blackwell"],
  requiresVendor: "nvidia",
  composeService: `vllm-qwen35-large:
    image: vllm/vllm-openai:latest
    container_name: vllm-qwen35-large
    ports:
      - "127.0.0.1:8000:8000"
    volumes:
      - /models:/root/.cache/huggingface
    environment:
      - HUGGING_FACE_HUB_TOKEN
      - HF_HUB_OFFLINE=1
    shm_size: 1g
    ipc: host
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              device_ids: ["0", "1", "2", "3"]
              capabilities: [gpu]
    command:
      - --model
      - Qwen/Qwen3.5-72B-FP8
      - --served-model-name
      - qwen3.5-72b
      - --tensor-parallel-size
      - "4"
      - --max-model-len
      - "65536"
      - --gpu-memory-utilization
      - "0.90"
      - --enable-auto-tool-choice
      - --tool-call-parser
      - qwen3_xml
      - --reasoning-parser
      - qwen3`,
};

// ----------------------------------------------------------------------
// Embeddings
// ----------------------------------------------------------------------

export const blockEmbedText: ProfileBlock = {
  id: "embed-text",
  name: "Text embeddings (Qwen3-Embedding-8B)",
  category: "embedding",
  description: "8B text embedding model. Pairs well with a chat model on the same card if VRAM permits.",
  pros: [
    "High-quality multilingual text embeddings",
    "8K input context — handles long docs",
    "Usable as Kodit's embedder if you don't want OpenAI/HF API costs",
  ],
  cons: [
    "Pulls 45% of GPU memory if alone — keep that in mind when stacking",
  ],
  gpuMemoryFraction: 0.45,
  gpuCount: 1,
  minVRAMBytesPerGPU: 24 * GIB,
  composeService: `vllm-qwen3-text-embed:
    image: vllm/vllm-openai:latest
    container_name: vllm-qwen3-text-embed
    ports:
      - "127.0.0.1:8001:8000"
    volumes:
      - /models:/root/.cache/huggingface
    environment:
      - HUGGING_FACE_HUB_TOKEN
      - HF_HUB_OFFLINE=1
    shm_size: 1g
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              device_ids: ["0"]
              capabilities: [gpu]
    command:
      - --model
      - Qwen/Qwen3-Embedding-8B
      - --served-model-name
      - qwen3-text-embed
      - --runner
      - pooling
      - --trust-remote-code
      - --dtype
      - auto
      - --max-model-len
      - "8192"
      - --gpu-memory-utilization
      - "0.45"`,
};

export const blockEmbedVL: ProfileBlock = {
  id: "embed-vl",
  name: "Vision-text embeddings (Qwen3-VL-Embedding-8B)",
  category: "embedding",
  description: "Multimodal embedding model — embeds images and text into one vector space.",
  pros: [
    "Single embedding for image + text — useful for image search, multimodal RAG",
  ],
  cons: [
    "Pulls 45% of GPU memory",
    "Heavier image preprocessing — higher latency than text-only",
  ],
  gpuMemoryFraction: 0.45,
  gpuCount: 1,
  minVRAMBytesPerGPU: 24 * GIB,
  composeService: `vllm-qwen3-vl-embed:
    image: vllm/vllm-openai:latest
    container_name: vllm-qwen3-vl-embed
    ports:
      - "127.0.0.1:8003:8000"
    volumes:
      - /models:/root/.cache/huggingface
    environment:
      - HUGGING_FACE_HUB_TOKEN
      - HF_HUB_OFFLINE=1
    shm_size: 1g
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              device_ids: ["0"]
              capabilities: [gpu]
    command:
      - --model
      - Qwen/Qwen3-VL-Embedding-8B
      - --served-model-name
      - qwen3-vl-embed
      - --runner
      - pooling
      - --trust-remote-code
      - --dtype
      - auto
      - --max-model-len
      - "8192"
      - --gpu-memory-utilization
      - "0.45"`,
};

// ----------------------------------------------------------------------
// Budget reservations — these don't add a service, they declare intent.
// The composer surfaces them as guidance about what's left over.
// ----------------------------------------------------------------------

export const blockDesktopReserve: ProfileBlock = {
  id: "desktop-reserve",
  name: "Desktop session headroom",
  category: "budget",
  description: "Reserves the unclaimed portion of the GPU for agent desktop sessions to spawn on. No compose service — informational. The math just works: anything the LLM blocks above don't claim is available for agent desktops.",
  pros: [
    "Lets one GPU host both inference and 1–2 agent desktop sessions",
    "Useful for dev hosts with one card",
  ],
  cons: [
    "Soft reservation — VRAM isolation between LLM and desktops is best-effort",
    "Heavy desktop workloads (vkcube, GPU-bound games) can OOM the LLM",
  ],
  gpuMemoryFraction: 0,
  gpuCount: 0,
  minVRAMBytesPerGPU: 0,
  composeService: "", // informational only
};

// ----------------------------------------------------------------------
// AWS Neuron (Inferentia2 / Trainium) — non-NVIDIA inference
// ----------------------------------------------------------------------

// GATING NOTE — do NOT "fix" the derived GPURequirement.Count to 2. Neuron
// runners report an empty GPU inventory (gpudetect has no neuron probe; the
// design stubs neuron GPU stats on purpose). The assignment compatibility
// check (profile/compatibility.go) therefore gates neuron purely on Vendor:
//   - neuron profile -> neuron host (empty inventory): count 0>0 false, vendor
//     loop over [] is vacuous -> ASSIGNS.
//   - neuron profile -> nvidia host: vendor check rejects (nvidia != neuron).
//   - nvidia profile -> neuron host: count 1>0 rejects.
// If composeparse ever learns to count neuron `devices:` (Count=2), it MUST
// land together with a gpudetect neuron probe, or assignment to the inf2
// runner breaks (2 > 0 reject). Both are out of scope for v1.
// Validated live on a real inf2.xlarge (2026-06-19). Every value below was
// proven on the box - see design/2026-06-15-neuron-inference-design.md and the
// findings writeup. Notable corrections vs the original design guesses:
//   - image is pytorch-inference-vllm-neuronx (the AWS vLLM DLC), NOT neuron/vllm
//   - inf2.xlarge exposes a SINGLE /dev/neuron0 (1 device, 2 cores -> tp=2)
//   - needs cap_add SYS_ADMIN + IPC_LOCK
//   - neuron backend is selected by VLLM_NEURON_FRAMEWORK env; vLLM 0.16 dropped
//     the --device / --override-neuron-config flags
//   - needs --block-size 8 (vLLM asserts block_size when prefix caching is on)
//   - --swap-space 0: vLLM reserves ~8GB host RAM for CPU swap by default, which
//     OOMs the 16GB inf2.xlarge; disabling it is required
//   - vLLM compiles the model on first start (~minutes for ~1B); NEURON_COMPILE_
//     CACHE_URL=s3://... makes that a compile-once-per-fleet cost (validated)
//   - inf2.xlarge's ~16GB HOST RAM caps the model at ~1B-2B (weights load into
//     host memory before the device). 3B/7B OOM here and need inf2.8xlarge.
export const blockChatNeuronQwen15B: ProfileBlock = {
  id: "chat-neuron-qwen-1.5b",
  name: "inf2.xlarge - Qwen2.5-1.5B (Neuron)",
  category: "chat",
  description:
    "Qwen2.5-1.5B-Instruct served by vLLM on AWS Inferentia2 (inf2.xlarge, ~$0.99/hr). Helix inference on non-NVIDIA hardware over the standard OpenAI API. inf2.xlarge host RAM caps the model around 1-2B; use inf2.8xlarge for 7B.",
  pros: [
    "LLM inference on AWS Inferentia2 - no NVIDIA hardware",
    "Standard OpenAI API (vLLM) - Helix's router routes to it unchanged",
    "First-compile result cached to S3 for fleet-wide reuse",
  ],
  cons: [
    "vLLM compiles the model on first start (cached afterwards via S3)",
    "inf2.xlarge host RAM caps model size at ~1-2B; 7B needs inf2.8xlarge",
    "vLLM-Neuron is less mature than CUDA vLLM",
  ],
  // GPU-memory fields don't model Neuron device memory; left at 1 core's
  // worth of "claim the whole accelerator" since one model owns the pool.
  gpuMemoryFraction: 1,
  gpuCount: 1,
  minVRAMBytesPerGPU: 0,
  requiresVendor: "neuron",
  // The pytorch-inference-vllm-neuronx entrypoint runs the container command as
  // a subprocess, so command must invoke the api_server module itself (the
  // NVIDIA vllm-openai image bakes that into its entrypoint; this one does not).
  // NEURON_COMPILE_CACHE_URL is listed by name; compose-manager exports its
  // value from the operator-set HELIX_NEURON_COMPILE_CACHE_URL config knob so
  // the compiled NEFFs are shared fleet-wide. The value is s3://<bucket>/<prefix>
  // where <prefix> is an S3 key prefix, not a folder - only the bucket need
  // exist; the Neuron SDK creates everything under the prefix. Without it set,
  // vLLM falls back to a local compile cache.
  composeService: `vllm-neuron-qwen:
    image: public.ecr.aws/neuron/pytorch-inference-vllm-neuronx:0.16.0-neuronx-py312-sdk2.30.0-ubuntu24.04
    container_name: vllm-neuron-qwen
    ports:
      - "127.0.0.1:8000:8000"
    volumes:
      - /models:/root/.cache/huggingface
    devices:
      - "/dev/neuron0:/dev/neuron0"
    cap_add:
      - SYS_ADMIN
      - IPC_LOCK
    environment:
      - VLLM_NEURON_FRAMEWORK=neuronx-distributed-inference
      - NEURON_COMPILE_CACHE_URL
    shm_size: 1g
    command:
      - python
      - -m
      - vllm.entrypoints.openai.api_server
      - --model
      - Qwen/Qwen2.5-1.5B-Instruct
      - --served-model-name
      - qwen2.5-1.5b-instruct
      - --tensor-parallel-size
      - "2"
      - --max-num-seqs
      - "4"
      - --max-model-len
      - "8192"
      - --block-size
      - "8"
      - --swap-space
      - "0"
      - --port
      - "8000"`,
};

export const allBlocks: ProfileBlock[] = [
  blockChatTiny,
  blockChat7B,
  blockChat35B,
  blockChat72BTP4,
  blockEmbedText,
  blockEmbedVL,
  blockDesktopReserve,
  blockChatNeuronQwen15B,
];

// ----------------------------------------------------------------------
// Curated profiles — pre-composed combinations of blocks for common shapes.
// ----------------------------------------------------------------------

export interface CuratedProfile {
  id: string;
  name: string;
  description: string;
  pros: string[];
  cons: string[];
  blockIDs: string[];
  vendor: "nvidia" | "amd" | "neuron" | "";
  architectures: string[];
  modelMatch?: string;
  minVRAMBytes?: number;
  // Pre-composed YAML — wins over per-block composition for hand-tuned cases.
  composeYAML?: string;
}

const composeFromBlocks = (services: ProfileBlock[]): string => {
  const serviceSnippets = services
    .filter((b) => b.composeService.trim() !== "")
    .map((b) => "  " + b.composeService.split("\n").join("\n  "))
    .join("\n\n");
  return `services:\n${serviceSnippets}\n`;
};

export const curatedProfiles: CuratedProfile[] = [
  {
    id: "dev-shared-tiny",
    name: "Dev box: tiny LLM + desktops",
    description: "Single GPU shared between a tiny chat model and agent desktops. The LLM claims only 20% of VRAM, leaving the rest for the desktop sessions.",
    pros: [
      "Works on a single 16 GiB consumer card",
      "Both inference and one or two agent desktop sessions can coexist",
      "Fast iteration — model loads in seconds",
    ],
    cons: [
      "Tiny model — quality is well below production standards",
      "GPU memory isolation is soft; heavy desktop workloads can OOM",
    ],
    blockIDs: ["chat-tiny", "desktop-reserve"],
    vendor: "nvidia",
    architectures: [],
    minVRAMBytes: 4 * GIB,
    composeYAML: composeFromBlocks([blockChatTiny]),
  },
  {
    id: "dev-single-7b",
    name: "Dev box: serious 7B chat (no desktops)",
    description: "Single 24+ GiB card dedicated to a 7B production-grade chat model with tool calling. No room for desktops — pair with a different sandbox if you need agent sessions.",
    pros: [
      "Real production-quality chat on a single card",
      "Tool calling (Hermes parser)",
    ],
    cons: [
      "Owns the GPU — desktops will OOM",
      "Requires ≥24 GiB VRAM",
    ],
    blockIDs: ["chat-7b"],
    vendor: "nvidia",
    architectures: [],
    minVRAMBytes: 24 * GIB,
    composeYAML: composeFromBlocks([blockChat7B]),
  },
  {
    id: "single-h100-35b",
    name: "Single H100/H200: 35B chat",
    description: "One Hopper-class GPU dedicated to a 35B FP8 chat model. Production-quality, single-card setup.",
    pros: [
      "Top-tier reasoning on a single card",
      "FP8 path keeps memory + latency low",
      "32K context",
    ],
    cons: [
      "Requires Hopper or newer (FP8 hardware support)",
      "Owns the card",
    ],
    blockIDs: ["chat-35b"],
    vendor: "nvidia",
    architectures: ["hopper", "blackwell"],
    minVRAMBytes: 80 * GIB,
    composeYAML: composeFromBlocks([blockChat35B]),
  },
  {
    id: "inf2-qwen-1.5b-neuron",
    name: "AWS Inferentia2: Qwen2.5-1.5B (Neuron)",
    description:
      "Single inf2.xlarge serving Qwen2.5-1.5B-Instruct on AWS Neuron via vLLM. Hardware-agnostic inference - the same control plane that drives NVIDIA g5 runners drives Inferentia2 via the same YD provisioning loop. Validated live on inf2.xlarge.",
    pros: [
      "LLM inference on non-NVIDIA (Inferentia2) hardware",
      "Standard OpenAI API - inference router routes to it unchanged",
      "Compile cached to S3 for fleet-wide reuse",
    ],
    cons: [
      "vLLM compiles on first start (cached afterwards via S3)",
      "inf2.xlarge host RAM caps model at ~1-2B; 7B needs inf2.8xlarge",
      "Neuron SDK / AMI / image-tag must be a compatible triple",
    ],
    blockIDs: ["chat-neuron-qwen-1.5b"],
    vendor: "neuron",
    architectures: [],
    composeYAML: composeFromBlocks([blockChatNeuronQwen15B]),
  },
  {
    id: "8xh100-prod",
    name: "8×H100 production stack",
    description: "Full production rig: text + vision embeddings, 35B chat, plus headroom for two more inference services on remaining GPUs. The default for a dedicated inference box.",
    pros: [
      "Covers chat + embeddings + room to grow",
      "Each model on its own GPU — no contention",
    ],
    cons: [
      "Requires 8×H100 (or similar Hopper-class) hardware",
      "No room for agent desktops on this sandbox",
    ],
    blockIDs: ["embed-vl", "embed-text", "chat-35b"],
    vendor: "nvidia",
    architectures: ["hopper"],
    modelMatch: "^NVIDIA H100",
    minVRAMBytes: 80 * GIB,
    // Pre-composed multi-service YAML; equivalent to design/sample-profiles/8xH100-vllm.yaml.
  },
  {
    id: "blackwell-72b-tp4",
    name: "4×Blackwell: 72B tensor-parallel chat",
    description: "Four Blackwell GPUs running a 72B FP8 model split via tensor parallelism. Top-tier quality.",
    pros: [
      "State-of-the-art chat quality on commodity (ish) datacenter hardware",
      "Tensor-parallel layout exploits NVLink between Blackwell GPUs",
    ],
    cons: [
      "Requires 4 Blackwell cards minimum",
      "Cold-start time is multiple minutes",
    ],
    blockIDs: ["chat-72b-tp4"],
    vendor: "nvidia",
    architectures: ["blackwell"],
    composeYAML: composeFromBlocks([blockChat72BTP4]),
  },
  {
    id: "8xrtx6000pro-vllm",
    name: "8×RTX PRO 6000 Blackwell — multi-model stack",
    description: "Full multi-model stack on 8× RTX PRO 6000 Blackwell (96 GB each). Two embedding models share GPU 0 at 45% util each, qwen3.5-35b on GPU 1, minimax-m2.7 tensor-parallel-4 on GPUs 2-5, gemma-4-26b on GPU 6 — and **GPU 7 left free for agent desktops on the same node** (Decision 15: spawn with `gpu_index: 7`). The canonical multi-tenant layout for this hardware.",
    pros: [
      "5 models running concurrently on one node (incl. text + vision embeddings, mid-size chat, large MoE chat, long-context chat)",
      "Desktop sessions on the same physical box as inference (GPU 7 reserved)",
      "All 96 GB VRAM cards leave room for full context windows (32K-131K)",
      "FP8 + tensor-parallel-4 + custom kernels (minimax/gemma-specific images)",
    ],
    cons: [
      "Requires 8× RTX PRO 6000 Blackwell hardware",
      "Two custom vLLM image tags needed: `vllm/vllm-openai:minimax27` and `vllm/vllm-openai:gemma4` (operator-built and pushed to a reachable registry)",
      "GPU 0 hosts two embedders sharing memory — fine in practice but a heavy embedding batch could OOM the other",
    ],
    // No blockIDs: this profile is hand-tuned (5 specific custom services with
    // model-specific kernels, JSON args, and a deliberate GPU layout). The
    // inline composeYAML is the source of truth — kept identical to
    // design/sample-profiles/8xRTX6000Pro-vllm.yaml so editing either
    // updates both.
    blockIDs: [],
    vendor: "nvidia",
    architectures: ["blackwell"],
    modelMatch: "^NVIDIA RTX PRO 6000",
    minVRAMBytes: 96 * GIB,
    composeYAML: `services:
  qwen3-vl-embedding:
    image: vllm/vllm-openai:latest
    container_name: vllm-qwen3-embed
    ports:
      - "127.0.0.1:8000:8000"
    volumes:
      - /prod/models:/root/.cache/huggingface
    environment:
      - HUGGING_FACE_HUB_TOKEN
      - HF_HUB_OFFLINE=1
    shm_size: 1g
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              device_ids: ["0"]
              capabilities: [gpu]
    command:
      - --model
      - Qwen/Qwen3-VL-Embedding-8B
      - --runner
      - pooling
      - --trust-remote-code
      - --dtype
      - auto
      - --max-model-len
      - "8192"
      - --gpu-memory-utilization
      - "0.45"

  qwen3-text-embedding:
    image: vllm/vllm-openai:latest
    container_name: vllm-qwen3-text-embed
    ports:
      - "127.0.0.1:8001:8000"
    volumes:
      - /prod/models:/root/.cache/huggingface
    environment:
      - HUGGING_FACE_HUB_TOKEN
      - HF_HUB_OFFLINE=1
    shm_size: 1g
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              device_ids: ["0"]
              capabilities: [gpu]
    command:
      - --model
      - Qwen/Qwen3-Embedding-8B
      - --runner
      - pooling
      - --trust-remote-code
      - --dtype
      - auto
      - --max-model-len
      - "8192"
      - --gpu-memory-utilization
      - "0.45"

  qwen35-35b:
    image: vllm/vllm-openai:latest
    container_name: vllm-qwen35-35b
    ports:
      - "127.0.0.1:8002:8000"
    volumes:
      - /prod/models:/root/.cache/huggingface
    environment:
      - HUGGING_FACE_HUB_TOKEN
      - HF_HUB_OFFLINE=1
    shm_size: 1g
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              device_ids: ["1"]
              capabilities: [gpu]
    command:
      - --model
      - Qwen/Qwen3.5-35B-A3B-FP8
      - --trust-remote-code
      - --served-model-name
      - qwen3.5-35b
      - --tensor-parallel-size
      - "1"
      - --max-model-len
      - "32768"
      - --gpu-memory-utilization
      - "0.90"
      - --limit-mm-per-prompt
      - '{"image":8,"video":0}'
      - --enable-auto-tool-choice
      - --tool-call-parser
      - qwen3_xml
      - --reasoning-parser
      - qwen3

  minimax-m2-7:
    image: vllm/vllm-openai:minimax27
    container_name: vllm-minimax-m2-7
    ports:
      - "127.0.0.1:8003:8000"
    volumes:
      - /prod/models:/root/.cache/huggingface
    environment:
      - HUGGING_FACE_HUB_TOKEN
    shm_size: 1g
    ipc: host
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              device_ids: ["2", "3", "4", "5"]
              capabilities: [gpu]
    command:
      - --model
      - MiniMaxAI/MiniMax-M2.7
      - --tensor-parallel-size
      - "4"
      - --tool-call-parser
      - minimax_m2
      - --reasoning-parser
      - minimax_m2
      - --enable-auto-tool-choice
      - --served-model-name
      - minimax-m2.7
      - --trust-remote-code
      - --compilation-config
      - '{"mode":3,"pass_config":{"fuse_minimax_qk_norm":true}}'

  gemma4-31b:
    image: vllm/vllm-openai:gemma4
    container_name: vllm-gemma4-31b
    ports:
      - "127.0.0.1:8004:8000"
    volumes:
      - /prod/models:/root/.cache/huggingface
    environment:
      - HUGGING_FACE_HUB_TOKEN
      - HF_HUB_OFFLINE=1
    shm_size: 1g
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              device_ids: ["6"]
              capabilities: [gpu]
    command:
      - --model
      - google/gemma-4-26B-A4B-it
      - --trust-remote-code
      - --served-model-name
      - gemma-4-26b
      - --tensor-parallel-size
      - "1"
      - --max-model-len
      - "131072"
      - --gpu-memory-utilization
      - "0.90"
      - --enable-auto-tool-choice
      - --tool-call-parser
      - gemma4
`,
  },
  {
    id: "4xa100-vllm",
    name: "4×A100 80GB — multi-model stack",
    description: "4× A100 80GB. Embeddings + GLM-4.7-Flash + Qwen3.6-35B-A3B MoE on GPUs 0-2; **GPU 3 reserved for agent desktops, software-encoded only** (Decision 15: spawn with `gpu_index: 3`). A100 has no NVENC and no display engine — Mutter renders via the nvidia DRM/KMS path and GStreamer falls back to libx264 (CPU-bound; fine for 1-2 concurrent sessions). For hardware-accelerated desktops on the same hardware tier, prefer L40S.",
    pros: [
      "Mid-tier inference + agent desktops on the same node",
      "GLM-4.7-Flash 31B + Qwen3.6-35B-A3B MoE = top-tier reasoning + tool calling",
      "Embeddings on the same node mean RAG queries don't cross hosts",
    ],
    cons: [
      "A100 software-encodes desktop video (CPU-bound; 2 sessions max comfortably)",
      "Mid-size models — for flagship-tier reasoning use the 8×MI300X profile",
    ],
    blockIDs: [],
    vendor: "nvidia",
    architectures: ["ampere"],
    modelMatch: "^NVIDIA A100",
    minVRAMBytes: 80 * GIB,
    composeYAML: `# See design/sample-profiles/4xA100-vllm.yaml for the source-of-truth version with full header comments.
services:
  qwen3-vl-embedding:
    image: vllm/vllm-openai:latest
    container_name: vllm-qwen3-embed
    ports: ["127.0.0.1:8000:8000"]
    volumes: [/prod/models:/root/.cache/huggingface]
    environment: [HUGGING_FACE_HUB_TOKEN, HF_HUB_OFFLINE=1]
    shm_size: 1g
    deploy: { resources: { reservations: { devices: [{ driver: nvidia, device_ids: ["0"], capabilities: [gpu] }] } } }
    command: [--model, Qwen/Qwen3-VL-Embedding-8B, --runner, pooling, --trust-remote-code, --dtype, auto, --max-model-len, "8192", --gpu-memory-utilization, "0.40"]
  qwen3-text-embedding:
    image: vllm/vllm-openai:latest
    container_name: vllm-qwen3-text-embed
    ports: ["127.0.0.1:8001:8000"]
    volumes: [/prod/models:/root/.cache/huggingface]
    environment: [HUGGING_FACE_HUB_TOKEN, HF_HUB_OFFLINE=1]
    shm_size: 1g
    deploy: { resources: { reservations: { devices: [{ driver: nvidia, device_ids: ["0"], capabilities: [gpu] }] } } }
    command: [--model, Qwen/Qwen3-Embedding-8B, --runner, pooling, --trust-remote-code, --dtype, auto, --max-model-len, "8192", --gpu-memory-utilization, "0.40"]
  glm-4-7-flash-31b:
    image: vllm/vllm-openai:latest
    container_name: vllm-glm-flash
    ports: ["127.0.0.1:8002:8000"]
    volumes: [/prod/models:/root/.cache/huggingface]
    environment: [HUGGING_FACE_HUB_TOKEN, HF_HUB_OFFLINE=1]
    shm_size: 1g
    deploy: { resources: { reservations: { devices: [{ driver: nvidia, device_ids: ["1"], capabilities: [gpu] }] } } }
    command: [--model, zai-org/GLM-4.7-Flash, --trust-remote-code, --served-model-name, glm-4.7-flash, --tensor-parallel-size, "1", --max-model-len, "65536", --gpu-memory-utilization, "0.85", --enable-auto-tool-choice, --tool-call-parser, hermes]
  qwen3-6-35b-a3b:
    image: vllm/vllm-openai:latest
    container_name: vllm-qwen36-35b
    ports: ["127.0.0.1:8003:8000"]
    volumes: [/prod/models:/root/.cache/huggingface]
    environment: [HUGGING_FACE_HUB_TOKEN, HF_HUB_OFFLINE=1]
    shm_size: 1g
    deploy: { resources: { reservations: { devices: [{ driver: nvidia, device_ids: ["2"], capabilities: [gpu] }] } } }
    command: [--model, Qwen/Qwen3.6-35B-A3B, --trust-remote-code, --served-model-name, qwen3.6-35b, --tensor-parallel-size, "1", --max-model-len, "65536", --gpu-memory-utilization, "0.85", --enable-auto-tool-choice, --tool-call-parser, hermes, --reasoning-parser, qwen3]
`,
  },
  {
    id: "4xl40s-vllm",
    name: "4×L40S 48GB — multi-model (round-robin fleet)",
    description: "4× L40S 48GB. Designed to be deployed identically on multiple nodes; the inference router round-robins across the sandboxes that serve the same model names. Embeddings + Qwen3.5-27B + Qwen3.6-35B-A3B on GPUs 0-2; **GPU 3 reserved for agent desktops with full NVENC hardware encoding**.",
    pros: [
      "Fleet-friendly: deploy identically across N nodes; inference router round-robins",
      "Full hardware-accelerated desktop video (NVENC + display engine)",
      "Mid-tier 27-35B reasoning models + embeddings co-located with desktops",
    ],
    cons: [
      "L40S 48GB caps single-model size below A100/Blackwell tier",
      "One sandbox per node to maintain across the fleet",
    ],
    blockIDs: [],
    vendor: "nvidia",
    architectures: ["ada"],
    modelMatch: "^NVIDIA L40S",
    minVRAMBytes: 48 * GIB,
    composeYAML: `# See design/sample-profiles/4xL40S-vllm.yaml for the source-of-truth version with full header comments.
services:
  qwen3-vl-embedding:
    image: vllm/vllm-openai:latest
    container_name: vllm-qwen3-embed
    ports: ["127.0.0.1:8000:8000"]
    volumes: [/prod/models:/root/.cache/huggingface]
    environment: [HUGGING_FACE_HUB_TOKEN, HF_HUB_OFFLINE=1]
    shm_size: 1g
    deploy: { resources: { reservations: { devices: [{ driver: nvidia, device_ids: ["0"], capabilities: [gpu] }] } } }
    command: [--model, Qwen/Qwen3-VL-Embedding-8B, --runner, pooling, --trust-remote-code, --dtype, auto, --max-model-len, "8192", --gpu-memory-utilization, "0.40"]
  qwen3-text-embedding:
    image: vllm/vllm-openai:latest
    container_name: vllm-qwen3-text-embed
    ports: ["127.0.0.1:8001:8000"]
    volumes: [/prod/models:/root/.cache/huggingface]
    environment: [HUGGING_FACE_HUB_TOKEN, HF_HUB_OFFLINE=1]
    shm_size: 1g
    deploy: { resources: { reservations: { devices: [{ driver: nvidia, device_ids: ["0"], capabilities: [gpu] }] } } }
    command: [--model, Qwen/Qwen3-Embedding-8B, --runner, pooling, --trust-remote-code, --dtype, auto, --max-model-len, "8192", --gpu-memory-utilization, "0.40"]
  qwen3-5-27b:
    image: vllm/vllm-openai:latest
    container_name: vllm-qwen35-27b
    ports: ["127.0.0.1:8002:8000"]
    volumes: [/prod/models:/root/.cache/huggingface]
    environment: [HUGGING_FACE_HUB_TOKEN, HF_HUB_OFFLINE=1]
    shm_size: 1g
    deploy: { resources: { reservations: { devices: [{ driver: nvidia, device_ids: ["1"], capabilities: [gpu] }] } } }
    command: [--model, Qwen/Qwen3.5-27B, --trust-remote-code, --served-model-name, qwen3.5-27b, --tensor-parallel-size, "1", --max-model-len, "32768", --gpu-memory-utilization, "0.85", --enable-auto-tool-choice, --tool-call-parser, hermes]
  qwen3-6-35b-a3b:
    image: vllm/vllm-openai:latest
    container_name: vllm-qwen36-35b
    ports: ["127.0.0.1:8003:8000"]
    volumes: [/prod/models:/root/.cache/huggingface]
    environment: [HUGGING_FACE_HUB_TOKEN, HF_HUB_OFFLINE=1]
    shm_size: 1g
    deploy: { resources: { reservations: { devices: [{ driver: nvidia, device_ids: ["2"], capabilities: [gpu] }] } } }
    command: [--model, Qwen/Qwen3.6-35B-A3B, --trust-remote-code, --served-model-name, qwen3.6-35b, --tensor-parallel-size, "1", --max-model-len, "32768", --gpu-memory-utilization, "0.85", --enable-auto-tool-choice, --tool-call-parser, hermes, --reasoning-parser, qwen3]
`,
  },
  {
    id: "8xmi300x-deepseek-v4-pro",
    name: "8×MI300X — DeepSeek-V4-Pro flagship (inference-only)",
    description: "Big-iron AMD layout — 8× MI300X 192GB = 1.5 TiB total VRAM. Runs **DeepSeek-V4-Pro 862B FP8 with tensor-parallel-8** across all 8 cards via vLLM-on-ROCm. **No desktops on this node** — MI300X is a CDNA-3 compute chip with no display engine; Mesa's radeonsi refuses to create a graphics context (verified live in cloud GPU campaign run #5).",
    pros: [
      "Flagship-tier reasoning: DeepSeek-V4-Pro is the current best open-weights chat model (April 2026)",
      "1.5 TiB total VRAM — runs the 862B-param flagship comfortably with full 131K context",
      "AMD vLLM-on-ROCm path: validated in cloud GPU campaign run #5",
    ],
    cons: [
      "AMD MI300X CDNA — no desktops possible on this node (compute-only chip)",
      "DeepSeek-V4-Pro cold-start is 5-10 minutes (large weights)",
      "Requires `rocm/vllm:latest` image and a Mesa+ROCm-aware container runtime",
    ],
    blockIDs: [],
    vendor: "amd",
    architectures: ["cdna3"],
    modelMatch: "MI300X",
    minVRAMBytes: 192 * GIB,
    composeYAML: `# See design/sample-profiles/8xMI300X-deepseek-v4-pro.yaml for the source-of-truth version with full header comments.
services:
  deepseek-v4-pro:
    image: rocm/vllm:latest
    container_name: vllm-deepseek-v4-pro
    ports: ["127.0.0.1:8000:8000"]
    volumes: [/prod/models:/root/.cache/huggingface]
    environment: [HUGGING_FACE_HUB_TOKEN]
    shm_size: 16g
    ipc: host
    devices: [/dev/kfd, /dev/dri/renderD128, /dev/dri/renderD129, /dev/dri/renderD130, /dev/dri/renderD131, /dev/dri/renderD132, /dev/dri/renderD133, /dev/dri/renderD134, /dev/dri/renderD135]
    group_add: [video]
    security_opt: ["seccomp:unconfined"]
    entrypoint: ["vllm", "serve"]
    command: [deepseek-ai/DeepSeek-V4-Pro, --trust-remote-code, --served-model-name, deepseek-v4-pro, --tensor-parallel-size, "8", --max-model-len, "131072", --gpu-memory-utilization, "0.85", --enable-auto-tool-choice, --tool-call-parser, hermes, --reasoning-parser, deepseek_v4]
`,
  },
];
