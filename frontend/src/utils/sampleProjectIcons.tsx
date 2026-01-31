import React from 'react'
import { FilePlus, Mail, Hash, TrendingUp, Target, Headphones, FileText, BookOpen, Megaphone, PenTool, MessageCircle, Newspaper, Rocket, Database, CheckCircle2, ShoppingCart, Cloud, FileCode } from 'lucide-react'
import {
  SiNodedotjs,
  SiReact,
  SiPython,
  SiNextdotjs,
  SiExpress,
  SiVuedotjs,
  SiAngular,
  SiDjango,
  SiFlask,
  SiSpringboot,
  SiLinkedin,
  SiJupyter,
  SiDotnet,
  SiPandas,
  SiApacheairflow,
  SiMongodb,
  SiStripe,
} from 'react-icons/si'

/**
 * Get technology-specific icon for sample projects/repositories
 * Uses consistent Helix brand color like the selected sidebar items
 */
export const getSampleProjectIcon = (
  sampleId?: string,
  category?: string,
  size: number = 18
): JSX.Element => {
  // Helix brand color - matches selected sidebar items (GREEN_BUTTON_HOVER)
  const iconColor = '#00d5ff'
  const iconStyle = { color: iconColor }

  // Map by specific ID first
  const iconMap: Record<string, JSX.Element> = {
    // Empty/Generic
    'empty': <FilePlus size={size} style={iconStyle} />,
    'blank': <FilePlus size={size} style={iconStyle} />,

    // Sample Projects (simple_sample_projects.go)
    'modern-todo-app': <SiReact size={size} style={iconStyle} />,
    'ecommerce-api': <ShoppingCart size={size} style={iconStyle} />,
    'weather-app': <Cloud size={size} style={iconStyle} />,
    'blog-cms': <SiNextdotjs size={size} style={iconStyle} />,

    // Development Projects
    'nodejs-todo': <SiNodedotjs size={size} style={iconStyle} />,
    'react-app': <SiReact size={size} style={iconStyle} />,
    'react-dashboard': <SiReact size={size} style={iconStyle} />,
    'python-api': <SiPython size={size} style={iconStyle} />,
    'nextjs-app': <SiNextdotjs size={size} style={iconStyle} />,
    'express-api': <SiExpress size={size} style={iconStyle} />,
    'vue-app': <SiVuedotjs size={size} style={iconStyle} />,
    'angular-app': <SiAngular size={size} style={iconStyle} />,
    'django-app': <SiDjango size={size} style={iconStyle} />,
    'flask-api': <SiFlask size={size} style={iconStyle} />,
    'spring-boot': <SiSpringboot size={size} style={iconStyle} />,

    // Business Tasks
    'linkedin-outreach': <SiLinkedin size={size} style={iconStyle} />,
    'email-campaign': <Mail size={size} style={iconStyle} />,
    'social-media': <Hash size={size} style={iconStyle} />,
    'sales-automation': <TrendingUp size={size} style={iconStyle} />,
    'lead-generation': <Target size={size} style={iconStyle} />,
    'customer-service': <Headphones size={size} style={iconStyle} />,

    // Content Creation
    'blog-posts': <FileText size={size} style={iconStyle} />,
    'helix-blog-posts': <FileCode size={size} style={iconStyle} />,
    'documentation': <BookOpen size={size} style={iconStyle} />,
    'marketing-content': <Megaphone size={size} style={iconStyle} />,
    'technical-writing': <PenTool size={size} style={iconStyle} />,
    'social-posts': <MessageCircle size={size} style={iconStyle} />,
    'newsletter': <Newspaper size={size} style={iconStyle} />,

    // Data & Analytics Projects
    'jupyter-financial-analysis': <SiJupyter size={size} style={iconStyle} />,
    'data-platform-api-migration': <SiApacheairflow size={size} style={iconStyle} />,
    'research-analysis-toolkit': <SiPandas size={size} style={iconStyle} />,
    'data-validation-toolkit': <CheckCircle2 size={size} style={iconStyle} />,

    // Enterprise & .NET
    'portfolio-management-dotnet': <SiDotnet size={size} style={iconStyle} />,

    // Frontend Frameworks
    'angular-analytics-dashboard': <SiAngular size={size} style={iconStyle} />,
    'angular-version-migration': <SiAngular size={size} style={iconStyle} />,

    // Legacy Modernization
    'cobol-modernization': <Database size={size} style={iconStyle} />,
  }

  if (sampleId && iconMap[sampleId]) {
    return iconMap[sampleId]
  }

  // Fallback to category-based icons
  if (category) {
    switch (category.toLowerCase()) {
      case 'development':
      case 'web':
      case 'api':
      case 'mobile':
        return <Rocket size={size} style={iconStyle} />
      case 'business':
        return <TrendingUp size={size} style={iconStyle} />
      case 'content':
        return <FileText size={size} style={iconStyle} />
      default:
        return <Rocket size={size} style={iconStyle} />
    }
  }

  // Default fallback
  return <Rocket size={size} style={iconStyle} />
}
 
// Build check
