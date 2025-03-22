import { describe, it, expect } from 'vitest';

// Simple function to test
function sum(a: number, b: number): number {
  return a + b;
}

describe('Basic test', () => {
  it('adds two numbers correctly', () => {
    expect(sum(1, 2)).toBe(3);
  });
}); 