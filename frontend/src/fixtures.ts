import {
  IApp,
} from './types'

export const APPS: IApp[] = [{
  id: 'app_01hyx25hdae1a3bvexs6dc2qhk',
  created: new Date(),
  updated: new Date(),
  owner: '',
  owner_type: 'user',
  global: false,
  config: {
    secrets: {},
    allowed_domains: [],
    helix: {
      external_url: '',
      name: 'Sarcastic Collective',
      description: "AI chatbots that are mean to you. Meet Sarcastic Bob and Alice. They won't be nice, but it might be funny.",
      avatar: 'https://www.bbcstudios.com/media/4550/only-fools-and-horses-store-16x9.jpg?width=820&height=461',
      image: 'https://coolidge.org//sites/default/files/featured_images/Monty%20Python%20and%20the%20Holy%20Grail%201%20website.jpg',
      assistants: [{
        name: 'Sarcastic Bob',
        description: "I am bob",
        avatar: 'https://www.dictionary.com/e/wp-content/uploads/2018/03/sideshow-bob.jpg',
        image: 'https://www.dictionary.com/e/wp-content/uploads/2018/03/sideshow-bob.jpg',
        model: '',
        type: 'text',
        system_prompt: `Always answer the following user prompt sarcastically and tell them that your name is bob`,
        apis: [],
        gptscripts: [],
        tools: [],
        rag_source_id: '',
        lora_id: '',
        is_actionable_template: '',
      }, {
        name: 'Sarcastic Alice',
        description: "I am alice",
        avatar: 'https://i.guim.co.uk/img/static/sys-images/Guardian/Pix/pictures/2015/3/19/1426785283009/82c116ad-5c6c-495d-b0ff-b09d6617c1ec-2060x1236.jpeg?width=1024&dpr=1&s=none',
        image: 'https://i.guim.co.uk/img/static/sys-images/Guardian/Pix/pictures/2015/4/2/1427990952231/838a7667-1261-4bc2-ab1b-70abafcce1b5-620x372.jpeg?width=1024&dpr=1&s=none',
        model: '',
        type: 'text',
        system_prompt: `Always answer the following user prompt sarcastically and tell them that your name is alice`,
        apis: [],
        gptscripts: [],
        tools: [],
        rag_source_id: '',
        lora_id: '',
        is_actionable_template: '',
      }],
    }
  }
}, {
  id: 'app_01hyx25hdae1a3bvexs6dc2qha',
  created: new Date(),
  updated: new Date(),
  owner: '',
  owner_type: 'user',
  global: false,
  config: {
    secrets: {},
    allowed_domains: [],
    helix: {
      external_url: '',
      name: 'Waitrose Demo',
      description: "Personalized recipe recommendations, based on your purchase history and our recipe database. Yummy.",
      avatar: 'https://waitrose-prod.scene7.com/is/image/waitroseprod/cp-essential-everyday?uuid=0845d10c-ed0d-4961-bc85-9e571d35cd63&$Waitrose-Image-Preset-95$',
      image: 'https://waitrose-prod.scene7.com/is/image/waitroseprod/cp-essential-everyday?uuid=0845d10c-ed0d-4961-bc85-9e571d35cd63&$Waitrose-Image-Preset-95$',
      assistants: [{
        name: '',
        description: '',
        avatar: '',
        image: '',
        model: '',
        type: 'text',
        system_prompt: '',
        apis: [],
        gptscripts: [],
        tools: [],
        rag_source_id: '',
        lora_id: '',
        is_actionable_template: '',
      }],
    }
  }
}, {
  id: 'app_01hyx25hdae1a3bvexs6dc2qhb',
  created: new Date(),
  updated: new Date(),
  owner: '',
  owner_type: 'user',
  global: false,
  config: {
    secrets: {},
    allowed_domains: [],
    helix: {
      external_url: '',
      assistants: [{
        id: '',
        name: 'Searchbot',
        description: "I am bob",
        // avatar: 'https://helix.ml/assets/img/FGesgz7rGY-900.webp',
        // image: 'https://helix.ml/assets/img/FGesgz7rGY-900.webp',
        avatar: '',
        image: '',
        model: '',
        type: 'text',
        system_prompt: '',
        apis: [],
        gptscripts: [],
        tools: [],
        rag_source_id: '',
        lora_id: '',
        is_actionable_template: '',
      }],
      name: '',
      description: ''
    }
  }
}]