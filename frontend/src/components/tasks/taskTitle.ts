import { TypesSpecTask } from "../../api/api";

type TitleFields = Pick<
  TypesSpecTask,
  "user_short_title" | "short_title" | "name"
>;

export const specTaskTitle = (
  task: TitleFields | null | undefined,
  fallback: string = "Untitled task",
): string =>
  task?.user_short_title ||
  task?.short_title ||
  task?.name ||
  fallback;
