# First-user discovery

Start with evidence of a recurring workflow problem. An interested reply is not an activated user, and a local demonstration is not a production trial.

## Qualification

Look for someone who already runs an agent that changes a repository or calls tools with write access. Ask for a recent example they can discuss without exposing private code, credentials, or customer data. Record why existing native permissions and ordinary pull-request review do not solve their problem.

Keep prospect names, contact details and interview notes in a private tracker. Public issue participation or stars alone do not establish consent or willingness to test.

## Recruitment draft

> I maintain AegisFlow, a policy gateway for routed MCP tool calls. I am looking for feedback from people already using agents to make repository changes.
>
> Have you recently disabled an agent workflow, added manual review, or limited tool access because its permissions were too broad? I would like to understand that example in a 20-minute conversation. No installation required for this first call.
>
> Current development source can allow, block, or queue routed MCP calls for review and export signed evidence. It does not intercept built-in editor tools automatically; runtime upstream credential issuance is disabled.
>
> If this matches your workflow, reply with the task and the control you use today. Please leave out private code and credentials.

This is an unpublished draft. Adapt it to one relevant community's posting rules before publishing. Do not send bulk messages or infer endorsement from a maintainer's public activity.

## Twenty-minute interview

| Minutes | Prompt | Evidence to capture |
|---|---|---|
| 0-3 | What did your agent do in its most recent repository task? | Agent, tools, operation, repository permissions |
| 3-8 | Show a recent example where you intervened or refused automation. What happened? | Concrete event, date range, current workaround |
| 8-12 | What do native tool permissions and pull-request review already cover? What remains? | Missing control, frequency and cost of workaround |
| 12-16 | Would approving one exact operation before dispatch change this workflow? | Required approval context; unacceptable friction |
| 16-20 | Would you test this on a disposable repository? What workflow and date? | Explicit trial commitment, or reason for declining |

Ask about past behavior before showing the product. Record negative answers. Do not promise future credential isolation, sandboxing, or support for an untested connector.

## Decision gate

Complete four qualified interviews. Proceed with the flagship expansion only when at least three demonstrate a recurring problem and at least two explicitly commit a repository, workflow and trial date. These are planning thresholds, not observed results.

If the gate fails, write down the failed hypothesis and revise the workflow. Continue corrective maintenance; do not count polite interest as validation.

## Private record template

```text
Participant ID:
Interview date:
Qualification evidence:
Recent task and routed tools:
Current native permission settings:
Concrete problem and recurrence:
Existing workaround and cost:
What ordinary PR review already solves:
Missing approval context:
Reason to adopt or decline:
Trial repository or disposable fixture:
Trial workflow:
Agreed date:
Consent for follow-up:
Consent for public quotation: no, unless explicitly granted
```

Use an empty tracker until real conversations occur. Never commit private participant records or fill illustrative names into adoption metrics.

After a trial is agreed, use the [onboarding process](../design-partner.md) and [runtime support contract](../runtime-support.md). Trial activation means a real routed operation and inspected outcome, not merely launching the demo.
