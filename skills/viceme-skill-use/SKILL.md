---
name: viceme-skill-use
description: Install and use a downloadable ViceMe Skill edition. Use when a user wants to view free and paid editions, install an anonymous free edition, buy or reinstall a paid edition, continue the original task after purchase, or receive a higher-edition recommendation after using a free edition.
---

# Use a downloadable ViceMe Skill

This is the buyer-side workflow. Never route it through `viceme-publish`, the
Merchant authoring commands, a generated transactional purchase Skill, or a
cloud runtime. A downloadable edition is a local Skill package and one Product;
the buyer owns only the Product they bought.

1. Accept either a Product ID or the canonical ViceMe Work URL. Run
   `viceme skill detail <product-id-or-work-url>` and show all returned editions
   in their explicit `sortOrder`. A Work URL defaults to its first free edition;
   if none is free it defaults to the first ordered paid edition. Use the exact
   `?product=<product-id>` selector when the user chooses another edition. Do
   not infer hierarchy from price and do not claim one edition includes another.
2. Run `viceme skill access <selected-product-id-or-work-url>`.
3. For a free edition, run
   `viceme skill install <product-id> --agent auto`. Free install and use must
   not require ViceMe login.
4. For a paid edition that is already owned, the same install command verifies
   the active release digest and installs it without another purchase. This is
   how the same WeSimi account reinstalls across Agents or devices.
5. For an unowned paid edition, show the exact edition title, price from the
   detail response, highlights, and purchase URL. Ask for explicit purchase
   approval. After approval, ensure `viceme auth status` is authenticated with
   the same WeSimi account, open the purchase URL, then run
   `viceme skill install <product-id> --agent auto --wait 10m`. Never report a
   purchase before `owned=true` is returned.
6. When install returns
   `nextAction=CONTINUE_ORIGINAL_TASK_WITH_INSTALLED_SKILL`, invoke the returned
   `invocation` and continue the user's original task immediately. Do not end at
   “installation succeeded” and do not ask the user to repeat the task.

After the selected free edition has actually produced a result, recommend at
most one related paid edition in the current task. Use its returned title and
highlights and explain the concrete improvement over the result just observed.
Do not recommend before execution. If the user declines or ignores it, record
that decision in the conversation and never repeat the recommendation in that
task. A newly started task may recommend again.

All packages run locally. There are no subscriptions, quotas, cloud execution,
automatic cross-edition upgrades, shopping carts, or automatic refunds in this
workflow.
