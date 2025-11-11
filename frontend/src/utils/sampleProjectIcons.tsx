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
 * Get technology-specific icon with brand color for sample projects/repositories
 */
export const getSampleProjectIcon = (
  sampleId?: string,
  category?: string,
  size: number = 18
): JSX.Element => {
  // Map by specific ID first - using actual brand icons with brand-appropriate colors
  // NOTE: Colors adjusted for visibility on dark backgrounds
  const iconMap: Record<string, JSX.Element> = {
    // Empty/Generic
    'empty': <FilePlus size={size} style={{ color: '#94a3b8' }} />,
    'blank': <FilePlus size={size} style={{ color: '#94a3b8' }} />,

    // Sample Projects (simple_sample_projects.go)
    'modern-todo-app': <SiReact size={size} style={{ color: '#61DAFB' }} />, // React cyan
    'ecommerce-api': <ShoppingCart size={size} style={{ color: '#10b981' }} />, // E-commerce green
    'weather-app': <Cloud size={size} style={{ color: '#38bdf8' }} />, // Weather sky blue
    'blog-cms': <SiNextdotjs size={size} style={{ color: '#ffffff' }} />, // Next.js white

    // Development Projects - Real brand icons with adjusted contrast
    'nodejs-todo': <SiNodedotjs size={size} style={{ color: '#68a063' }} />, // Node.js green (brighter)
    'react-app': <SiReact size={size} style={{ color: '#61DAFB' }} />, // React cyan
    'react-dashboard': <SiReact size={size} style={{ color: '#61DAFB' }} />, // React cyan
    'python-api': <SiPython size={size} style={{ color: '#4B8BBE' }} />, // Python blue (brighter)
    'nextjs-app': <SiNextdotjs size={size} style={{ color: '#ffffff' }} />, // Next.js white (not black)
    'express-api': <SiExpress size={size} style={{ color: '#ffffff' }} />, // Express white (not black)
    'vue-app': <SiVuedotjs size={size} style={{ color: '#42b883' }} />, // Vue green
    'angular-app': <SiAngular size={size} style={{ color: '#DD0031' }} />, // Angular red
    'django-app': <SiDjango size={size} style={{ color: '#0C4B33' }} />, // Django green (much brighter)
    'flask-api': <SiFlask size={size} style={{ color: '#ffffff' }} />, // Flask white (not black)
    'spring-boot': <SiSpringboot size={size} style={{ color: '#6DB33F' }} />, // Spring Boot green

    // Business Tasks
    'linkedin-outreach': <SiLinkedin size={size} style={{ color: '#0A66C2' }} />, // LinkedIn blue
    'email-campaign': <Mail size={size} style={{ color: '#ea4335' }} />, // Email red
    'social-media': <Hash size={size} style={{ color: '#1da1f2' }} />, // Twitter blue
    'sales-automation': <TrendingUp size={size} style={{ color: '#10b981' }} />, // Growth green
    'lead-generation': <Target size={size} style={{ color: '#f59e0b' }} />, // Target amber
    'customer-service': <Headphones size={size} style={{ color: '#a78bfa' }} />, // Support purple (brighter)

    // Content Creation
    'blog-posts': <FileText size={size} style={{ color: '#60a5fa' }} />, // Blog blue (brighter)
    'helix-blog-posts': <FileCode size={size} style={{ color: '#a78bfa' }} />, // Technical blog with code icon
    'documentation': <BookOpen size={size} style={{ color: '#34d399' }} />, // Docs green (brighter)
    'marketing-content': <Megaphone size={size} style={{ color: '#f472b6' }} />, // Marketing pink (brighter)
    'technical-writing': <PenTool size={size} style={{ color: '#818cf8' }} />, // Writing indigo (brighter)
    'social-posts': <MessageCircle size={size} style={{ color: '#2dd4bf' }} />, // Social teal (brighter)
    'newsletter': <Newspaper size={size} style={{ color: '#fb923c' }} />, // Newsletter orange (brighter)

    // Data & Analytics Projects
    'jupyter-financial-analysis': <SiJupyter size={size} style={{ color: '#F37626' }} />, // Jupyter orange
    'data-platform-api-migration': <SiApacheairflow size={size} style={{ color: '#E43921' }} />, // Airflow red (official color, better contrast)
    'research-analysis-toolkit': <SiPandas size={size} style={{ color: '#E70488' }} />, // Pandas pink (much brighter official alt color)
    'data-validation-toolkit': <CheckCircle2 size={size} style={{ color: '#22c55e' }} />, // Validation green (brighter)

    // Enterprise & .NET
    'portfolio-management-dotnet': <SiDotnet size={size} style={{ color: '#7c3aed' }} />, // .NET purple (brighter)

    // Frontend Frameworks
    'angular-analytics-dashboard': <SiAngular size={size} style={{ color: '#DD0031' }} />, // Angular red
    'angular-version-migration': <SiAngular size={size} style={{ color: '#DD0031' }} />, // Angular red

    // Legacy Modernization
    'cobol-modernization': <Database size={size} style={{ color: '#9ca3af' }} />, // Legacy gray (brighter)
  }

  if (sampleId && iconMap[sampleId]) {
    return iconMap[sampleId]
  }

  // Fallback to category-based icons (with good contrast on dark backgrounds)
  if (category) {
    switch (category.toLowerCase()) {
      case 'development':
      case 'web':
      case 'api':
      case 'mobile':
        return <Rocket size={size} style={{ color: '#60a5fa' }} />
      case 'business':
        return <TrendingUp size={size} style={{ color: '#34d399' }} />
      case 'content':
        return <FileText size={size} style={{ color: '#a78bfa' }} />
      default:
        return <Rocket size={size} style={{ color: '#94a3b8' }} />
    }
  }

  // Default fallback
  return <Rocket size={size} style={{ color: '#94a3b8' }} />
}
 
