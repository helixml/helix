/** @type {import('tailwindcss').Config} */
export default {
  content: [
    './index.html',
    './src/**/*.{ts,tsx}',
  ],
  theme: {
    extend: {
      colors: {
        compliance: {
          green: '#22c55e',
          'green-dark': '#16a34a',
          amber: '#f59e0b',
          'amber-dark': '#d97706',
          red: '#ef4444',
          'red-dark': '#dc2626',
        },
        surface: {
          50: '#f8fafc',
          100: '#1e293b',
          200: '#1a2332',
          300: '#151d2b',
          400: '#111827',
          500: '#0d1320',
          600: '#0a0f1a',
          700: '#070b14',
        },
      },
    },
  },
  plugins: [],
}
