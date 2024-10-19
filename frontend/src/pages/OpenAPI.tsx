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
              primary: { main: '#4caf50' },
              text: {
                primary: '#ffffff',
                secondary: '#c0c0c0',
              },
              http: {
                get: '#61affe',
                post: '#49cc90',
                put: '#fca130',
                options: '#0d5aa7',
                patch: '#50e3c2',
                delete: '#f93e3e',
                basic: '#999',
                link: '#31bbb6',
                head: '#9012fe',
              },
              responses: {
                success: {
                  color: '#ffffff',
                  backgroundColor: '#49cc90',
                },
                error: {
                  color: '#ffffff',
                  backgroundColor: '#f93e3e',
                },
                redirect: {
                  color: '#ffffff',
                  backgroundColor: '#fca130',
                },
                info: {
                  color: '#ffffff',
                  backgroundColor: '#61affe',
                },
              },
            },
            typography: {
              fontSize: '14px',
              lineHeight: '1.5em',
              fontFamily: '"Roboto", sans-serif',
              smoothing: 'antialiased',
              code: {
                fontSize: '13px',
                fontFamily: '"Roboto Mono", monospace',
                lineHeight: '1.6em',
                fontWeight: '400',
                color: '#ffffff',
                backgroundColor: '#2d3748',
              },
            },
            sidebar: {
              backgroundColor: '#1a202c',
              textColor: '#ffffff',
              activeTextColor: '#45a049',
            },
            rightPanel: {
              backgroundColor: '#2d3748',
              textColor: '#ffffff',
            },
            codeBlock: {
              backgroundColor: '#2d3748',
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
