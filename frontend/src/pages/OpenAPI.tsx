import React, { useEffect } from 'react';

// Define proper types for RedocStandaloneProps
interface RedocStandaloneProps {
  spec?: object;
  specUrl?: string;
  options?: {
    nativeScrollbars?: boolean;
    theme?: any;
    hideDownloadButton?: boolean;
    expandResponses?: string;
    [key: string]: any;
  };
  onLoaded?: () => void;
}

// Local path for Redoc that won't be ignored by git
declare const Redoc: any;

async function loadScript(scriptSrc: string) {
  return new Promise((resolve, reject) => {
    const script = document.createElement('script');
    script.type = 'text/javascript';
    script.src = scriptSrc;
    script.async = true;
    script.onload = resolve;
    script.onerror = reject;
    document.head.appendChild(script);
  });
}

function RedocStandalone({ spec, specUrl, options, onLoaded }: RedocStandaloneProps) {
  useEffect(() => {
    async function setupRedoc() {
      // Load from local path instead of CDN
      if (typeof Redoc === 'undefined') await loadScript('/external-libs/redoc/redoc.standalone.js');

      Redoc.init(spec || specUrl, options, document.getElementById('redoc-container'), onLoaded);
    }

    setupRedoc();
  }, []); // Add empty dependency array to prevent infinite re-renders

  return <div id="redoc-container" data-testid="redoc-container" style={{ backgroundColor: 'white' }} />;
}

const OpenAPIPage: React.FC = () => {
  return (
    <div style={{ height: '100vh' }}>
      <RedocStandalone
        specUrl="/api/v1/swagger"
        options={{
          nativeScrollbars: true,
          hideDownloadButton: true,
          expandResponses: "all",
        }}
      />
    </div>
  );
};

export default OpenAPIPage;
