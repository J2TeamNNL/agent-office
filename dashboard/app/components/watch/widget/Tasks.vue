<script setup lang="ts">
// The work boards (ADR-210): of the box's project and those it links to, or
// of every project; per project its open work, the part being done, what was
// added, done and failed in the period. A project opens on its Việc page.
const { t } = useLang()
const { projectId, config, narrow } = useWatchBox()
const days = computed(() => Number(config.value.days) || 1)
const { data, error, refresh } = useLiveFetch<Activity>(() => `/api/activity?hours=${days.value * 24}${projectId.value ? `&project=${projectId.value}` : ''}`, { lazy: true, server: false })
const rows = computed(() => busyOf(data.value?.projects ?? []))
// the bar: done, being done, waiting, failed, by the largest
const max = computed(() => Math.max(1, ...rows.value.map(p => p.done + openOf(p) + p.failed)))
</script>

<template>
  <div class="flex h-full min-h-0 flex-col gap-2 p-3">
    <LoadingRows v-if="!data && !error" :n="3" />
    <UAlert v-else-if="error" color="error" variant="subtle" :title="apiError(error)" :actions="[{ label: t('common.refresh'), onClick: () => refresh() }]" />
    <WatchEmpty v-else-if="!rows.length" icon="i-lucide-list-checks" :text="t('activity.quiet')" />
    <ul v-else class="min-h-0 flex-1 space-y-2.5 overflow-y-auto">
      <li v-for="p in rows" :key="p.project_id">
        <NuxtLink :to="{ path: `/projects/${p.project_id}`, query: { tab: 'burn' } }" class="flex items-baseline justify-between gap-2 text-xs hover:underline">
          <span class="truncate font-medium">{{ p.project }}</span>
          <span class="shrink-0 text-(--ui-text-muted) tabular-nums">
            <template v-if="narrow">{{ p.done }}/{{ p.done + openOf(p) }}</template>
            <template v-else>{{ t('watchW.tasks.line', { open: openOf(p), doing: doingOf(p), done: p.done }) }}<template v-if="p.failed"> · <span class="text-(--ui-error)">{{ t('watchW.tasks.failed', { n: p.failed }) }}</span></template></template>
          </span>
        </NuxtLink>
        <div class="mt-1 flex h-1.5 overflow-hidden rounded-full bg-(--ui-bg-elevated)" :style="{ width: `${(p.done + openOf(p) + p.failed) / max * 100}%` }">
          <span class="bg-(--ui-success)" :style="{ flex: p.done }" />
          <span class="bg-(--ui-warning)" :style="{ flex: doingOf(p) }" />
          <span class="bg-(--ui-text-dimmed)/50" :style="{ flex: openOf(p) - doingOf(p) }" />
          <span class="bg-(--ui-error)" :style="{ flex: p.failed }" />
        </div>
      </li>
    </ul>
  </div>
</template>
