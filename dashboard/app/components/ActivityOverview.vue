<script setup lang="ts">
// How the projects worked over the period picked (ADR-210), on the overview:
// each project's work board, the agents' runs and cost, the changes to the
// setup (agents, workflows, schedules, tables) and the schedules.
const { t, when } = useLang()

const hours = ref(24)
const { data, error, refresh } = useLiveFetch<Activity>(() => `/api/activity?hours=${hours.value}`, { lazy: true })
const periods = computed(() => [{ label: t('activity.h24'), value: 24 }, { label: t('activity.d7'), value: 168 }, { label: t('activity.d30'), value: 720 }])
const busy = computed(() => busyOf(data.value?.projects ?? []))
const schedulesOn = computed(() => (data.value?.schedules ?? []).filter(s => s.enabled))
</script>

<template>
  <UCard :ui="{ body: 'space-y-3 sm:p-4' }">
    <div class="flex flex-wrap items-center gap-2">
      <p class="flex items-center gap-2 font-medium"><UIcon name="i-lucide-activity" class="size-4 text-(--ui-text-muted)" />{{ t('activity.title') }}</p>
      <span v-if="data" class="text-xs text-(--ui-text-muted)">{{ t('activity.runs', { n: data.runs }) }} · {{ usd(data.cost_usd) }}</span>
      <USelect v-model="hours" :items="periods" size="sm" class="ms-auto w-32" :aria-label="t('activity.period')" />
    </div>
    <UAlert v-if="error" color="error" variant="subtle" icon="i-lucide-circle-alert" :title="apiError(error)">
      <template #actions><UButton size="xs" color="neutral" variant="outline" :label="t('common.refresh')" @click="refresh()" /></template>
    </UAlert>
    <LoadingRows v-else-if="!data" :n="3" />
    <template v-else>
      <!-- each project's work board -->
      <p v-if="!busy.length" class="text-sm text-(--ui-text-muted)">{{ t('activity.quiet') }}</p>
      <div v-else class="grid gap-2 sm:grid-cols-2 xl:grid-cols-3">
        <NuxtLink v-for="p in busy" :key="p.project_id" :to="{ path: `/projects/${p.project_id}`, query: { tab: 'burn' } }" class="rounded-lg border border-(--ui-border) p-3 hover:bg-(--ui-bg-elevated)/40">
          <p class="flex items-center justify-between gap-2 text-sm font-medium">
            <span class="truncate">{{ p.project }}</span><span class="shrink-0 text-xs font-normal text-(--ui-text-muted)">{{ usd(p.cost_usd) }}</span>
          </p>
          <p class="mt-1 text-xs text-(--ui-text-muted)">{{ t('activity.board', { open: openOf(p), doing: doingOf(p), added: p.added, done: p.done, failed: p.failed }) }}</p>
        </NuxtLink>
      </div>
      <div class="grid gap-3 lg:grid-cols-3">
        <div class="space-y-1 text-sm">
          <p class="text-xs font-medium text-(--ui-text-muted)">{{ t('activity.agents') }}</p>
          <p v-if="!data.agents.length" class="text-xs text-(--ui-text-dimmed)">—</p>
          <p v-for="a in data.agents.slice(0, 8)" :key="a.project + a.agent" class="flex items-center gap-2">
            <span class="min-w-0 flex-1 truncate">{{ a.agent }} <span class="text-xs text-(--ui-text-muted)">· {{ a.project }}</span></span>
            <span class="shrink-0 text-xs tabular-nums text-(--ui-text-muted)">{{ a.runs }}<template v-if="a.errors"> · <span class="text-(--ui-error)">{{ a.errors }}</span></template> · {{ usd(a.cost_usd) }}</span>
          </p>
        </div>
        <div class="space-y-1 text-sm">
          <p class="text-xs font-medium text-(--ui-text-muted)">{{ t('activity.changes') }}</p>
          <p v-if="!data.changes.length" class="text-xs text-(--ui-text-dimmed)">—</p>
          <p v-for="(ch, i) in data.changes.slice(0, 8)" :key="i" class="truncate text-xs">
            <span class="text-(--ui-text-muted)">{{ when(ch.at) }} · {{ ch.project }} ·</span> {{ ch.who }} <code>{{ ch.action }}</code> {{ ch.target }}
          </p>
        </div>
        <div class="space-y-1 text-sm">
          <p class="text-xs font-medium text-(--ui-text-muted)">{{ t('activity.schedules') }}</p>
          <p v-if="!schedulesOn.length" class="text-xs text-(--ui-text-dimmed)">—</p>
          <p v-for="s in schedulesOn.slice(0, 8)" :key="s.id" class="flex items-center gap-2 text-xs">
            <UIcon name="i-lucide-circle-play" class="size-3.5 shrink-0 text-(--ui-success)" />
            <span class="min-w-0 flex-1 truncate">{{ s.name }} <span class="text-(--ui-text-muted)">· {{ s.project }}</span></span>
            <span class="shrink-0 text-(--ui-text-muted)">{{ s.last_run_at ? when(s.last_run_at) : t('activity.never') }}</span>
          </p>
        </div>
      </div>
    </template>
  </UCard>
</template>
