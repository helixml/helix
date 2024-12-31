export const delay = (ms: number): Promise<void> => {
  return new Promise(resolve => setTimeout(resolve, ms));
};

export const map = <T, U>(items: Promise<T>[] | T[], mapper: (item: T) => Promise<U> | U): Promise<U[]> => {
    return Promise.all(items.map(async (item) => mapper(await item)));
};