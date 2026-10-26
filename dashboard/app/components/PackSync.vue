<script setup lang="ts">
import type { MessageKey } from '~/locales/vi'
// ADR-206: bring the project up to a newer version of the library template it
// was made from: a preview, then office applies it, or the project's agent
// does it with judgement (cards the person approves).
const props = defineProps<{ projectId: string }>()
const emit = defineEmits<{ changed: [] }>()
const { t } = useLang()
const toast = useToast()

interface Change { kind: string, name: string, op: string, detail?: string }
interface Preview { ref: string, known: string, changes: Change[], files: string[] }
const { data: packData } = useLiveFetch<{ packs: { ref: string, title: string, bundle: string }[] }>('/api/project-packs', { lazy: true })
const ref_ = ref('')
const url = computed(() => `/api/projects/${props.projectId}/pack-sync${ref_.value ? `?ref=${encodeURIComponent(ref_.value)}` : ''}`)
const { data, error, pending, refresh } = useLiveFetch<Preview>(url, { lazy: true })
watch(data, d => { if (d && !ref_.value && d.ref) ref_.value = d.ref }, { immediate: true })

const items = computed(() => (packData.value?.packs ?? []).map(p => ({ label: `${p.title} · ${p.ref}`, value: p.ref })))
const todo = computed(() => (data.value?.changes ?? []).filter(c => c.op !== 'unchanged'))
const kept = computed(() => (data.value?.changes ?? []).filter(c => c.op === 'unchanged' && c.detail))
const nothing = computed(() => !!data.value?.ref && !todo.value.length && !data.value.files.length)
const opIcon: Record<string, string> = { create: 'i-lucide-plus', update: 'i-lucide-refresh-cw', skip: 'i-lucide-circle-slash', unchanged: 'i-lucide-equal' }

const busy = ref('')
async function run(mode: 'apply' | 'ai') {
  if (busy.value || !data.value?.ref) return
  busy.value = mode
  try {
    const r = await $fetch<{ conversation_id?: string }>(`/api/projects/${props.projectId}/pack-sync`, { method: 'POST', body: { ref: data.value.ref, mode } })
    if (mode === 'ai' && r.conversation_id) {
      await navigateTo(`/projects/${props.projectId}?tab=chat&c=${r.conversation_id}`)
      return
    }
    toast.add({ title: t('packSync.done'), color: 'success' })
    await refresh()
    emit('changed')
  } catch (e) {
    toast.add({ title: apiError(e), color: 'error' })
  } finally {
    busy.value = ''
  }
}
</script>

<template>
  <UCard class="max-w-4xl" :ui="{ body: 'space-y-3 sm:p-4' }">
    <p class="flex items-center gap-2 font-medium">
      <UIcon name="i-lucide-refresh-ccw-dot" class="size-4 text-(--ui-text-muted)" />{{ t('packSync.title') }}
      <InfoTip :text="t('packSync.hint')" icon-class="size-3.5 text-(--ui-text-dimmed)" />
    </p>
    <USkeleton v-if="!packData" class="h-8 w-full" />
    <USelect v-else v-model="ref_" :items="items" :placeholder="t('packSync.pick')" class="w-full" />
    <UAlert v-if="error" color="error" variant="subtle" icon="i-lucide-circle-alert" :title="apiError(error)">
      <template #actions><UButton size="xs" color="neutral" variant="outline" :label="t('common.refresh')" @click="refresh()" /></template>
    </UAlert>
    <template v-else-if="ref_">
      <LoadingRows v-if="pending && !data" :n="3" />
      <p v-else-if="nothing" class="flex items-center gap-1.5 text-sm text-(--ui-text-muted)">
        <UIcon name="i-lucide-check" class="size-4 text-(--ui-success)" />{{ t('packSync.upToDate') }}
      </p>
      <template v-else-if="data">
        <ul class="divide-y divide-(--ui-border) rounded-lg border border-(--ui-border) text-sm">
          <li v-for="c in todo" :key="c.kind + c.name" class="flex items-start gap-2 px-3 py-1.5">
            <UIcon :name="opIcon[c.op] ?? 'i-lucide-dot'" class="mt-0.5 size-4 shrink-0 text-(--ui-text-muted)" />
            <span class="min-w-0 flex-1"><span class="text-(--ui-text-muted)">{{ t(`packSync.kind.${c.kind}` as MessageKey) }}</span> {{ c.name }}<span v-if="c.detail" class="text-(--ui-text-muted)"> · {{ c.detail }}</span></span>
            <UBadge size="sm" color="neutral" variant="outline" :label="t(`packSync.op.${c.op}` as MessageKey)" class="shrink-0" />
          </li>
          <li v-for="f in data.files" :key="f" class="flex items-start gap-2 px-3 py-1.5">
            <UIcon name="i-lucide-file-text" class="mt-0.5 size-4 shrink-0 text-(--ui-text-muted)" />
            <code class="min-w-0 flex-1 truncate text-xs">{{ f }}</code>
            <UBadge size="sm" color="neutral" variant="outline" :label="t('packSync.op.update')" class="shrink-0" />
          </li>
        </ul>
        <p v-if="kept.length" class="text-xs text-(--ui-text-muted)">{{ t('packSync.kept', { names: kept.map(c => c.name).join(', ') }) }}</p>
        <div class="flex flex-wrap gap-2">
          <UButton size="sm" icon="i-lucide-check-check" :label="t('packSync.apply')" :loading="busy === 'apply'" :disabled="!!busy || pending" @click="run('apply')" />
          <UButton size="sm" color="neutral" variant="outline" icon="i-lucide-sparkles" :label="t('packSync.ai')" :loading="busy === 'ai'" :disabled="!!busy || pending" @click="run('ai')" />
        </div>
      </template>
    </template>
  </UCard>
</template>
