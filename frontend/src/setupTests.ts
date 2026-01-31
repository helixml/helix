import '@testing-library/jest-dom'
import { vi } from 'vitest'

// Mock for DOMPurify that simulates basic sanitization behavior
vi.mock('dompurify', () => {
  return {
    default: {
      sanitize: (html: string, config: any = {}) => {
        // Basic simulation of DOMPurify's sanitization
        if (!html) return '';
        
        // Parse allowed tags from config or use defaults
        const allowedTags = config.ALLOWED_TAGS || [
          'b', 'i', 'u', 'strong', 'em', 'code', 'pre', 
          'a', 'span', 'div', 'p', 'ul', 'ol', 'li',
          'h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'details', 'summary'
        ];
        
        // Keep think-container and thinking classes for testing
        let sanitized = html;
        
        // Simulate preserving allowed tags
        if (allowedTags.includes('div')) {
          // Preserve think-container divs for testing
          const thinkingPattern = /<div class="think-container( thinking)?"/g;
          sanitized = sanitized.replace(thinkingPattern, (match) => match);
        }
        
        return sanitized;
      }
    }
  }
})

// Stub global window objects used in components
global.window.scrollTo = vi.fn();

// Mock for react-markdown that preserves HTML structure
vi.mock('react-markdown', () => {
  return {
    default: ({ children }: { children: string }) => {
      return children // Pass through content for tests
    }
  }
})

// Mock react-syntax-highlighter to pass through content
vi.mock('react-syntax-highlighter', () => {
  const SyntaxHighlighterMock = ({ children }: { children: string }) => {
    return children
  }
  
  return {
    Prism: SyntaxHighlighterMock
  }
})

// Mock styles
vi.mock('react-syntax-highlighter/dist/esm/styles/prism', () => {
  return {
    oneDark: {}
  }
}) 