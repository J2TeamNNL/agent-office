<script setup lang="ts">
// The skills an agent keeps at hand (ADR-185): their names and why are in
// every chat, the instructions read only when a task needs them. The list
// grows from the setup, the library agent it came from, the agent and
// the person's /skill calls; it is capped. People see how each is used; admins add and remove.
interface Item { name: string, description: string, why: string, source: 'person' | 'agent' | 'setup' | 'library' | 'call', uses: number, last_used_at: string | null, missing: boolean }
interface Skill { name: string, description: string }
const props = defineProps<{ projectId: string, agentId: string }>()
const { t, when } = useLang()
const { isAdmin } = useAuth()
const saveError = useSaveError()
const base = computed(() => `/api/projects/${props.projectId}/agents/${props.agentId}/skills`)
const { data, refresh, error: loadError } = useLiveFetch<{ items: Item[], limit: number }>(base, { lazy: true })
const { data: projectSkills } = useLiveFetch<{ skills: Skill[] }>(() => `/api/projects/${props.projectId}/skills`, { lazy: true })
const addable = computed(() => (projectSkills.value?.skills ?? [])
  .filter(s => !data.value?.items.some(i => i.name === s.name))
  .map(s => ({ label: s.name, value: s.name, description: s.description })))
const full = computed(() => !!data.value && data.value.items.length >= data.value.limit)
const sourceLabel = (s: Item['source']) => ({ person: t('askill.fromPerson'), agent: t('askill.fromAgent'), setup: t('askill.fromSetup'), library: t('askill.fromLibrary'), call: t('askill.fromCall') })[s]
const sourceIcon = (s: Item['source']) => ({ person: 'i-lucide-user', agent: 'i-lucide-bot', setup: 'i-lucide-sparkles', library: 'i-lucide-library', call: 'i-lucide-slash' })[s]

async function run(fn: () => Promise<unknown>) {
  try {
    await fn()
    await refresh()
  } catch (e) {
    saveError(e, refresh)
  }
}
const pick = ref<string>()
const why = ref('')
const saving = ref(false)
const add = () => {
  if (saving.value || !pick.value) return
  saving.value = true
  run(async () => {
    await $fetch(base.value, { method: 'POST', body: { name: pick.value, why: why.value } })
    pick.value = undefined
    why.value = ''
  }).finally(() => { saving.value = false })
}
const remove = (i: Item) => { if (confirm(t('askill.removeConfirm', { name: i.name }))) run(() => $fetch(`${base.value}/${encodeURIComponent(i.name)}`, { method: 'DELETE' })) }
</script>

<template>
  <UCard :ui="{ body: 'space-y-3 sm:p-4' }">
    <div class="flex items-center gap-2">
      <p class="font-medium">{{ t('askill.title') }}</p>
      <InfoTip :text="t('askill.help')" icon-class="size-4 text-(--ui-text-muted)" />
      <span v-if="data" class="ms-auto text-xs text-(--ui-text-muted) tabular-nums">{{ t('askill.count', { n: data.items.length, max: data.limit }) }}</span>
    </div>

    <UAlert v-if="loadError" color="error" variant="subtle" icon="i-lucide-triangle-alert" :title="t('common.loadError')" :actions="[{ label: t('common.refresh'), onClick: () => refresh() }]" />
    <LoadingRows v-else-if="!data" :n="2" :icon="false" />
    <p v-else-if="!data.items.length" class="rounded-lg border border-dashed border-(--ui-border) p-4 text-center text-sm text-(--ui-text-muted)">{{ t('askill.empty') }}</p>
    <ul v-else class="divide-y divide-(--ui-border) rounded-lg border border-(--ui-border)">
      <li v-for="i in data.items" :key="i.name" class="group flex items-start gap-2 px-3 py-2 text-sm" :class="{ 'opacity-60': i.missing }">
        <UIcon :name="sourceIcon(i.source)" class="mt-0.5 size-4 shrink-0 text-(--ui-text-muted)" :title="sourceLabel(i.source)" />
        <div class="min-w-0 flex-1">
          <p class="flex flex-wrap items-center gap-x-2">
            <span class="font-mono">{{ i.name }}</span>
            <UBadge v-if="i.missing" size="sm" color="warning" variant="subtle" :label="t('askill.missing')" />
          </p>
          <p v-if="i.why || i.description" class="break-words text-(--ui-text-muted)">{{ i.why || i.description }}</p>
          <p class="text-xs text-(--ui-text-muted)">
            {{ sourceLabel(i.source) }} ·
            <template v-if="i.uses">{{ t('askill.uses', { n: i.uses }) }}<template v-if="i.last_used_at">, {{ t('askill.last', { when: when(i.last_used_at) }) }}</template></template>
            <template v-else>{{ t('askill.never') }}</template>
          </p>
        </div>
        <UButton v-if="isAdmin" size="xs" color="neutral" variant="ghost" icon="i-lucide-trash-2" class="shrink-0 hover:text-(--ui-error) md:invisible md:group-hover:visible" :aria-label="t('chat.delete')" @click="remove(i)" />
      </li>
    </ul>
    <form v-if="isAdmin && data && !full && addable.length" class="flex flex-wrap gap-2" @submit.prevent="add">
      <USelectMenu v-model="pick" :items="addable" value-key="value" class="min-w-0 basis-52" :placeholder="t('askill.pick')" />
      <UInput v-model="why" class="min-w-0 flex-1 basis-52" :placeholder="t('askill.why')" />
      <UButton type="submit" icon="i-lucide-plus" :label="t('askill.add')" :loading="saving" :disabled="!pick || saving" />
    </form>
  </UCard>
</template>
