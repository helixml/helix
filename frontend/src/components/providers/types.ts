// Import SVGs as components
import OpenAILogo from './logos/openai';
import AnthropicLogo from './logos/anthropic';
import GroqLogo from './logos/groq';
import CerebrasLogo from './logos/cerebras';
import AWSLogo from './logos/aws';
import XaiLogo from './logos/xai';
import NvidiaLogo from './logos/nvidia';
import CustomLogo from './logos/custom';

// Direct image imports
import togetheraiLogo from '../../../assets/img/together-logo.png'
import googleLogo from '../../../assets/img/providers/google.svg';
import fireworksLogo from '../../../assets/img/providers/fireworks.png';
import { OllamaIcon } from "../icons/ProviderIcons";

export interface Provider {
  id: string;
  alias: string[];
  name: string;
  description: string;
  logo: string | React.ComponentType<React.SVGProps<SVGSVGElement>> | React.ComponentType<any>;
  
  base_url: string;
  configurable_base_url?: boolean;

  optional_api_key?: boolean; // If provider doesn't need an API key

  // A custom provider lets the user pick a name and a base URL for any
  // OpenAI-compatible endpoint; multiple instances per user are allowed.
  is_custom?: boolean;

  setup_instructions: string;

  // Direct URL to the provider's API key / console page.
  // Used by the Create Provider Endpoint dialog to render a "Get API key" link.
  api_key_url?: string;
}

export const PROVIDERS: Provider[] = [
  {
    id: 'user/openai',
    alias: ['openai', 'openai-api'],
    name: 'OpenAI',
    description: 'Connect to OpenAI for GPT models, image generation, and more.',
    logo: OpenAILogo,
    base_url: "https://api.openai.com/v1",
    setup_instructions: "Get your API key from https://platform.openai.com/settings/organization/api-keys",
    api_key_url: "https://platform.openai.com/settings/organization/api-keys"
  },
  {
    id: 'user/google',
    alias: ['google', 'google-api'],
    name: 'Google Gemini',
    description: 'Use Google AI models and services.',
    logo: googleLogo,
    // Gemini URL
    base_url: "https://generativelanguage.googleapis.com/v1beta/openai",
    setup_instructions: "Get your API key from https://aistudio.google.com/apikey",
    api_key_url: "https://aistudio.google.com/apikey"
  },
  {
    id: 'user/anthropic',
    alias: ['anthropic', 'anthropic-api'],
    name: 'Anthropic',
    description: 'Access Anthropic Claude models for advanced language tasks.',
    logo: AnthropicLogo,
    base_url: "https://api.anthropic.com/v1",
    setup_instructions: "Get your API key from https://platform.claude.com/settings/keys",
    api_key_url: "https://platform.claude.com/settings/keys"
  },
  {
    id: 'user/nvidia',
    alias: ['nvidia', 'nvidia-nim', 'nvidia-api'],
    name: 'NVIDIA NIM',
    description: 'NVIDIA-hosted, OpenAI-compatible inference for hosted LLMs.',
    logo: NvidiaLogo,
    base_url: "https://integrate.api.nvidia.com/v1",
    setup_instructions: "Get your API key from https://build.nvidia.com/ (Settings → API Keys)",
    api_key_url: "https://build.nvidia.com/"
  },
  {
    id: 'user/aws',
    alias: ['aws', 'aws-api'],
    name: 'Amazon Bedrock',
    description: 'Use AWS for AI models and services.',
    logo: AWSLogo,
    base_url: "https://bedrock.us-east-1.amazonaws.com",
    setup_instructions: "Get your API key from https://console.aws.amazon.com/bedrock/home?region=us-east-1#/providers",
    api_key_url: "https://console.aws.amazon.com/bedrock/home?region=us-east-1#/providers"
  },
  {
    id: 'user/groq',
    alias: ['groq', 'groq-api'],
    name: 'Groq',
    description: 'Integrate with Groq for ultra-fast LLM inference.',
    logo: GroqLogo,
    base_url: "https://api.groq.com/openai/v1",
    setup_instructions: "Get your API key from https://console.groq.com/",
    api_key_url: "https://console.groq.com/"
  },
  {
    id: 'user/cerebras',
    alias: ['cerebras', 'cerebras-api'],
    name: 'Cerebras',
    description: 'Integrate with Cerebras for ultra-fast LLM inference.',
    logo: CerebrasLogo,
    base_url: "https://api.cerebras.ai/v1",
    setup_instructions: "Get your API key from https://cloud.cerebras.ai/",
    api_key_url: "https://cloud.cerebras.ai/"
  },
  {
    id: 'user/xai',
    alias: ['xai', 'xai-api', 'grok'],
    name: 'xAI Grok',
    description: 'Access xAI Grok models via their OpenAI-compatible API.',
    logo: XaiLogo,
    base_url: "https://api.x.ai/v1",
    setup_instructions: "Get your API key from https://console.x.ai/ (API Keys section)",
    api_key_url: "https://console.x.ai/"
  },
  {
    id: 'user/togetherai',
    alias: ['togetherai', 'togetherai-api'],
    name: 'TogetherAI',
    description: 'Integrate with TogetherAI for ultra-fast LLM inference.',
    logo: togetheraiLogo,
    base_url: "https://api.together.xyz/v1",
    setup_instructions: "Get your API key from https://api.together.xyz/",
    api_key_url: "https://api.together.xyz/"
  },
  {
    id: 'user/fireworks',
    alias: ['fireworks', 'fireworks-api'],
    name: 'Fireworks AI',
    description: 'Fireworks AI offers cheap and fast LLM infernece.',
    logo: fireworksLogo,
    base_url: "https://api.fireworks.ai/inference/v1",
    setup_instructions: "Register at https://fireworks.ai and get your API key from https://app.fireworks.ai/settings/users/api-keys",
    api_key_url: "https://app.fireworks.ai/settings/users/api-keys"
  },
  {
    id: 'user/ollama',
    alias: ['ollama', 'ollama-api'],
    name: 'Ollama',
    description: 'Integrate with Ollama which is running on your local machine or server.',
    logo: OllamaIcon,
    base_url: "http://host.docker.internal:11434/v1",
    configurable_base_url: true,
    optional_api_key: true,
    setup_instructions: "Open Ollama settings and turn on 'Expose Ollama to the network'"
  },
  {
    id: 'user/custom',
    alias: ['custom'],
    name: 'Custom Provider',
    description: 'Connect any OpenAI-compatible API by providing a base URL and optional API key.',
    logo: CustomLogo,
    base_url: '',
    configurable_base_url: true,
    optional_api_key: true,
    is_custom: true,
    setup_instructions: 'Give your provider a unique name, the OpenAI-compatible base URL, and an API key if the endpoint requires one.'
  }
];