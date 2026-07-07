# Reference Games

These are not exact matches for Dr. Kaya. The useful point is to identify nearby mechanics and decide what to borrow or avoid.

## Closest Mechanical References

### Lifeline, 2015

Link: https://en.wikipedia.org/wiki/Lifeline_%282015_video_game%29

Relevant ideas:

- The player guides a stranded person through text.
- The character takes real time to complete tasks.
- Bad decisions can kill the character.
- Some decisions require outside knowledge or reasoning.

Differences:

- Mostly choice-based, not free-form natural language.
- Less systemic inventory/world simulation than Dr. Kaya should have.

Useful takeaway:

Real-time waiting can make a remote character feel physically present. Dr. Kaya should use this idea, but with a stronger underlying engine.

### Lifeline / Operator's Side, 2003

Link: https://en.wikipedia.org/wiki/Lifeline_%28video_game%29

Relevant ideas:

- Survival horror where the player does not directly control the protagonist.
- The player gives spoken commands to another character.
- The character can inspect, move, use objects, and fight based on player instructions.

Differences:

- Voice-command interface instead of text.
- Recognition reliability was a major weakness.

Useful takeaway:

This is one of the closest ancestors to the Dr. Kaya control fantasy: the player as remote operator guiding a vulnerable person through horror.

### Event[0]

Link: https://en.wikipedia.org/wiki/Event_0

Relevant ideas:

- Free-form typing to communicate with an AI character.
- Relationship/trust with the AI affects the experience.
- The player gathers information and decides whether to trust the entity guiding them.

Differences:

- The player directly explores the environment.
- The chat partner is the ship AI, not a vulnerable remote survivor.

Useful takeaway:

Free-form conversation works best when the other side has a strong personality and limited authority over the world.

### Duskers

Link: https://www.wired.com/2016/05/duskers-review

Relevant ideas:

- Remote survival through typed commands.
- Horror created through limited information.
- The player controls units indirectly through a command-line interface.
- Randomness and permanent consequences create tension.

Differences:

- Command syntax is explicit and mechanical.
- No emotional companion character.

Useful takeaway:

Limited information plus remote control is enough to create fear. Dr. Kaya can use the same tension, but replace drones with a human character.

## Puzzle and Investigation References

### Stories Untold

Link: https://en.wikipedia.org/wiki/Stories_Untold_%28video_game%29

Relevant ideas:

- Text interfaces mixed with horror.
- Fictional computers, manuals, terminals, and procedures.
- Puzzle solving through reading, interpreting, and interacting with devices.
- A laboratory episode with experimental procedures.

Differences:

- Mostly authored sequences.
- No free-form chat companion.

Useful takeaway:

Documents, manuals, terminals, and experimental procedures can become horror puzzles without needing complex combat.

### Orwell

Link: https://en.wikipedia.org/wiki/Orwell_%28video_game%29

Relevant ideas:

- Investigation through documents, messages, profiles, and surveillance data.
- The player extracts meaningful facts from messy information.
- Choices about which information to submit have consequences.

Differences:

- Surveillance thriller, not survival horror.
- No inventory or spatial escape engine.

Useful takeaway:

The research-puzzle layer should require interpretation, not just finding a highlighted answer.

### A Normal Lost Phone

Link: https://en.wikipedia.org/wiki/A_Normal_Lost_Phone

Relevant ideas:

- The player investigates a simulated phone.
- Passwords and locked areas are solved by cross-referencing clues.
- Story is discovered through private documents and messages.

Differences:

- Static investigation.
- No live character under threat.

Useful takeaway:

Password and clue chains can feel personal if the documents reveal human context, not just puzzle data.

### The Black Watchmen / The Secret World ARGs

Link: https://en.wikipedia.org/wiki/The_Secret_World

Relevant ideas:

- Alternate reality puzzles.
- Players use websites, clues, codes, coordinates, and external research.
- Fictional organizations and documents create a larger conspiracy layer.

Differences:

- More ARG/MMO-adjacent than single-player survival horror.
- Often depends on real-world community solving.

Useful takeaway:

Dr. Kaya's research puzzles can use simulated intranet pages, PDFs, staff records, lab manuals, or websites to make the player feel like an outside analyst.

## Atmosphere and Messaging References

### Presentable Liberty

Link: https://en.wikipedia.org/wiki/Presentable_Liberty

Relevant ideas:

- Isolation.
- Timed letters/messages.
- Horror through limited interaction.
- The player waits for communication instead of forcing progress.

Differences:

- The player cannot meaningfully guide another character.
- No systemic escape puzzle engine.

Useful takeaway:

Delays and silence can be part of the horror loop.

### Silent Hill: The Short Message

Link: https://en.wikipedia.org/wiki/Silent_Hill%3A_The_Short_Message

Relevant ideas:

- Horror framed around messages, social pressure, and psychological threat.
- Text communication is part of the atmosphere.

Differences:

- First-person horror, not text-command survival.
- No remote companion systems.

Useful takeaway:

Messaging can carry emotional horror, but Dr. Kaya should avoid becoming only mood and chase scenes. The engine systems are the stronger differentiator.

### Calling

Link: https://en.wikipedia.org/wiki/Calling_%28video_game%29

Relevant ideas:

- Horror using phone communication.
- Supernatural/digital-space framing.
- Communication device as both interface and threat source.

Differences:

- First-person Wii survival horror.
- Not free-form chat.

Useful takeaway:

The communication channel itself can become suspicious or unsafe.

## AI Storytelling References

### Hidden Door

Link: https://www.theverge.com/games/757816/hidden-door-early-access-ai-story

Relevant ideas:

- AI-supported narrative generation inside bounded fictional worlds.
- Uses structure to prevent uncontrolled story drift.
- Closer to a tabletop roleplaying structure than a pure chatbot.

Differences:

- Not survival horror.
- Not built around deterministic item/world puzzles.

Useful takeaway:

Bounded AI is more useful than unrestricted AI. Dr. Kaya should keep the LLM constrained by engine facts.

## Main Design Conclusion

There are games that match parts of Dr. Kaya:

- Lifeline matches remote text guidance and real-time waiting.
- Operator's Side matches remote survival horror control.
- Event[0] matches free-form typed conversation.
- Duskers matches remote command horror and procedural tension.
- Stories Untold matches terminal/document/procedure horror.
- Orwell and A Normal Lost Phone match clue extraction and password investigation.
- The Black Watchmen matches external research and ARG-style puzzle solving.

The specific combination for Dr. Kaya is still distinct:

- Free-form natural language.
- A vulnerable remote survivor with autonomy.
- Deterministic inventory and room simulation.
- Darkness and visibility rules.
- Stress, trust, refusal, and hesitation.
- Real-time action delays.
- Seeded roguelike variation.
- Player-side research puzzles.
- Local LLM used for intent and voice, not truth.
