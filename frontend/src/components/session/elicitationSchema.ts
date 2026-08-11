/**
 * Turns an ACP elicitation's `requestedSchema` into a form description.
 *
 * This is deliberately generic. Claude Code's AskUserQuestion is the shape we care about
 * most, but the same channel carries MCP elicitation forwarding and the refusal-fallback
 * consent dialog, and other ACP agents will send shapes nobody here has seen. So nothing
 * below matches on a literal field name like `question_0`, and nothing matches on a
 * literal `_meta` key — the adapter has already renamed those once
 * (`_askUserQuestionCustomAnswer` in 0.66.0, not the `claudeCode/customAnswer` the
 * original brief described). Custom-answer fields are found by the *shape* of their
 * metadata instead, which survives another rename.
 *
 * Anything unrecognised degrades to a text input rather than throwing: a schema we can't
 * read should still let the user answer, and must never take down the conversation view.
 */

export interface ElicitationOption {
  value: string;
  label: string;
  description?: string;
  /** Longer preview content some options carry (mockups, code snippets). */
  preview?: string;
}

export type ElicitationFieldKind =
  | "select"
  | "multiselect"
  | "text"
  | "boolean"
  | "number";

export interface ElicitationField {
  /** Schema property name — the key to send back in the answer. */
  name: string;
  kind: ElicitationFieldKind;
  /** Short header, e.g. "Colour". */
  title?: string;
  /** The question text. Absent for single-question elicitations, where the prompt
   *  lives in the elicitation's `message` instead. */
  description?: string;
  options: ElicitationOption[];
  required: boolean;
  /**
   * Companion free-text field ("Other"), when the schema declares one for this field.
   * Its answer takes precedence over any selection — see buildElicitationContent.
   */
  customAnswerField?: {
    name: string;
    title?: string;
    description?: string;
  };
}

export interface ElicitationForm {
  fields: ElicitationField[];
}

type JsonRecord = Record<string, unknown>;

const isRecord = (value: unknown): value is JsonRecord =>
  typeof value === "object" && value !== null && !Array.isArray(value);

const asString = (value: unknown): string | undefined =>
  typeof value === "string" && value.length > 0 ? value : undefined;

/**
 * Finds the field this property is the "Other" box for.
 *
 * Matches on metadata shape, not on the metadata key's name: any `_meta` entry that
 * either sets `isCustomAnswer` or names a sibling property via `questionId` counts. The
 * adapter namespaces this key and has changed it before, so keying off the name would
 * silently break the "none of the above" path on the next rename.
 */
const customAnswerTargetOf = (property: JsonRecord): string | undefined => {
  const meta = property._meta;
  if (!isRecord(meta)) return undefined;

  for (const entry of Object.values(meta)) {
    if (!isRecord(entry)) continue;
    const questionId = asString(entry.questionId);
    if (entry.isCustomAnswer === true || questionId) {
      return questionId;
    }
  }
  return undefined;
};

/** Pulls the option preview out of `_meta`, again by shape rather than by key name. */
const previewOf = (option: JsonRecord): string | undefined => {
  const meta = option._meta;
  if (!isRecord(meta)) return undefined;
  for (const entry of Object.values(meta)) {
    if (isRecord(entry)) {
      const preview = asString(entry.preview);
      if (preview) return preview;
    }
  }
  return undefined;
};

const parseOptions = (raw: unknown): ElicitationOption[] => {
  if (!Array.isArray(raw)) return [];
  const options: ElicitationOption[] = [];
  for (const item of raw) {
    if (!isRecord(item)) continue;
    // `const` is the value the agent records as the answer; `title` is what to show.
    const value =
      typeof item.const === "string"
        ? item.const
        : typeof item.const === "number" || typeof item.const === "boolean"
          ? String(item.const)
          : asString(item.title);
    if (value === undefined) continue;
    options.push({
      value,
      label: asString(item.title) ?? value,
      description: asString(item.description),
      preview: previewOf(item),
    });
  }
  return options;
};

const kindOf = (property: JsonRecord, options: ElicitationOption[]): ElicitationFieldKind => {
  const type = asString(property.type);
  if (type === "array") return "multiselect";
  if (options.length > 0) return "select";
  if (type === "boolean") return "boolean";
  if (type === "number" || type === "integer") return "number";
  return "text";
};

/**
 * Parses a `requestedSchema` into an ordered list of fields.
 *
 * Returns an empty form rather than throwing for anything malformed — a question we
 * cannot parse should degrade, not break the page.
 */
export function parseElicitationSchema(schema: unknown): ElicitationForm {
  if (!isRecord(schema)) return { fields: [] };
  const properties = schema.properties;
  if (!isRecord(properties)) return { fields: [] };

  const required = Array.isArray(schema.required)
    ? schema.required.filter((name): name is string => typeof name === "string")
    : [];

  // First pass: identify the "Other" boxes so they are attached to their parent rather
  // than rendered as separate questions.
  const customAnswerFields = new Map<string, string>();
  for (const [name, raw] of Object.entries(properties)) {
    if (!isRecord(raw)) continue;
    const target = customAnswerTargetOf(raw);
    if (target && target !== name) {
      customAnswerFields.set(target, name);
    }
  }

  const fields: ElicitationField[] = [];
  for (const [name, raw] of Object.entries(properties)) {
    if (!isRecord(raw)) continue;
    // Skip the companions; they are rendered inside their parent field.
    if (customAnswerTargetOf(raw)) continue;

    // Single-select puts options in `oneOf`; multi-select nests them in `items.anyOf`.
    const items = isRecord(raw.items) ? raw.items : undefined;
    const options = parseOptions(raw.oneOf ?? raw.enum ?? items?.anyOf ?? items?.oneOf);

    const field: ElicitationField = {
      name,
      kind: kindOf(raw, options),
      title: asString(raw.title),
      description: asString(raw.description),
      options,
      required: required.includes(name),
    };

    const customName = customAnswerFields.get(name);
    if (customName) {
      const customRaw = properties[customName];
      field.customAnswerField = {
        name: customName,
        title: isRecord(customRaw) ? asString(customRaw.title) : undefined,
        description: isRecord(customRaw) ? asString(customRaw.description) : undefined,
      };
    }

    fields.push(field);
  }

  return { fields };
}

export type ElicitationAnswers = Record<string, string | string[] | boolean | number>;

export interface ElicitationDraft {
  /** Selections keyed by field name. */
  values: Record<string, string | string[] | boolean>;
  /** Free-text answers keyed by the *parent* field name. */
  customValues: Record<string, string>;
}

/**
 * Builds the payload to send back, mirroring the adapter's own folding rules
 * (`applyAskElicitationResponse` in claude-agent-acp 0.66.0):
 *
 *   - A custom answer that is non-empty after trimming wins over that field's selection,
 *     and the selection is not sent at all. Submitting *only* a custom answer is
 *     therefore valid — that is the "none of the above" case.
 *   - A field with neither a selection nor a custom answer is omitted entirely. Partial
 *     answers are legal; nothing in these schemas is marked required.
 *
 * Getting this wrong is silent: the agent would just receive the wrong answer.
 */
export function buildElicitationContent(
  form: ElicitationForm,
  draft: ElicitationDraft,
): ElicitationAnswers {
  const content: ElicitationAnswers = {};

  for (const field of form.fields) {
    const custom = draft.customValues[field.name];
    if (field.customAnswerField && typeof custom === "string" && custom.trim() !== "") {
      content[field.customAnswerField.name] = custom.trim();
      continue;
    }

    const value = draft.values[field.name];
    if (value === undefined || value === null) continue;

    if (Array.isArray(value)) {
      if (value.length > 0) content[field.name] = value;
      continue;
    }
    if (typeof value === "string") {
      if (value.trim() !== "") content[field.name] = value;
      continue;
    }
    if (typeof value === "boolean") {
      content[field.name] = value;
    }
  }

  return content;
}

/** True when the user has entered enough to submit — any answer at all, or a schema
 *  with required fields that are all satisfied. */
export function canSubmitElicitation(
  form: ElicitationForm,
  draft: ElicitationDraft,
): boolean {
  const content = buildElicitationContent(form, draft);
  const requiredFields = form.fields.filter((field) => field.required);
  if (requiredFields.length > 0) {
    return requiredFields.every(
      (field) =>
        content[field.name] !== undefined ||
        (field.customAnswerField && content[field.customAnswerField.name] !== undefined),
    );
  }
  return Object.keys(content).length > 0;
}

/** Human-readable summary of what was submitted, for the answered card. */
export function summariseAnswers(
  form: ElicitationForm,
  content: ElicitationAnswers,
): Array<{ question: string; answer: string }> {
  const summary: Array<{ question: string; answer: string }> = [];
  for (const field of form.fields) {
    const custom = field.customAnswerField
      ? content[field.customAnswerField.name]
      : undefined;
    const raw = custom !== undefined ? custom : content[field.name];
    if (raw === undefined) continue;
    const answer = Array.isArray(raw) ? raw.join(", ") : String(raw);
    if (answer === "") continue;
    summary.push({
      question: field.title || field.description || field.name,
      answer,
    });
  }
  return summary;
}
