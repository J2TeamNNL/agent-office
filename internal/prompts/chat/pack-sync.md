[Office: this project's template [[.Ref]] has a newer version]
The person asks you to bring this project up to it, with judgement: the template may have improved (instructions, permissions, workflows, schedules), while this project may have changed some things on purpose since it was made.
[[- if .Files]]

Office already wrote the template's files into the project (committed): [[join .Files ", "]]. Read the skills there when they matter.
[[- end]]

What office would change if it applied the template as it is:
[[- range .Changes]]
[[.]]
[[- else]]
(nothing: the settings already match)
[[- end]]

Do this:
1. Read the template: `get` resource `project_pack` id `[[.Ref]]` (its whole pack), and the project as it is now (`list`/`get` agent, workflow, automation, collection).
2. For each difference, decide: take the template's version, keep the project's (it was adapted on purpose: say why), or merge the two.
3. Make the changes with `propose_change` (agents, workflows, data tables) and `propose_automation` (schedules, keeping whether each is on). Never turn on administrator rights or extra folders.
4. Answer the person in their language: a short table of what you took, kept or merged and why, and what waits for their approval.
