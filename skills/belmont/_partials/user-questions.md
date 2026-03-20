## Asking Questions (MANDATORY)

When you need to ask the user a question:

1. **Use your structured question tool** (e.g. `AskUserQuestion`, or equivalent). This is NON-NEGOTIABLE when such a tool is available.
2. **Ask ONE set of related questions at a time** — group related questions into a single tool call, then wait for answers before asking the next set.
3. **NEVER print the question as inline text AND use the tool.** The tool call IS the question — do not duplicate it in your response body.
4. **NEVER ask questions as plain inline text** when a structured question tool exists. No "Question 1: ..." followed by more text. Use the tool.
5. **Fallback**: If no structured question tool is available in your environment, ask questions as plain text — one set at a time, clearly formatted.