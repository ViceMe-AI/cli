# Scenario analysis and experience compilation

Run this analysis before compiling any service Interaction. Do not select a
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

## Show one recommended experience candidate

Before requesting creator decisions, show:

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

## Compile to scenario-neutral primitives

Encode the confirmed recommendation in `definition.experience`:

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
