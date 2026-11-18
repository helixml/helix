import { describe, expect, it } from "vitest";

import { allowedUrlView } from "./SpecTaskDetailContent";

describe("allowedUrlView", () => {
  it("selects known views from the URL", () => {
    expect(allowedUrlView("usage", false)).toBe("usage");
    expect(allowedUrlView("details", false)).toBe("details");
    expect(allowedUrlView("desktop", true)).toBe("desktop");
  });

  it("ignores unknown or missing views", () => {
    expect(allowedUrlView("bogus", false)).toBeUndefined();
    expect(allowedUrlView(undefined, false)).toBeUndefined();
  });

  // A restricted embed (?controls=0) hides the Usage tab; the URL must not
  // reopen it, since it shows spend.
  it("refuses the usage view in a restricted embed", () => {
    expect(allowedUrlView("usage", true)).toBeUndefined();
  });
});
