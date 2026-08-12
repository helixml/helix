declare module "@pierre/diffs/editor" {
  interface EditorFile {
    name: string;
    contents: string;
    cacheKey?: string;
  }

  interface EditorOptions<LAnnotation> {
    persistState?: boolean;
    persistStateStorage?: "inMemory" | "localStorage";
    onChange?: (
      file: EditorFile,
      lineAnnotations?: import("@pierre/diffs").LineAnnotation<LAnnotation>[],
    ) => void;
  }

  export class Editor<LAnnotation = undefined> {
    constructor(options?: EditorOptions<LAnnotation>);
    cleanUp(recycle?: boolean): void;
  }
}
