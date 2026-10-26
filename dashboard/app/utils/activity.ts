// How projects worked over a period (ADR-210): GET /api/activity, on the
// overview and in watch boxes.
export interface Activity {
  cost_usd: number, runs: number
  projects: ActivityProject[]
  agents: { project: string, agent: string, runs: number, errors: number, cost_usd: number }[]
  changes: { at: string, project: string, who: string, action: string, target: string }[]
  schedules: { id: string, project: string, name: string, enabled: boolean, last_run_at?: string }[]
}
export interface ActivityProject { project_id: string, project: string, by_status: Record<string, number>, added: number, done: number, failed: number, cost_usd: number }

const openStatuses = ['found', 'queued', 'handed', 'doing', 'paused', 'review']
// a board's open work, and the part of it being done (by Burn or an agent)
export const openOf = (p: ActivityProject) => openStatuses.reduce((n, s) => n + (p.by_status[s] ?? 0), 0)
export const doingOf = (p: ActivityProject) => (p.by_status.handed ?? 0) + (p.by_status.doing ?? 0) + (p.by_status.review ?? 0)
// projects with nothing on their board and no runs in the period: not listed
export const busyOf = (ps: ActivityProject[]) => ps.filter(p => openOf(p) || p.added || p.done || p.failed || p.cost_usd)
export const usd = (n: number) => `$${n.toFixed(2)}`
