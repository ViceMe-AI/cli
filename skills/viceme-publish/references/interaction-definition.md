# Interaction Definition publication

Use this workflow when a Work needs a structured direct or purchased process.
The Agent compiles natural language or Markdown into deterministic JSON; the
CLI and Shop API only validate, preview, freeze, and persist it. Never ask the
creator to write `SKILL.md`.

Choose the publication entry from business facts:

- `DIRECT` means no Quote, Order, price, or payment. Activating the confirmed
  Definition also publishes the reviewed Work revision when it is the Work's
  first release, and generates a signed Work-bound Interaction Skill.
- `PURCHASE` means the Product/Commerce workflow owns price and payment. The
  paid Order creates the same kind of Interaction instance after settlement;
  do not create a second direct instance for that purchase.
- A Definition may allow both modes when the creator intentionally supports
  both entry paths. The generated Interaction Skill uses only `DIRECT`; the
  generated Purchase Skill uses only the Product/Commerce path.

## Facts to establish

Before creating the draft, establish and show:

- the existing `workId` and its owning `merchantAccountId`;
- entry modes (`DIRECT`, `PURCHASE`, or `INVITATION`);
- participant, creator, and optional executor roles;
- initial input JSON Schema and its sensitive fields;
- every state, action, allowed actor role, source state, target state, and
  terminal state;
- Task and Record schemas plus their audiences;
- each notification-worthy instance event or transition, its optional Action
  and state filters, recipient roles, actor exclusion, and scenario-specific
  title and body;
- a Default Presentation view for every state;
- optional Vibe origin, view paths, compatibility versions, and fallback.

Ask concise questions for missing business facts. Do not infer sensitive input,
prices, payment results, actor permissions, audience, or terminal decisions.
`PURCHASE` controls only the instance entry condition; Product price and Order
authority remain in Commerce.

## Strict draft input

Create a private temporary JSON file with this envelope:

```json
{
  "workId": "uuid",
  "merchantAccountId": "uuid",
  "sourceType": "NATURAL_LANGUAGE",
  "definition": {
    "schemaVersion": 1,
    "entryModes": ["DIRECT"],
    "roles": [
      {"code": "APPLICANT", "label": "Applicant", "kind": "PARTICIPANT"},
      {"code": "OWNER", "label": "Creator", "kind": "CREATOR"}
    ],
    "initialInput": {"type": "object", "additionalProperties": false},
    "states": [
      {"code": "OPEN", "label": "Open", "terminal": false},
      {"code": "DONE", "label": "Done", "terminal": true}
    ],
    "initialState": "OPEN",
    "actions": [{
      "code": "SUBMIT",
      "label": "Submit",
      "actorRoles": ["APPLICANT"],
      "fromStates": ["OPEN"],
      "toState": "DONE",
      "inputSchema": {"type": "object", "additionalProperties": false},
      "audience": ["PARTICIPANT", "CREATOR"],
      "idempotencyRequired": true,
      "notificationEvent": "INSTANCE_COMPLETED"
    }],
    "recordTypes": [],
    "notificationRules": [
      {
        "code": "APPLICATION_SUBMITTED",
        "event": "INSTANCE_CREATED",
        "actionCodes": [],
        "fromStates": [],
        "toStates": ["OPEN"],
        "recipientRoles": ["OWNER"],
        "excludeActor": true,
        "content": {
          "title": "收到新的申请",
          "body": "请进入流程实例查看并处理。"
        }
      }
    ],
    "dataPolicies": [],
    "presentation": {
      "views": [{"key": "form", "title": "Submit", "blocks": [{"kind": "form"}]}],
      "stateViews": {"OPEN": "form", "DONE": "form"}
    }
  }
}
```

`notificationRules` is the only scenario authority for sending Interaction
status notifications. Match rules with generic events (`INSTANCE_CREATED`,
`TASK_CREATED`, `TASK_SUBMITTED`, `STATE_CHANGED`, `MESSAGE_CREATED`, or
`INSTANCE_COMPLETED`) plus optional `actionCodes`, `fromStates`, and `toStates`.
An empty filter means “any”; every referenced Action, state, and recipient role
must exist in the same Definition. Keep notification copy specific to the
business event, but do not encode scenario names in CLI or Shop runtime logic.

For example, an application Definition can notify its creator role when the
instance is created, then notify its applicant role when a screening Action
creates an answer Task. Do not leave `notificationRules` empty merely because
the CLI has no built-in template; use an empty array only when the creator has
confirmed that the process needs no status notification. Channel selection is
not part of the Definition: Shop applies the user's notification preference and
tries WeChat service account, verified email, then verified phone/SMS.

Run `viceme merchant work draft create --input <file>`, then delete the
temporary file. Reuse the returned Draft ID, revision, digest, Default
Presentation views, HTML and Markdown previews, frozen Agent Skill identity,
and optional Vibe preview. Run `viceme merchant work
draft show <work-id> --merchant <merchant-id>` to recover a lost response.

## Preview and activation

Run `viceme merchant work preview create <work-id> --merchant <merchant-id>`
without `--expected-revision` to preview the current Interaction candidate.
(`--expected-revision` retains the separate legacy HTML/Markdown Work preview.)

Display the public Work HTML/Markdown and every fact listed above. Activation
requires explicit confirmation of the exact candidate digest. After
confirmation, write a private temporary file:

```json
{
  "merchantAccountId": "uuid",
  "expectedDraftRevision": 1,
  "candidateDigest": "64 lowercase hexadecimal characters"
}
```

Run `viceme merchant work activate <work-id> --input <file>` and delete the
file. Any change creates a new revision and digest and invalidates the earlier
confirmation. If Shop reports `VIBE_UI_ACTIVATION_DISABLED`, keep the Vibe
candidate as a local preview; never claim it was activated.

For a Definition containing `DIRECT`, activation returns the generated Agent
Skill `stableName`. Report its install command as:

```text
viceme interaction skill install <stable-name> --agent auto
```

The installed Skill starts with `viceme interaction flow start --skill
<stable-name>`. The Runtime returns the frozen `initialInput` JSON Schema and a
field guide. The Agent asks only for missing scenario fields, confirms the
completed values once, then calls `viceme interaction flow create --skill
<stable-name> --idempotency-key <createIdempotencyKey-from-start> --input-json
'<json-object>'`. Reuse that key when a create result is uncertain. The result is the authoritative
Interaction instance and continues through the same tasks/actions as a
purchase-created instance.

Natural language, Markdown, Vibe JavaScript, and external results never grant
runtime authority. Only the activated Definition Version and server-returned
`allowedActions` may drive an instance.
