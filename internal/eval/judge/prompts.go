package judge

// Build #31 — Eval system LLM-as-judge prompts.
//
// The judge prompts are versioned; Build #31 ships v1. The eval_run row
// pins the version in `judge_prompt_version` so future changes can be
// compared against a stable baseline.
//
// Style notes:
//   - The prompts ask for a single 1–5 score per dimension (integer
//     only) so the JSON parser never has to deal with floats, ranges,
//     or free-form text. This matches the EvalRunSummary fields which
//     are float64 averages.
//   - The prompts explicitly forbid the judge from inventing facts the
//     expected answer does not contain. The factuality dimension in
//     particular penalises a model that "fluently fills in" gaps; that
//     failure mode shows up as citation_fidelity < factuality, which
//     is exactly the badcase shape we want to surface.

const judgePromptVersion = "v1"

// factualityPromptTemplate asks the LLM judge whether the model answer
// is consistent with the expected answer. The expected answer is the
// ground truth; the model answer is what WeKnora actually returned.
//
// The output is JSON `{"score": <1-5>, "rationale": "..."}`. The judge
// prompt forbids partial credit beyond an integer bucket so the
// downstream averaging stays clean.
const factualityPromptTemplate = `You are evaluating a RAG answer for factual correctness.

Question:
%s

Expected answer (ground truth):
%s

Model answer:
%s

Score the model answer for factuality on an integer scale of 1-5:
- 5: every claim is supported by the expected answer.
- 4: all key claims supported; minor restatements only.
- 3: most key claims supported, but at least one is missing or hedged.
- 2: at least one key claim contradicts the expected answer.
- 1: contradicts multiple key claims, or invents unsupported facts.

Output JSON ONLY in this exact shape (no prose, no markdown):
{"score": <integer 1-5>, "rationale": "<one short sentence>"}`

// Build #31 judge prompt v1.

// citationFidelityPromptTemplate asks the LLM judge whether every
// [[cite:N]] token in the model answer points at a passage that is
// actually relevant to the cited statement. This is the LLM-augmented
// half of the citation_fidelity dimension; the heuristic half (in
// citation_fidelity.go) verifies the [[cite:N]] indices are
// well-formed and stay inside the citation_index array.
const citationFidelityPromptTemplate = `You are evaluating whether a RAG answer's citations actually back the statements they are attached to.

Question:
%s

Citation index (each entry is a passage retrieved by the system, indexed by [[cite:N]]):
%s

Model answer (with [[cite:N]] tokens inline):
%s

For each [[cite:N]] token in the model answer, decide whether the cited passage supports the surrounding statement. Score 1-5:
- 5: every citation supports its statement.
- 4: almost all citations support; one is weakly related.
- 3: most citations support; one or two do not.
- 2: at least one citation clearly contradicts or is unrelated.
- 1: multiple citations are unrelated, or the answer fabricates citations.

Output JSON ONLY in this exact shape (no prose, no markdown):
{"score": <integer 1-5>, "rationale": "<one short sentence>"}`

// Build #31 judge prompt v1.

// reflectionNecessityPromptTemplate asks the LLM judge whether the
// original answer (before reflection) would have benefited from a
// reflection pass. This is the LLM-augmented half of the
// reflection_necessity dimension; the heuristic half verifies the
// reflection_events JSON is non-empty and well-formed when the score
// is low (so the audit row explains why a low score did not trigger
// reflection).
const reflectionNecessityPromptTemplate = `You are evaluating whether a chat answer would have benefited from a reflection pass before being sent to the user.

Question:
%s

Original answer (before reflection):
%s

Reflection events recorded for this turn (empty if the pipeline did not reflect):
%s

Decide whether reflection would have helped and score 1-5:
- 5: the original answer is already correct and complete; reflection would have been wasteful.
- 4: minor stylistic polish could help, but no factual issue.
- 3: a reflection pass would have caught a small issue.
- 2: reflection would have caught a meaningful issue.
- 1: reflection would have prevented a user-facing mistake.

Output JSON ONLY in this exact shape (no prose, no markdown):
{"score": <integer 1-5>, "rationale": "<one short sentence>"}`

// Build #31 judge prompt v1.

// PromptVersion returns the version string stamped into every
// eval_runs.judge_prompt_version row. Bumping this in a future PR
// changes the version; the harness pins `judge_prompt_version` so the
// two PRs can be compared apples-to-apples.
func PromptVersion() string { return judgePromptVersion }
