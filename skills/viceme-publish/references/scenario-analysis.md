# Scenario analysis and experience compilation

Run this analysis for every `SERVICE` Work before compiling any Product or
service Interaction. Recruitment and `FULFILLMENT_ONLY` are included. Do not select a
workflow from an industry name. Derive a candidate from the outcome, available
input sources, actor handoffs, decisions, transaction boundary, and failure
paths described by the creator.

## Analyze before asking for fields

Establish in business language:

- the participant's intended outcome and each valid terminal outcome;
- the durable source material or context already available;
- which values may be derived and which require explicit human input;
- when derived values require review or correction;
- participant, creator, and executor responsibilities;
- synchronous, asynchronous, iterative, approval, and waiting boundaries;
- transaction facts required before Quote or payment versus information that
  belongs inside the Interaction;
- cancellation, rejection, retry, unavailable-capability, and recovery paths;
- field sensitivity, source provenance, audience, and retention;
- each actor handoff that warrants a notification.

Do not infer price, payment state, actor authority, terminal decisions,
sensitive values, or unsupported automatic execution. A derivation or external
operation must name its executor as the current participant Agent, creator
Agent, a human actor, or an actually registered Adapter.

## Explain one recommended experience candidate

Before requesting creator decisions, summarize the proposed Skill experience in
natural language. Cover only facts that materially explain this Skill, such as:

1. the understood outcome and entry mode;
2. the preferred source-first acquisition strategy;
3. an ordered participant/creator/executor journey;
4. values collected directly, derived, reviewed, or deferred;
5. decision and handoff points;
6. normal, cancellation, and failure outcomes;
7. assumptions and unavailable capabilities;
8. only the unresolved business decisions that materially change the flow.

Prefer progressive disclosure: use an available durable source before asking
the participant to repeat values that can be derived, require confirmation when
the Definition says so, and ask only for values still missing when they become
required. Never treat a derived value as a creator decision or verified
external result.

Do not expose the underlying JSON, schema names, enum values, IDs, revisions,
digests, or review codes in the normal creator conversation. Keep those values
in private task state for deterministic CLI calls. Translate roles, entry
modes, data audiences, failure paths, and execution boundaries into the words a
creator would use to describe their service. Show technical details only when
the creator explicitly asks for them or when a precise error cannot otherwise
be explained.

Then compare that candidate with the creator's source. Identify only points the
published Skill has not already considered: missing outcomes, ambiguous actor
responsibility, information with no source or audience, unsupported execution,
unhandled failure or cancellation, missing completion evidence, or another
scenario-specific gap. These are examples for analysis, not required categories.
Do not create a finding for a platform invariant that Shop already enforces, a
fact explicitly settled by the source, or an empty category.

## Obtain one interactive user confirmation

After preparing the recommendation, run `viceme merchant contract show
analysis-create`, create a private strict JSON file from its complete example,
and run `viceme merchant contract validate analysis-create --input <file>`.
Only after validation succeeds, run `viceme merchant work analysis create
--input <file>`. The input contains
`workId`, `merchantAccountId`, `sourceType`, and `analysis`. Put each actual
Skill-specific omission, ambiguity, risk, or unresolved assumption in
`confirmationItems` with a unique descriptive code, natural-language title, and
concrete finding. The list is dynamic and may be empty; never add generic items
to reach a predetermined count. Put only genuine business choices in
`openDecisions`.

Keep IDs, digests, codes, and schemas from the returned payload private, but do
not keep its business conclusions hidden. Obtain confirmation as follows:

1. Resolve every open business decision through ordinary, natural-language
   questions before asking for confirmation. These answers collect missing
   business facts; they are not separate analysis confirmations.
2. Display the complete natural-language analysis overview before any approval
   prompt. Explain the proposed experience and then clearly distinguish what
   the source already covers from what the analysis found missing or ambiguous.
   Do not render empty generic sections.
3. Present every returned Skill-specific finding separately and in the returned
   order. Do not merge findings or concatenate their contents into one
   paragraph. Every entry must show both a natural-language title and the
   concrete proposed conclusion the creator is being asked to approve. A title
   such as “Data and audience” without its conclusion is not reviewable.
4. Give every entry exactly two mutually exclusive decisions: “Confirm” and
   “Needs changes”. Do not use a bare checkbox whose unchecked state could mean
   either rejection or no answer. When “Needs changes” is selected, collect a
   comment for that item. “Other notes” may be an optional text field, but must
   not appear as another confirmation item.
5. Prefer one form, checklist-like review, or equivalent structured interaction
   that shows the dynamic findings and submits the complete review once. Let the
   Agent choose the control best supported by its platform; do not prescribe
   cards, tabs, or another fixed UI primitive. Never preselect or submit an
   answer for the creator.
6. If the platform has no suitable structured control, show a numbered list in
   which every number includes its concrete conclusion. Ask the creator either
   to confirm all findings or to name the numbers that need changes and explain why.
   Do not present category names without their conclusions.
7. Ask for one explicit submission covering the complete list. Do not force the
   creator through one sequential chat turn per finding.
8. Require a new, explicit user response. The original publication request, an
   earlier broad approval, silence, a tool result, or the Agent's own assessment
   is not confirmation.
9. If the creator requests a change, revise the recommendation and create a new
   server analysis. Present the changed complete analysis and ask once for its
   confirmation; a response to an older digest never confirms a changed one.
10. After a positive response covering all listed items, immediately persist the
   confirmation and continue to compilation. Do not ask whether to proceed,
   repeat the same confirmation, or split the dynamic acknowledgments into
   additional user prompts.

If `confirmationItems` is empty, say that the analysis found no unaddressed
points, show the proposed experience, and ask once whether to continue with that
exact analysis. Do not invent a review item merely to populate a control.

Never replace the visible analysis with a generic prompt such as “Do you agree
with the scenario analysis and confirmation items?” or “Confirm analysis and
continue”. Such a response exposes no reviewable conclusion and is not user
confirmation. Review one Work's analysis at a time; when publishing several
Works, present and confirm each complete analysis separately rather than asking
for one combined approval.

If the conversation is interrupted before the user answers, recover the same
analysis with `analysis show`, present the same separately listed review items,
and ask once for confirmation. If Shop already reports `status: CONFIRMED`, do
not ask again for that analysis.

## Persist the completed user review

Only after the single interactive confirmation is complete, run:

```text
viceme merchant work analysis confirm <work-id> \
  --merchant <merchant-account-id> \
  --analysis <analysis-id> \
  --digest <analysis-digest> \
  [--resolution <decision-code>=<confirmed-value> ...]
```

The CLI reads the exact current analysis, rejects a changed digest, validates
every open-decision resolution, and generates acknowledgments for the exact
dynamic findings from Shop's response. Do not hand-write a confirmation JSON file or call the
legacy `--input` mode in a new workflow. The single API call persists the
complete user-confirmed analysis; automatic construction of protocol fields
does not authorize the Agent to manufacture the user's confirmation. Do not
create an Interaction Draft while `requiresUserConfirmation` is true.

The initial request to publish is not this confirmation. The later exact
Definition/Product preview still requires its separate activation
confirmation.

## Compile to scenario-neutral primitives

Only after Shop returns `status: CONFIRMED`, encode that exact confirmed
recommendation in `definition.experience`:

- `summary`: concise business outcome, not Agent instructions;
- `assumptions`: confirmed limitations or fallbacks;
- `fields`: stable keys, labels, allowed and preferred sources, `requiredAt`,
  confirmation policy, sensitivity, and audience;
- `steps`: an ordered experience using only `CREATE_INSTANCE`,
  `COLLECT_INPUT`, `COLLECT_ARTIFACT`, `DERIVE_DATA`, `REVIEW_DATA`,
  `EXECUTE_ACTION`, `WAIT_FOR_ROLE`, and `COMPLETE_TASK`.

An Interaction has exactly one `CREATE_INSTANCE` step. Artifact upload and all
server actions are instance-scoped and therefore follow that step. Input may be
collected, derived, and reviewed before creation only when it does not require
an instance-scoped artifact. State and Action definitions remain the runtime
authority for branching; the ordered experience is the recommended journey.

For a task created by an Action, set `task.create.actionCode` to the separate
Action that the assigned role will execute. That Action must be authorized for
the role in the resulting state. Use `resolveCurrent` on the completion Action
to complete, cancel, or retain the open Task.

## Semantic review

Reject a candidate when a state is unreachable, no terminal state is reachable,
a task has no authorized completion Action, a field is required before any step
can acquire it, an actor handoff lacks a recovery path, an executor capability
is unavailable, checkout duplicates non-transaction intake, or sensitive data
has no explicit audience. Preview every role's normal path and the declared
exception paths before requesting activation confirmation.
