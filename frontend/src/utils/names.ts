const adjectives: string[] = [
  "enchanting",
  "fascinating",
  "elucidating",
  "useful",
  "helpful",
  "constructive",
  "charming",
  "playful",
  "whimsical",
  "delightful",
  "fantastical",
  "magical",
  "spellbinding",
  "dazzling",
];

const nouns: string[] = [
  "discussion",
  "dialogue",
  "convo",
  "conversation",
  "chat",
  "talk",
  "exchange",
  "debate",
  "conference",
  "seminar",
  "symposium",
];

export function generateAmusingName(): string {
  const adj: string = adjectives[Math.floor(Math.random() * adjectives.length)];
  const noun: string = nouns[Math.floor(Math.random() * nouns.length)];
  const number: number = Math.floor(Math.random() * 900) + 100; // generates a random 3 digit number
  return `${adj}-${noun}-${number}`;
}
