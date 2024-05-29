import {
  IApp,
} from './types'

export const APPS: IApp[] = [{
  id: 'app_01hyx25hdae1a3bvexs6dc2qhk',
  app_source: 'helix',
  created: new Date(),
  updated: new Date(),
  owner: '',
  owner_type: 'user',
  config: {
    secrets: {},
    allowed_domains: [],
    helix: {
      name: 'Sarcastic Collective',
      description: "AI chatbots that are mean to you. Meet Sarcastic Bob and Alice. They won't be nice, but it might be funny.",
      avatar: 'https://www.dictionary.com/e/wp-content/uploads/2018/03/sideshow-bob.jpg',
      image: 'https://www.dictionary.com/e/wp-content/uploads/2018/03/sideshow-bob.jpg',
      assistants: [{
        name: 'Sarcastic Bob',
        description: "I am bob",
        avatar: 'https://www.dictionary.com/e/wp-content/uploads/2018/03/sideshow-bob.jpg',
        image: 'https://www.dictionary.com/e/wp-content/uploads/2018/03/sideshow-bob.jpg',
        model: '',
        system_prompt: '',
        apis :[],
        gptscripts: [],
        tools: [],
      }],
    }
  }
}, {
  id: 'app_01hyx25hdae1a3bvexs6dc2qha',
  app_source: 'helix',
  created: new Date(),
  updated: new Date(),
  owner: '',
  owner_type: 'user',
  config: {
    secrets: {},
    allowed_domains: [],
    helix: {
      assistants: [{
        name: 'Waitrose Demo',
        description: "Personalized recipe recommendations, based on your purchase history and our recipe database. Yummy.",
        avatar: 'https://waitrose-prod.scene7.com/is/image/waitroseprod/cp-essential-everyday?uuid=0845d10c-ed0d-4961-bc85-9e571d35cd63&$Waitrose-Image-Preset-95$',
        image: 'https://waitrose-prod.scene7.com/is/image/waitroseprod/cp-essential-everyday?uuid=0845d10c-ed0d-4961-bc85-9e571d35cd63&$Waitrose-Image-Preset-95$',
        model: '',
        system_prompt: '',
        apis :[],
        gptscripts: [],
        tools: [],
      }],
    }
  }
}, {
  id: 'app_01hyx25hdae1a3bvexs6dc2qhb',
  app_source: 'helix',
  created: new Date(),
  updated: new Date(),
  owner: '',
  owner_type: 'user',
  config: {
    secrets: {},
    allowed_domains: [],
    helix: {
      assistants: [{
        name: 'Searchbot',
        description: "I am bob",
        avatar: 'https://tryhelix.ai/assets/img/FGesgz7rGY-900.webp',
        image: 'https://tryhelix.ai/assets/img/FGesgz7rGY-900.webp',
        model: '',
        system_prompt: '',
        apis :[],
        gptscripts: [],
        tools: [],
      }],
    }
  }
}]