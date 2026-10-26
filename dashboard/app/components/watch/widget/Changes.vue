<script setup lang="ts">
// What changed in the setup (ADR-210): agents, workflows, schedules, tables,
// by whom, the latest first; of the box's project and those it links to, or
// of every project.
const { t, when } = useLang()
const { projectId, config, narrow } = useWatchBox()
const days = computed(() => Number(config.value.days) || 1)
const { data, error, refresh } = useLiveFetch<Activity>(() => `/api/activity?hours=${days.value * 24}${projectId.value ? `&project=${projectId.value}` : ''}`, { lazy: true, server: false })
const rows = computed(() => (data.value?.changes ?? []).slice(0, 50))
</script>

<template>
  <div class="flex h-full min-h-0 flex-col">
    <div v-if="!data && !error" class="p-2"><LoadingRows :n="4" /></div>
    <div v-else-if="error" class="p-3"><UAlert color="error" variant="subtle" :title="apiError(error)" :actions="[{ label: t('common.refresh'), onClick: () => refresh() }]" /></div>
    <WatchEmpty v-else-if="!rows.length" icon="i-lucide-history" :text="t('watchW.changes.none')" />
    <ul v-else class="min-h-0 flex-1 divide-y divide-(--ui-border) overflow-y-auto">
      <li v-for="(ch, i) in rows" :key="i" class="px-2.5 py-1.5 text-xs">
        <p class="truncate"><span class="font-medium">{{ ch.who }}</span> <code>{{ ch.action }}</code> {{ ch.target }}</p>
        <p class="truncate text-(--ui-text-muted)">{{ when(ch.at) }}<template v-if="!narrow || !projectId"> · {{ ch.project }}</template></p>
      </li>
    </ul>
  </div>
</template>
