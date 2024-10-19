import React, { useEffect } from 'react';
import { RedocStandaloneProps } from 'redoc';

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
      if (typeof Redoc === 'undefined') await loadScript('https://cdn.jsdelivr.net/npm/redoc@latest/bundles/redoc.standalone.js');

      Redoc.init(spec || specUrl, options, document.getElementById('redoc-container'), onLoaded);
    }

    setupRedoc();
  });

  return <div id="redoc-container" data-testid="redoc-container" />;
}

const OpenAPIPage: React.FC = () => {
  return (
    <div style={{ height: '100vh' }}>
      <RedocStandalone
        specUrl="/api/v1/swagger"
        options={{
          nativeScrollbars: true,
          theme: {
            colors: {
              primary: { main: '#3f51b5' }, // You can adjust this color to match your app's theme
            },
          },
          hideDownloadButton: true,
          expandResponses: "all",
        }}
      />
    </div>
  );
};

export default OpenAPIPage;
