import React from 'react';

const CustomLogo = (props: React.SVGProps<SVGSVGElement>) => (
  <svg
    fill="none"
    viewBox="0 0 24 24"
    stroke="currentColor"
    strokeWidth={1.6}
    strokeLinecap="round"
    strokeLinejoin="round"
    style={{ color: 'currentcolor' }}
    {...props}
  >
    <title>Custom Provider</title>
    <path d="M8 6.5 3.5 12 8 17.5" />
    <path d="m16 6.5 4.5 5.5L16 17.5" />
    <path d="m14 4-4 16" />
  </svg>
);

export default CustomLogo;
