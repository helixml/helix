import React from 'react';
import { Document, Page, Text, View, StyleSheet, Image, Link } from '@react-pdf/renderer';

const styles = StyleSheet.create({
  page: {
    padding: 40,
    fontFamily: 'Helvetica',
    fontSize: 11,
    lineHeight: 1.5,
    color: '#000000',
  },
  h1: {
    fontSize: 24,
    marginBottom: 12,
    marginTop: 10,
    fontFamily: 'Helvetica-Bold',
  },
  h2: {
    fontSize: 20,
    marginBottom: 10,
    marginTop: 10,
    fontFamily: 'Helvetica-Bold',
  },
  h3: {
    fontSize: 16,
    marginBottom: 8,
    marginTop: 8,
    fontFamily: 'Helvetica-Bold',
  },
  h4: {
    fontSize: 14,
    marginBottom: 8,
    marginTop: 8,
    fontFamily: 'Helvetica-Bold',
  },
  paragraph: {
    marginBottom: 8,
  },
  bold: {
    fontFamily: 'Helvetica-Bold',
  },
  italic: {
    fontStyle: 'italic',
  },
  code: {
    fontFamily: 'Courier',
    backgroundColor: '#f5f5f5',
    fontSize: 10,
  },
  codeBlock: {
    fontFamily: 'Courier',
    backgroundColor: '#f5f5f5',
    padding: 10,
    marginBottom: 10,
    fontSize: 10,
    borderRadius: 4,
  },
  link: {
    color: '#1976d2',
    textDecoration: 'none',
  },
  image: {
    marginVertical: 10,
    maxWidth: '100%',
  },
  blockquote: {
    borderLeftWidth: 4,
    borderLeftColor: '#ddd',
    paddingLeft: 10,
    marginBottom: 10,
  },
  blockquoteText: {
    fontStyle: 'italic',
    color: '#666',
  },
  list: {
    marginBottom: 10,
  },
  listItem: {
    flexDirection: 'row',
    marginBottom: 4,
  },
  bullet: {
    width: 20,
    fontSize: 11,
  },
  listItemContent: {
    flex: 1,
  },
  table: {
    marginVertical: 10,
    borderTopWidth: 1,
    borderTopColor: '#dfdfdf',
    borderLeftWidth: 1,
    borderLeftColor: '#dfdfdf',
  },
  tableRow: {
    flexDirection: 'row',
    borderBottomWidth: 1,
    borderBottomColor: '#dfdfdf',
  },
  tableHeaderRow: {
    flexDirection: 'row',
    borderBottomWidth: 1,
    borderBottomColor: '#dfdfdf',
    backgroundColor: '#f5f5f5',
  },
  tableCell: {
    flex: 1,
    padding: 5,
    borderRightWidth: 1,
    borderRightColor: '#dfdfdf',
  },
  tableCellText: {
    fontSize: 10,
  },
  tableHeaderText: {
    fontSize: 10,
    fontFamily: 'Helvetica-Bold',
  },
  hr: {
    borderBottomWidth: 1,
    borderBottomColor: '#e0e0e0',
    marginVertical: 10,
  },
});

interface ParsedNode {
  type: string;
  content?: string;
  children?: ParsedNode[];
  href?: string;
  src?: string;
  ordered?: boolean;
  rows?: string[][];
  headers?: string[];
  language?: string;
}

function parseMarkdown(markdown: string): ParsedNode[] {
  const lines = markdown.split('\n');
  const nodes: ParsedNode[] = [];
  let i = 0;

  while (i < lines.length) {
    const line = lines[i];

    // Code block
    if (line.startsWith('```')) {
      const language = line.slice(3).trim();
      const codeLines: string[] = [];
      i++;
      while (i < lines.length && !lines[i].startsWith('```')) {
        codeLines.push(lines[i]);
        i++;
      }
      nodes.push({ type: 'codeBlock', content: codeLines.join('\n'), language });
      i++;
      continue;
    }

    // Table
    if (line.includes('|') && i + 1 < lines.length && lines[i + 1].match(/^\|?[\s-:|]+\|?$/)) {
      const headerLine = line;
      const separatorLine = lines[i + 1];
      const headers = headerLine.split('|').map(h => h.trim()).filter(h => h);
      const rows: string[][] = [];
      i += 2;
      while (i < lines.length && lines[i].includes('|')) {
        const cells = lines[i].split('|').map(c => c.trim()).filter(c => c !== '');
        if (cells.length > 0) {
          rows.push(cells);
        }
        i++;
      }
      nodes.push({ type: 'table', headers, rows });
      continue;
    }

    // Heading
    const headingMatch = line.match(/^(#{1,6})\s+(.+)$/);
    if (headingMatch) {
      const level = headingMatch[1].length;
      nodes.push({ type: `h${level}`, content: headingMatch[2] });
      i++;
      continue;
    }

    // HR
    if (line.match(/^[-*_]{3,}$/)) {
      nodes.push({ type: 'hr' });
      i++;
      continue;
    }

    // Blockquote
    if (line.startsWith('>')) {
      const quoteLines: string[] = [];
      while (i < lines.length && lines[i].startsWith('>')) {
        quoteLines.push(lines[i].replace(/^>\s?/, ''));
        i++;
      }
      nodes.push({ type: 'blockquote', content: quoteLines.join('\n') });
      continue;
    }

    // Unordered list
    if (line.match(/^[\s]*[-*+]\s+/)) {
      const items: ParsedNode[] = [];
      while (i < lines.length && lines[i].match(/^[\s]*[-*+]\s+/)) {
        const itemContent = lines[i].replace(/^[\s]*[-*+]\s+/, '');
        items.push({ type: 'listItem', content: itemContent });
        i++;
      }
      nodes.push({ type: 'ul', children: items });
      continue;
    }

    // Ordered list
    if (line.match(/^[\s]*\d+\.\s+/)) {
      const items: ParsedNode[] = [];
      while (i < lines.length && lines[i].match(/^[\s]*\d+\.\s+/)) {
        const itemContent = lines[i].replace(/^[\s]*\d+\.\s+/, '');
        items.push({ type: 'listItem', content: itemContent });
        i++;
      }
      nodes.push({ type: 'ol', children: items, ordered: true });
      continue;
    }

    // Image (standalone)
    const imgMatch = line.match(/^!\[([^\]]*)\]\(([^)]+)\)$/);
    if (imgMatch) {
      nodes.push({ type: 'image', content: imgMatch[1], src: imgMatch[2] });
      i++;
      continue;
    }

    // Empty line
    if (line.trim() === '') {
      i++;
      continue;
    }

    // Paragraph (collect consecutive non-empty lines)
    const paragraphLines: string[] = [];
    while (i < lines.length && lines[i].trim() !== '' && !lines[i].startsWith('#') && !lines[i].startsWith('```') && !lines[i].startsWith('>') && !lines[i].match(/^[-*+]\s+/) && !lines[i].match(/^\d+\.\s+/) && !lines[i].match(/^[-*_]{3,}$/)) {
      paragraphLines.push(lines[i]);
      i++;
    }
    if (paragraphLines.length > 0) {
      nodes.push({ type: 'paragraph', content: paragraphLines.join(' ') });
    }
  }

  return nodes;
}

interface InlineSegment {
  type: 'text' | 'bold' | 'italic' | 'bolditalic' | 'code' | 'link' | 'image';
  content: string;
  href?: string;
  src?: string;
}

function parseInline(text: string): InlineSegment[] {
  const segments: InlineSegment[] = [];
  let remaining = text;

  while (remaining.length > 0) {
    // Image: ![alt](src)
    const imgMatch = remaining.match(/^!\[([^\]]*)\]\(([^)]+)\)/);
    if (imgMatch) {
      segments.push({ type: 'image', content: imgMatch[1], src: imgMatch[2] });
      remaining = remaining.slice(imgMatch[0].length);
      continue;
    }

    // Link: [text](url)
    const linkMatch = remaining.match(/^\[([^\]]+)\]\(([^)]+)\)/);
    if (linkMatch) {
      segments.push({ type: 'link', content: linkMatch[1], href: linkMatch[2] });
      remaining = remaining.slice(linkMatch[0].length);
      continue;
    }

    // Inline code: `code`
    const codeMatch = remaining.match(/^`([^`]+)`/);
    if (codeMatch) {
      segments.push({ type: 'code', content: codeMatch[1] });
      remaining = remaining.slice(codeMatch[0].length);
      continue;
    }

    // Bold+Italic: ***text*** or ___text___
    const boldItalicMatch = remaining.match(/^(\*\*\*|___)(.+?)\1/);
    if (boldItalicMatch) {
      segments.push({ type: 'bolditalic', content: boldItalicMatch[2] });
      remaining = remaining.slice(boldItalicMatch[0].length);
      continue;
    }

    // Bold: **text** or __text__
    const boldMatch = remaining.match(/^(\*\*|__)(.+?)\1/);
    if (boldMatch) {
      segments.push({ type: 'bold', content: boldMatch[2] });
      remaining = remaining.slice(boldMatch[0].length);
      continue;
    }

    // Italic: *text* or _text_
    const italicMatch = remaining.match(/^(\*|_)(.+?)\1/);
    if (italicMatch) {
      segments.push({ type: 'italic', content: italicMatch[2] });
      remaining = remaining.slice(italicMatch[0].length);
      continue;
    }

    // Plain text until next special character
    const nextSpecial = remaining.search(/[*_`\[!]/);
    if (nextSpecial === -1) {
      segments.push({ type: 'text', content: remaining });
      break;
    } else if (nextSpecial === 0) {
      segments.push({ type: 'text', content: remaining[0] });
      remaining = remaining.slice(1);
    } else {
      segments.push({ type: 'text', content: remaining.slice(0, nextSpecial) });
      remaining = remaining.slice(nextSpecial);
    }
  }

  return segments;
}

interface RenderInlineProps {
  text: string;
  serverConfig?: any;
}

const RenderInline: React.FC<RenderInlineProps> = ({ text, serverConfig }) => {
  const segments = parseInline(text);
  
  return (
    <>
      {segments.map((seg, idx) => {
        switch (seg.type) {
          case 'bold':
            return <Text key={idx} style={styles.bold}>{seg.content}</Text>;
          case 'italic':
            return <Text key={idx} style={styles.italic}>{seg.content}</Text>;
          case 'bolditalic':
            return <Text key={idx} style={[styles.bold, styles.italic]}>{seg.content}</Text>;
          case 'code':
            return <Text key={idx} style={styles.code}>{seg.content}</Text>;
          case 'link':
            return <Link key={idx} src={seg.href || ''} style={styles.link}>{seg.content}</Link>;
          case 'image':
            const src = seg.src || '';
            const finalSrc = src.startsWith('http') 
              ? src 
              : serverConfig?.filestore_prefix 
                ? `${serverConfig.filestore_prefix}/${src}?redirect_urls=true`
                : src;
            return <Image key={idx} style={styles.image} src={finalSrc} />;
          default:
            return <Text key={idx}>{seg.content}</Text>;
        }
      })}
    </>
  );
};

interface RenderNodeProps {
  node: ParsedNode;
  serverConfig?: any;
}

const RenderNode: React.FC<RenderNodeProps> = ({ node, serverConfig }) => {
  switch (node.type) {
    case 'h1':
      return <Text style={styles.h1} minPresenceAhead={20}><RenderInline text={node.content || ''} serverConfig={serverConfig} /></Text>;
    case 'h2':
      return <Text style={styles.h2} minPresenceAhead={20}><RenderInline text={node.content || ''} serverConfig={serverConfig} /></Text>;
    case 'h3':
      return <Text style={styles.h3} minPresenceAhead={15}><RenderInline text={node.content || ''} serverConfig={serverConfig} /></Text>;
    case 'h4':
    case 'h5':
    case 'h6':
      return <Text style={styles.h4} minPresenceAhead={15}><RenderInline text={node.content || ''} serverConfig={serverConfig} /></Text>;
    
    case 'paragraph':
      return <Text style={styles.paragraph}><RenderInline text={node.content || ''} serverConfig={serverConfig} /></Text>;
    
    case 'codeBlock':
      return (
        <View style={styles.codeBlock} wrap={false}>
          <Text>{node.content}</Text>
        </View>
      );
    
    case 'blockquote':
      return (
        <View style={styles.blockquote}>
          <Text style={styles.blockquoteText}>{node.content}</Text>
        </View>
      );
    
    case 'hr':
      return <View style={styles.hr} />;
    
    case 'image':
      const src = node.src || '';
      const finalSrc = src.startsWith('http') 
        ? src 
        : serverConfig?.filestore_prefix 
          ? `${serverConfig.filestore_prefix}/${src}?redirect_urls=true`
          : src;
      return <Image style={styles.image} src={finalSrc} />;
    
    case 'ul':
      return (
        <View style={styles.list}>
          {node.children?.map((item, idx) => (
            <View key={idx} style={styles.listItem}>
              <Text style={styles.bullet}>•</Text>
              <View style={styles.listItemContent}>
                <Text><RenderInline text={item.content || ''} serverConfig={serverConfig} /></Text>
              </View>
            </View>
          ))}
        </View>
      );
    
    case 'ol':
      return (
        <View style={styles.list}>
          {node.children?.map((item, idx) => (
            <View key={idx} style={styles.listItem}>
              <Text style={styles.bullet}>{idx + 1}.</Text>
              <View style={styles.listItemContent}>
                <Text><RenderInline text={item.content || ''} serverConfig={serverConfig} /></Text>
              </View>
            </View>
          ))}
        </View>
      );
    
    case 'table':
      return (
        <View style={styles.table}>
          {node.headers && (
            <View style={styles.tableHeaderRow} wrap={false}>
              {node.headers.map((header, idx) => (
                <View key={idx} style={styles.tableCell}>
                  <Text style={styles.tableHeaderText}>{header}</Text>
                </View>
              ))}
            </View>
          )}
          {node.rows?.map((row, rowIdx) => (
            <View key={rowIdx} style={styles.tableRow} wrap={false}>
              {row.map((cell, cellIdx) => (
                <View key={cellIdx} style={styles.tableCell}>
                  <Text style={styles.tableCellText}>{cell}</Text>
                </View>
              ))}
            </View>
          ))}
        </View>
      );
    
    default:
      return null;
  }
};

export interface PdfDocumentProps {
  markdown: string;
  serverConfig?: any;
}

export const PdfDocument: React.FC<PdfDocumentProps> = ({ markdown, serverConfig }) => {
  const nodes = React.useMemo(() => parseMarkdown(markdown), [markdown]);

  return (
    <Document>
      <Page size="A4" style={styles.page}>
        {nodes.map((node, idx) => (
          <RenderNode key={idx} node={node} serverConfig={serverConfig} />
        ))}
      </Page>
    </Document>
  );
};
