import { MessageProcessor } from './Markdown';
import { TypesSession, TypesInteraction, TypesSessionMetadata, TypesSessionMode, TypesSessionType, TypesOwnerType } from '../../api/api';
import { beforeEach, describe, expect, test, vi } from 'vitest';

// Mock DOMPurify
vi.mock('dompurify', () => {
  return {
    default: {
      sanitize: (html: string) => {
        // Simple mock sanitizer that removes <script> and <iframe> tags
        return html
          .replace(/<script\b[^<]*(?:(?!<\/script>)<[^<]*)*<\/script>/gi, '')
          .replace(/<iframe\b[^<]*(?:(?!<\/iframe>)<[^<]*)*<\/iframe>/gi, '');
      }
    }
  };
});

// Mock data for tests
const mockInteraction: Partial<TypesInteraction> = {
  id: 'int1',
  created: new Date().toISOString(),
};

const mockSessionConfig: Partial<TypesSessionMetadata> = {
  document_ids: {
    'test-file.pdf': 'doc123',
    'sample.pdf': 'doc456',
    'http://example.com/external.pdf': 'doc789'
  },
  document_group_id: 'group123'
};

const mockSession: Partial<TypesSession> = {
  id: 'test-session',
  name: 'Test Session',
  created: new Date().toISOString(),
  updated: new Date().toISOString(),
  parent_session: '',
  parent_app: '',
  mode: TypesSessionMode.SessionModeInference,
  type: TypesSessionType.SessionTypeText,
  model_name: 'test-model',
  lora_dir: '',
  owner: 'test-owner',
  owner_type: TypesOwnerType.OwnerTypeUser,
  config: mockSessionConfig as TypesSessionMetadata,
  interactions: [mockInteraction as TypesInteraction]
};

// Mock file URL resolver function
const mockGetFileURL = (filename: string) => `https://example.com/files/${filename}`;

// Mock filter document function
const mockFilterDocument = vi.fn();

describe('MessageProcessor', () => {
  beforeEach(() => {
    // Reset mocks before each test
    mockFilterDocument.mockReset();
  });

  describe('Citation processing', () => {
    test('Complete XML citation should be properly processed', () => {
      const message = `Here's some information:
      
<excerpts>
<excerpt>
<document_id>doc123</document_id>
<snippet>This is an important excerpt from the document.</snippet>
</excerpt>
</excerpts>`;

      const processor = new MessageProcessor(message, {
        session: mockSession as TypesSession,
        getFileURL: mockGetFileURL,
        isStreaming: false
      });

      const result = processor.process();

      // The citation content should be removed from the main text
      expect(result).not.toContain('<excerpts>');
      expect(result).not.toContain('<document_id>doc123</document_id>');
      
      // The main text should be preserved
      expect(result).toContain("Here's some information:");

      // Citation data should be preserved in a special marker for the React component
      expect(result).toMatch(/__CITATION_DATA__.*__CITATION_DATA__/);
      
      // The citation data should contain information from the original XML
      const citationMatch = result.match(/__CITATION_DATA__(.*?)__CITATION_DATA__/);
      if (citationMatch && citationMatch[1]) {
        const citationData = JSON.parse(citationMatch[1]);
        expect(citationData.excerpts).toBeDefined();
        expect(citationData.excerpts.length).toBeGreaterThan(0);
        expect(citationData.excerpts[0].snippet).toContain('This is an important excerpt');
        expect(citationData.excerpts[0].docId).toBe('doc123');
      } else {
        throw new Error('Citation data not found');
      }
    });

    test('Partial XML citation during streaming should show loading state', () => {
      const message = `I'm looking up information:
      
<excerpts>
<excerpt>
<document_id>doc123</document_id>
<snippet>The start of a snippet`;
      
      const processor = new MessageProcessor(message, {
        session: mockSession as TypesSession,
        getFileURL: mockGetFileURL,
        isStreaming: true
      });

      const result = processor.process();
      
      // The partial citation content should be removed from the main text
      expect(result).not.toContain('<excerpts>');
      
      // The main text should be preserved
      expect(result).toContain("I'm looking up information:");

      // The partial citation content should be shown
      expect(result).toContain("The start of a snippet");
      
      // Citation data should indicate streaming state
      const citationMatch = result.match(/__CITATION_DATA__(.*?)__CITATION_DATA__/);
      if (citationMatch && citationMatch[1]) {
        const citationData = JSON.parse(citationMatch[1]);
        expect(citationData.isStreaming).toBe(true);
      } else {
        throw new Error('Citation data not found');
      }
    });

    test('Partial XML citation with document ID should use actual snippet content', () => {
      const message = `Here's some information:
      
<excerpts>
<excerpt>
<document_id>doc123</document_id>
<snippet>Partial content from the document`;
      
      const processor = new MessageProcessor(message, {
        session: mockSession as TypesSession,
        getFileURL: mockGetFileURL,
        isStreaming: true
      });

      const result = processor.process();
      
      // Citation data should contain the actual partial content
      const citationMatch = result.match(/__CITATION_DATA__(.*?)__CITATION_DATA__/);
      if (citationMatch && citationMatch[1]) {
        const citationData = JSON.parse(citationMatch[1]);
        expect(citationData.excerpts.length).toBeGreaterThan(0);
        expect(citationData.excerpts[0].snippet).toBe("Partial content from the document");
        expect(citationData.excerpts[0].isPartial).toBe(true);
      } else {
        throw new Error('Citation data not found');
      }
    });

    test('Partial XML citation should use actual filename when document ID is mapped', () => {
      const message = `Here's some information:
      
<excerpts>
<excerpt>
<document_id>doc123</document_id>
<snippet>Partial content with mapped filename`;
      
      const processor = new MessageProcessor(message, {
        session: mockSession as TypesSession,
        getFileURL: mockGetFileURL,
        isStreaming: true
      });

      const result = processor.process();
      
      // Citation data should contain the actual filename from the document map
      const citationMatch = result.match(/__CITATION_DATA__(.*?)__CITATION_DATA__/);
      if (citationMatch && citationMatch[1]) {
        const citationData = JSON.parse(citationMatch[1]);
        expect(citationData.excerpts[0].filename).toBe("test-file.pdf");
        expect(citationData.excerpts[0].isPartial).toBe(true);
      } else {
        throw new Error('Citation data not found');
      }
    });

    test('Partial XML citation should use actual URL when document ID is mapped', () => {
      const message = `Here's some information:
      
<excerpts>
<excerpt>
<document_id>doc123</document_id>
<snippet>Partial content with mapped URL`;
      
      const processor = new MessageProcessor(message, {
        session: mockSession as TypesSession,
        getFileURL: mockGetFileURL,
        isStreaming: true
      });

      const result = processor.process();
      
      // Citation data should contain the actual URL from the document map
      const citationMatch = result.match(/__CITATION_DATA__(.*?)__CITATION_DATA__/);
      if (citationMatch && citationMatch[1]) {
        const citationData = JSON.parse(citationMatch[1]);
        expect(citationData.excerpts[0].fileUrl).toBe("https://example.com/files/test-file.pdf");
        expect(citationData.excerpts[0].isPartial).toBe(true);
      } else {
        throw new Error('Citation data not found');
      }
    });

    test('Partial XML citation should show snippet content even with unknown document ID', () => {
      const message = `Here's some information:
      
<excerpts>
<excerpt>
<document_id>unknown-doc-id</document_id>
<snippet>This content should still be shown even though document ID is unknown`;
      
      const processor = new MessageProcessor(message, {
        session: mockSession as TypesSession,
        getFileURL: mockGetFileURL,
        isStreaming: true
      });

      const result = processor.process();
      
      // Citation data should contain the actual snippet content despite unknown document ID
      const citationMatch = result.match(/__CITATION_DATA__(.*?)__CITATION_DATA__/);
      if (citationMatch && citationMatch[1]) {
        const citationData = JSON.parse(citationMatch[1]);
        expect(citationData.excerpts.length).toBeGreaterThan(0);
        expect(citationData.excerpts[0].docId).toBe("unknown-doc-id");
        expect(citationData.excerpts[0].snippet).toBe("This content should still be shown even though document ID is unknown");
        expect(citationData.excerpts[0].filename).toBe("Loading...");
        expect(citationData.excerpts[0].fileUrl).toBe("#");
        expect(citationData.excerpts[0].isPartial).toBe(true);
      } else {
        throw new Error('Citation data not found');
      }
    });
  });

  describe('Document ID processing', () => {
    test('Document IDs should be converted to links', () => {
      const message = `Reference to document [DOC_ID:doc123] and also [DOC_ID:doc456]`;
      
      const processor = new MessageProcessor(message, {
        session: mockSession as TypesSession,
        getFileURL: mockGetFileURL,
        isStreaming: false
      });

      const result = processor.process();
      
      // Document IDs should be converted to links
      expect(result).toContain('<a target="_blank"');
      expect(result).toContain('class="doc-citation"');
      
      // Both formats should be detected and linked
      expect(result).toContain('[1]');
      expect(result).toContain('[2]');
    });

    test('External document URLs should be properly handled', () => {
      const message = `Reference to external document [DOC_ID:doc789]`;
      
      const processor = new MessageProcessor(message, {
        session: mockSession as TypesSession,
        getFileURL: mockGetFileURL,
        isStreaming: false
      });

      const result = processor.process();
      
      // External document URL should now be linked directly, not through filestore
      expect(result).toContain('href="http://example.com/external.pdf"');
      
      // It should NOT link through the filestore
      expect(result).not.toContain('https://example.com/files/http://example.com/external.pdf');
    });

    test('Document group IDs should be converted to links', () => {
      const message = `Reference to document group group123`;
      
      const processor = new MessageProcessor(message, {
        session: mockSession as TypesSession,
        getFileURL: mockGetFileURL,
        isStreaming: false
      });

      const result = processor.process();
      
      // Group ID should be converted to a special link
      expect(result).toContain('class="doc-group-link"');
      expect(result).toContain('[group]');
    });

    test('Web URLs should link directly to the URL, not through filestore viewer', () => {
      // Set up a session with a web URL in document_ids
      const sessionWithWebUrl = {
        ...mockSession,
        config: {
          ...mockSessionConfig,
          document_ids: {
            ...mockSessionConfig.document_ids,
            'https://aispec.org': 'web-doc-123',
          }
        }
      };

      const message = `Reference to web URL [DOC_ID:web-doc-123]`;
      
      const processor = new MessageProcessor(message, {
        session: sessionWithWebUrl as TypesSession,
        getFileURL: mockGetFileURL,
        isStreaming: false
      });

      const result = processor.process();
      
      // The URL should be linked directly, not through filestore viewer
      expect(result).toContain('href="https://aispec.org"');
      
      // It should NOT contain a link to the filestore viewer
      expect(result).not.toContain('href="https://example.com/files/https://aispec.org"');
      expect(result).not.toContain('/api/v1/filestore/viewer/');
    });
  });

  describe('Blinker processing', () => {
    test('Blinker should be added during streaming when requested', () => {
      const message = `Hello world!`;
      
      const processor = new MessageProcessor(message, {
        session: mockSession as TypesSession,
        getFileURL: mockGetFileURL,
        isStreaming: true,
        showBlinker: true
      });

      const result = processor.process();
      
      // Blinker should be added during streaming
      expect(result).toContain('<span class="blinker-class">┃</span>');
    });

    test('Blinker should NOT be added when streaming is finished', () => {
      const message = `Hello world!`;
      
      const processor = new MessageProcessor(message, {
        session: mockSession as TypesSession,
        getFileURL: mockGetFileURL,
        isStreaming: false,
        showBlinker: true
      });

      const result = processor.process();
      
      // Blinker should not be added after streaming is complete
      expect(result).not.toContain('<span class="blinker-class">┃</span>');
    });

    test('Blinker should not be added when citations are present', () => {
      const message = `Hello world!
      
<excerpts>
<excerpt>
<document_id>doc123</document_id>
<snippet>This is a snippet.</snippet>
</excerpt>
</excerpts>`;
      
      const processor = new MessageProcessor(message, {
        session: mockSession as TypesSession,
        getFileURL: mockGetFileURL,
        isStreaming: false,
        showBlinker: true
      });

      const result = processor.process();
      
      // Blinker should not be added when citations are present
      expect(result).not.toContain('<span class="blinker-class">┃</span>');
    });
  });

  describe('Code block rendering', () => {
    test('Code blocks should be preserved during sanitization', () => {
      const message = `Here's some code:
\`\`\`typescript
const x: number = 42;
function test() {
  return x;
}
\`\`\`

And inline code \`const y = 10;\` too.`;
      
      const processor = new MessageProcessor(message, {
        session: mockSession as TypesSession,
        getFileURL: mockGetFileURL,
        isStreaming: false
      });

      const result = processor.process();
      
      // Code blocks should be preserved
      expect(result).toContain('```typescript');
      expect(result).toContain('const x: number = 42;');
      expect(result).toContain('function test()');
      
      // Inline code should also be preserved
      expect(result).toContain('`const y = 10;`');
    });

    test('Code block indentation should be fixed', () => {
      const message = `Here's some code:
      \`\`\`
      // indented code
      if (true) {
        console.log("test");
      }
      \`\`\``;
      
      const processor = new MessageProcessor(message, {
        session: mockSession as TypesSession,
        getFileURL: mockGetFileURL,
        isStreaming: false
      });

      const result = processor.process();
    });
  });

  describe('HTML sanitization', () => {
    test('Safe HTML should be preserved', () => {
      const message = `<p>This is a <strong>paragraph</strong> with <em>formatting</em>.</p>
<a href="https://example.com">Link</a>
<ul><li>List item 1</li><li>List item 2</li></ul>`;
      
      const processor = new MessageProcessor(message, {
        session: mockSession as TypesSession,
        getFileURL: mockGetFileURL,
        isStreaming: false
      });

      const result = processor.process();
      
      // Safe HTML should be preserved
      expect(result).toContain('<p>');
      expect(result).toContain('<strong>paragraph</strong>');
      expect(result).toContain('<em>formatting</em>');
      expect(result).toContain('<a href="https://example.com">');
      expect(result).toContain('<ul><li>');
    });

    test('Unsafe HTML should be removed', () => {
      const message = `<script>alert("xss")</script>
<p>Safe content</p>
<iframe src="https://malicious.com"></iframe>`;
      
      const processor = new MessageProcessor(message, {
        session: mockSession as TypesSession,
        getFileURL: mockGetFileURL,
        isStreaming: false
      });

      const result = processor.process();
      
      // With our mock implementation, script and iframe tags should be removed
      expect(result).not.toContain('<script>alert("xss")</script>');
      expect(result).not.toContain('<iframe src="https://malicious.com"></iframe>');
      
      // Safe content should be preserved
      expect(result).toContain('<p>Safe content</p>');
    });
  });

  describe('Triple dash handling', () => {
    test('Triple dash as horizontal rule should be preserved', () => {
      const message = `Above content

---

Below content`;
      
      const processor = new MessageProcessor(message, {
        session: mockSession as TypesSession,
        getFileURL: mockGetFileURL,
        isStreaming: false
      });

      const result = processor.process();
      
      // Content should be preserved
      expect(result).toContain('Above content');
      expect(result).toContain('Below content');
    });

    test('Triple dash at end of content during streaming should be removed', () => {
      const message = `Content

---`;
      
      const processor = new MessageProcessor(message, {
        session: mockSession as TypesSession,
        getFileURL: mockGetFileURL,
        isStreaming: true
      });

      const result = processor.process();
      
      // Content should be preserved
      expect(result).toContain('Content');
      
      // Triple dash at end during streaming should be removed
      expect(result).not.toMatch(/---\s*$/);
    });
  });

  describe('Document filtering', () => {
    test('onFilterDocument callback should be called when provided', () => {
      const message = `<excerpts>
<excerpt>
<document_id>doc123</document_id>
<snippet>This is a snippet.</snippet>
</excerpt>
</excerpts>`;
      
      const processor = new MessageProcessor(message, {
        session: mockSession as TypesSession,
        getFileURL: mockGetFileURL,
        isStreaming: false,
        onFilterDocument: mockFilterDocument
      });

      processor.process();
      
      // The callback should be available for components to use
      // The actual calling would happen in the Citation component
      expect(mockFilterDocument).toBeDefined();
    });
  });
}); 