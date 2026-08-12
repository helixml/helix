import { describe, expect, it } from "vitest";
import {
  buildElicitationContent,
  canSubmitElicitation,
  parseElicitationSchema,
  summariseAnswers,
} from "./elicitationSchema";

/**
 * These fixtures are the real output of claude-agent-acp 0.66.0's
 * `askUserQuestionsToCreateRequest`, including its actual `_meta` key names
 * (`_askUserQuestionCustomAnswer`, `_claude/askUserQuestionOption`). They are not the
 * names the original task brief quoted — the adapter renamed them — which is exactly why
 * the parser matches on metadata shape instead.
 */
const customAnswerProperty = (index: number) => ({
  type: "string",
  title: "Other",
  description: "Type your own answer instead of choosing an option above (optional).",
  _meta: {
    _askUserQuestionCustomAnswer: {
      questionId: `question_${index}`,
      isCustomAnswer: true,
    },
  },
});

const singleQuestionSchema = {
  type: "object",
  properties: {
    question_0: {
      type: "string",
      title: "Colour",
      oneOf: [
        { const: "Red", title: "Red", description: "Warm and loud" },
        { const: "Blue", title: "Blue", description: "Calm and cool" },
        { const: "Green", title: "Green" },
      ],
    },
    question_0_custom: customAnswerProperty(0),
  },
};

const multiSelectSchema = {
  type: "object",
  properties: {
    question_0: {
      type: "array",
      title: "Features",
      description: "Which features should ship?",
      items: {
        anyOf: [
          { const: "auth", title: "Auth", description: "Login and sessions" },
          { const: "billing", title: "Billing" },
        ],
      },
    },
    question_0_custom: customAnswerProperty(0),
  },
};

describe("parseElicitationSchema", () => {
  it("parses a single question with its options and custom-answer companion", () => {
    const form = parseElicitationSchema(singleQuestionSchema);

    expect(form.fields).toHaveLength(1);
    const [field] = form.fields;
    expect(field.name).toBe("question_0");
    expect(field.kind).toBe("select");
    expect(field.title).toBe("Colour");
    // With one question the prompt lives in the elicitation message, not here.
    expect(field.description).toBeUndefined();
    expect(field.options.map((o) => o.value)).toEqual(["Red", "Blue", "Green"]);
    expect(field.options[0].description).toBe("Warm and loud");
    expect(field.customAnswerField?.name).toBe("question_0_custom");
  });

  it("does not render the custom-answer companion as its own question", () => {
    const form = parseElicitationSchema(singleQuestionSchema);
    expect(form.fields.map((f) => f.name)).not.toContain("question_0_custom");
  });

  it("parses multi-select questions from items.anyOf", () => {
    const form = parseElicitationSchema(multiSelectSchema);
    expect(form.fields).toHaveLength(1);
    expect(form.fields[0].kind).toBe("multiselect");
    expect(form.fields[0].options.map((o) => o.value)).toEqual(["auth", "billing"]);
    expect(form.fields[0].description).toBe("Which features should ship?");
  });

  it("parses all four questions when several are asked at once", () => {
    const properties: Record<string, unknown> = {};
    for (let i = 0; i < 4; i += 1) {
      properties[`question_${i}`] = {
        type: "string",
        title: `H${i}`,
        description: `Question ${i}?`,
        oneOf: [{ const: "Yes", title: "Yes" }],
      };
      properties[`question_${i}_custom`] = customAnswerProperty(i);
    }
    const form = parseElicitationSchema({ type: "object", properties });

    expect(form.fields).toHaveLength(4);
    expect(form.fields.map((f) => f.name)).toEqual([
      "question_0",
      "question_1",
      "question_2",
      "question_3",
    ]);
    expect(form.fields[2].customAnswerField?.name).toBe("question_2_custom");
  });

  it("finds custom-answer fields by metadata shape, not by key name", () => {
    // A future adapter renames the _meta key again. The link must still be found.
    const form = parseElicitationSchema({
      type: "object",
      properties: {
        question_0: { type: "string", oneOf: [{ const: "A", title: "A" }] },
        question_0_custom: {
          type: "string",
          title: "Other",
          _meta: {
            "some/entirely-new-key": { questionId: "question_0", isCustomAnswer: true },
          },
        },
      },
    });

    expect(form.fields).toHaveLength(1);
    expect(form.fields[0].customAnswerField?.name).toBe("question_0_custom");
  });

  it("carries an option preview through", () => {
    const form = parseElicitationSchema({
      type: "object",
      properties: {
        question_0: {
          type: "string",
          oneOf: [
            {
              const: "A",
              title: "A",
              _meta: { "_claude/askUserQuestionOption": { preview: "some mockup" } },
            },
          ],
        },
      },
    });
    expect(form.fields[0].options[0].preview).toBe("some mockup");
  });

  it("degrades unknown shapes to plain inputs rather than throwing", () => {
    // MCP forwarding and the refusal-fallback consent dialog send other shapes.
    const form = parseElicitationSchema({
      type: "object",
      properties: {
        note: { type: "string", title: "Note" },
        confirm: { type: "boolean", title: "Are you sure?" },
        count: { type: "integer", title: "How many" },
        weird: { type: "hyperdimensional-vector" },
      },
    });

    expect(form.fields.map((f) => f.kind)).toEqual([
      "text",
      "boolean",
      "number",
      "text",
    ]);
  });

  it("returns an empty form for junk instead of throwing", () => {
    expect(parseElicitationSchema(null).fields).toEqual([]);
    expect(parseElicitationSchema("nope").fields).toEqual([]);
    expect(parseElicitationSchema({}).fields).toEqual([]);
    expect(parseElicitationSchema({ properties: 42 }).fields).toEqual([]);
  });

  it("marks required fields when the schema declares them", () => {
    const form = parseElicitationSchema({
      type: "object",
      required: ["question_0"],
      properties: { question_0: { type: "string" } },
    });
    expect(form.fields[0].required).toBe(true);
  });
});

describe("buildElicitationContent", () => {
  const form = parseElicitationSchema(singleQuestionSchema);

  it("sends the selected option", () => {
    const content = buildElicitationContent(form, {
      values: { question_0: "Red" },
      customValues: {},
    });
    expect(content).toEqual({ question_0: "Red" });
  });

  it("lets a custom answer win over the selection, and omits the selection entirely", () => {
    // Mirrors applyAskElicitationResponse: a non-empty trimmed custom answer means the
    // user typed their own answer instead of picking, so the selection is not sent.
    const content = buildElicitationContent(form, {
      values: { question_0: "Red" },
      customValues: { question_0: "  Purple  " },
    });
    expect(content).toEqual({ question_0_custom: "Purple" });
    expect(content.question_0).toBeUndefined();
  });

  it("accepts a custom answer alone — the 'none of the above' case", () => {
    const content = buildElicitationContent(form, {
      values: {},
      customValues: { question_0: "Chartreuse" },
    });
    expect(content).toEqual({ question_0_custom: "Chartreuse" });
  });

  it("ignores a whitespace-only custom answer and falls back to the selection", () => {
    const content = buildElicitationContent(form, {
      values: { question_0: "Blue" },
      customValues: { question_0: "   " },
    });
    expect(content).toEqual({ question_0: "Blue" });
  });

  it("omits questions with neither a selection nor a custom answer", () => {
    expect(buildElicitationContent(form, { values: {}, customValues: {} })).toEqual({});
  });

  it("sends multi-select answers as an array, and omits an empty one", () => {
    const multi = parseElicitationSchema(multiSelectSchema);
    expect(
      buildElicitationContent(multi, {
        values: { question_0: ["auth", "billing"] },
        customValues: {},
      }),
    ).toEqual({ question_0: ["auth", "billing"] });

    expect(
      buildElicitationContent(multi, { values: { question_0: [] }, customValues: {} }),
    ).toEqual({});
  });
});

describe("canSubmitElicitation", () => {
  const form = parseElicitationSchema(singleQuestionSchema);

  it("requires at least one answer when nothing is mandatory", () => {
    expect(canSubmitElicitation(form, { values: {}, customValues: {} })).toBe(false);
    expect(
      canSubmitElicitation(form, { values: { question_0: "Red" }, customValues: {} }),
    ).toBe(true);
    expect(
      canSubmitElicitation(form, { values: {}, customValues: { question_0: "Teal" } }),
    ).toBe(true);
  });

  it("accepts a required field satisfied by its custom answer", () => {
    const required = parseElicitationSchema({
      type: "object",
      required: ["question_0"],
      properties: {
        question_0: { type: "string", oneOf: [{ const: "A", title: "A" }] },
        question_0_custom: customAnswerProperty(0),
      },
    });
    expect(
      canSubmitElicitation(required, { values: {}, customValues: { question_0: "B" } }),
    ).toBe(true);
  });
});

describe("summariseAnswers", () => {
  it("shows what the user chose, including custom answers", () => {
    const form = parseElicitationSchema(singleQuestionSchema);
    expect(summariseAnswers(form, { question_0: "Red" })).toEqual([
      { question: "Colour", answer: "Red" },
    ]);
    expect(summariseAnswers(form, { question_0_custom: "Puce" })).toEqual([
      { question: "Colour", answer: "Puce" },
    ]);
  });

  it("joins multi-select answers", () => {
    const multi = parseElicitationSchema(multiSelectSchema);
    expect(summariseAnswers(multi, { question_0: ["auth", "billing"] })).toEqual([
      { question: "Features", answer: "auth, billing" },
    ]);
  });
});
