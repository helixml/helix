import React from 'react'
import { FilePlus, Mail, Hash, TrendingUp, Target, Headphones, FileText, BookOpen, Megaphone, PenTool, MessageCircle, Newspaper, Rocket, Sparkles } from 'lucide-react'
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
} from 'react-icons/si'

/**
 * Get technology-specific icon with brand color for sample projects/repositories
 */
export const getSampleProjectIcon = (
  sampleId?: string,
  category?: string,
  size: number = 18
): JSX.Element => {
  // Map by specific ID first - using actual brand icons with official colors
  const iconMap: Record<string, JSX.Element> = {
    // Empty/Generic
    'empty': <FilePlus size={size} style={{ color: '#64748b' }} />,
    'blank': <FilePlus size={size} style={{ color: '#64748b' }} />,

    // Development Projects - Real brand icons!
    'nodejs-todo': <SiNodedotjs size={size} style={{ color: '#5FA04E' }} />, // Node.js official green
    'react-app': <SiReact size={size} style={{ color: '#61DAFB' }} />, // React official cyan
    'react-dashboard': <SiReact size={size} style={{ color: '#61DAFB' }} />, // React official cyan
    'python-api': <SiPython size={size} style={{ color: '#3776AB' }} />, // Python official blue
    'nextjs-app': <SiNextdotjs size={size} style={{ color: '#000000' }} />, // Next.js black
    'express-api': <SiExpress size={size} style={{ color: '#000000' }} />, // Express black
    'vue-app': <SiVuedotjs size={size} style={{ color: '#4FC08D' }} />, // Vue official green
    'angular-app': <SiAngular size={size} style={{ color: '#DD0031' }} />, // Angular official red
    'django-app': <SiDjango size={size} style={{ color: '#092E20' }} />, // Django official dark green
    'flask-api': <SiFlask size={size} style={{ color: '#000000' }} />, // Flask black
    'spring-boot': <SiSpringboot size={size} style={{ color: '#6DB33F' }} />, // Spring Boot official green

    // Business Tasks
    'linkedin-outreach': <SiLinkedin size={size} style={{ color: '#0A66C2' }} />, // LinkedIn official blue
    'email-campaign': <Mail size={size} style={{ color: '#ea4335' }} />, // Email red
    'social-media': <Hash size={size} style={{ color: '#1da1f2' }} />, // Twitter blue
    'sales-automation': <TrendingUp size={size} style={{ color: '#10b981' }} />, // Growth green
    'lead-generation': <Target size={size} style={{ color: '#f59e0b' }} />, // Target amber
    'customer-service': <Headphones size={size} style={{ color: '#8b5cf6' }} />, // Support purple

    // Content Creation
    'blog-posts': <FileText size={size} style={{ color: '#3b82f6' }} />, // Blog blue
    'helix-blog-posts': <Sparkles size={size} style={{ color: '#8b5cf6' }} />, // Helix purple
    'documentation': <BookOpen size={size} style={{ color: '#059669' }} />, // Docs green
    'marketing-content': <Megaphone size={size} style={{ color: '#ec4899' }} />, // Marketing pink
    'technical-writing': <PenTool size={size} style={{ color: '#6366f1' }} />, // Writing indigo
    'social-posts': <MessageCircle size={size} style={{ color: '#14b8a6' }} />, // Social teal
    'newsletter': <Newspaper size={size} style={{ color: '#f97316' }} />, // Newsletter orange
  }

  if (sampleId && iconMap[sampleId]) {
    return iconMap[sampleId]
  }

  // Fallback to category-based icons
  if (category) {
    switch (category.toLowerCase()) {
      case 'development':
        return <Rocket size={size} style={{ color: '#3b82f6' }} />
      case 'business':
        return <TrendingUp size={size} style={{ color: '#10b981' }} />
      case 'content':
        return <FileText size={size} style={{ color: '#8b5cf6' }} />
      default:
        return <Sparkles size={size} style={{ color: '#64748b' }} />
    }
  }

  // Default fallback
  return <Sparkles size={size} style={{ color: '#64748b' }} />
}
 
