<script setup lang="ts">
// Who did the work (ADR-210): each agent's runs, errors and cost in the
// period, the busiest first; of the box's project and those it links to, or
// of every project.
const { t } = useLang()
const { projectId, config, narrow } = useWatchBox()
const days = computed(() => Number(config.value.days) || 1)
const { data, error, refresh } = useLiveFetch<Activity>(() => `/api/activity?hours=${days.value * 24}${projectId.value ? `&project=${projectId.value}` : ''}`, { lazy: true, server: false })
const rows = computed(() => [...(data.value?.agents ?? [])].sort((a, b) => b.runs - a.runs))
const max = computed(() => Math.max(1, ...rows.value.map(a => a.runs)))
</script>

<template>
  <div class="flex h-full min-h-0 flex-col gap-2 p-3">
    <div v-if="data" class="flex items-baseline justify-between gap-2 text-xs text-(--ui-text-muted)">
      <span>{{ t('activity.runs', { n: data.runs }) }}</span><span class="font-semibold tabular-nums text-(--ui-text)">{{ usd(data.cost_usd) }}</span>
    </div>
    <LoadingRows v-if="!data && !error" :n="4" />
    <UAlert v-else-if="error" color="error" variant="subtle" :title="apiError(error)" :actions="[{ label: t('common.refresh'), onClick: () => refresh() }]" />
    <WatchEmpty v-else-if="!rows.length" icon="i-lucide-users" :text="t('watchW.team.none')" />
    <ul v-else class="min-h-0 flex-1 space-y-2 overflow-y-auto">
      <li v-for="a in rows" :key="a.project + a.agent">
        <div class="flex items-baseline justify-between gap-2 text-xs">
          <span class="truncate">{{ a.agent }}<span v-if="!narrow" class="text-(--ui-text-muted)"> · {{ a.project }}</span></span>
          <span class="shrink-0 text-(--ui-text-muted) tabular-nums">{{ a.runs }}<template v-if="a.errors"> · <span class="text-(--ui-error)">{{ t('watchW.team.errors', { n: a.errors }) }}</span></template> · {{ usd(a.cost_usd) }}</span>
        </div>
        <div class="mt-1 h-1.5 overflow-hidden rounded-full bg-(--ui-primary)/10">
          <div class="h-full rounded-full bg-(--ui-primary)" :style="{ width: `${a.runs / max * 100}%` }" />
        </div>
      </li>
    </ul>
  </div>
</template>
