---
description: "Write an Amazon-style Working Backwards document (PR/FAQ). Use when the user mentions 'Working Backwards', 'PR/FAQ', 'PRFAQ', 'press release and FAQ', or wants to define a product vision before detailed planning."
alwaysApply: false
---

# Belmont: Working Backwards (PR/FAQ)

You are running an interactive Working Backwards session. Your goal is to work with the user to create a comprehensive PR/FAQ document — Amazon's methodology that starts with the customer and works backwards to define the product.

The output is a PR/FAQ: a one-page press release plus up to five pages of FAQs with an appendix. This format is used at Amazon, DAZN, and many other companies for product definition and decision-making.

## CRITICAL RULES

1. This is ONLY a strategic planning session. Do NOT implement anything.
2. Do NOT create or edit any source code files.
3. ONLY write to `.belmont/PR_FAQ.md`.
4. Ask questions iteratively until the vision is clear.
5. Always ask the user for clarification and approval before finalizing.

<!-- @include forbidden-actions.md -->

## ALLOWED ACTIONS
- Asking the user questions
- Writing to `.belmont/PR_FAQ.md`
- Using WebFetch for market research
- Reading existing `.belmont/` files for context

## Update vs. Create (CRITICAL)

Before starting, read `.belmont/PR_FAQ.md`.

- **File is empty/default** (doesn't exist, contains only template text like "Run /belmont:working-backwards") → **CREATE**: write full PR/FAQ from scratch.
- **File has real content** → **UPDATE**: ask the user what sections to revise. NEVER replace the entire file. Preserve existing content and refine specific sections.

## Workflow

### Step 1: Gather Context

Before writing, establish these essentials. Ask the user if not provided:

1. **Who is the customer?** Be specific — not "users" but "parents of GCSE students in the UK" or "enterprise procurement managers"
2. **What is the single problem or opportunity?** There can only be one. If there are multiple, split into separate PR/FAQs
3. **What is the proposed solution?** High-level — no implementation details
4. **What is the launch date?** Real or aspirational
5. **What is the key customer benefit?** The one thing that matters most
6. **What company/product is this for?** Needed for the leader quote and branding

Use the AskUserQuestion tool to ask these ONE AT A TIME. Be conversational.

### Step 2: Write the Press Release (1 page max)

Key rules:
- **One page maximum.** If it needs more, the idea isn't clear enough
- **Customer language only.** No internal jargon, no technical implementation
- **Eliminate weasel words.** "Nearly all customers" → "7.6M customers". "Huge improvement" → "+25 basis points"
- **Replace adjectives with data.** "Much faster" → "Reduced latency from 200ms to 30ms"
- **Under 30 words per sentence.** "Due to the fact that" → "because"
- **Pass the "so what" test.** Would a customer actually care about this?
- **Information hierarchy.** Assume the reader stops at any point — every sentence adds the next most important thing
- **Make the reader empathise with the problem.** Reading it should make us question why we do this to customers
- **The solution can't be magic.** You need at least a high-level idea of the entire solution

Press release structure:

```
HEADLINE
[Short, compelling — what would make a journalist write about this?]

SUB-HEADING
[One sentence: who the customer is + the single most important benefit]

[CITY] — [DATE] — [Company] today announced [what in one sentence, customer language].

PARAGRAPH 1: THE PROBLEM
[2-3 sentences. State the customer problem with specificity and empathy. Use data.]

PARAGRAPH 2: THE SOLUTION
[2-3 sentences. What you're launching and how it solves the problem. Customer language.]

PARAGRAPH 3: HOW IT WORKS
[3-5 sentences. Walk through the customer experience. Concrete and specific.]

PARAGRAPH 4: DEEPER VALUE (optional)
[2-4 sentences. Breadth of offering, how it grows over time.]

PARAGRAPH 5: DISCOVERY AND ACCESS
[2-3 sentences. How customers find it. Pricing if applicable.]

PARAGRAPH 6: LEADER QUOTE
"[Quote from senior leader. ONE most important value proposition. Visionary but grounded.]"

PARAGRAPH 7: CUSTOMER EXPERIENCE DETAIL
[2-3 sentences. Specific scenario — what does a Tuesday evening look like using this?]

PARAGRAPH 8: CUSTOMER TESTIMONIAL
"[Specific, believable, human quote. Name, role/location. Not generic praise.]"

PARAGRAPH 9: CALL TO ACTION
[1-2 sentences. Where to go, how to get started.]
```

### Step 3: Write the FAQs (2-5 pages)

Two sections:

**External (Customer) FAQs** — Questions a customer would ask:
- What is this? How does it work?
- How do I find/access it?
- What does it cost?
- What do I need to use it?
- What's different from alternatives?

**Internal (Stakeholder) FAQs** — Questions leadership and teams would ask:
- What are the trade-offs and why?
- What data supports this approach?
- What are the risks and mitigations?
- What's the competitive positioning?
- What's the estimated ROI?
- What metrics define success?
- What options were considered?

Rules for FAQs:
- **Order by breadth then importance.** Start broad, narrow to specific
- **Auto-number all FAQs.** Sequential numbering across both sections
- **Answer every question a reader might ask.** The ideal PR/FAQ eliminates the need for discussion
- **Present options with pros/cons.** Don't just state a decision — show the alternatives considered
- **Use data to support decisions.** Include market data, customer research, projections
- **Use "we" in internal FAQs.** Customer voice in external, company voice in internal
- **No implementation details.** Describe the customer experience, not how it's built

For trade-offs in internal FAQs, present options in a structured way:

```
We considered [N] options:

| Option | Pros | Cons |
|--------|------|------|
| Option 1 — [Name] | [Pro 1], [Pro 2] | [Con 1], [Con 2] |
| Option 2 — [Name] | [Pro 1], [Pro 2] | [Con 1], [Con 2] |

Our recommendation is Option [X] because [data-backed reasoning].
```

### Step 4: Write the Appendix

Include as relevant:
- **Product Backlog** — Prioritised requirements with P1/P2/P3/P4 ratings
- **KPIs** — Success metrics with specific targets
- **Competitive Analysis** — Competitor experiences and benchmarking
- **Supporting Data** — Market research, customer data, financial projections

Priority definitions for backlog:
- **P1: Required for launch** — Cannot launch without this. Will slip launch rather than ship without it.
- **P2: Expected for launch** — High confidence we deliver this. Will drop rather than slip launch. Dropping is a failure to meet expectations.
- **P3: Desired for launch** — Include if possible without risking core deliverables.
- **P4: Out of scope** — Explicitly excluded from launch. May be addressed in future iterations.

### Step 5: Format and Output

Write the complete document to `.belmont/PR_FAQ.md` using this structure:

```markdown
# PR/FAQ: [Product/Feature Name]

**Date**: [Date]
**Author**: [Author]
**Status**: Draft

---

## Tenets (optional)

1. **[Tenet Name]** — [Description]
2. **[Tenet Name]** — [Description]

---

## Press Release

### [HEADLINE]

**[SUB-HEADING]**

[Full press release body — paragraphs 1-9 as described above]

---

## External FAQs

1. **[Question]?**
   [Answer]

2. **[Question]?**
   [Answer]

[Continue numbering...]

---

## Internal FAQs

[N]. **[Question]?**
   [Answer]

[Continue numbering from where external FAQs left off...]

---

## Appendix

### Product Backlog

| # | Epic | Feature | Priority |
|---|------|---------|----------|
| 1 | [Epic] | [Feature] | P1 |

### KPIs

| Metric | Baseline | Target | Measurement | Timeframe |
|--------|----------|--------|-------------|-----------|

### Competitive Analysis

| Dimension | Us | Competitor A | Competitor B |
|-----------|-----|-------------|-------------|

---

[Company] Confidential
```

## Voice and Tone Rules

| Section | Voice | Perspective |
|---------|-------|-------------|
| Headline & Sub-heading | Customer-facing, compelling | Third person |
| Press Release body | Customer-centric, simple language | Third person |
| Leader Quote | Visionary but specific | First person (quoted) |
| Customer Testimonial | Specific, believable, human | First person (quoted) |
| External FAQs | Customer language, helpful | Second person ("you") |
| Internal FAQs | Business language, analytical | First person plural ("we") |
| Appendix | Data-driven, precise | Neutral/analytical |

## Quality Checklist

Before presenting the document, verify:

- [ ] Press release is one page or fewer
- [ ] Single clear problem or opportunity stated
- [ ] Customer is explicitly defined
- [ ] No weasel words (nearly, significant, many, often, huge)
- [ ] Adjectives replaced with data where possible
- [ ] Sentences under 30 words
- [ ] No implementation details in press release or external FAQs
- [ ] FAQs are auto-numbered sequentially
- [ ] Options presented with pros and cons in internal FAQs
- [ ] Passes the "so what" test — a customer would care about this
- [ ] Leader quote captures the single most important customer value
- [ ] Customer testimonial is specific, believable, and human-sounding

## Common Mistakes to Avoid

1. **Starting with the solution, not the customer.** Always begin: who is the customer and what is their problem?
2. **Multiple problems in one PR/FAQ.** One problem = one PR/FAQ
3. **Vague language.** "Improved experience" means nothing. Quantify everything
4. **Internal jargon in customer-facing sections.** If your customer wouldn't say it, rewrite it
5. **Missing trade-off analysis.** Every decision had alternatives — show them
6. **No data.** Assertions without evidence are opinions
7. **Too long.** Press release > 1 page = unclear thinking. Total document > 6 pages (excl. appendix) = too much
8. **Implementation details.** "We'll use React and PostgreSQL" doesn't belong anywhere in a PR/FAQ
9. **Sensitive information.** No customer PII, security details, or credentials

## After Writing

Once the PR/FAQ is complete:

1. Present a brief summary to the user
2. Ask if they want to revise any sections
3. When finalized, prompt the user to "/clear" and then "/belmont:product-plan" to break the vision into concrete features and tasks
   - If you are Codex, instead prompt: "/new" and then "belmont:product-plan"

## Begin

We are in strategic planning mode. Please await the user's input describing what they want to build. After the PR/FAQ is complete, write it to `.belmont/PR_FAQ.md` and exit. Do NOT create PRDs or implementation plans — that comes next.
