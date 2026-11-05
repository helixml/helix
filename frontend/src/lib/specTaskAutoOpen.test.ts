import { beforeEach, describe, expect, it } from "vitest";

import {
  addAutoOpenedSpecTask,
  AUTO_OPENED_KEY,
  getAutoOpenedSpecTasks,
} from "./specTaskAutoOpen";

describe("spec task review auto-open storage", () => {
  beforeEach(() => {
    sessionStorage.clear();
  });

  it("records each task once", () => {
    addAutoOpenedSpecTask("task-1");
    addAutoOpenedSpecTask("task-1");

    expect(getAutoOpenedSpecTasks()).toEqual(new Set(["task-1"]));
    expect(JSON.parse(sessionStorage.getItem(AUTO_OPENED_KEY) || "[]")).toEqual([
      "task-1",
    ]);
  });
});
