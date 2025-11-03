import { ServerSampleType } from '../api/api';

export interface SampleType extends ServerSampleType {}

// Sample type categories
export type SampleTypeCategory = 'empty' | 'development' | 'business' | 'content';

// Get icon for sample type
export const getSampleTypeIcon = (typeId: string): string => {
  const iconMap: Record<string, string> = {
    // Empty/Generic
    'empty': '📄',
    'blank': '📄',
    
    // Development Projects
    'nodejs-todo': '📗',
    'react-app': '⚛️',
    'python-api': '🐍',
    'nextjs-app': '🔷',
    'express-api': '🟢',
    'vue-app': '💚',
    'angular-app': '🔴',
    'django-app': '🐍',
    'flask-api': '🐍',
    'spring-boot': '☕',
    
    // Business Tasks
    'linkedin-outreach': '💼',
    'email-campaign': '📧',
    'social-media': '📱',
    'sales-automation': '💰',
    'lead-generation': '🎯',
    'customer-service': '🤝',
    
    // Content Creation
    'blog-posts': '📝',
    'documentation': '📚',
    'marketing-content': '📢',
    'technical-writing': '✍️',
    'social-posts': '📱',
    'newsletter': '📰',
    
    // Default fallback
    'default': '📦'
  };

  return iconMap[typeId] || iconMap['default'];
};

// Get category for sample type
export const getSampleTypeCategory = (typeId: string): SampleTypeCategory => {
  // Empty repositories
  if (typeId === 'empty' || typeId === 'blank') {
    return 'empty';
  }

  // Development projects
  const developmentTypes = [
    'nodejs-todo', 'react-app', 'python-api', 'nextjs-app', 
    'express-api', 'vue-app', 'angular-app', 'django-app', 
    'flask-api', 'spring-boot'
  ];
  if (developmentTypes.includes(typeId)) {
    return 'development';
  }

  // Business tasks
  const businessTypes = [
    'linkedin-outreach', 'email-campaign', 'social-media',
    'sales-automation', 'lead-generation', 'customer-service'
  ];
  if (businessTypes.includes(typeId)) {
    return 'business';
  }

  // Content creation
  const contentTypes = [
    'blog-posts', 'documentation', 'marketing-content',
    'technical-writing', 'social-posts', 'newsletter'
  ];
  if (contentTypes.includes(typeId)) {
    return 'content';
  }

  // Default to development
  return 'development';
};

// Check if it's a business task
export const isBusinessTask = (typeId: string): boolean => {
  return getSampleTypeCategory(typeId) === 'business';
};

// Get business task description
export const getBusinessTaskDescription = (typeId: string): string => {
  const descriptions: Record<string, string> = {
    'linkedin-outreach': 'Automated LinkedIn prospecting and personalized outreach campaigns',
    'email-campaign': 'Email marketing automation with personalization and tracking',
    'social-media': 'Social media content creation and posting automation',
    'sales-automation': 'Sales process automation and lead nurturing workflows',
    'lead-generation': 'Automated lead generation and qualification processes',
    'customer-service': 'Customer service automation and response templates',
    'blog-posts': 'Technical blog post creation and content strategy',
    'documentation': 'Technical documentation generation and maintenance',
    'marketing-content': 'Marketing content creation and campaign management',
    'technical-writing': 'Technical writing and documentation projects',
    'social-posts': 'Social media post creation and scheduling',
    'newsletter': 'Newsletter content creation and distribution'
  };

  return descriptions[typeId] || 'Automated task workflow';
};

// Get category display name
export const getCategoryDisplayName = (category: SampleTypeCategory): string => {
  const displayNames: Record<SampleTypeCategory, string> = {
    'empty': 'Start Fresh',
    'development': 'Development Projects',
    'business': 'Business Automation',
    'content': 'Content Creation'
  };

  return displayNames[category];
};

// Get category description
export const getCategoryDescription = (category: SampleTypeCategory): string => {
  const descriptions: Record<SampleTypeCategory, string> = {
    'empty': 'Begin with a blank repository for any technology stack',
    'development': 'Pre-configured development projects with common frameworks',
    'business': 'Business process automation and workflow templates',
    'content': 'Content creation and marketing automation workflows'
  };

  return descriptions[category];
};

// Sort sample types by category and priority
export const sortSampleTypes = (sampleTypes: SampleType[]): SampleType[] => {
  const categoryOrder: SampleTypeCategory[] = ['empty', 'business', 'development', 'content'];
  
  return sampleTypes.sort((a, b) => {
    const categoryA = getSampleTypeCategory(a.id || '');
    const categoryB = getSampleTypeCategory(b.id || '');
    
    const orderA = categoryOrder.indexOf(categoryA);
    const orderB = categoryOrder.indexOf(categoryB);
    
    if (orderA !== orderB) {
      return orderA - orderB;
    }
    
    // Within same category, sort by name
    return (a.name || '').localeCompare(b.name || '');
  });
};

// Group sample types by category
export const groupSampleTypesByCategory = (sampleTypes: SampleType[]): Record<SampleTypeCategory, SampleType[]> => {
  const grouped: Record<SampleTypeCategory, SampleType[]> = {
    empty: [],
    development: [],
    business: [],
    content: []
  };

  sampleTypes.forEach(type => {
    const category = getSampleTypeCategory(type.id || '');
    grouped[category].push(type);
  });

  return grouped;
};