import type vi from './team.vi'

const en: Record<keyof typeof vi, string> = {
  'team.agentCount': '{n} agents',
  'team.noAgents': 'No agents yet',
  'team.starterPack': 'Starter pack',
  'team.default': 'Default',
  'team.defaultHint': 'This agent answers when a chat, bot or automation names no agent',
  'team.defaultAgent': 'Default agent',
  'team.defaultAgentLower': 'default agent',
  'team.defaultSet': '{name} is now the default agent',
  'team.makeDefault': 'Make default',
  'team.open': 'Open agent page',
  'team.export': 'Download agents JSON',
  'team.applyPack': 'Use this pack',
  'team.packApplied': 'Pack applied',
  'team.replacePack': 'Replace with another pack',
  'team.replaceWarn': 'The pack\'s agents replace the current ones; an agent with the same key as an old one keeps its old chats. The current state can still be restored from History.',
  'team.replaceBtn': 'Replace agents',
  'team.addAndConfigure': 'Add and configure',
  'team.empty.title': 'This project has no agents yet',
  'team.empty.desc': 'Pick a starter pack (agents and workflows), or let AI propose one for the project.',

  // AgentLibrary (ADR-153)
  'team.lib.tab': 'From the library',
  'team.lib.blankTab': 'From scratch',
  'team.lib.addOne': 'Add one agent from the library',
  'team.lib.all': 'All',
  'team.lib.featured': 'Featured',
  'team.lib.cat.design': 'Design',
  'team.lib.count': '{n} agents',
  'team.lib.words': '{n} words',
  'team.lib.agencyNote': 'Original English text, fairly long: trim it after adding to save tokens',
  'team.lib.cat.code': 'Code',
  'team.lib.cat.plan': 'Planning',
  'team.lib.cat.customer': 'Customers',
  'team.lib.cat.content': 'Content',
  'team.lib.search': 'Search agents…',
  'team.lib.none': 'No agent matches',
  'team.lib.source': 'Original',
  'team.lib.use': 'Use this one',
  'team.lib.from': 'Based on {name}: with its description, instructions and permissions',
  'team.lib.clear': 'Drop the template',

  // PackPicker
  'team.pack.aiSuggest': 'AI suggestion',
  'team.pack.aiRecommended': '· recommended',
  'team.pack.aiDesc': 'Scans the project, reads CLAUDE.md, AGENTS.md, .claude/agents… then picks a pack and tunes the agents. Needs an AI connection.',
  'team.pack.workflowCount': '{n} workflows',

  // RevisionHistory
  'team.rev.intro': 'Each time agents change, office saves the state just before. Restoring is saved too, so it can always be undone.',
  'team.rev.restoreConfirm': 'Bring the project\'s agents back to this state? The current state is still kept in the history.',
  'team.rev.pack': 'applying pack {arg}',

  // setup with AI
  'team.setup.pack': 'Starter pack',
  'team.setup.hasAgentsWarn': 'This project has {n} agents. Applying only adds the missing ones; existing agents stay as they are.',

  // home: getting started
  'team.home.step3Title': 'Give projects their agents',
  'team.home.step3TextDone': '{n}/{total} projects have agents',
  'team.home.step3TextTodo': 'One agent, a dev team or a council'
}

export default en
