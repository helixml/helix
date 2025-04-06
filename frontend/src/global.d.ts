declare module 'hookrouter'
declare module 'lang-detector'

import { ReactNode } from 'react';

declare global {
  namespace JSX {
    interface IntrinsicAttributes {
      children?: ReactNode; // Makes children optional everywhere
    }
  }
}