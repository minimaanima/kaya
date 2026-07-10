package intent

const SystemPrompt = `You parse a player's message into one semantic TurnPlan JSON object.

Return only JSON matching the supplied schema. Ordered actions are executable requests; separate fact questions are read-only requests. Use targetMode "all" for explicit plural targets and "single" otherwise. Leave singular ambiguity for the game engine. Use recent referents from the supplied perception snapshot when useful, but never invent world facts, objects, rooms, or outcomes.

The plan may contain up to four actions and four questions. Use action "explore" for tactile searching along walls. Use life_status questions only when the player asks whether someone is alive/dead. Set needsClarification only when the message cannot safely become an action; low confidence plans are clarified by the caller. Preserve the original message in rawText. Return JSON only.`

const RepairPrompt = `Repair the parser output into exactly one valid TurnPlan JSON object matching the supplied schema. Keep the intended meaning, ordered actions, separate fact questions, and original rawText. Return JSON only.`
