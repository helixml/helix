// Import SVGs as components
import OpenAILogo from './logos/openai';
import AnthropicLogo from './logos/anthropic';
import GroqLogo from './logos/groq';
import CerebrasLogo from './logos/cerebras';
import AWSLogo from './logos/aws';

// Direct image imports
import togetheraiLogo from '../../../assets/img/together-logo.png'
import googleLogo from '../../../assets/img/providers/google.svg';
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

  setup_instructions: string;
}

export const PROVIDERS: Provider[] = [
  {
    id: 'user/openai',
    alias: ['openai', 'openai-api'],
    name: 'OpenAI',
    description: 'Connect to OpenAI for GPT models, image generation, and more.',
    logo: OpenAILogo,
    base_url: "https://api.openai.com/v1",
    setup_instructions: "Get your API key from https://platform.openai.com/settings/organization/api-keys"
  },
  {
    id: 'user/google',
    alias: ['google', 'google-api'],
    name: 'Google Gemini',
    description: 'Use Google AI models and services.',
    logo: googleLogo,
    // Gemini URL
    base_url: "https://generativelanguage.googleapis.com/v1beta/openai",
    setup_instructions: "Get your API key from https://aistudio.google.com/apikey"
  },
  {
    id: 'user/anthropic',
    alias: ['anthropic', 'anthropic-api'],
    name: 'Anthropic',
    description: 'Access Anthropic Claude models for advanced language tasks.',
    logo: AnthropicLogo,
    base_url: "https://api.anthropic.com",
    setup_instructions: "Get your API key from https://console.anthropic.com/api-keys"
  },
  {
    id: 'user/aws',
    alias: ['aws', 'aws-api'],
    name: 'Amazon Bedrock',
    description: 'Use AWS for AI models and services.',
    logo: AWSLogo,
    base_url: "https://bedrock.us-east-1.amazonaws.com",
    setup_instructions: "Get your API key from https://console.aws.amazon.com/bedrock/home?region=us-east-1#/providers"
  },
  {
    id: 'user/groq',
    alias: ['groq', 'groq-api'],
    name: 'Groq',
    description: 'Integrate with Groq for ultra-fast LLM inference.',
    logo: GroqLogo,
    base_url: "https://api.groq.com/openai/v1",
    setup_instructions: "Get your API key from https://console.groq.com/"
  },
  {
    id: 'user/cerebras',
    alias: ['cerebras', 'cerebras-api'],
    name: 'Cerebras',
    description: 'Integrate with Cerebras for ultra-fast LLM inference.',
    logo: CerebrasLogo,
    base_url: "https://api.cerebras.ai/v1",
    setup_instructions: "Get your API key from https://cloud.cerebras.ai/"
  },
  {
    id: 'user/togetherai',
    alias: ['togetherai', 'togetherai-api'],
    name: 'TogetherAI',
    description: 'Integrate with TogetherAI for ultra-fast LLM inference.',
    logo: togetheraiLogo, 
    base_url: "https://api.together.xyz/v1",
    setup_instructions: "Get your API key from https://api.together.xyz/"
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
  }
];