Instructions of the skill [[.Name]], for: [[.Task]]
Follow what fits the task; leave out steps it does not need.

<skill name=[[printf "%q" .Name]] dir=[[printf "%q" .Dir]]>
[[.Body]]
</skill>
[[if .Others]]Other files in the skill folder (read when needed): [[join .Others ", "]]
[[end]][[if not .OnList]]It is not on your skills: add it with my_skills only if you will need it again.
[[end]]
