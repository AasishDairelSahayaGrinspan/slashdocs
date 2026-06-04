# Writing voice — make the prose read human, not AI-generated

Run this over your document's PROSE before `aidocs docs create`. A polished layout
on text that reads machine-generated gets distrusted and skimmed. The goal is
writing a sharp colleague would have written: specific, plain, unembellished.

Two passes:
1. **Humanize** — rewrite anything matching the tells below.
2. **Audit** — reread and ask: "would a reader assume a bot wrote this?" Fix what
   still trips it.

Calibrate to the user's own writing when you have a sample; otherwise default to
plain, direct, concrete.

## The tells to remove

**Content**
- Significance inflation — "pivotal moment", "marks a turning point", "stands as a
  testament". State the fact, drop the trumpet.
- Notability name-dropping and vague authority ("experts agree", "studies show")
  with no specific source.
- Superficial `-ing` analysis tacked on: "…, highlighting the importance of…",
  "…, showcasing its ability to…". Cut it or make it a real claim.
- Promotional adjectives: "powerful", "seamless", "robust", "cutting-edge",
  "game-changing". Show the property instead of asserting it.

**Language**
- AI vocabulary: "delve", "leverage", "landscape", "realm", "tapestry",
  "testament", "navigate the complexities", "it's worth noting". Replace with
  plain words.
- Copula avoidance: "serves as a", "acts as a", "plays a role in" → "is".
- Negative parallelism: "It's not just X, it's Y." Say Y.
- Rule-of-three overuse: every list padded to three items for rhythm. Use the
  number of items that are true.
- Synonym cycling to avoid repeating a term — just repeat the term.
- False ranges / fake precision: "anywhere from a handful to dozens".
- Passive voice with no agent where the agent matters.

**Style**
- Em/en-dash overuse — prefer a period or a comma; one dash per few paragraphs.
- Bold sprayed across half the sentence; bold at most the few words that matter.
- Inline pseudo-headers ("**Key takeaway:** …") instead of real structure.
- Title Case On Every Heading — use sentence case.
- Emoji as decoration or section markers.
- Hyphenated compound buzzwords stacked up ("data-driven, future-proof").

**Communication**
- Chatbot pleasantries: "I hope this helps!", "Great question!", "Let's dive in!"
- Knowledge-cutoff / "as an AI" disclaimers.
- Sycophancy — "excellent point", "absolutely right" — and hedging stacks:
  "it could potentially possibly".
- Generic wrap-up paragraph that restates everything ("In conclusion, …").

**Filler / hedging**
- "in order to" → "to"; "due to the fact that" → "because"; "a number of" →
  "several" or the actual count.

## Quick before/after

- Before: "This pivotal feature leverages a robust, cutting-edge pipeline to
  seamlessly navigate the complexities of payments."
- After: "This feature retries failed payments through a second processor. In
  testing it recovered 4% of declines."

## Done when

The prose names specifics, repeats terms rather than dressing them up, carries no
trumpet adjectives or AI vocabulary, uses sentence-case headings and sparing bold,
and ends without a hollow summary. Then publish.
