import { describe, expect, it } from "vitest";
import { reconcileWorkspaceFileSave } from "./workspaceFileSaveState";

describe("workspace file save reconciliation", () => {
  it("accepts the server-confirmed contents when editing stayed idle", () => {
    expect(reconcileWorkspaceFileSave("submitted", "submitted", "confirmed"))
      .toEqual({ contents: "confirmed", savedContents: "confirmed" });
  });

  it("does not overwrite changes typed while the save was pending", () => {
    expect(reconcileWorkspaceFileSave("newer edit", "submitted", "confirmed"))
      .toEqual({ contents: "newer edit", savedContents: "confirmed" });
  });
});
