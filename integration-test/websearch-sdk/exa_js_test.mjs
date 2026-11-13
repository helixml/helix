// Drives Helix's Exa-compatible API with the unmodified exa-js SDK.
// Usage: HELIX_URL=http://localhost:8080 HELIX_API_KEY=... node exa_js_test.mjs
import Exa from "exa-js";

const exa = new Exa(process.env.HELIX_API_KEY, process.env.HELIX_URL.replace(/\/$/, "") + "/api/v1/websearch");
const failures = [];
const check = (name, cond, detail = "") => {
  console.log(`${cond ? "PASS" : "FAIL"} ${name}${detail ? " — " + detail : ""}`);
  if (!cond) failures.push(name);
};

let r = await exa.search("go generics tutorial", { numResults: 3 });
check("search", r.results.length > 0, `${r.results.length} results`);

r = await exa.search("go generics tutorial", {
  numResults: 3,
  contents: { text: { maxCharacters: 500 }, highlights: true, summary: true },
});
check("search with contents", r.results.every((x) => x.text && x.highlights?.length && x.summary));

const c = await exa.getContents(["https://go.dev/doc/tutorial/generics"], { text: true });
check("getContents", c.results.length === 1 && c.results[0].text.length > 0);

const s = await exa.findSimilar("https://go.dev/doc/tutorial/generics", { numResults: 3, excludeSourceDomain: true });
check("findSimilar", s.results.length > 0);

const a = await exa.answer("In which Go version were generics introduced?");
check("answer", String(a.answer).includes("1.18"), String(a.answer).slice(0, 80));

let text = "";
let cited = false;
for await (const chunk of exa.streamAnswer("In which Go version were generics introduced?")) {
  text += chunk.content ?? "";
  cited ||= Boolean(chunk.citations?.length);
}
check("streamAnswer", text.includes("1.18") && cited);

try {
  await exa.search("x", { numResults: 500 });
  check("invalid request rejects", false);
} catch (e) {
  check("invalid request rejects", e.statusCode === 400, e.message.slice(0, 100));
}

process.exit(failures.length ? 1 : 0);
