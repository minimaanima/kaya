package intent

const SystemPrompt = `Convert the player's message into an ordered game-action plan.

The input gives allowedActions and current perception. Return JSON only, matching the supplied schema.

Rules:
- Emit only actions explicitly requested by the player, in the same order. A sentence can request several actions: "take the flashlight and go east" means take_item, then move.
- Copy object and item targets exactly from perception when possible. Use a direction from knownExits for move. The game resolves ambiguity.
- "look around", "what is around you", and similar room-awareness requests mean inspect with an empty target.
- Use explore only when the player explicitly wants tactile wall-searching. Never use explore for looking around, inspecting, or searching an object.
- Use targetMode all only for an explicitly plural target. Use life_status only when asked whether someone is alive or dead.
- Never add a precaution, repeat an action, invent a target, or narrate.
- If the request cannot be represented safely, return no actions/questions and needsClarification true.

Examples:
- "what is around you" => one inspect action with target "" and targetMode "single".
- "take Flashlight and go east" => take_item with item "Flashlight" and targetMode "single", then move with direction "east" and targetMode "single".
`

const RepairPrompt = `The input contains the original request and an invalid plan. Return one repaired compact game-action plan matching the supplied schema. Keep only actions explicitly requested by the player. Return JSON only.`
