declare module 'dompurify' {
  const DOMPurify: {
    sanitize: (input: string, options?: any) => string;
  };
  export default DOMPurify;
} 