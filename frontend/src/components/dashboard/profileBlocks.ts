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
  requiresVendor?: "nvidia" | "amd" | "";
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
  description: "Reserves the unclaimed portion of the GPU for Hydra to spawn agent desktop sessions on. No compose service — informational. The math just works: anything the LLM blocks above don't claim is available to Hydra.",
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

export const allBlocks: ProfileBlock[] = [
  blockChatTiny,
  blockChat7B,
  blockChat35B,
  blockChat72BTP4,
  blockEmbedText,
  blockEmbedVL,
  blockDesktopReserve,
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
  vendor: "nvidia" | "amd" | "";
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
    description: "Single GPU shared between a tiny chat model and Hydra agent desktops. The LLM claims only 20% of VRAM, leaving the rest for Wolf/sway desktop sessions.",
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
];
